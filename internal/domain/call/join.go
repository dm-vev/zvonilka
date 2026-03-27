package call

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AcceptCall accepts one pending incoming invite.
func (s *Service) AcceptCall(ctx context.Context, params AcceptParams) (Call, []Event, error) {
	if err := s.validateContext(ctx, "accept call"); err != nil {
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
		if callRow.State == StateEnded {
			return ErrConflict
		}

		invite, loadErr := store.InviteByCallAndAccount(ctx, callRow.ID, params.AccountID)
		if loadErr != nil {
			if loadErr == ErrNotFound {
				return ErrForbidden
			}
			return fmt.Errorf("load invite %s/%s: %w", callRow.ID, params.AccountID, loadErr)
		}
		if invite.State == InviteStateAccepted {
			result = callRow
			return nil
		}
		if invite.State != InviteStatePending {
			return ErrConflict
		}

		invite.State = InviteStateAccepted
		invite.AnsweredAt = now
		invite.UpdatedAt = now
		if _, saveErr := store.SaveInvite(ctx, invite); saveErr != nil {
			return fmt.Errorf("save invite %s/%s: %w", invite.CallID, invite.AccountID, saveErr)
		}

		if callRow.State == StateRinging {
			callRow.State = StateActive
			callRow.AnsweredAt = now
			callRow.UpdatedAt = now
			saved, saveErr := store.SaveCall(ctx, callRow)
			if saveErr != nil {
				return fmt.Errorf("activate call %s: %w", callRow.ID, saveErr)
			}
			callRow = saved
		}

		event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeAccepted, params.AccountID, params.DeviceID, nil, now)
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

// DeclineCall declines one pending invite and ends the call if no live invite remains.
func (s *Service) DeclineCall(ctx context.Context, params DeclineParams) (Call, []Event, error) {
	if err := s.validateContext(ctx, "decline call"); err != nil {
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
		if callRow.State == StateEnded {
			result = callRow
			return nil
		}

		invite, loadErr := store.InviteByCallAndAccount(ctx, callRow.ID, params.AccountID)
		if loadErr != nil {
			if loadErr == ErrNotFound {
				return ErrForbidden
			}
			return fmt.Errorf("load invite %s/%s: %w", callRow.ID, params.AccountID, loadErr)
		}
		if invite.State == InviteStateDeclined {
			result = callRow
			return nil
		}
		if invite.State != InviteStatePending {
			return ErrConflict
		}

		invite.State = InviteStateDeclined
		invite.AnsweredAt = now
		invite.UpdatedAt = now
		if _, saveErr := store.SaveInvite(ctx, invite); saveErr != nil {
			return fmt.Errorf("save invite %s/%s: %w", invite.CallID, invite.AccountID, saveErr)
		}

		event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeDeclined, params.AccountID, params.DeviceID, nil, now)
		if appendErr != nil {
			return appendErr
		}
		events = append(events, event)

		invites, loadErr := store.InvitesByCall(ctx, callRow.ID)
		if loadErr != nil {
			return fmt.Errorf("load invites for call %s: %w", callRow.ID, loadErr)
		}
		if liveInvite(invites) {
			result, loadErr = s.hydrateCall(ctx, store, callRow.ID)
			return loadErr
		}

		callRow.State = StateEnded
		callRow.EndReason = EndReasonDeclined
		callRow.EndedAt = now
		callRow.UpdatedAt = now
		callRow, loadErr = store.SaveCall(ctx, callRow)
		if loadErr != nil {
			return fmt.Errorf("end declined call %s: %w", callRow.ID, loadErr)
		}

		event, appendErr = s.appendEvent(ctx, store, callRow, EventTypeEnded, params.AccountID, params.DeviceID, map[string]string{
			"reason": string(EndReasonDeclined),
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

// CancelCall cancels a ringing call from the initiator side.
func (s *Service) CancelCall(ctx context.Context, params CancelParams) (Call, []Event, error) {
	if err := s.validateContext(ctx, "cancel call"); err != nil {
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
		if callRow.InitiatorAccountID != params.AccountID {
			return ErrForbidden
		}
		if callRow.State == StateEnded {
			result = callRow
			return nil
		}
		if callRow.State != StateRinging {
			return ErrConflict
		}

		callRow.State = StateEnded
		callRow.EndReason = EndReasonCancelled
		callRow.EndedAt = now
		callRow.UpdatedAt = now
		saved, saveErr := store.SaveCall(ctx, callRow)
		if saveErr != nil {
			return fmt.Errorf("cancel call %s: %w", callRow.ID, saveErr)
		}
		callRow = saved

		invites, loadErr := store.InvitesByCall(ctx, callRow.ID)
		if loadErr != nil {
			return fmt.Errorf("load invites for call %s: %w", callRow.ID, loadErr)
		}
		for _, invite := range invites {
			if invite.State != InviteStatePending {
				continue
			}
			invite.State = InviteStateCancelled
			invite.AnsweredAt = now
			invite.UpdatedAt = now
			if _, saveErr := store.SaveInvite(ctx, invite); saveErr != nil {
				return fmt.Errorf("cancel invite %s/%s: %w", invite.CallID, invite.AccountID, saveErr)
			}
		}

		event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeEnded, params.AccountID, params.DeviceID, map[string]string{
			"reason": string(EndReasonCancelled),
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

// EndCall ends one active call from the initiator or an active participant device.
func (s *Service) EndCall(ctx context.Context, params EndParams) (Call, []Event, error) {
	if err := s.validateContext(ctx, "end call"); err != nil {
		return Call{}, nil, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.CallID == "" || params.AccountID == "" || params.DeviceID == "" {
		return Call{}, nil, ErrInvalidInput
	}
	if params.Reason == EndReasonUnspecified {
		params.Reason = EndReasonEnded
	}

	now := s.currentTime()
	var result Call
	var events []Event
	var sessionID string
	err := s.runTx(ctx, func(store Store) error {
		callRow, loadErr := s.visibleCall(ctx, store, params.CallID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		if callRow.State == StateEnded {
			result = callRow
			return nil
		}
		if callRow.InitiatorAccountID != params.AccountID {
			participant, loadErr := store.ParticipantByCallAndDevice(ctx, callRow.ID, params.DeviceID)
			if loadErr != nil || participant.AccountID != params.AccountID || participant.State != ParticipantStateJoined {
				return ErrForbidden
			}
		}

		callRow.State = StateEnded
		callRow.EndReason = params.Reason
		callRow.EndedAt = now
		callRow.UpdatedAt = now
		saved, saveErr := store.SaveCall(ctx, callRow)
		if saveErr != nil {
			return fmt.Errorf("end call %s: %w", callRow.ID, saveErr)
		}
		callRow = saved
		sessionID = callRow.ActiveSessionID

		event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeEnded, params.AccountID, params.DeviceID, map[string]string{
			"reason": string(params.Reason),
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

	if sessionID != "" && result.State == StateEnded {
		if closeErr := s.runtime.CloseSession(ctx, sessionID); closeErr != nil {
			return Call{}, nil, fmt.Errorf("close runtime session %s: %w", sessionID, closeErr)
		}
	}

	for i := range events {
		events[i].Call = cloneCall(result)
	}

	return result, cloneEvents(events), nil
}

// JoinCall joins or rejoins one device to the media session.
func (s *Service) JoinCall(ctx context.Context, params JoinParams) (Call, JoinDetails, []Event, error) {
	if err := s.validateContext(ctx, "join call"); err != nil {
		return Call{}, JoinDetails{}, nil, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.CallID == "" || params.AccountID == "" || params.DeviceID == "" {
		return Call{}, JoinDetails{}, nil, ErrInvalidInput
	}

	now := s.currentTime()
	var result Call
	var details JoinDetails
	var events []Event
	err := s.runTx(ctx, func(store Store) error {
		callRow, loadErr := s.visibleCall(ctx, store, params.CallID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		if callRow.State == StateEnded {
			return ErrConflict
		}

		if callRow.InitiatorAccountID != params.AccountID {
			invite, loadErr := store.InviteByCallAndAccount(ctx, callRow.ID, params.AccountID)
			if loadErr != nil {
				if loadErr == ErrNotFound {
					return ErrForbidden
				}
				return fmt.Errorf("load invite %s/%s: %w", callRow.ID, params.AccountID, loadErr)
			}
			if invite.State != InviteStateAccepted {
				return ErrConflict
			}
		}

		if callRow.ActiveSessionID == "" {
			runtimeSession, runtimeErr := s.runtime.EnsureSession(ctx, callRow)
			if runtimeErr != nil {
				return fmt.Errorf("ensure runtime session for %s: %w", callRow.ID, runtimeErr)
			}
			callRow.ActiveSessionID = runtimeSession.SessionID
			callRow.UpdatedAt = now
			saved, saveErr := store.SaveCall(ctx, callRow)
			if saveErr != nil {
				return fmt.Errorf("save runtime session for %s: %w", callRow.ID, saveErr)
			}
			callRow = saved
		}

		runtimeJoin, runtimeErr := s.runtime.JoinSession(ctx, callRow.ActiveSessionID, RuntimeParticipant{
			CallID:    callRow.ID,
			AccountID: params.AccountID,
			DeviceID:  params.DeviceID,
			WithVideo: params.WithVideo,
			Media:     defaultJoinMediaState(params.WithVideo),
		})
		if runtimeErr != nil {
			return fmt.Errorf("join runtime session %s: %w", callRow.ActiveSessionID, runtimeErr)
		}

		participant, loadErr := store.ParticipantByCallAndDevice(ctx, callRow.ID, params.DeviceID)
		if loadErr != nil && loadErr != ErrNotFound {
			return fmt.Errorf("load participant %s/%s: %w", callRow.ID, params.DeviceID, loadErr)
		}
		if loadErr == ErrNotFound {
			participant = Participant{
				CallID:    callRow.ID,
				AccountID: params.AccountID,
				DeviceID:  params.DeviceID,
				JoinedAt:  now,
			}
		}
		participant.State = ParticipantStateJoined
		participant.MediaState = defaultJoinMediaState(params.WithVideo)
		participant.LeftAt = timeZero()
		participant.UpdatedAt = now
		savedParticipant, saveErr := store.SaveParticipant(ctx, participant)
		if saveErr != nil {
			return fmt.Errorf("save participant %s/%s: %w", participant.CallID, participant.DeviceID, saveErr)
		}

		event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeJoined, params.AccountID, params.DeviceID, map[string]string{
			"with_video": boolString(params.WithVideo),
		}, now)
		if appendErr != nil {
			return appendErr
		}
		events = append(events, event)

		result, loadErr = s.hydrateCall(ctx, store, callRow.ID)
		if loadErr != nil {
			return loadErr
		}

		iceServers, expiresAt, iceErr := s.iceServersForAccount(params.AccountID, runtimeJoin.ExpiresAt)
		if iceErr != nil {
			return iceErr
		}
		details = JoinDetails{
			SessionID:       runtimeJoin.SessionID,
			SessionToken:    runtimeJoin.SessionToken,
			RuntimeEndpoint: s.resolveRuntimeEndpoint(runtimeJoin.RuntimeEndpoint),
			ExpiresAt:       expiresAt,
			IceUfrag:        runtimeJoin.IceUfrag,
			IcePwd:          runtimeJoin.IcePwd,
			DTLSFingerprint: runtimeJoin.DTLSFingerprint,
			CandidateHost:   runtimeJoin.CandidateHost,
			CandidatePort:   runtimeJoin.CandidatePort,
			IceServers:      iceServers,
		}
		_ = savedParticipant
		return nil
	})
	if err != nil {
		return Call{}, JoinDetails{}, nil, err
	}

	for i := range events {
		events[i].Call = cloneCall(result)
	}

	return result, details, cloneEvents(events), nil
}

// LeaveCall removes one device from the active participant set.
func (s *Service) LeaveCall(ctx context.Context, params LeaveParams) (Call, []Event, error) {
	if err := s.validateContext(ctx, "leave call"); err != nil {
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
	var sessionID string
	var closeRuntime bool
	err := s.runTx(ctx, func(store Store) error {
		callRow, loadErr := s.visibleCall(ctx, store, params.CallID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		if callRow.State == StateEnded {
			result = callRow
			return nil
		}

		participant, loadErr := store.ParticipantByCallAndDevice(ctx, callRow.ID, params.DeviceID)
		if loadErr != nil {
			if loadErr == ErrNotFound {
				result = callRow
				return nil
			}
			return fmt.Errorf("load participant %s/%s: %w", callRow.ID, params.DeviceID, loadErr)
		}
		if participant.AccountID != params.AccountID {
			return ErrForbidden
		}
		if participant.State == ParticipantStateJoined {
			participant.State = ParticipantStateLeft
			participant.LeftAt = now
			participant.UpdatedAt = now
			if _, saveErr := store.SaveParticipant(ctx, participant); saveErr != nil {
				return fmt.Errorf("save participant %s/%s: %w", participant.CallID, participant.DeviceID, saveErr)
			}
			event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeLeft, params.AccountID, params.DeviceID, nil, now)
			if appendErr != nil {
				return appendErr
			}
			events = append(events, event)
		}

		participants, loadErr := store.ParticipantsByCall(ctx, callRow.ID)
		if loadErr != nil {
			return fmt.Errorf("load participants for call %s: %w", callRow.ID, loadErr)
		}
		if joinedParticipants(participants) == 0 {
			sessionID = callRow.ActiveSessionID
			closeRuntime = false
		}

		result, loadErr = s.hydrateCall(ctx, store, callRow.ID)
		return loadErr
	})
	if err != nil {
		return Call{}, nil, err
	}

	if result.ActiveSessionID != "" {
		if leaveErr := s.runtime.LeaveSession(ctx, result.ActiveSessionID, params.AccountID, params.DeviceID); leaveErr != nil {
			return Call{}, nil, fmt.Errorf("leave runtime session %s: %w", result.ActiveSessionID, leaveErr)
		}
	}
	if closeRuntime {
		if closeErr := s.runtime.CloseSession(ctx, sessionID); closeErr != nil {
			return Call{}, nil, fmt.Errorf("close runtime session %s: %w", sessionID, closeErr)
		}
	}

	for i := range events {
		events[i].Call = cloneCall(result)
	}

	return result, cloneEvents(events), nil
}

// UpdateCallMediaState updates one joined participant media-state flags.
func (s *Service) UpdateCallMediaState(
	ctx context.Context,
	params UpdateParams,
) (Call, Participant, []Event, error) {
	if err := s.validateContext(ctx, "update call media state"); err != nil {
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
		if callRow.State == StateEnded {
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

		participant.MediaState = params.Media
		participant.UpdatedAt = now
		savedParticipant, saveErr := store.SaveParticipant(ctx, participant)
		if saveErr != nil {
			return fmt.Errorf("save participant media %s/%s: %w", participant.CallID, participant.DeviceID, saveErr)
		}
		saved = savedParticipant
		if callRow.ActiveSessionID != "" {
			runtimeErr := s.runtime.UpdateParticipant(ctx, callRow.ActiveSessionID, RuntimeParticipant{
				CallID:    callRow.ID,
				AccountID: params.AccountID,
				DeviceID:  params.DeviceID,
				WithVideo: params.Media.CameraEnabled,
				Media:     params.Media,
			})
			if runtimeErr != nil {
				return fmt.Errorf("update runtime participant %s/%s: %w", callRow.ID, params.DeviceID, runtimeErr)
			}
		}

		event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeMediaUpdated, params.AccountID, params.DeviceID, map[string]string{
			"audio_muted":          boolString(params.Media.AudioMuted),
			"video_muted":          boolString(params.Media.VideoMuted),
			"camera_enabled":       boolString(params.Media.CameraEnabled),
			"screen_share_enabled": boolString(params.Media.ScreenShareEnabled),
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

	for i := range events {
		events[i].Call = cloneCall(result)
	}

	return result, cloneParticipant(saved), cloneEvents(events), nil
}

// AcknowledgeCallAdaptation confirms that one joined participant applied the current adaptation revision.
func (s *Service) AcknowledgeCallAdaptation(
	ctx context.Context,
	params AcknowledgeAdaptationParams,
) (Call, Participant, []Event, error) {
	if err := s.validateContext(ctx, "acknowledge call adaptation"); err != nil {
		return Call{}, Participant{}, nil, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.SessionID = strings.TrimSpace(params.SessionID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	params.AppliedProfile = strings.TrimSpace(params.AppliedProfile)
	if params.CallID == "" || params.SessionID == "" || params.AccountID == "" || params.DeviceID == "" {
		return Call{}, Participant{}, nil, ErrInvalidInput
	}
	if params.AdaptationRevision == 0 || params.AppliedProfile == "" {
		return Call{}, Participant{}, nil, ErrInvalidInput
	}

	now := s.currentTime()
	var result Call
	var acknowledged Participant
	var events []Event
	err := s.runTx(ctx, func(store Store) error {
		callRow, loadErr := s.visibleCall(ctx, store, params.CallID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		if callRow.State != StateActive || strings.TrimSpace(callRow.ActiveSessionID) != params.SessionID {
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

		if runtimeErr := s.runtime.AcknowledgeAdaptation(ctx, params.SessionID, RuntimeParticipant{
			CallID:    callRow.ID,
			AccountID: params.AccountID,
			DeviceID:  params.DeviceID,
			WithVideo: participant.MediaState.CameraEnabled,
			Media:     participant.MediaState,
		}, params.AdaptationRevision, params.AppliedProfile); runtimeErr != nil {
			return fmt.Errorf("acknowledge runtime adaptation %s/%s: %w", callRow.ID, params.DeviceID, runtimeErr)
		}

		event, appendErr := s.appendEvent(ctx, store, callRow, EventTypeMediaUpdated, params.AccountID, params.DeviceID, map[string]string{
			"adaptation_revision":       fmt.Sprintf("%d", params.AdaptationRevision),
			"pending_adaptation":        "false",
			"acked_adaptation_revision": fmt.Sprintf("%d", params.AdaptationRevision),
			"applied_profile":           params.AppliedProfile,
		}, now)
		if appendErr != nil {
			return appendErr
		}
		events = append(events, event)

		result, loadErr = s.hydrateCall(ctx, store, callRow.ID)
		if loadErr != nil {
			return loadErr
		}
		for _, item := range result.Participants {
			if item.AccountID == params.AccountID && item.DeviceID == params.DeviceID {
				acknowledged = cloneParticipant(item)
				break
			}
		}

		return nil
	})
	if err != nil {
		return Call{}, Participant{}, nil, err
	}

	for i := range events {
		events[i].Call = cloneCall(result)
	}

	return result, acknowledged, cloneEvents(events), nil
}

func liveInvite(invites []Invite) bool {
	for _, invite := range invites {
		if invite.State == InviteStatePending || invite.State == InviteStateAccepted {
			return true
		}
	}

	return false
}

func joinedParticipants(participants []Participant) int {
	count := 0
	for _, participant := range participants {
		if participant.State == ParticipantStateJoined {
			count++
		}
	}

	return count
}

func defaultJoinMediaState(withVideo bool) MediaState {
	return MediaState{
		AudioMuted:         false,
		VideoMuted:         !withVideo,
		CameraEnabled:      withVideo,
		ScreenShareEnabled: false,
	}
}

func timeZero() time.Time {
	return time.Time{}
}
