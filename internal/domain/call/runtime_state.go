package call

import (
	"context"
	"fmt"
	"strings"
)

// GetRuntimeState returns the current stable runtime placement for one visible call.
func (s *Service) GetRuntimeState(ctx context.Context, params GetParams) (RuntimeState, error) {
	if err := s.validateContext(ctx, "get call runtime state"); err != nil {
		return RuntimeState{}, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.CallID == "" || params.AccountID == "" {
		return RuntimeState{}, ErrInvalidInput
	}

	callRow, err := s.GetCall(ctx, params)
	if err != nil {
		return RuntimeState{}, err
	}

	if reader, ok := s.runtime.(RuntimeStateReader); ok {
		state, readErr := reader.SessionState(ctx, callRow)
		if readErr != nil {
			return RuntimeState{}, readErr
		}
		state.CallID = callRow.ID
		state.ConversationID = callRow.ConversationID
		state.Active = callRow.State == StateActive && strings.TrimSpace(callRow.ActiveSessionID) != ""
		if state.SessionID == "" {
			state.SessionID = callRow.ActiveSessionID
		}
		if state.NodeID == "" {
			state.NodeID = NodeIDFromSessionID(state.SessionID)
		}
		if state.RuntimeEndpoint == "" {
			state.RuntimeEndpoint = s.rtc.endpointForSession(state.SessionID)
		}
		if state.ObservedAt.IsZero() {
			state.ObservedAt = s.currentTime()
		}
		return cloneRuntimeState(state), nil
	}

	state := RuntimeState{
		CallID:          callRow.ID,
		ConversationID:  callRow.ConversationID,
		SessionID:       callRow.ActiveSessionID,
		NodeID:          NodeIDFromSessionID(callRow.ActiveSessionID),
		RuntimeEndpoint: s.rtc.endpointForSession(callRow.ActiveSessionID),
		Active:          callRow.State == StateActive && strings.TrimSpace(callRow.ActiveSessionID) != "",
		Healthy:         strings.TrimSpace(callRow.ActiveSessionID) != "",
		ObservedAt:      s.currentTime(),
	}

	return state, nil
}

// GetSessionSnapshot returns the current stable runtime session snapshot for one visible call.
func (s *Service) GetSessionSnapshot(ctx context.Context, params GetParams) (RuntimeSnapshot, error) {
	if err := s.validateContext(ctx, "get call session snapshot"); err != nil {
		return RuntimeSnapshot{}, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.CallID == "" || params.AccountID == "" {
		return RuntimeSnapshot{}, ErrInvalidInput
	}

	callRow, err := s.GetCall(ctx, params)
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	if callRow.State != StateActive || strings.TrimSpace(callRow.ActiveSessionID) == "" {
		return RuntimeSnapshot{}, ErrConflict
	}

	reader, ok := s.runtime.(RuntimeSnapshotReader)
	if !ok {
		return RuntimeSnapshot{}, ErrConflict
	}

	snapshot, err := reader.SessionSnapshot(ctx, callRow)
	if err != nil {
		return RuntimeSnapshot{}, fmt.Errorf("export runtime snapshot for call %s: %w", callRow.ID, err)
	}
	if snapshot.CallID == "" {
		snapshot.CallID = callRow.ID
	}
	if snapshot.ConversationID == "" {
		snapshot.ConversationID = callRow.ConversationID
	}
	if snapshot.SessionID == "" {
		snapshot.SessionID = callRow.ActiveSessionID
	}
	if snapshot.NodeID == "" {
		snapshot.NodeID = NodeIDFromSessionID(snapshot.SessionID)
	}
	if snapshot.ObservedAt.IsZero() {
		snapshot.ObservedAt = s.currentTime()
	}

	return cloneRuntimeSnapshot(snapshot), nil
}

// MigrateCallSession performs one explicit active-session migration for a visible host-owned call.
func (s *Service) MigrateCallSession(ctx context.Context, params MigrateParams) (Call, []Event, error) {
	if err := s.validateContext(ctx, "migrate call session"); err != nil {
		return Call{}, nil, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.CallID == "" || params.AccountID == "" || params.DeviceID == "" {
		return Call{}, nil, ErrInvalidInput
	}

	var result Call
	var events []Event
	err := s.runTx(ctx, func(store Store) error {
		callRow, loadErr := s.visibleCall(ctx, store, params.CallID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		if callRow.State != StateActive || strings.TrimSpace(callRow.ActiveSessionID) == "" {
			return ErrConflict
		}
		if currentGroupCallHost(callRow) != params.AccountID {
			return ErrForbidden
		}

		now := s.currentTime()
		migrated, _, migratedEvents, migrateErr := s.failoverActiveSession(ctx, store, callRow, now, "manual")
		if migrateErr != nil {
			return fmt.Errorf("migrate active call %s: %w", callRow.ID, migrateErr)
		}

		result = migrated
		events = migratedEvents
		return nil
	})
	if err != nil {
		return Call{}, nil, err
	}

	for i := range events {
		events[i].Call = cloneCall(result)
	}

	return result, cloneEvents(events), nil
}
