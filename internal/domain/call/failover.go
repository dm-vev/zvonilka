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

	runtimeSession, err := s.runtime.EnsureSession(ctx, replacement)
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

		targetEvent, appendErr := s.appendEvent(ctx, store, savedCall, EventTypeSessionMigrated, "", "", map[string]string{
			callMetadataTargetAccountID:   participant.AccountID,
			callMetadataTargetDeviceID:    participant.DeviceID,
			callMetadataPreviousSessionID: previousSessionID,
			callMetadataSessionID:         runtimeJoin.SessionID,
			callMetadataRuntimeEndpoint:   s.resolveRuntimeEndpoint(runtimeJoin.RuntimeEndpoint),
			callMetadataFailoverReason:    reason,
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

func runtimeParticipantFromParticipant(callID string, participant Participant) RuntimeParticipant {
	return RuntimeParticipant{
		CallID:    callID,
		AccountID: participant.AccountID,
		DeviceID:  participant.DeviceID,
		WithVideo: participantWantsVideo(participant),
		Media:     participant.MediaState,
	}
}
