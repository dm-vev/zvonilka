package call

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// GetCall returns one call visible to the requesting account.
func (s *Service) GetCall(ctx context.Context, params GetParams) (Call, error) {
	if err := s.validateContext(ctx, "get call"); err != nil {
		return Call{}, err
	}
	if strings.TrimSpace(params.CallID) == "" || strings.TrimSpace(params.AccountID) == "" {
		return Call{}, ErrInvalidInput
	}

	var result Call
	err := s.runTx(ctx, func(store Store) error {
		callRow, loadErr := s.visibleCall(ctx, store, params.CallID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		result = cloneCall(callRow)
		return nil
	})
	if err != nil {
		return Call{}, err
	}

	return result, nil
}

// ListCalls returns visible calls inside one conversation.
func (s *Service) ListCalls(ctx context.Context, params ListParams) ([]Call, error) {
	if err := s.validateContext(ctx, "list calls"); err != nil {
		return nil, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.ConversationID == "" || params.AccountID == "" {
		return nil, ErrInvalidInput
	}

	if _, _, err := s.visibleConversation(ctx, params.ConversationID, params.AccountID); err != nil {
		return nil, err
	}

	var calls []Call
	err := s.runTx(ctx, func(store Store) error {
		rows, listErr := store.CallsByConversation(ctx, params.ConversationID, params.IncludeEnded)
		if listErr != nil {
			return fmt.Errorf("list calls for conversation %s: %w", params.ConversationID, listErr)
		}

		calls = make([]Call, 0, len(rows))
		for _, row := range rows {
			visible, loadErr := s.visibleCall(ctx, store, row.ID, params.AccountID)
			if loadErr != nil {
				if loadErr == ErrNotFound {
					continue
				}
				return loadErr
			}
			if !params.IncludeEnded && visible.State == StateEnded {
				continue
			}
			calls = append(calls, cloneCall(visible))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return calls, nil
}

// Events returns the durable call-event log visible to one account.
func (s *Service) Events(ctx context.Context, params EventParams) ([]Event, error) {
	if err := s.validateContext(ctx, "list call events"); err != nil {
		return nil, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.AccountID == "" {
		return nil, ErrInvalidInput
	}
	if params.CallID == "" && params.ConversationID == "" {
		return nil, ErrInvalidInput
	}

	var conversationID string
	if params.CallID != "" {
		callRow, err := s.GetCall(ctx, GetParams{CallID: params.CallID, AccountID: params.AccountID})
		if err != nil {
			return nil, err
		}
		conversationID = callRow.ConversationID
	} else {
		if _, _, err := s.visibleConversation(ctx, params.ConversationID, params.AccountID); err != nil {
			return nil, err
		}
		conversationID = params.ConversationID
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	events, err := s.store.EventsAfterSequence(ctx, params.FromSequence, params.CallID, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list call events: %w", err)
	}

	hydrated := make(map[string]Call)
	filtered := make([]Event, 0, len(events))
	for i := range events {
		if !visibleSignalEvent(events[i], params.AccountID, params.DeviceID) {
			continue
		}
		callID := events[i].CallID
		if callID == "" {
			filtered = append(filtered, cloneEvent(events[i]))
			continue
		}
		callRow, ok := hydrated[callID]
		if !ok {
			loaded, loadErr := s.GetCall(ctx, GetParams{CallID: callID, AccountID: params.AccountID})
			if loadErr != nil {
				return nil, loadErr
			}
			callRow = loaded
			hydrated[callID] = loaded
		}
		events[i].Call = cloneCall(callRow)
		filtered = append(filtered, events[i])
	}

	return cloneEvents(filtered), nil
}

func visibleSignalEvent(event Event, accountID string, deviceID string) bool {
	targetAccountID := strings.TrimSpace(event.Metadata[callMetadataTargetAccountID])
	targetDeviceID := strings.TrimSpace(event.Metadata[callMetadataTargetDeviceID])
	if targetAccountID == "" && targetDeviceID == "" {
		return true
	}
	if targetAccountID != "" && targetAccountID != accountID {
		return false
	}
	if targetDeviceID != "" && targetDeviceID != deviceID {
		return false
	}

	return true
}

func (s *Service) visibleConversation(
	ctx context.Context,
	conversationID string,
	accountID string,
) (conversation.Conversation, conversation.ConversationMember, error) {
	conversationID = strings.TrimSpace(conversationID)
	accountID = strings.TrimSpace(accountID)
	if conversationID == "" || accountID == "" {
		return conversation.Conversation{}, conversation.ConversationMember{}, ErrInvalidInput
	}

	conversationRow, err := s.conversations.ConversationByID(ctx, conversationID)
	if err != nil {
		if err == conversation.ErrNotFound {
			return conversation.Conversation{}, conversation.ConversationMember{}, ErrNotFound
		}
		return conversation.Conversation{}, conversation.ConversationMember{}, fmt.Errorf(
			"load conversation %s: %w",
			conversationID,
			err,
		)
	}
	member, err := s.conversations.ConversationMemberByConversationAndAccount(ctx, conversationID, accountID)
	if err != nil {
		if err == conversation.ErrNotFound {
			return conversation.Conversation{}, conversation.ConversationMember{}, ErrForbidden
		}
		return conversation.Conversation{}, conversation.ConversationMember{}, fmt.Errorf(
			"load conversation member %s/%s: %w",
			conversationID,
			accountID,
			err,
		)
	}
	if !activeMember(member) {
		return conversation.Conversation{}, conversation.ConversationMember{}, ErrForbidden
	}

	return conversationRow, member, nil
}

func (s *Service) visibleCall(ctx context.Context, store Store, callID string, accountID string) (Call, error) {
	callRow, err := s.loadCall(ctx, store, callID)
	if err != nil {
		return Call{}, err
	}
	if _, _, err := s.visibleConversation(ctx, callRow.ConversationID, accountID); err != nil {
		return Call{}, err
	}

	return s.hydrateCall(ctx, store, callRow.ID)
}

func (s *Service) loadCall(ctx context.Context, store Store, callID string) (Call, error) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return Call{}, ErrInvalidInput
	}

	callRow, err := store.CallByID(ctx, callID)
	if err != nil {
		if err == ErrNotFound {
			return Call{}, ErrNotFound
		}
		return Call{}, fmt.Errorf("load call %s: %w", callID, err)
	}

	return s.expireCallIfNeeded(ctx, store, callRow)
}

func (s *Service) hydrateCall(ctx context.Context, store Store, callID string) (Call, error) {
	callRow, err := store.CallByID(ctx, callID)
	if err != nil {
		if err == ErrNotFound {
			return Call{}, ErrNotFound
		}
		return Call{}, fmt.Errorf("reload call %s: %w", callID, err)
	}

	invites, err := store.InvitesByCall(ctx, callID)
	if err != nil {
		return Call{}, fmt.Errorf("load invites for call %s: %w", callID, err)
	}
	participants, err := store.ParticipantsByCall(ctx, callID)
	if err != nil {
		return Call{}, fmt.Errorf("load participants for call %s: %w", callID, err)
	}

	callRow.Invites = cloneInvites(invites)
	callRow.Participants = cloneParticipants(participants)
	if callRow.ActiveSessionID != "" && callRow.State == StateActive {
		stats, statsErr := s.runtime.SessionStats(ctx, callRow.ActiveSessionID)
		if statsErr != nil {
			return Call{}, fmt.Errorf("load runtime stats for call %s: %w", callID, statsErr)
		}
		applyRuntimeStats(callRow.Participants, stats)
	}
	conversationRow, conversationErr := s.conversations.ConversationByID(ctx, callRow.ConversationID)
	if conversationErr == nil {
		applyParticipantPolicies(callRow.Participants, conversationRow.Kind, s.settings)
	}
	callRow.QualitySummary = summarizeCallQuality(callRow.Participants)
	if callRow.State == StateEnded {
		events, eventsErr := store.EventsAfterSequence(ctx, 0, callID, "", 1000)
		if eventsErr != nil {
			return Call{}, fmt.Errorf("load call quality history for %s: %w", callID, eventsErr)
		}
		applyEndedCallQualitySummary(&callRow, events)
	}
	return cloneCall(callRow), nil
}

func applyRuntimeStats(participants []Participant, stats []RuntimeStats) {
	if len(participants) == 0 || len(stats) == 0 {
		return
	}

	byKey := make(map[string]TransportStats, len(stats))
	for _, item := range stats {
		byKey[callParticipantKey(item.AccountID, item.DeviceID)] = cloneTransportStats(item.Transport)
	}
	for i := range participants {
		if transport, ok := byKey[callParticipantKey(participants[i].AccountID, participants[i].DeviceID)]; ok {
			participants[i].Transport = transport
		}
	}
}

func callParticipantKey(accountID string, deviceID string) string {
	return strings.TrimSpace(accountID) + "|" + strings.TrimSpace(deviceID)
}

func applyParticipantPolicies(
	participants []Participant,
	kind conversation.ConversationKind,
	settings Settings,
) {
	if len(participants) == 0 {
		return
	}

	applyHostMutePolicies(participants)
	if kind != conversation.ConversationKindGroup {
		return
	}
	applyGroupVideoScaling(participants, settings)
}

func applyHostMutePolicies(participants []Participant) {
	for i := range participants {
		if participants[i].HostMutedAudio {
			participants[i].Transport.SuppressOutgoingAudio = true
		}
		if participants[i].HostMutedVideo {
			participants[i].Transport.SuppressOutgoingVideo = true
			participants[i].Transport.SuppressCameraVideo = true
		}
	}
}

func applyGroupVideoScaling(participants []Participant, settings Settings) {
	if settings.MaxVideoParticipants == 0 {
		return
	}

	videoParticipants := 0
	for _, participant := range participants {
		if participant.State != ParticipantStateJoined {
			continue
		}
		if !participantWantsVideo(participant) {
			continue
		}
		videoParticipants++
	}
	if videoParticipants <= int(settings.MaxVideoParticipants) {
		return
	}

	for i := range participants {
		participant := &participants[i]
		if participant.State != ParticipantStateJoined {
			continue
		}
		if !participantWantsVideo(*participant) {
			continue
		}
		if participant.Transport.ScreenSharePriority || participant.Transport.DominantSpeaker {
			continue
		}

		participant.Transport.RecommendedProfile = "audio_only"
		participant.Transport.RecommendationReason = "group_call_scaling"
		participant.Transport.VideoFallbackRecommended = true
		participant.Transport.SuppressOutgoingVideo = true
		participant.Transport.SuppressIncomingVideo = true
	}
}

func participantWantsVideo(participant Participant) bool {
	if participant.MediaState.ScreenShareEnabled {
		return true
	}
	if participant.MediaState.VideoMuted {
		return false
	}

	return participant.MediaState.CameraEnabled
}

func summarizeCallQuality(participants []Participant) QualitySummary {
	summary := QualitySummary{
		WorstQuality:    "unknown",
		DominantProfile: "full",
	}
	if len(participants) == 0 {
		return summary
	}

	profileCounts := make(map[string]uint32, 4)
	worstRank := int(^uint(0) >> 1)
	for _, participant := range participants {
		if participant.State != ParticipantStateJoined && participant.State != ParticipantStateLeft {
			continue
		}
		summary.ParticipantCount++
		stats := participant.Transport
		if stats.ActiveSpeaker {
			summary.ActiveSpeakerCount++
		}
		if stats.DominantSpeaker {
			summary.DominantSpeakerAccountID = participant.AccountID
			summary.DominantSpeakerDeviceID = participant.DeviceID
		}
		if stats.VideoFallbackRecommended {
			summary.VideoFallbackParticipants++
		}
		if stats.ScreenSharePriority {
			summary.ScreenSharePriorityParticipants++
		}
		if stats.ReconnectRecommended {
			summary.ReconnectParticipants++
		}
		if stats.SuppressCameraVideo {
			summary.CameraVideoSuppressed++
		}
		if stats.SuppressOutgoingVideo {
			summary.OutgoingVideoSuppressed++
		}
		if stats.SuppressIncomingVideo {
			summary.IncomingVideoSuppressed++
		}
		if stats.SuppressOutgoingAudio {
			summary.OutgoingAudioSuppressed++
		}
		if stats.SuppressIncomingAudio {
			summary.IncomingAudioSuppressed++
		}
		summary.DegradedTransitions += stats.DegradedTransitions
		summary.RecoveredTransitions += stats.RecoveredTransitions
		if stats.LastQualityChangeAt.After(summary.LastChangedAt) {
			summary.LastChangedAt = stats.LastQualityChangeAt
		}
		profile := strings.TrimSpace(stats.RecommendedProfile)
		if profile == "" {
			profile = "full"
		}
		profileCounts[profile]++
		rank := qualityRankForSummary(stats.Quality)
		if rank < worstRank {
			worstRank = rank
			summary.WorstQuality = normalizeQualityForSummary(stats.Quality)
		}
	}

	var dominantCount uint32
	for profile, count := range profileCounts {
		if count > dominantCount {
			dominantCount = count
			summary.DominantProfile = profile
		}
	}

	return summary
}

func applyEndedCallQualitySummary(callRow *Call, events []Event) {
	if callRow == nil || len(events) == 0 {
		return
	}

	summary := callRow.QualitySummary
	videoFallbackParticipants := make(map[string]struct{})
	screenSharePriorityParticipants := make(map[string]struct{})
	reconnectParticipants := make(map[string]struct{})
	cameraVideoParticipants := make(map[string]struct{})
	outgoingVideoParticipants := make(map[string]struct{})
	incomingVideoParticipants := make(map[string]struct{})
	outgoingAudioParticipants := make(map[string]struct{})
	incomingAudioParticipants := make(map[string]struct{})
	for _, event := range events {
		if event.EventType != EventTypeMediaUpdated {
			continue
		}
		quality := normalizeQualityForSummary(event.Metadata["transport_quality"])
		if qualityRankForSummary(quality) < qualityRankForSummary(summary.WorstQuality) {
			summary.WorstQuality = quality
		}
		participantKey := strings.TrimSpace(event.Metadata[callMetadataTargetAccountID]) + "|" +
			strings.TrimSpace(event.Metadata[callMetadataTargetDeviceID])
		if event.Metadata["video_fallback_recommended"] == "true" && participantKey != "|" {
			videoFallbackParticipants[participantKey] = struct{}{}
		}
		if event.Metadata["screen_share_priority"] == "true" && participantKey != "|" {
			screenSharePriorityParticipants[participantKey] = struct{}{}
		}
		if event.Metadata["reconnect_recommended"] == "true" && participantKey != "|" {
			reconnectParticipants[participantKey] = struct{}{}
		}
		if event.Metadata["suppress_camera_video"] == "true" && participantKey != "|" {
			cameraVideoParticipants[participantKey] = struct{}{}
		}
		if event.Metadata["suppress_outgoing_video"] == "true" && participantKey != "|" {
			outgoingVideoParticipants[participantKey] = struct{}{}
		}
		if event.Metadata["suppress_incoming_video"] == "true" && participantKey != "|" {
			incomingVideoParticipants[participantKey] = struct{}{}
		}
		if event.Metadata["suppress_outgoing_audio"] == "true" && participantKey != "|" {
			outgoingAudioParticipants[participantKey] = struct{}{}
		}
		if event.Metadata["suppress_incoming_audio"] == "true" && participantKey != "|" {
			incomingAudioParticipants[participantKey] = struct{}{}
		}
		if value := parseUint32Metadata(event.Metadata["reconnect_attempt"]); value > 0 && event.CreatedAt.After(summary.LastChangedAt) {
			summary.LastChangedAt = event.CreatedAt
		}
	}
	if uint32(len(videoFallbackParticipants)) > summary.VideoFallbackParticipants {
		summary.VideoFallbackParticipants = uint32(len(videoFallbackParticipants))
	}
	if uint32(len(screenSharePriorityParticipants)) > summary.ScreenSharePriorityParticipants {
		summary.ScreenSharePriorityParticipants = uint32(len(screenSharePriorityParticipants))
	}
	if uint32(len(reconnectParticipants)) > summary.ReconnectParticipants {
		summary.ReconnectParticipants = uint32(len(reconnectParticipants))
	}
	if uint32(len(cameraVideoParticipants)) > summary.CameraVideoSuppressed {
		summary.CameraVideoSuppressed = uint32(len(cameraVideoParticipants))
	}
	if uint32(len(outgoingVideoParticipants)) > summary.OutgoingVideoSuppressed {
		summary.OutgoingVideoSuppressed = uint32(len(outgoingVideoParticipants))
	}
	if uint32(len(incomingVideoParticipants)) > summary.IncomingVideoSuppressed {
		summary.IncomingVideoSuppressed = uint32(len(incomingVideoParticipants))
	}
	if uint32(len(outgoingAudioParticipants)) > summary.OutgoingAudioSuppressed {
		summary.OutgoingAudioSuppressed = uint32(len(outgoingAudioParticipants))
	}
	if uint32(len(incomingAudioParticipants)) > summary.IncomingAudioSuppressed {
		summary.IncomingAudioSuppressed = uint32(len(incomingAudioParticipants))
	}
	callRow.QualitySummary = summary
}

func parseUint32Metadata(value string) uint32 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	var parsed uint32
	_, _ = fmt.Sscanf(value, "%d", &parsed)
	return parsed
}

func normalizeQualityForSummary(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}

	return value
}

func qualityRankForSummary(value string) int {
	switch normalizeQualityForSummary(value) {
	case "failed":
		return 0
	case "degraded":
		return 1
	case "connecting":
		return 2
	case "connected":
		return 3
	default:
		return 4
	}
}

func (s *Service) ensureCallableConversation(
	ctx context.Context,
	conversationID string,
	accountID string,
) (conversation.Conversation, []conversation.ConversationMember, error) {
	conversationRow, _, err := s.visibleConversation(ctx, conversationID, accountID)
	if err != nil {
		return conversation.Conversation{}, nil, err
	}
	if conversationRow.Kind != conversation.ConversationKindDirect &&
		conversationRow.Kind != conversation.ConversationKindGroup {
		return conversation.Conversation{}, nil, ErrConflict
	}

	members, err := s.conversations.ConversationMembersByConversationID(ctx, conversationID)
	if err != nil {
		return conversation.Conversation{}, nil, fmt.Errorf("load members for conversation %s: %w", conversationID, err)
	}

	activeMembers := make([]conversation.ConversationMember, 0, len(members))
	for _, member := range members {
		if activeMember(member) {
			activeMembers = append(activeMembers, member)
		}
	}
	if len(activeMembers) < 2 {
		return conversation.Conversation{}, nil, ErrConflict
	}

	return conversationRow, activeMembers, nil
}

func (s *Service) expireCallIfNeeded(ctx context.Context, store Store, callRow Call) (Call, error) {
	if !callRow.EndedAt.IsZero() {
		return callRow, nil
	}

	now := s.currentTime()
	switch callRow.State {
	case StateRinging:
		if now.Before(callRow.StartedAt.Add(s.settings.RingingTimeout)) {
			return callRow, nil
		}

		callRow.State = StateEnded
		callRow.EndReason = EndReasonMissed
		callRow.EndedAt = now
		callRow.UpdatedAt = now

		saved, err := store.SaveCall(ctx, callRow)
		if err != nil {
			return Call{}, fmt.Errorf("expire call %s: %w", callRow.ID, err)
		}

		invites, err := store.InvitesByCall(ctx, callRow.ID)
		if err != nil {
			return Call{}, fmt.Errorf("load invites for expiring call %s: %w", callRow.ID, err)
		}
		for _, invite := range invites {
			if invite.State != InviteStatePending {
				continue
			}
			invite.State = InviteStateExpired
			invite.UpdatedAt = now
			if invite.AnsweredAt.IsZero() {
				invite.AnsweredAt = now
			}
			if _, saveErr := store.SaveInvite(ctx, invite); saveErr != nil {
				return Call{}, fmt.Errorf("expire invite %s/%s: %w", invite.CallID, invite.AccountID, saveErr)
			}
		}
		if _, err := s.appendEvent(ctx, store, saved, EventTypeEnded, "", "", map[string]string{
			"reason": string(EndReasonMissed),
		}, now); err != nil {
			return Call{}, err
		}

		return saved, nil
	case StateActive:
		if now.After(callRow.StartedAt.Add(s.settings.MaxDuration)) {
			return s.endExpiredActiveCall(ctx, store, callRow, now, "max_duration")
		}

		participants, err := store.ParticipantsByCall(ctx, callRow.ID)
		if err != nil {
			return Call{}, fmt.Errorf("load participants for expiring call %s: %w", callRow.ID, err)
		}
		if joinedParticipants(participants) > 0 {
			return callRow, nil
		}

		lastLeftAt := latestParticipantLeftAt(participants)
		if lastLeftAt.IsZero() || now.Before(lastLeftAt.Add(s.settings.ReconnectGrace)) {
			return callRow, nil
		}

		return s.endExpiredActiveCall(ctx, store, callRow, now, "reconnect_grace")
	default:
		return callRow, nil
	}
}

func (s *Service) endExpiredActiveCall(
	ctx context.Context,
	store Store,
	callRow Call,
	now time.Time,
	policy string,
) (Call, error) {
	callRow.State = StateEnded
	callRow.EndReason = EndReasonEnded
	callRow.EndedAt = now
	callRow.UpdatedAt = now

	saved, err := store.SaveCall(ctx, callRow)
	if err != nil {
		return Call{}, fmt.Errorf("end expired active call %s: %w", callRow.ID, err)
	}
	if _, err := s.appendEvent(ctx, store, saved, EventTypeEnded, "", "", map[string]string{
		"reason": string(EndReasonEnded),
		"policy": policy,
	}, now); err != nil {
		return Call{}, err
	}

	return saved, nil
}

func latestParticipantLeftAt(participants []Participant) time.Time {
	var latest time.Time
	for _, participant := range participants {
		if participant.LeftAt.IsZero() {
			continue
		}
		if latest.IsZero() || participant.LeftAt.After(latest) {
			latest = participant.LeftAt
		}
	}

	return latest
}
