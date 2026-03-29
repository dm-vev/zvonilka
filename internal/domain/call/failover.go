package call

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	callMetadataPreviousSessionID = "previous_session_id"
	callMetadataRuntimeEndpoint   = "runtime_endpoint"
	callMetadataFailoverReason    = "failover_reason"
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
) (Call, RuntimeSession, Event, error) {
	if callRow.State != StateActive || strings.TrimSpace(callRow.ActiveSessionID) == "" {
		return Call{}, RuntimeSession{}, Event{}, ErrConflict
	}

	previousSessionID := callRow.ActiveSessionID
	replacement := callRow
	replacement.ActiveSessionID = ""

	runtimeSession, err := s.runtime.EnsureSession(ctx, replacement)
	if err != nil {
		return Call{}, RuntimeSession{}, Event{}, err
	}
	if strings.TrimSpace(runtimeSession.SessionID) == "" {
		return Call{}, RuntimeSession{}, Event{}, ErrConflict
	}

	callRow.ActiveSessionID = runtimeSession.SessionID
	callRow.UpdatedAt = now
	savedCall, err := store.SaveCall(ctx, callRow)
	if err != nil {
		return Call{}, RuntimeSession{}, Event{}, err
	}

	event, err := s.appendEvent(ctx, store, savedCall, EventTypeSessionMigrated, "", "", map[string]string{
		callMetadataPreviousSessionID: previousSessionID,
		callMetadataSessionID:         runtimeSession.SessionID,
		callMetadataRuntimeEndpoint:   runtimeSession.RuntimeEndpoint,
		callMetadataFailoverReason:    reason,
	}, now)
	if err != nil {
		return Call{}, RuntimeSession{}, Event{}, err
	}

	return savedCall, runtimeSession, event, nil
}
