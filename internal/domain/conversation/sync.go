package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// RecordDelivery marks a message as delivered to a device.
func (s *Service) RecordDelivery(ctx context.Context, params RecordDeliveryParams) (ReadState, EventEnvelope, error) {
	if err := s.validateContext(ctx, "record delivery"); err != nil {
		return ReadState{}, EventEnvelope{}, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	params.MessageID = strings.TrimSpace(params.MessageID)
	if params.ConversationID == "" || params.AccountID == "" || params.DeviceID == "" || params.MessageID == "" {
		return ReadState{}, EventEnvelope{}, ErrInvalidInput
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var (
		savedState ReadState
		savedEvent EventEnvelope
	)

	err := s.runTx(ctx, func(tx Store) error {
		member, err := tx.ConversationMemberByConversationAndAccount(ctx, params.ConversationID, params.AccountID)
		if err != nil {
			return fmt.Errorf("authorize device delivery for account %s in conversation %s: %w", params.AccountID, params.ConversationID, err)
		}
		if !isActiveMember(member) {
			return ErrForbidden
		}
		state, err := tx.ReadStateByConversationAndDevice(ctx, params.ConversationID, params.DeviceID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("load read state: %w", err)
		}
		if state.ConversationID == "" {
			state = ReadState{
				ConversationID: params.ConversationID,
				AccountID:      params.AccountID,
				DeviceID:       params.DeviceID,
			}
		}
		state.AccountID = params.AccountID
		state.DeviceID = params.DeviceID
		state.LastDeliveredSequence = maxUint64(state.LastDeliveredSequence, params.DeliveredThroughSequence)
		state.UpdatedAt = now
		savedState, err = tx.SaveReadState(ctx, state)
		if err != nil {
			return fmt.Errorf("save delivery state: %w", err)
		}

		event := EventEnvelope{
			EventID:             eventID(),
			EventType:           EventTypeMessageDelivered,
			ConversationID:      params.ConversationID,
			ActorAccountID:      params.AccountID,
			ActorDeviceID:       params.DeviceID,
			CausationID:         params.CausationID,
			CorrelationID:       params.CorrelationID,
			MessageID:           params.MessageID,
			PayloadType:         "delivery",
			ReadThroughSequence: params.DeliveredThroughSequence,
			Metadata: map[string]string{
				"message_id": params.MessageID,
			},
			CreatedAt: now,
		}
		savedEvent, err = tx.SaveEvent(ctx, event)
		if err != nil {
			return fmt.Errorf("save delivery event: %w", err)
		}

		syncState := SyncState{
			DeviceID:            params.DeviceID,
			AccountID:           params.AccountID,
			LastAppliedSequence: savedEvent.Sequence,
			LastAckedSequence:    savedState.LastAckedSequence,
			ServerTime:          now,
		}
		if _, err := tx.SaveSyncState(ctx, syncState); err != nil {
			return fmt.Errorf("save sync state after delivery: %w", err)
		}

		return nil
	})
	if err != nil {
		return ReadState{}, EventEnvelope{}, err
	}

	return savedState, savedEvent, nil
}

// MarkRead advances the read watermark for a device.
func (s *Service) MarkRead(ctx context.Context, params MarkReadParams) (ReadState, EventEnvelope, error) {
	if err := s.validateContext(ctx, "mark read"); err != nil {
		return ReadState{}, EventEnvelope{}, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.ConversationID == "" || params.AccountID == "" || params.DeviceID == "" {
		return ReadState{}, EventEnvelope{}, ErrInvalidInput
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var (
		savedState ReadState
		savedEvent EventEnvelope
	)

	err := s.runTx(ctx, func(tx Store) error {
		member, err := tx.ConversationMemberByConversationAndAccount(ctx, params.ConversationID, params.AccountID)
		if err != nil {
			return fmt.Errorf("authorize read for account %s in conversation %s: %w", params.AccountID, params.ConversationID, err)
		}
		if !isActiveMember(member) {
			return ErrForbidden
		}
		state, err := tx.ReadStateByConversationAndDevice(ctx, params.ConversationID, params.DeviceID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("load read state: %w", err)
		}
		if state.ConversationID == "" {
			state = ReadState{
				ConversationID: params.ConversationID,
				AccountID:      params.AccountID,
				DeviceID:       params.DeviceID,
			}
		}
		state.AccountID = params.AccountID
		state.DeviceID = params.DeviceID
		state.LastDeliveredSequence = maxUint64(state.LastDeliveredSequence, params.ReadThroughSequence)
		state.LastReadSequence = maxUint64(state.LastReadSequence, params.ReadThroughSequence)
		state.UpdatedAt = now
		savedState, err = tx.SaveReadState(ctx, state)
		if err != nil {
			return fmt.Errorf("save read state: %w", err)
		}

		event := EventEnvelope{
			EventID:             eventID(),
			EventType:           EventTypeMessageRead,
			ConversationID:      params.ConversationID,
			ActorAccountID:      params.AccountID,
			ActorDeviceID:       params.DeviceID,
			CausationID:         params.CausationID,
			CorrelationID:       params.CorrelationID,
			MessageID:           "",
			PayloadType:         "read",
			ReadThroughSequence: params.ReadThroughSequence,
			CreatedAt:           now,
		}
		savedEvent, err = tx.SaveEvent(ctx, event)
		if err != nil {
			return fmt.Errorf("save read event: %w", err)
		}

		syncState := SyncState{
			DeviceID:            params.DeviceID,
			AccountID:           params.AccountID,
			LastAppliedSequence: savedEvent.Sequence,
			LastAckedSequence:    maxUint64(savedState.LastAckedSequence, params.ReadThroughSequence),
			ServerTime:          now,
		}
		if _, err := tx.SaveSyncState(ctx, syncState); err != nil {
			return fmt.Errorf("save sync state after read: %w", err)
		}

		return nil
	})
	if err != nil {
		return ReadState{}, EventEnvelope{}, err
	}

	return savedState, savedEvent, nil
}

// PullEvents reads sync events after the requested sequence.
func (s *Service) PullEvents(ctx context.Context, params PullEventsParams) ([]EventEnvelope, SyncState, error) {
	if err := s.validateContext(ctx, "pull events"); err != nil {
		return nil, SyncState{}, err
	}
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	params.ConversationIDs = uniqueIDs(params.ConversationIDs)
	if params.DeviceID == "" {
		return nil, SyncState{}, ErrInvalidInput
	}

	events, err := s.store.EventsAfterSequence(ctx, params.FromSequence, clampLimit(params.Limit, 100, 1000), params.ConversationIDs)
	if err != nil {
		return nil, SyncState{}, fmt.Errorf("load events after %d: %w", params.FromSequence, err)
	}

	state, _, err := s.GetSyncState(ctx, GetSyncStateParams{DeviceID: params.DeviceID})
	if err != nil {
		return nil, SyncState{}, err
	}
	if len(events) > 0 {
		state.LastAppliedSequence = events[len(events)-1].Sequence
		state.ServerTime = s.currentTime()
		if _, err := s.store.SaveSyncState(ctx, state); err != nil {
			return nil, SyncState{}, fmt.Errorf("advance sync state: %w", err)
		}
	}

	return events, state, nil
}

// GetSyncState resolves the current sync cursors for a device.
func (s *Service) GetSyncState(ctx context.Context, params GetSyncStateParams) (SyncState, []string, error) {
	if err := s.validateContext(ctx, "get sync state"); err != nil {
		return SyncState{}, nil, err
	}
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.DeviceID == "" {
		return SyncState{}, nil, ErrInvalidInput
	}

	state, err := s.store.SyncStateByDevice(ctx, params.DeviceID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return SyncState{}, nil, fmt.Errorf("load sync state for device %s: %w", params.DeviceID, err)
	}

	readStates, err := s.store.ReadStatesByDevice(ctx, params.DeviceID)
	if err != nil {
		return SyncState{}, nil, fmt.Errorf("load read states for device %s: %w", params.DeviceID, err)
	}

	derived := make(map[string]uint64, len(readStates))
	accountID := state.AccountID
	if accountID == "" && len(readStates) > 0 {
		accountID = readStates[0].AccountID
	}
	for _, readState := range readStates {
		derived[readState.ConversationID] = maxUint64(
			readState.LastDeliveredSequence,
			readState.LastReadSequence,
			readState.LastAckedSequence,
		)
	}

	if state.DeviceID == "" {
		state.DeviceID = params.DeviceID
	}
	if state.AccountID == "" {
		state.AccountID = accountID
	}
	if state.ConversationWatermarks == nil {
		state.ConversationWatermarks = make(map[string]uint64, len(derived))
	}
	for conversationID, watermark := range derived {
		if existing, ok := state.ConversationWatermarks[conversationID]; !ok || watermark > existing {
			state.ConversationWatermarks[conversationID] = watermark
		}
	}
	if state.ServerTime.IsZero() {
		state.ServerTime = s.currentTime()
	}

	pending := make([]string, 0)
	if state.AccountID != "" {
		conversations, err := s.store.ConversationsByAccountID(ctx, state.AccountID)
		if err != nil {
			return SyncState{}, nil, fmt.Errorf("load conversations for sync state %s: %w", params.DeviceID, err)
		}
		for _, conversation := range conversations {
			watermark := state.ConversationWatermarks[conversation.ID]
			if conversation.LastSequence > watermark {
				pending = append(pending, conversation.ID)
			}
		}
	}

	return state, pending, nil
}

// AcknowledgeEvents advances the device ack cursor.
func (s *Service) AcknowledgeEvents(ctx context.Context, params AcknowledgeEventsParams) (SyncState, error) {
	if err := s.validateContext(ctx, "acknowledge events"); err != nil {
		return SyncState{}, err
	}
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.DeviceID == "" {
		return SyncState{}, ErrInvalidInput
	}

	state, pending, err := s.GetSyncState(ctx, GetSyncStateParams{DeviceID: params.DeviceID})
	if err != nil {
		return SyncState{}, err
	}
	_ = pending

	state.LastAckedSequence = maxUint64(state.LastAckedSequence, params.AckedSequence)
	state.ServerTime = s.currentTime()
	if _, err := s.store.SaveSyncState(ctx, state); err != nil {
		return SyncState{}, fmt.Errorf("save acknowledged sync state: %w", err)
	}

	return state, nil
}
