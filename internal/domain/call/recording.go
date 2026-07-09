package call

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

const (
	callMetadataRecordingState     = "recording_state"
	callMetadataTranscriptionState = "transcription_state"
)

// UpdateRecording updates call-level recording state and emits durable hook events.
func (s *Service) UpdateRecording(
	ctx context.Context,
	params UpdateRecordingParams,
) (Call, []Event, error) {
	if err := s.validateContext(ctx, "update call recording"); err != nil {
		return Call{}, nil, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.CallID == "" || params.AccountID == "" || params.DeviceID == "" {
		return Call{}, nil, ErrInvalidInput
	}

	now := s.currentTime()
	var result Call
	var events []Event
	err := s.runTx(ctx, func(store Store) error {
		callRow, loadErr := s.visibleCall(ctx, store, params.CallID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		conversationRow, member, loadErr := s.visibleConversation(ctx, callRow.ConversationID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		if callRow.State != StateActive {
			return ErrConflict
		}
		if !canControlCapture(callRow, conversationRow.Kind, member.Role, params.AccountID, params.DeviceID) {
			return ErrForbidden
		}

		nextState := RecordingStateInactive
		if params.Enabled {
			nextState = RecordingStateActive
		}
		if callRow.RecordingState == nextState {
			result, loadErr = s.hydrateCall(ctx, store, callRow.ID)
			return loadErr
		}

		callRow.RecordingState = nextState
		if params.Enabled {
			callRow.RecordingStartedAt = now
			callRow.RecordingStoppedAt = timeZero()
		} else {
			callRow.RecordingStoppedAt = now
			if callRow.TranscriptionState == TranscriptionStateActive {
				callRow.TranscriptionState = TranscriptionStateInactive
				callRow.TranscriptionStoppedAt = now
			}
		}
		callRow.UpdatedAt = now

		savedCall, saveErr := store.SaveCall(ctx, callRow)
		if saveErr != nil {
			return fmt.Errorf("save recording state for %s: %w", callRow.ID, saveErr)
		}
		callRow = savedCall

		recordingEvent, appendErr := s.appendEvent(ctx, store, callRow, EventTypeRecordingUpdated, params.AccountID, params.DeviceID, map[string]string{
			callMetadataRecordingState: string(callRow.RecordingState),
		}, now)
		if appendErr != nil {
			return appendErr
		}
		events = append(events, recordingEvent)

		if !params.Enabled && callRow.TranscriptionStoppedAt.Equal(now) {
			transcriptionEvent, appendErr := s.appendEvent(ctx, store, callRow, EventTypeTranscriptionUpdated, params.AccountID, params.DeviceID, map[string]string{
				callMetadataTranscriptionState: string(callRow.TranscriptionState),
				"cascade_from_recording":       "true",
			}, now)
			if appendErr != nil {
				return appendErr
			}
			events = append(events, transcriptionEvent)
		}

		result, loadErr = s.hydrateCall(ctx, store, callRow.ID)
		return loadErr
	})
	if err != nil {
		return Call{}, nil, err
	}

	for i := range events {
		events[i].Call = cloneCall(result)
	}

	return result, cloneEvents(events), nil
}

// UpdateTranscription updates call-level transcription state and emits durable hook events.
func (s *Service) UpdateTranscription(
	ctx context.Context,
	params UpdateTranscriptionParams,
) (Call, []Event, error) {
	if err := s.validateContext(ctx, "update call transcription"); err != nil {
		return Call{}, nil, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.CallID == "" || params.AccountID == "" || params.DeviceID == "" {
		return Call{}, nil, ErrInvalidInput
	}

	now := s.currentTime()
	var result Call
	var events []Event
	err := s.runTx(ctx, func(store Store) error {
		callRow, loadErr := s.visibleCall(ctx, store, params.CallID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		conversationRow, member, loadErr := s.visibleConversation(ctx, callRow.ConversationID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		if callRow.State != StateActive {
			return ErrConflict
		}
		if !canControlCapture(callRow, conversationRow.Kind, member.Role, params.AccountID, params.DeviceID) {
			return ErrForbidden
		}

		nextState := TranscriptionStateInactive
		if params.Enabled {
			nextState = TranscriptionStateActive
		}
		if callRow.TranscriptionState == nextState {
			result, loadErr = s.hydrateCall(ctx, store, callRow.ID)
			return loadErr
		}

		callRow.TranscriptionState = nextState
		if params.Enabled {
			callRow.TranscriptionStartedAt = now
			callRow.TranscriptionStoppedAt = timeZero()
		} else {
			callRow.TranscriptionStoppedAt = now
		}
		callRow.UpdatedAt = now

		savedCall, saveErr := store.SaveCall(ctx, callRow)
		if saveErr != nil {
			return fmt.Errorf("save transcription state for %s: %w", callRow.ID, saveErr)
		}
		callRow = savedCall

		event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeTranscriptionUpdated, params.AccountID, params.DeviceID, map[string]string{
			callMetadataTranscriptionState: string(callRow.TranscriptionState),
		}, now)
		if appendErr != nil {
			return appendErr
		}
		events = append(events, event)

		result, loadErr = s.hydrateCall(ctx, store, callRow.ID)
		return loadErr
	})
	if err != nil {
		return Call{}, nil, err
	}

	for i := range events {
		events[i].Call = cloneCall(result)
	}

	return result, cloneEvents(events), nil
}

func canControlCapture(
	callRow Call,
	kind conversation.ConversationKind,
	role conversation.MemberRole,
	accountID string,
	deviceID string,
) bool {
	if kind == conversation.ConversationKindGroup {
		return canManageGroupCall(callRow, role, accountID)
	}

	for _, participant := range callRow.Participants {
		if participant.AccountID == accountID &&
			participant.DeviceID == deviceID &&
			participant.State == ParticipantStateJoined {
			return true
		}
	}

	return false
}
