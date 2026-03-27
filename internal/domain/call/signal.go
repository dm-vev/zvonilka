package call

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	callMetadataSessionID       = "session_id"
	callMetadataTargetAccountID = "target_account_id"
	callMetadataTargetDeviceID  = "target_device_id"
	callMetadataDescriptionType = "description_type"
	callMetadataDescriptionSDP  = "description_sdp"
	callMetadataCandidateValue  = "candidate"
	callMetadataCandidateSDPMid = "candidate_sdp_mid"
	callMetadataCandidateLine   = "candidate_sdp_mline_index"
	callMetadataCandidateUfrag  = "candidate_username_fragment"
)

// PublishDescription appends one SDP signaling event for a joined participant.
func (s *Service) PublishDescription(ctx context.Context, params PublishDescriptionParams) (Event, error) {
	if err := s.validateContext(ctx, "publish call description"); err != nil {
		return Event{}, err
	}

	params.CallID = strings.TrimSpace(params.CallID)
	params.SessionID = strings.TrimSpace(params.SessionID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	params.Description.Type = strings.ToLower(strings.TrimSpace(params.Description.Type))

	if params.CallID == "" || params.SessionID == "" || params.AccountID == "" || params.DeviceID == "" {
		return Event{}, ErrInvalidInput
	}
	if params.Description.Type == "" || strings.TrimSpace(params.Description.SDP) == "" {
		return Event{}, ErrInvalidInput
	}
	if !validDescriptionType(params.Description.Type) {
		return Event{}, ErrInvalidInput
	}

	return s.publishSignalEvent(
		ctx,
		params.CallID,
		params.SessionID,
		params.AccountID,
		params.DeviceID,
		EventTypeSignalDescription,
		map[string]string{
			callMetadataDescriptionType: params.Description.Type,
			callMetadataDescriptionSDP:  params.Description.SDP,
		},
	)
}

// PublishIceCandidate appends one ICE-candidate signaling event for a joined participant.
func (s *Service) PublishIceCandidate(ctx context.Context, params PublishCandidateParams) (Event, error) {
	if err := s.validateContext(ctx, "publish call ice candidate"); err != nil {
		return Event{}, err
	}

	params.CallID = strings.TrimSpace(params.CallID)
	params.SessionID = strings.TrimSpace(params.SessionID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	params.IceCandidate.Candidate = strings.TrimSpace(params.IceCandidate.Candidate)
	params.IceCandidate.SDPMid = strings.TrimSpace(params.IceCandidate.SDPMid)
	params.IceCandidate.UsernameFragment = strings.TrimSpace(params.IceCandidate.UsernameFragment)

	if params.CallID == "" || params.SessionID == "" || params.AccountID == "" || params.DeviceID == "" {
		return Event{}, ErrInvalidInput
	}
	if params.IceCandidate.Candidate == "" {
		return Event{}, ErrInvalidInput
	}

	metadata := map[string]string{
		callMetadataCandidateValue: params.IceCandidate.Candidate,
		callMetadataCandidateLine:  fmt.Sprintf("%d", params.IceCandidate.SDPMLineIndex),
	}
	if params.IceCandidate.SDPMid != "" {
		metadata[callMetadataCandidateSDPMid] = params.IceCandidate.SDPMid
	}
	if params.IceCandidate.UsernameFragment != "" {
		metadata[callMetadataCandidateUfrag] = params.IceCandidate.UsernameFragment
	}

	return s.publishSignalEvent(
		ctx,
		params.CallID,
		params.SessionID,
		params.AccountID,
		params.DeviceID,
		EventTypeSignalCandidate,
		metadata,
	)
}

func (s *Service) publishSignalEvent(
	ctx context.Context,
	callID string,
	sessionID string,
	accountID string,
	deviceID string,
	eventType EventType,
	metadata map[string]string,
) (Event, error) {
	now := s.currentTime()

	var result Event
	err := s.runTx(ctx, func(store Store) error {
		callRow, loadErr := s.visibleCall(ctx, store, callID, accountID)
		if loadErr != nil {
			return loadErr
		}
		if callRow.State != StateActive {
			return ErrConflict
		}
		if strings.TrimSpace(callRow.ActiveSessionID) == "" || callRow.ActiveSessionID != sessionID {
			return ErrConflict
		}

		participant, loadErr := store.ParticipantByCallAndDevice(ctx, callRow.ID, deviceID)
		if loadErr != nil {
			if loadErr == ErrNotFound {
				return ErrForbidden
			}
			return fmt.Errorf("load call participant %s/%s: %w", callRow.ID, deviceID, loadErr)
		}
		if participant.AccountID != accountID || participant.State != ParticipantStateJoined {
			return ErrForbidden
		}

		var generated []RuntimeSignal
		switch eventType {
		case EventTypeSignalDescription:
			runtimeSignals, runtimeErr := s.runtime.PublishDescription(ctx, sessionID, RuntimeParticipant{
				CallID:    callRow.ID,
				AccountID: accountID,
				DeviceID:  deviceID,
				WithVideo: participant.MediaState.CameraEnabled,
			}, SessionDescription{
				Type: metadata[callMetadataDescriptionType],
				SDP:  metadata[callMetadataDescriptionSDP],
			})
			if runtimeErr != nil {
				return fmt.Errorf("publish runtime description %s/%s: %w", callRow.ID, sessionID, runtimeErr)
			}
			generated = runtimeSignals
		case EventTypeSignalCandidate:
			line := uint32(0)
			if _, scanErr := fmt.Sscanf(metadata[callMetadataCandidateLine], "%d", &line); scanErr != nil {
				return ErrInvalidInput
			}
			runtimeSignals, runtimeErr := s.runtime.PublishCandidate(ctx, sessionID, RuntimeParticipant{
				CallID:    callRow.ID,
				AccountID: accountID,
				DeviceID:  deviceID,
				WithVideo: participant.MediaState.CameraEnabled,
			}, Candidate{
				Candidate:        metadata[callMetadataCandidateValue],
				SDPMid:           metadata[callMetadataCandidateSDPMid],
				SDPMLineIndex:    line,
				UsernameFragment: metadata[callMetadataCandidateUfrag],
			})
			if runtimeErr != nil {
				return fmt.Errorf("publish runtime candidate %s/%s: %w", callRow.ID, sessionID, runtimeErr)
			}
			generated = runtimeSignals
		}

		metadata = cloneSignalMetadata(metadata)
		metadata[callMetadataSessionID] = sessionID

		event, appendErr := s.appendEvent(ctx, store, callRow, eventType, accountID, deviceID, metadata, now)
		if appendErr != nil {
			return appendErr
		}
		if appendErr := s.appendGeneratedSignals(ctx, store, callRow, generated, now); appendErr != nil {
			return appendErr
		}

		hydrated, loadErr := s.hydrateCall(ctx, store, callRow.ID)
		if loadErr != nil {
			return loadErr
		}
		event.Call = hydrated
		result = cloneEvent(event)

		return nil
	})
	if err != nil {
		return Event{}, err
	}

	return result, nil
}

func validDescriptionType(value string) bool {
	switch value {
	case "offer", "answer", "pranswer", "rollback":
		return true
	default:
		return false
	}
}

func cloneSignalMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}

	return dst
}

func (s *Service) appendGeneratedSignals(
	ctx context.Context,
	store Store,
	callRow Call,
	signals []RuntimeSignal,
	createdAt time.Time,
) error {
	for _, signal := range signals {
		metadata := map[string]string{
			callMetadataSessionID: signal.SessionID,
		}
		if signal.TargetAccountID != "" {
			metadata[callMetadataTargetAccountID] = signal.TargetAccountID
		}
		if signal.TargetDeviceID != "" {
			metadata[callMetadataTargetDeviceID] = signal.TargetDeviceID
		}

		eventType := EventTypeUnspecified
		switch {
		case signal.Description != nil:
			eventType = EventTypeSignalDescription
			metadata[callMetadataDescriptionType] = signal.Description.Type
			metadata[callMetadataDescriptionSDP] = signal.Description.SDP
		case signal.IceCandidate != nil:
			eventType = EventTypeSignalCandidate
			metadata[callMetadataCandidateValue] = signal.IceCandidate.Candidate
			metadata[callMetadataCandidateLine] = fmt.Sprintf("%d", signal.IceCandidate.SDPMLineIndex)
			if signal.IceCandidate.SDPMid != "" {
				metadata[callMetadataCandidateSDPMid] = signal.IceCandidate.SDPMid
			}
			if signal.IceCandidate.UsernameFragment != "" {
				metadata[callMetadataCandidateUfrag] = signal.IceCandidate.UsernameFragment
			}
		case len(signal.Metadata) > 0:
			eventType = EventTypeMediaUpdated
			for key, value := range signal.Metadata {
				metadata[key] = value
			}
		default:
			continue
		}

		if _, err := s.appendEvent(ctx, store, callRow, eventType, "", "", metadata, createdAt); err != nil {
			return err
		}
	}

	return nil
}
