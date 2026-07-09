package call

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// StartCall creates a new call for a callable conversation and invite rows for peer members.
func (s *Service) StartCall(ctx context.Context, params StartParams) (Call, []Event, error) {
	if err := s.validateContext(ctx, "start call"); err != nil {
		return Call{}, nil, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.ConversationID == "" || params.AccountID == "" || params.DeviceID == "" {
		return Call{}, nil, ErrInvalidInput
	}

	conversationRow, members, err := s.ensureCallableConversation(ctx, params.ConversationID, params.AccountID)
	if err != nil {
		return Call{}, nil, err
	}
	groupCall := conversationRow.Kind == conversation.ConversationKindGroup

	now := s.currentTime()
	var result Call
	var events []Event
	err = s.runTx(ctx, func(store Store) error {
		if active, activeErr := store.ActiveCallByConversation(ctx, params.ConversationID); activeErr == nil {
			active, activeErr = s.expireCallIfNeeded(ctx, store, active)
			if activeErr != nil {
				return activeErr
			}
			if active.State != StateEnded {
				return ErrConflict
			}
		} else if activeErr != ErrNotFound {
			return fmt.Errorf("load active call for conversation %s: %w", params.ConversationID, activeErr)
		}

		callID, idErr := newID("call")
		if idErr != nil {
			return fmt.Errorf("generate call id: %w", idErr)
		}

		callRow, saveErr := store.SaveCall(ctx, Call{
			ID:                 callID,
			ConversationID:     conversationRow.ID,
			InitiatorAccountID: params.AccountID,
			HostAccountID:      params.AccountID,
			RequestedVideo:     params.WithVideo,
			State:              stateForConversationKind(conversationRow.Kind),
			RecordingState:     RecordingStateInactive,
			TranscriptionState: TranscriptionStateInactive,
			StartedAt:          now,
			AnsweredAt:         answeredAtForConversationKind(conversationRow.Kind, now),
			UpdatedAt:          now,
		})
		if saveErr != nil {
			return fmt.Errorf("save call %s: %w", callID, saveErr)
		}

		targets := directTargets(members, params.AccountID)
		if len(targets) == 0 {
			return ErrConflict
		}

		startedEvent, appendErr := s.appendEvent(ctx, store, callRow, EventTypeStarted, params.AccountID, params.DeviceID, map[string]string{
			"with_video": boolString(params.WithVideo),
			"call_scope": string(conversationRow.Kind),
		}, now)
		if appendErr != nil {
			return appendErr
		}
		events = append(events, startedEvent)

		for _, target := range targets {
			inviteRow := Invite{
				CallID:    callRow.ID,
				AccountID: target,
				State:     InviteStatePending,
				ExpiresAt: now.Add(s.settings.InviteTimeout),
				UpdatedAt: now,
			}
			if _, saveErr := store.SaveInvite(ctx, inviteRow); saveErr != nil {
				return fmt.Errorf("save invite %s/%s: %w", inviteRow.CallID, inviteRow.AccountID, saveErr)
			}
			invitedEvent, appendErr := s.appendEvent(ctx, store, callRow, EventTypeInvited, params.AccountID, params.DeviceID, map[string]string{
				"target_account_id": target,
				"call_scope":        string(conversationRow.Kind),
			}, now)
			if appendErr != nil {
				return appendErr
			}
			events = append(events, invitedEvent)
		}

		if groupCall {
			selfParticipant, saveErr := store.SaveParticipant(ctx, Participant{
				CallID:    callRow.ID,
				AccountID: params.AccountID,
				DeviceID:  params.DeviceID,
				State:     ParticipantStateJoined,
				MediaState: MediaState{
					CameraEnabled: params.WithVideo,
				},
				JoinedAt:  now,
				UpdatedAt: now,
			})
			if saveErr != nil {
				return fmt.Errorf("save group initiator participant %s/%s: %w", callRow.ID, params.DeviceID, saveErr)
			}

			runtimeSession, runtimeErr := s.runtime.EnsureSession(ctx, callRow)
			if runtimeErr != nil {
				return fmt.Errorf("ensure runtime session for group call %s: %w", callRow.ID, runtimeErr)
			}
			callRow.ActiveSessionID = runtimeSession.SessionID
			callRow.UpdatedAt = now
			savedCall, saveErr := store.SaveCall(ctx, callRow)
			if saveErr != nil {
				return fmt.Errorf("save runtime session for group call %s: %w", callRow.ID, saveErr)
			}
			callRow = savedCall

			if _, runtimeErr = s.runtime.JoinSession(ctx, callRow.ActiveSessionID, RuntimeParticipant{
				CallID:    callRow.ID,
				AccountID: params.AccountID,
				DeviceID:  params.DeviceID,
				WithVideo: params.WithVideo,
				Media:     selfParticipant.MediaState,
			}); runtimeErr != nil {
				return fmt.Errorf("join runtime session for group initiator %s/%s: %w", callRow.ID, params.DeviceID, runtimeErr)
			}

			joinedEvent, appendErr := s.appendEvent(ctx, store, callRow, EventTypeJoined, params.AccountID, params.DeviceID, map[string]string{
				"with_video": boolString(params.WithVideo),
				"call_scope": string(conversationRow.Kind),
			}, now)
			if appendErr != nil {
				return appendErr
			}
			events = append(events, joinedEvent)
		}

		result, saveErr = s.hydrateCall(ctx, store, callRow.ID)
		if saveErr != nil {
			return saveErr
		}
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

func directTargets(members []conversation.ConversationMember, accountID string) []string {
	targets := make([]string, 0, len(members))
	for _, member := range members {
		if member.AccountID == accountID {
			continue
		}
		targets = append(targets, member.AccountID)
	}
	return targets
}

func stateForConversationKind(kind conversation.ConversationKind) State {
	if kind == conversation.ConversationKindGroup {
		return StateActive
	}
	return StateRinging
}

func answeredAtForConversationKind(kind conversation.ConversationKind, now time.Time) time.Time {
	if kind == conversation.ConversationKindGroup {
		return now
	}
	return time.Time{}
}

func boolString(value bool) string {
	if value {
		return "true"
	}

	return "false"
}
