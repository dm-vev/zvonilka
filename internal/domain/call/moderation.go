package call

import (
	"context"
	"fmt"
	"slices"
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
		if !canManageGroupCall(callRow, member.Role, params.AccountID) {
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

func canManageGroupCall(callRow Call, role conversation.MemberRole, accountID string) bool {
	if strings.TrimSpace(accountID) == currentGroupCallHost(callRow) {
		return true
	}

	return canModerateGroupCall(role)
}

// MuteAllParticipants applies host mute flags to every joined participant except the acting host device.
func (s *Service) MuteAllParticipants(
	ctx context.Context,
	params MuteAllParticipantsParams,
) (Call, []Participant, []Event, error) {
	if err := s.validateContext(ctx, "mute all participants"); err != nil {
		return Call{}, nil, nil, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.CallID == "" || params.AccountID == "" || params.DeviceID == "" {
		return Call{}, nil, nil, ErrInvalidInput
	}

	now := s.currentTime()
	var result Call
	var participants []Participant
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
		if !canManageGroupCall(callRow, member.Role, params.AccountID) {
			return ErrForbidden
		}

		currentParticipants, loadErr := store.ParticipantsByCall(ctx, callRow.ID)
		if loadErr != nil {
			return fmt.Errorf("load participants for call %s: %w", callRow.ID, loadErr)
		}
		for _, participant := range currentParticipants {
			if participant.State != ParticipantStateJoined {
				continue
			}
			if participant.AccountID == params.AccountID && participant.DeviceID == params.DeviceID {
				continue
			}
			participant.HostMutedAudio = params.MuteAudio
			participant.HostMutedVideo = params.MuteVideo
			if params.LowerHands {
				participant.HandRaised = false
				participant.RaisedHandAt = time.Time{}
			}
			participant.UpdatedAt = now
			saved, saveErr := store.SaveParticipant(ctx, participant)
			if saveErr != nil {
				return fmt.Errorf("save muted participant %s/%s: %w", participant.CallID, participant.DeviceID, saveErr)
			}
			participants = append(participants, saved)
		}

		event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeMediaUpdated, params.AccountID, params.DeviceID, map[string]string{
			"mute_all_audio": boolString(params.MuteAudio),
			"mute_all_video": boolString(params.MuteVideo),
			"lower_hands":    boolString(params.LowerHands),
		}, now)
		if appendErr != nil {
			return appendErr
		}
		events = append(events, event)

		result, loadErr = s.hydrateCall(ctx, store, callRow.ID)
		return loadErr
	})
	if err != nil {
		return Call{}, nil, nil, err
	}

	for i := range events {
		events[i].Call = cloneCall(result)
	}

	return result, cloneParticipants(participants), cloneEvents(events), nil
}

// RemoveParticipant removes one joined participant from an active group call.
func (s *Service) RemoveParticipant(
	ctx context.Context,
	params RemoveParticipantParams,
) (Call, Participant, []Event, error) {
	if err := s.validateContext(ctx, "remove participant"); err != nil {
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
	var removed Participant
	var events []Event
	var sessionID string
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
		if !canManageGroupCall(callRow, member.Role, params.AccountID) {
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
		if target.AccountID == params.AccountID && target.DeviceID == params.DeviceID {
			return ErrForbidden
		}

		target.State = ParticipantStateLeft
		target.LeftAt = now
		target.UpdatedAt = now
		saved, saveErr := store.SaveParticipant(ctx, target)
		if saveErr != nil {
			return fmt.Errorf("save removed participant %s/%s: %w", target.CallID, target.DeviceID, saveErr)
		}
		removed = saved
		sessionID = callRow.ActiveSessionID

		event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeLeft, params.AccountID, params.DeviceID, map[string]string{
			callMetadataTargetAccountID: target.AccountID,
			callMetadataTargetDeviceID:  target.DeviceID,
			"removed_by_host":           "true",
		}, now)
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

	if sessionID != "" {
		if leaveErr := s.runtime.LeaveSession(ctx, sessionID, removed.AccountID, removed.DeviceID); leaveErr != nil && leaveErr != ErrNotFound {
			return Call{}, Participant{}, nil, fmt.Errorf("remove runtime participant %s/%s: %w", removed.CallID, removed.DeviceID, leaveErr)
		}
	}

	for i := range events {
		events[i].Call = cloneCall(result)
	}

	return result, cloneParticipant(removed), cloneEvents(events), nil
}

// TransferHost transfers group-call host controls to another joined participant account.
func (s *Service) TransferHost(
	ctx context.Context,
	params TransferHostParams,
) (Call, []Event, error) {
	if err := s.validateContext(ctx, "transfer host"); err != nil {
		return Call{}, nil, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	params.TargetAccountID = strings.TrimSpace(params.TargetAccountID)
	if params.CallID == "" || params.AccountID == "" || params.DeviceID == "" || params.TargetAccountID == "" {
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
		if conversationRow.Kind != conversation.ConversationKindGroup || callRow.State != StateActive {
			return ErrConflict
		}
		if !canManageGroupCall(callRow, member.Role, params.AccountID) {
			return ErrForbidden
		}
		if params.TargetAccountID == currentGroupCallHost(callRow) {
			result, loadErr = s.hydrateCall(ctx, store, callRow.ID)
			return loadErr
		}

		participants, loadErr := store.ParticipantsByCall(ctx, callRow.ID)
		if loadErr != nil {
			return fmt.Errorf("load participants for call %s: %w", callRow.ID, loadErr)
		}
		joined := false
		for _, participant := range participants {
			if participant.AccountID == params.TargetAccountID && participant.State == ParticipantStateJoined {
				joined = true
				break
			}
		}
		if !joined {
			return ErrConflict
		}

		callRow.HostAccountID = params.TargetAccountID
		callRow.UpdatedAt = now
		saved, saveErr := store.SaveCall(ctx, callRow)
		if saveErr != nil {
			return fmt.Errorf("save transferred host %s: %w", callRow.ID, saveErr)
		}
		callRow = saved

		event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeMediaUpdated, params.AccountID, params.DeviceID, map[string]string{
			"host_transferred":  "true",
			"target_account_id": params.TargetAccountID,
			"previous_host_id":  params.AccountID,
			"current_host_id":   params.TargetAccountID,
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

// RaisedHands returns the current queue of raised hands in an active group call.
func (s *Service) RaisedHands(ctx context.Context, params GetParams) ([]Participant, error) {
	if err := s.validateContext(ctx, "list raised hands"); err != nil {
		return nil, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.CallID == "" || params.AccountID == "" {
		return nil, ErrInvalidInput
	}

	var participants []Participant
	err := s.runTx(ctx, func(store Store) error {
		callRow, loadErr := s.visibleCall(ctx, store, params.CallID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		conversationRow, _, loadErr := s.visibleConversation(ctx, callRow.ConversationID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		if conversationRow.Kind != conversation.ConversationKindGroup {
			return ErrConflict
		}

		rows, loadErr := store.ParticipantsByCall(ctx, callRow.ID)
		if loadErr != nil {
			return fmt.Errorf("load participants for call %s: %w", callRow.ID, loadErr)
		}
		for _, participant := range rows {
			if participant.State != ParticipantStateJoined || !participant.HandRaised {
				continue
			}
			participants = append(participants, participant)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.SortFunc(participants, func(left Participant, right Participant) int {
		if left.RaisedHandAt.Before(right.RaisedHandAt) {
			return -1
		}
		if left.RaisedHandAt.After(right.RaisedHandAt) {
			return 1
		}
		return strings.Compare(left.DeviceID, right.DeviceID)
	})

	return cloneParticipants(participants), nil
}
