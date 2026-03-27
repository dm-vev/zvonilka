package call

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// RaiseHand updates the caller's hand state inside an active group call.
func (s *Service) RaiseHand(
	ctx context.Context,
	params RaiseHandParams,
) (Call, Participant, []Event, error) {
	if err := s.validateContext(ctx, "raise hand"); err != nil {
		return Call{}, Participant{}, nil, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.CallID == "" || params.AccountID == "" || params.DeviceID == "" {
		return Call{}, Participant{}, nil, ErrInvalidInput
	}

	now := s.currentTime()
	var result Call
	var saved Participant
	var events []Event
	err := s.runTx(ctx, func(store Store) error {
		callRow, loadErr := s.visibleCall(ctx, store, params.CallID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		conversationRow, _, loadErr := s.visibleConversation(ctx, callRow.ConversationID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		if conversationRow.Kind != conversation.ConversationKindGroup || callRow.State != StateActive {
			return ErrConflict
		}

		participant, loadErr := store.ParticipantByCallAndDevice(ctx, callRow.ID, params.DeviceID)
		if loadErr != nil {
			if loadErr == ErrNotFound {
				return ErrForbidden
			}
			return fmt.Errorf("load participant %s/%s: %w", callRow.ID, params.DeviceID, loadErr)
		}
		if participant.AccountID != params.AccountID || participant.State != ParticipantStateJoined {
			return ErrForbidden
		}

		participant.HandRaised = params.Raised
		if params.Raised {
			participant.RaisedHandAt = now
		} else {
			participant.RaisedHandAt = time.Time{}
		}
		participant.UpdatedAt = now

		savedParticipant, saveErr := store.SaveParticipant(ctx, participant)
		if saveErr != nil {
			return fmt.Errorf("save raised-hand state %s/%s: %w", participant.CallID, participant.DeviceID, saveErr)
		}
		saved = savedParticipant

		eventType := EventTypeMediaUpdated
		metadata := map[string]string{
			"hand_raised": boolString(params.Raised),
		}
		event, appendErr := s.appendEvent(ctx, store, callRow, eventType, params.AccountID, params.DeviceID, metadata, now)
		if appendErr != nil {
			return appendErr
		}
		events = append(events, event)

		result, loadErr = s.hydrateCall(ctx, store, callRow.ID)
		return loadErr
	})
	if err != nil {
		return Call{}, Participant{}, nil, err
	}

	for i := range events {
		events[i].Call = cloneCall(result)
	}

	return result, cloneParticipant(saved), cloneEvents(events), nil
}

// ModerateParticipant applies host controls to one joined participant in an active group call.
func (s *Service) ModerateParticipant(
	ctx context.Context,
	params ModerateParticipantParams,
) (Call, Participant, []Event, error) {
	if err := s.validateContext(ctx, "moderate participant"); err != nil {
		return Call{}, Participant{}, nil, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	params.TargetDeviceID = strings.TrimSpace(params.TargetDeviceID)
	if params.CallID == "" || params.AccountID == "" || params.DeviceID == "" || params.TargetDeviceID == "" {
		return Call{}, Participant{}, nil, ErrInvalidInput
	}

	now := s.currentTime()
	var result Call
	var saved Participant
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
		if conversationRow.Kind != conversation.ConversationKindGroup || callRow.State != StateActive {
			return ErrConflict
		}
		if !canModerateGroupCall(member.Role) {
			return ErrForbidden
		}

		target, loadErr := store.ParticipantByCallAndDevice(ctx, callRow.ID, params.TargetDeviceID)
		if loadErr != nil {
			if loadErr == ErrNotFound {
				return ErrNotFound
			}
			return fmt.Errorf("load target participant %s/%s: %w", callRow.ID, params.TargetDeviceID, loadErr)
		}
		if target.State != ParticipantStateJoined {
			return ErrConflict
		}

		target.HostMutedAudio = params.HostMutedAudio
		target.HostMutedVideo = params.HostMutedVideo
		if params.LowerHand {
			target.HandRaised = false
			target.RaisedHandAt = time.Time{}
		}
		target.UpdatedAt = now

		savedParticipant, saveErr := store.SaveParticipant(ctx, target)
		if saveErr != nil {
			return fmt.Errorf("save moderated participant %s/%s: %w", target.CallID, target.DeviceID, saveErr)
		}
		saved = savedParticipant

		metadata := map[string]string{
			"target_device_id":  target.DeviceID,
			"target_account_id": target.AccountID,
			"host_muted_audio":  boolString(target.HostMutedAudio),
			"host_muted_video":  boolString(target.HostMutedVideo),
			"lower_hand":        boolString(params.LowerHand),
		}
		event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeMediaUpdated, params.AccountID, params.DeviceID, metadata, now)
		if appendErr != nil {
			return appendErr
		}
		events = append(events, event)

		result, loadErr = s.hydrateCall(ctx, store, callRow.ID)
		return loadErr
	})
	if err != nil {
		return Call{}, Participant{}, nil, err
	}

	for i := range events {
		events[i].Call = cloneCall(result)
	}

	return result, cloneParticipant(saved), cloneEvents(events), nil
}

func canModerateGroupCall(role conversation.MemberRole) bool {
	return role == conversation.MemberRoleOwner || role == conversation.MemberRoleAdmin
}
