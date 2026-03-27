package call

import (
	"context"
	"fmt"
	"strings"
)

const (
	callMetadataSessionID       = "session_id"
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
	params.Description.SDP = strings.TrimSpace(params.Description.SDP)

	if params.CallID == "" || params.SessionID == "" || params.AccountID == "" || params.DeviceID == "" {
		return Event{}, ErrInvalidInput
	}
	if params.Description.Type == "" || params.Description.SDP == "" {
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

		metadata = cloneSignalMetadata(metadata)
		metadata[callMetadataSessionID] = sessionID

		event, appendErr := s.appendEvent(ctx, store, callRow, eventType, accountID, deviceID, metadata, now)
		if appendErr != nil {
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
