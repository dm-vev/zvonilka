package call

import (
	"context"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	callMetadataPreviousSessionID = "previous_session_id"
	callMetadataRuntimeEndpoint   = "runtime_endpoint"
	callMetadataFailoverReason    = "failover_reason"
	callMetadataReconnectRequired = "reconnect_required"
	callMetadataMigrationMode     = "migration_mode"
	callMetadataCutoverDeadline   = "cutover_deadline_unix"
	callMetadataSessionToken      = "session_token"
	callMetadataExpiresAtUnix     = "expires_at_unix"
	callMetadataIceUfrag          = "ice_ufrag"
	callMetadataIcePwd            = "ice_pwd"
	callMetadataDTLSFingerprint   = "dtls_fingerprint"
	callMetadataCandidateHost     = "candidate_host"
	callMetadataCandidatePort     = "candidate_port"
)

func runtimeUnavailable(err error) bool {
	code := status.Code(err)
	return code == codes.Unavailable || code == codes.DeadlineExceeded
}

type sessionMigrator interface {
	MigrateSession(ctx context.Context, callRow Call) (RuntimeSession, error)
}

func (s *Service) failoverActiveSession(
	ctx context.Context,
	store Store,
	callRow Call,
	now time.Time,
	reason string,
) (Call, RuntimeSession, []Event, error) {
	if callRow.State != StateActive || strings.TrimSpace(callRow.ActiveSessionID) == "" {
		return Call{}, RuntimeSession{}, nil, ErrConflict
	}

	previousSessionID := callRow.ActiveSessionID
	replacement := callRow
	replacement.ActiveSessionID = ""

	runtimeSession, migrationMode, err := s.replacementRuntimeSession(ctx, callRow, replacement)
	if err != nil {
		return Call{}, RuntimeSession{}, nil, err
	}
	if strings.TrimSpace(runtimeSession.SessionID) == "" {
		return Call{}, RuntimeSession{}, nil, ErrConflict
	}

	callRow.ActiveSessionID = runtimeSession.SessionID
	callRow.UpdatedAt = now
	savedCall, err := store.SaveCall(ctx, callRow)
	if err != nil {
		return Call{}, RuntimeSession{}, nil, err
	}

	events := make([]Event, 0, 8)
	globalEvent, err := s.appendEvent(ctx, store, savedCall, EventTypeSessionMigrated, "", "", map[string]string{
		callMetadataPreviousSessionID: previousSessionID,
		callMetadataSessionID:         runtimeSession.SessionID,
		callMetadataRuntimeEndpoint:   runtimeSession.RuntimeEndpoint,
		callMetadataFailoverReason:    reason,
		callMetadataMigrationMode:     migrationMode,
	}, now)
	if err != nil {
		return Call{}, RuntimeSession{}, nil, err
	}
	events = append(events, globalEvent)

	participants, err := store.ParticipantsByCall(ctx, savedCall.ID)
	if err != nil {
		return Call{}, RuntimeSession{}, nil, err
	}
	for _, participant := range participants {
		if participant.State != ParticipantStateJoined {
			continue
		}

		runtimeJoin, joinErr := s.runtime.JoinSession(ctx, runtimeSession.SessionID, runtimeParticipantFromParticipant(savedCall.ID, participant))
		if joinErr != nil {
			return Call{}, RuntimeSession{}, nil, joinErr
		}
		cutoverDeadline := now.Add(s.settings.ReconnectGrace).UTC()

		targetEvent, appendErr := s.appendEvent(ctx, store, savedCall, EventTypeSessionMigrated, "", "", map[string]string{
			callMetadataTargetAccountID:   participant.AccountID,
			callMetadataTargetDeviceID:    participant.DeviceID,
			callMetadataPreviousSessionID: previousSessionID,
			callMetadataSessionID:         runtimeJoin.SessionID,
			callMetadataRuntimeEndpoint:   s.resolveRuntimeEndpoint(runtimeJoin.RuntimeEndpoint),
			callMetadataFailoverReason:    reason,
			callMetadataMigrationMode:     migrationMode,
			callMetadataCutoverDeadline:   strconv.FormatInt(cutoverDeadline.Unix(), 10),
			callMetadataReconnectRequired: "true",
			callMetadataSessionToken:      runtimeJoin.SessionToken,
			callMetadataExpiresAtUnix:     strconv.FormatInt(runtimeJoin.ExpiresAt.UTC().Unix(), 10),
			callMetadataIceUfrag:          runtimeJoin.IceUfrag,
			callMetadataIcePwd:            runtimeJoin.IcePwd,
			callMetadataDTLSFingerprint:   runtimeJoin.DTLSFingerprint,
			callMetadataCandidateHost:     runtimeJoin.CandidateHost,
			callMetadataCandidatePort:     strconv.Itoa(runtimeJoin.CandidatePort),
		}, now)
		if appendErr != nil {
			return Call{}, RuntimeSession{}, nil, appendErr
		}
		events = append(events, targetEvent)
	}

	return savedCall, runtimeSession, events, nil
}

func (s *Service) replacementRuntimeSession(
	ctx context.Context,
	current Call,
	replacement Call,
) (RuntimeSession, string, error) {
	if migrator, ok := s.runtime.(sessionMigrator); ok {
		runtimeSession, err := migrator.MigrateSession(ctx, current)
		if err == nil {
			return runtimeSession, "graceful", nil
		}
		if !runtimeUnavailable(err) {
			return RuntimeSession{}, "", err
		}
	}

	runtimeSession, err := s.runtime.EnsureSession(ctx, replacement)
	if err != nil {
		return RuntimeSession{}, "", err
	}

	return runtimeSession, "reconnect", nil
}

func runtimeParticipantFromParticipant(callID string, participant Participant) RuntimeParticipant {
	return RuntimeParticipant{
		CallID:    callID,
		AccountID: participant.AccountID,
		DeviceID:  participant.DeviceID,
		WithVideo: participantWantsVideo(participant),
		Media:     participant.MediaState,
	}
}
