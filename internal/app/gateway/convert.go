package gateway

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	callv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/call/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	mediav1 "github.com/dm-vev/zvonilka/gen/proto/contracts/media/v1"
	searchv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/search/v1"
	usersv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/users/v1"
	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
	domaine2ee "github.com/dm-vev/zvonilka/internal/domain/e2ee"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func protoTime(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}

	return timestamppb.New(value.UTC())
}

func protoDuration(value time.Duration) *durationpb.Duration {
	if value <= 0 {
		return nil
	}

	return durationpb.New(value)
}

const gatewayModerationTargetSeparator = "\x1f"

func callStateToProto(state domaincall.State) callv1.CallState {
	switch state {
	case domaincall.StateRinging:
		return callv1.CallState_CALL_STATE_RINGING
	case domaincall.StateActive:
		return callv1.CallState_CALL_STATE_ACTIVE
	case domaincall.StateEnded:
		return callv1.CallState_CALL_STATE_ENDED
	default:
		return callv1.CallState_CALL_STATE_UNSPECIFIED
	}
}

func callEndReasonToProto(reason domaincall.EndReason) callv1.CallEndReason {
	switch reason {
	case domaincall.EndReasonCancelled:
		return callv1.CallEndReason_CALL_END_REASON_CANCELLED
	case domaincall.EndReasonDeclined:
		return callv1.CallEndReason_CALL_END_REASON_DECLINED
	case domaincall.EndReasonMissed:
		return callv1.CallEndReason_CALL_END_REASON_MISSED
	case domaincall.EndReasonEnded:
		return callv1.CallEndReason_CALL_END_REASON_ENDED
	case domaincall.EndReasonFailed:
		return callv1.CallEndReason_CALL_END_REASON_FAILED
	default:
		return callv1.CallEndReason_CALL_END_REASON_UNSPECIFIED
	}
}

func callEndReasonFromProto(reason callv1.CallEndReason) domaincall.EndReason {
	switch reason {
	case callv1.CallEndReason_CALL_END_REASON_CANCELLED:
		return domaincall.EndReasonCancelled
	case callv1.CallEndReason_CALL_END_REASON_DECLINED:
		return domaincall.EndReasonDeclined
	case callv1.CallEndReason_CALL_END_REASON_MISSED:
		return domaincall.EndReasonMissed
	case callv1.CallEndReason_CALL_END_REASON_ENDED:
		return domaincall.EndReasonEnded
	case callv1.CallEndReason_CALL_END_REASON_FAILED:
		return domaincall.EndReasonFailed
	default:
		return domaincall.EndReasonUnspecified
	}
}

func callInviteStateToProto(state domaincall.InviteState) callv1.CallInviteState {
	switch state {
	case domaincall.InviteStatePending:
		return callv1.CallInviteState_CALL_INVITE_STATE_PENDING
	case domaincall.InviteStateAccepted:
		return callv1.CallInviteState_CALL_INVITE_STATE_ACCEPTED
	case domaincall.InviteStateDeclined:
		return callv1.CallInviteState_CALL_INVITE_STATE_DECLINED
	case domaincall.InviteStateCancelled:
		return callv1.CallInviteState_CALL_INVITE_STATE_CANCELLED
	case domaincall.InviteStateExpired:
		return callv1.CallInviteState_CALL_INVITE_STATE_EXPIRED
	default:
		return callv1.CallInviteState_CALL_INVITE_STATE_UNSPECIFIED
	}
}

func callParticipantStateToProto(state domaincall.ParticipantState) callv1.CallParticipantState {
	switch state {
	case domaincall.ParticipantStateJoined:
		return callv1.CallParticipantState_CALL_PARTICIPANT_STATE_JOINED
	case domaincall.ParticipantStateLeft:
		return callv1.CallParticipantState_CALL_PARTICIPANT_STATE_LEFT
	default:
		return callv1.CallParticipantState_CALL_PARTICIPANT_STATE_UNSPECIFIED
	}
}

func callEventTypeToProto(eventType domaincall.EventType) callv1.CallEventType {
	switch eventType {
	case domaincall.EventTypeStarted:
		return callv1.CallEventType_CALL_EVENT_TYPE_STARTED
	case domaincall.EventTypeInvited:
		return callv1.CallEventType_CALL_EVENT_TYPE_INVITED
	case domaincall.EventTypeAccepted:
		return callv1.CallEventType_CALL_EVENT_TYPE_ACCEPTED
	case domaincall.EventTypeDeclined:
		return callv1.CallEventType_CALL_EVENT_TYPE_DECLINED
	case domaincall.EventTypeJoined:
		return callv1.CallEventType_CALL_EVENT_TYPE_JOINED
	case domaincall.EventTypeLeft:
		return callv1.CallEventType_CALL_EVENT_TYPE_LEFT
	case domaincall.EventTypeMediaUpdated:
		return callv1.CallEventType_CALL_EVENT_TYPE_MEDIA_UPDATED
	case domaincall.EventTypeSessionMigrated:
		return callv1.CallEventType_CALL_EVENT_TYPE_SESSION_MIGRATED
	case domaincall.EventTypeEnded:
		return callv1.CallEventType_CALL_EVENT_TYPE_ENDED
	case domaincall.EventTypeSignalDescription:
		return callv1.CallEventType_CALL_EVENT_TYPE_SIGNAL_DESCRIPTION
	case domaincall.EventTypeSignalCandidate:
		return callv1.CallEventType_CALL_EVENT_TYPE_SIGNAL_CANDIDATE
	case domaincall.EventTypeRecordingUpdated:
		return callv1.CallEventType_CALL_EVENT_TYPE_RECORDING_UPDATED
	case domaincall.EventTypeTranscriptionUpdated:
		return callv1.CallEventType_CALL_EVENT_TYPE_TRANSCRIPTION_UPDATED
	default:
		return callv1.CallEventType_CALL_EVENT_TYPE_UNSPECIFIED
	}
}

func callRecordingStateToProto(state domaincall.RecordingState) callv1.CallRecordingState {
	switch state {
	case domaincall.RecordingStateInactive:
		return callv1.CallRecordingState_CALL_RECORDING_STATE_INACTIVE
	case domaincall.RecordingStateActive:
		return callv1.CallRecordingState_CALL_RECORDING_STATE_ACTIVE
	case domaincall.RecordingStateFailed:
		return callv1.CallRecordingState_CALL_RECORDING_STATE_FAILED
	default:
		return callv1.CallRecordingState_CALL_RECORDING_STATE_UNSPECIFIED
	}
}

func callTranscriptionStateToProto(state domaincall.TranscriptionState) callv1.CallTranscriptionState {
	switch state {
	case domaincall.TranscriptionStateInactive:
		return callv1.CallTranscriptionState_CALL_TRANSCRIPTION_STATE_INACTIVE
	case domaincall.TranscriptionStateActive:
		return callv1.CallTranscriptionState_CALL_TRANSCRIPTION_STATE_ACTIVE
	case domaincall.TranscriptionStateFailed:
		return callv1.CallTranscriptionState_CALL_TRANSCRIPTION_STATE_FAILED
	default:
		return callv1.CallTranscriptionState_CALL_TRANSCRIPTION_STATE_UNSPECIFIED
	}
}

func callMediaStateFromProto(state *callv1.CallMediaState) domaincall.MediaState {
	if state == nil {
		return domaincall.MediaState{}
	}

	return domaincall.MediaState{
		AudioMuted:         state.GetAudioMuted(),
		VideoMuted:         state.GetVideoMuted(),
		CameraEnabled:      state.GetCameraEnabled(),
		ScreenShareEnabled: state.GetScreenShareEnabled(),
	}
}

func callMediaStateProto(state domaincall.MediaState) *callv1.CallMediaState {
	return &callv1.CallMediaState{
		AudioMuted:         state.AudioMuted,
		VideoMuted:         state.VideoMuted,
		CameraEnabled:      state.CameraEnabled,
		ScreenShareEnabled: state.ScreenShareEnabled,
	}
}

func callTransportStatsProto(value domaincall.TransportStats) *callv1.CallTransportStats {
	samples := make([]*callv1.CallTransportQualitySample, 0, len(value.RecentSamples))
	for _, sample := range value.RecentSamples {
		samples = append(samples, &callv1.CallTransportQualitySample{
			Quality:            sample.Quality,
			RecommendedProfile: sample.RecommendedProfile,
			RecordedAt:         protoTime(sample.RecordedAt),
		})
	}
	qosSamples := make([]*callv1.CallTransportQoSSample, 0, len(value.RecentQoSSamples))
	for _, sample := range value.RecentQoSSamples {
		qosSamples = append(qosSamples, &callv1.CallTransportQoSSample{
			PacketLossPct: sample.PacketLossPct,
			JitterScore:   sample.JitterScore,
			Escalation:    sample.Escalation,
			RecordedAt:    protoTime(sample.RecordedAt),
		})
	}

	return &callv1.CallTransportStats{
		PeerConnectionState:      value.PeerConnectionState,
		IceConnectionState:       value.IceConnectionState,
		SignalingState:           value.SignalingState,
		Quality:                  value.Quality,
		AdaptationRevision:       value.AdaptationRevision,
		PendingAdaptation:        value.PendingAdaptation,
		AckedAdaptationRevision:  value.AckedAdaptationRevision,
		AppliedProfile:           value.AppliedProfile,
		AppliedAt:                protoTime(value.AppliedAt),
		RecommendedProfile:       value.RecommendedProfile,
		RecommendationReason:     value.RecommendationReason,
		VideoFallbackRecommended: value.VideoFallbackRecommended,
		ScreenSharePriority:      value.ScreenSharePriority,
		ReconnectRecommended:     value.ReconnectRecommended,
		SuppressCameraVideo:      value.SuppressCameraVideo,
		SuppressOutgoingVideo:    value.SuppressOutgoingVideo,
		SuppressIncomingVideo:    value.SuppressIncomingVideo,
		SuppressOutgoingAudio:    value.SuppressOutgoingAudio,
		SuppressIncomingAudio:    value.SuppressIncomingAudio,
		ReconnectAttempt:         value.ReconnectAttempt,
		ReconnectBackoffUntil:    protoTime(value.ReconnectBackoffUntil),
		QualityTrend:             value.QualityTrend,
		DegradedTransitions:      value.DegradedTransitions,
		RecoveredTransitions:     value.RecoveredTransitions,
		LastQualityChangeAt:      protoTime(value.LastQualityChangeAt),
		RecentSamples:            samples,
		PacketLossPct:            value.PacketLossPct,
		JitterScore:              value.JitterScore,
		QosEscalation:            value.QoSEscalation,
		QosTrend:                 value.QoSTrend,
		QosBadStreak:             value.QoSBadStreak,
		LastQosUpdatedAt:         protoTime(value.LastQoSUpdatedAt),
		RecentQosSamples:         qosSamples,
		RelayTracks:              value.RelayTracks,
		ScreenShareRelayTracks:   value.ScreenShareRelayTracks,
		RelayPackets:             value.RelayPackets,
		RelayBytes:               value.RelayBytes,
		RelayWriteErrors:         value.RelayWriteErrors,
		ActiveSpeaker:            value.ActiveSpeaker,
		DominantSpeaker:          value.DominantSpeaker,
		LastSpokeAt:              protoTime(value.LastSpokeAt),
		LastUpdatedAt:            protoTime(value.LastUpdatedAt),
	}
}

func callQualitySummaryProto(value domaincall.QualitySummary) *callv1.CallQualitySummary {
	return &callv1.CallQualitySummary{
		WorstQuality:                    value.WorstQuality,
		DominantProfile:                 value.DominantProfile,
		ParticipantCount:                value.ParticipantCount,
		ActiveSpeakerCount:              value.ActiveSpeakerCount,
		VideoFallbackParticipants:       value.VideoFallbackParticipants,
		ScreenSharePriorityParticipants: value.ScreenSharePriorityParticipants,
		ReconnectParticipants:           value.ReconnectParticipants,
		CameraVideoSuppressed:           value.CameraVideoSuppressed,
		OutgoingVideoSuppressed:         value.OutgoingVideoSuppressed,
		IncomingVideoSuppressed:         value.IncomingVideoSuppressed,
		OutgoingAudioSuppressed:         value.OutgoingAudioSuppressed,
		IncomingAudioSuppressed:         value.IncomingAudioSuppressed,
		DominantSpeakerUserId:           value.DominantSpeakerAccountID,
		DominantSpeakerDeviceId:         value.DominantSpeakerDeviceID,
		DegradedTransitions:             value.DegradedTransitions,
		RecoveredTransitions:            value.RecoveredTransitions,
		LastChangedAt:                   protoTime(value.LastChangedAt),
	}
}

func callDiagnosticsProto(value domaincall.Diagnostics) *callv1.CallDiagnostics {
	return &callv1.CallDiagnostics{
		Call:                     callProto(value.Call),
		DurationSeconds:          value.DurationSeconds,
		ActiveDurationSeconds:    value.ActiveDurationSeconds,
		PeakQosEscalation:        value.PeakQoSEscalation,
		MaxReconnectAttempt:      value.MaxReconnectAttempt,
		TotalAdaptationRevisions: value.TotalAdaptationRevisions,
		TotalAdaptationAcks:      value.TotalAdaptationAcks,
		LastAppliedProfile:       value.LastAppliedProfile,
		LastAppliedAt:            protoTime(value.LastAppliedAt),
	}
}

func callStatsSnapshotProto(value domaincall.Call, observedAt time.Time) *callv1.CallStatsSnapshot {
	return &callv1.CallStatsSnapshot{
		Call:       callProto(value),
		ObservedAt: protoTime(observedAt),
	}
}

func callRuntimeStateProto(value domaincall.RuntimeState) *callv1.CallRuntimeState {
	return &callv1.CallRuntimeState{
		CallId:                        value.CallID,
		ConversationId:                value.ConversationID,
		SessionId:                     value.SessionID,
		NodeId:                        value.NodeID,
		RuntimeEndpoint:               value.RuntimeEndpoint,
		Active:                        value.Active,
		Healthy:                       value.Healthy,
		ConfiguredReplicaNodeIds:      append([]string(nil), value.ConfiguredReplicaNodeIDs...),
		HealthyMigrationTargetNodeIds: append([]string(nil), value.HealthyMigrationTargetNodeIDs...),
		ObservedAt:                    protoTime(value.ObservedAt),
	}
}

func callSessionSnapshotProto(value domaincall.RuntimeSnapshot) *callv1.CallSessionSnapshot {
	participants := make([]*callv1.CallSessionSnapshotParticipant, 0, len(value.Participants))
	for _, participant := range value.Participants {
		relayTracks := make([]*callv1.CallSessionRelayTrack, 0, len(participant.Relay))
		for _, relay := range participant.Relay {
			relayTracks = append(relayTracks, &callv1.CallSessionRelayTrack{
				SourceUserId:   relay.SourceAccountID,
				SourceDeviceId: relay.SourceDeviceID,
				TrackId:        relay.TrackID,
				StreamId:       relay.StreamID,
				Kind:           relay.Kind,
				ScreenShare:    relay.ScreenShare,
				CodecMimeType:  relay.CodecMimeType,
				CodecClockRate: relay.CodecClockRate,
				CodecChannels:  relay.CodecChannels,
			})
		}
		participants = append(participants, &callv1.CallSessionSnapshotParticipant{
			UserId:         participant.AccountID,
			DeviceId:       participant.DeviceID,
			WithVideo:      participant.WithVideo,
			MediaState:     callMediaStateProto(participant.Media),
			TransportStats: callTransportStatsProto(participant.Transport),
			RelayTracks:    relayTracks,
		})
	}

	return &callv1.CallSessionSnapshot{
		CallId:         value.CallID,
		ConversationId: value.ConversationID,
		SessionId:      value.SessionID,
		NodeId:         value.NodeID,
		ObservedAt:     protoTime(value.ObservedAt),
		Participants:   participants,
	}
}

func iceServerProto(server domaincall.IceServer) *callv1.IceServer {
	return &callv1.IceServer{
		Urls:       append([]string(nil), server.URLs...),
		Username:   server.Username,
		Credential: server.Credential,
		ExpiresAt:  protoTime(server.ExpiresAt),
	}
}

func joinDetailsProto(details domaincall.JoinDetails) *callv1.JoinTransport {
	servers := make([]*callv1.IceServer, 0, len(details.IceServers))
	for _, server := range details.IceServers {
		servers = append(servers, iceServerProto(server))
	}

	return &callv1.JoinTransport{
		SessionId:       details.SessionID,
		SessionToken:    details.SessionToken,
		RuntimeEndpoint: details.RuntimeEndpoint,
		ExpiresAt:       protoTime(details.ExpiresAt),
		IceUfrag:        details.IceUfrag,
		IcePwd:          details.IcePwd,
		DtlsFingerprint: details.DTLSFingerprint,
		CandidateHost:   details.CandidateHost,
		CandidatePort:   uint32(details.CandidatePort),
		IceServers:      servers,
	}
}

func callInviteProto(value domaincall.Invite) *callv1.CallInvite {
	return &callv1.CallInvite{
		UserId:     value.AccountID,
		State:      callInviteStateToProto(value.State),
		ExpiresAt:  protoTime(value.ExpiresAt),
		AnsweredAt: protoTime(value.AnsweredAt),
	}
}

func callParticipantProto(value domaincall.Participant) *callv1.CallParticipant {
	return &callv1.CallParticipant{
		UserId:         value.AccountID,
		DeviceId:       value.DeviceID,
		State:          callParticipantStateToProto(value.State),
		MediaState:     callMediaStateProto(value.MediaState),
		JoinedAt:       protoTime(value.JoinedAt),
		LeftAt:         protoTime(value.LeftAt),
		UpdatedAt:      protoTime(value.UpdatedAt),
		TransportStats: callTransportStatsProto(value.Transport),
		HandRaised:     value.HandRaised,
		RaisedHandAt:   protoTime(value.RaisedHandAt),
		HostMutedAudio: value.HostMutedAudio,
		HostMutedVideo: value.HostMutedVideo,
		PinnedSpeaker:  value.PinnedSpeaker,
		StageSlot:      value.StageSlot,
	}
}

func callProto(value domaincall.Call) *callv1.Call {
	invites := make([]*callv1.CallInvite, 0, len(value.Invites))
	for _, invite := range value.Invites {
		invites = append(invites, callInviteProto(invite))
	}
	participants := make([]*callv1.CallParticipant, 0, len(value.Participants))
	for _, participant := range value.Participants {
		participants = append(participants, callParticipantProto(participant))
	}

	return &callv1.Call{
		CallId:                 value.ID,
		ConversationId:         value.ConversationID,
		InitiatorUserId:        value.InitiatorAccountID,
		HostUserId:             value.HostAccountID,
		ActiveSessionId:        value.ActiveSessionID,
		RequestedVideo:         value.RequestedVideo,
		State:                  callStateToProto(value.State),
		EndReason:              callEndReasonToProto(value.EndReason),
		StartedAt:              protoTime(value.StartedAt),
		AnsweredAt:             protoTime(value.AnsweredAt),
		EndedAt:                protoTime(value.EndedAt),
		UpdatedAt:              protoTime(value.UpdatedAt),
		Invites:                invites,
		Participants:           participants,
		QualitySummary:         callQualitySummaryProto(value.QualitySummary),
		StageModeEnabled:       value.StageModeEnabled,
		PinnedSpeakerUserId:    value.PinnedSpeakerAccountID,
		PinnedSpeakerDeviceId:  value.PinnedSpeakerDeviceID,
		RecordingState:         callRecordingStateToProto(value.RecordingState),
		RecordingStartedAt:     protoTime(value.RecordingStartedAt),
		RecordingStoppedAt:     protoTime(value.RecordingStoppedAt),
		TranscriptionState:     callTranscriptionStateToProto(value.TranscriptionState),
		TranscriptionStartedAt: protoTime(value.TranscriptionStartedAt),
		TranscriptionStoppedAt: protoTime(value.TranscriptionStoppedAt),
	}
}

func callEventProto(value domaincall.Event) *callv1.CallEvent {
	metadata := make(map[string]string, len(value.Metadata))
	for key, item := range value.Metadata {
		metadata[key] = item
	}

	return &callv1.CallEvent{
		EventId:        value.EventID,
		CallId:         value.CallID,
		ConversationId: value.ConversationID,
		EventType:      callEventTypeToProto(value.EventType),
		ActorUserId:    value.ActorAccountID,
		ActorDeviceId:  value.ActorDeviceID,
		Sequence:       value.Sequence,
		Metadata:       metadata,
		CreatedAt:      protoTime(value.CreatedAt),
		Call:           callProto(value.Call),
		SessionId:      value.Metadata["session_id"],
		Description:    callDescriptionProto(value),
		IceCandidate:   callCandidateProto(value),
	}
}

func callDescriptionProto(value domaincall.Event) *callv1.SessionDescription {
	if value.EventType != domaincall.EventTypeSignalDescription {
		return nil
	}
	descriptionType := strings.TrimSpace(value.Metadata["description_type"])
	sdp := value.Metadata["description_sdp"]
	if descriptionType == "" || sdp == "" {
		return nil
	}

	return &callv1.SessionDescription{
		Type: descriptionType,
		Sdp:  sdp,
	}
}

func callCandidateProto(value domaincall.Event) *callv1.IceCandidate {
	if value.EventType != domaincall.EventTypeSignalCandidate {
		return nil
	}

	candidate := strings.TrimSpace(value.Metadata["candidate"])
	if candidate == "" {
		return nil
	}
	line, _ := strconv.ParseUint(strings.TrimSpace(value.Metadata["candidate_sdp_mline_index"]), 10, 32)

	return &callv1.IceCandidate{
		Candidate:        candidate,
		SdpMid:           strings.TrimSpace(value.Metadata["candidate_sdp_mid"]),
		SdpMlineIndex:    uint32(line),
		UsernameFragment: strings.TrimSpace(value.Metadata["candidate_username_fragment"]),
	}
}

func identityPlatformFromProto(platform commonv1.DevicePlatform) domainidentity.DevicePlatform {
	switch platform {
	case commonv1.DevicePlatform_DEVICE_PLATFORM_IOS:
		return domainidentity.DevicePlatformIOS
	case commonv1.DevicePlatform_DEVICE_PLATFORM_ANDROID:
		return domainidentity.DevicePlatformAndroid
	case commonv1.DevicePlatform_DEVICE_PLATFORM_WEB:
		return domainidentity.DevicePlatformWeb
	case commonv1.DevicePlatform_DEVICE_PLATFORM_DESKTOP:
		return domainidentity.DevicePlatformDesktop
	case commonv1.DevicePlatform_DEVICE_PLATFORM_SERVER:
		return domainidentity.DevicePlatformServer
	default:
		return domainidentity.DevicePlatformUnspecified
	}
}

func identityPlatformToProto(platform domainidentity.DevicePlatform) commonv1.DevicePlatform {
	switch platform {
	case domainidentity.DevicePlatformIOS:
		return commonv1.DevicePlatform_DEVICE_PLATFORM_IOS
	case domainidentity.DevicePlatformAndroid:
		return commonv1.DevicePlatform_DEVICE_PLATFORM_ANDROID
	case domainidentity.DevicePlatformWeb:
		return commonv1.DevicePlatform_DEVICE_PLATFORM_WEB
	case domainidentity.DevicePlatformDesktop:
		return commonv1.DevicePlatform_DEVICE_PLATFORM_DESKTOP
	case domainidentity.DevicePlatformServer:
		return commonv1.DevicePlatform_DEVICE_PLATFORM_SERVER
	default:
		return commonv1.DevicePlatform_DEVICE_PLATFORM_UNSPECIFIED
	}
}

func identityDeviceStatusToProto(status domainidentity.DeviceStatus) commonv1.DeviceStatus {
	switch status {
	case domainidentity.DeviceStatusActive:
		return commonv1.DeviceStatus_DEVICE_STATUS_ACTIVE
	case domainidentity.DeviceStatusSuspended:
		return commonv1.DeviceStatus_DEVICE_STATUS_SUSPENDED
	case domainidentity.DeviceStatusRevoked:
		return commonv1.DeviceStatus_DEVICE_STATUS_REVOKED
	case domainidentity.DeviceStatusUnverified:
		return commonv1.DeviceStatus_DEVICE_STATUS_UNVERIFIED
	default:
		return commonv1.DeviceStatus_DEVICE_STATUS_UNSPECIFIED
	}
}

func conversationKindFromProto(kind commonv1.ConversationKind) domainconversation.ConversationKind {
	switch kind {
	case commonv1.ConversationKind_CONVERSATION_KIND_DIRECT:
		return domainconversation.ConversationKindDirect
	case commonv1.ConversationKind_CONVERSATION_KIND_GROUP:
		return domainconversation.ConversationKindGroup
	case commonv1.ConversationKind_CONVERSATION_KIND_CHANNEL:
		return domainconversation.ConversationKindChannel
	case commonv1.ConversationKind_CONVERSATION_KIND_SAVED_MESSAGES:
		return domainconversation.ConversationKindSavedMessages
	default:
		return domainconversation.ConversationKindUnspecified
	}
}

func conversationKindToProto(kind domainconversation.ConversationKind) commonv1.ConversationKind {
	switch kind {
	case domainconversation.ConversationKindDirect:
		return commonv1.ConversationKind_CONVERSATION_KIND_DIRECT
	case domainconversation.ConversationKindGroup:
		return commonv1.ConversationKind_CONVERSATION_KIND_GROUP
	case domainconversation.ConversationKindChannel:
		return commonv1.ConversationKind_CONVERSATION_KIND_CHANNEL
	case domainconversation.ConversationKindSavedMessages:
		return commonv1.ConversationKind_CONVERSATION_KIND_SAVED_MESSAGES
	default:
		return commonv1.ConversationKind_CONVERSATION_KIND_UNSPECIFIED
	}
}

func memberRoleToProto(role domainconversation.MemberRole) commonv1.MemberRole {
	switch role {
	case domainconversation.MemberRoleOwner:
		return commonv1.MemberRole_MEMBER_ROLE_OWNER
	case domainconversation.MemberRoleAdmin:
		return commonv1.MemberRole_MEMBER_ROLE_ADMIN
	case domainconversation.MemberRoleMember:
		return commonv1.MemberRole_MEMBER_ROLE_MEMBER
	case domainconversation.MemberRoleGuest:
		return commonv1.MemberRole_MEMBER_ROLE_GUEST
	default:
		return commonv1.MemberRole_MEMBER_ROLE_UNSPECIFIED
	}
}

func memberRoleFromProto(role commonv1.MemberRole) domainconversation.MemberRole {
	switch role {
	case commonv1.MemberRole_MEMBER_ROLE_OWNER:
		return domainconversation.MemberRoleOwner
	case commonv1.MemberRole_MEMBER_ROLE_ADMIN:
		return domainconversation.MemberRoleAdmin
	case commonv1.MemberRole_MEMBER_ROLE_MEMBER:
		return domainconversation.MemberRoleMember
	case commonv1.MemberRole_MEMBER_ROLE_GUEST:
		return domainconversation.MemberRoleGuest
	default:
		return domainconversation.MemberRoleUnspecified
	}
}

func moderationTargetKindFromProto(
	kind conversationv1.ModerationTargetKind,
) domainconversation.ModerationTargetKind {
	switch kind {
	case conversationv1.ModerationTargetKind_MODERATION_TARGET_KIND_CONVERSATION:
		return domainconversation.ModerationTargetKindConversation
	case conversationv1.ModerationTargetKind_MODERATION_TARGET_KIND_THREAD:
		return domainconversation.ModerationTargetKindTopic
	case conversationv1.ModerationTargetKind_MODERATION_TARGET_KIND_CHANNEL:
		return domainconversation.ModerationTargetKindChannel
	default:
		return domainconversation.ModerationTargetKindUnspecified
	}
}

func moderationTargetKindToProto(
	kind domainconversation.ModerationTargetKind,
) conversationv1.ModerationTargetKind {
	switch kind {
	case domainconversation.ModerationTargetKindConversation:
		return conversationv1.ModerationTargetKind_MODERATION_TARGET_KIND_CONVERSATION
	case domainconversation.ModerationTargetKindTopic:
		return conversationv1.ModerationTargetKind_MODERATION_TARGET_KIND_THREAD
	case domainconversation.ModerationTargetKindChannel:
		return conversationv1.ModerationTargetKind_MODERATION_TARGET_KIND_CHANNEL
	default:
		return conversationv1.ModerationTargetKind_MODERATION_TARGET_KIND_UNSPECIFIED
	}
}

func moderationRestrictionStateFromProto(
	state conversationv1.ModerationRestrictionState,
) domainconversation.ModerationRestrictionState {
	switch state {
	case conversationv1.ModerationRestrictionState_MODERATION_RESTRICTION_STATE_MUTED:
		return domainconversation.ModerationRestrictionStateMuted
	case conversationv1.ModerationRestrictionState_MODERATION_RESTRICTION_STATE_BANNED:
		return domainconversation.ModerationRestrictionStateBanned
	case conversationv1.ModerationRestrictionState_MODERATION_RESTRICTION_STATE_SHADOWED:
		return domainconversation.ModerationRestrictionStateShadowed
	default:
		return domainconversation.ModerationRestrictionStateUnspecified
	}
}

func moderationRestrictionStateToProto(
	state domainconversation.ModerationRestrictionState,
) conversationv1.ModerationRestrictionState {
	switch state {
	case domainconversation.ModerationRestrictionStateMuted:
		return conversationv1.ModerationRestrictionState_MODERATION_RESTRICTION_STATE_MUTED
	case domainconversation.ModerationRestrictionStateBanned:
		return conversationv1.ModerationRestrictionState_MODERATION_RESTRICTION_STATE_BANNED
	case domainconversation.ModerationRestrictionStateShadowed:
		return conversationv1.ModerationRestrictionState_MODERATION_RESTRICTION_STATE_SHADOWED
	default:
		return conversationv1.ModerationRestrictionState_MODERATION_RESTRICTION_STATE_UNSPECIFIED
	}
}

func moderationReportStatusToProto(
	status domainconversation.ModerationReportStatus,
) conversationv1.ModerationReportStatus {
	switch status {
	case domainconversation.ModerationReportStatusPending:
		return conversationv1.ModerationReportStatus_MODERATION_REPORT_STATUS_PENDING
	case domainconversation.ModerationReportStatusResolved:
		return conversationv1.ModerationReportStatus_MODERATION_REPORT_STATUS_RESOLVED
	case domainconversation.ModerationReportStatusRejected:
		return conversationv1.ModerationReportStatus_MODERATION_REPORT_STATUS_REJECTED
	default:
		return conversationv1.ModerationReportStatus_MODERATION_REPORT_STATUS_UNSPECIFIED
	}
}

func moderationActionTypeToProto(
	actionType domainconversation.ModerationActionType,
) conversationv1.ModerationActionType {
	switch actionType {
	case domainconversation.ModerationActionTypeBan:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_BAN
	case domainconversation.ModerationActionTypeKick:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_KICK
	case domainconversation.ModerationActionTypeMute:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_MUTE
	case domainconversation.ModerationActionTypeUnmute:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_UNMUTE
	case domainconversation.ModerationActionTypeShadowBan:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_SHADOW_BAN
	case domainconversation.ModerationActionTypeShadowUnban:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_SHADOW_UNBAN
	case domainconversation.ModerationActionTypePolicySet:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_POLICY_SET
	case domainconversation.ModerationActionTypeReportSet:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_REPORT_SET
	case domainconversation.ModerationActionTypeReportResolve:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_REPORT_RESOLVE
	case domainconversation.ModerationActionTypeReportReject:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_REPORT_REJECT
	case domainconversation.ModerationActionTypeSlowModeSet:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_SLOW_MODE_SET
	case domainconversation.ModerationActionTypeAntiSpamSet:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_ANTI_SPAM_SET
	default:
		return conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_UNSPECIFIED
	}
}

func moderationTargetProto(
	targetKind domainconversation.ModerationTargetKind,
	targetID string,
) *conversationv1.ModerationTarget {
	targetID = strings.TrimSpace(targetID)
	conversationID := targetID
	threadID := ""

	if targetKind == domainconversation.ModerationTargetKindTopic {
		conversationID, threadID = splitGatewayModerationTarget(targetID)
	}

	return &conversationv1.ModerationTarget{
		Kind:           moderationTargetKindToProto(targetKind),
		ConversationId: conversationID,
		ThreadId:       threadID,
	}
}

func splitGatewayModerationTarget(targetID string) (string, string) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return "", ""
	}

	conversationID, threadID, found := strings.Cut(targetID, gatewayModerationTargetSeparator)
	if !found {
		return targetID, ""
	}

	return conversationID, threadID
}

func messageKindFromProto(kind commonv1.MessageKind) domainconversation.MessageKind {
	switch kind {
	case commonv1.MessageKind_MESSAGE_KIND_TEXT:
		return domainconversation.MessageKindText
	case commonv1.MessageKind_MESSAGE_KIND_IMAGE:
		return domainconversation.MessageKindImage
	case commonv1.MessageKind_MESSAGE_KIND_VIDEO:
		return domainconversation.MessageKindVideo
	case commonv1.MessageKind_MESSAGE_KIND_DOCUMENT:
		return domainconversation.MessageKindDocument
	case commonv1.MessageKind_MESSAGE_KIND_VOICE:
		return domainconversation.MessageKindVoice
	case commonv1.MessageKind_MESSAGE_KIND_STICKER:
		return domainconversation.MessageKindSticker
	case commonv1.MessageKind_MESSAGE_KIND_GIF:
		return domainconversation.MessageKindGIF
	case commonv1.MessageKind_MESSAGE_KIND_SYSTEM:
		return domainconversation.MessageKindSystem
	default:
		return domainconversation.MessageKindUnspecified
	}
}

func messageKindToProto(kind domainconversation.MessageKind) commonv1.MessageKind {
	switch kind {
	case domainconversation.MessageKindText:
		return commonv1.MessageKind_MESSAGE_KIND_TEXT
	case domainconversation.MessageKindImage:
		return commonv1.MessageKind_MESSAGE_KIND_IMAGE
	case domainconversation.MessageKindVideo:
		return commonv1.MessageKind_MESSAGE_KIND_VIDEO
	case domainconversation.MessageKindDocument:
		return commonv1.MessageKind_MESSAGE_KIND_DOCUMENT
	case domainconversation.MessageKindVoice:
		return commonv1.MessageKind_MESSAGE_KIND_VOICE
	case domainconversation.MessageKindSticker:
		return commonv1.MessageKind_MESSAGE_KIND_STICKER
	case domainconversation.MessageKindGIF:
		return commonv1.MessageKind_MESSAGE_KIND_GIF
	case domainconversation.MessageKindSystem:
		return commonv1.MessageKind_MESSAGE_KIND_SYSTEM
	default:
		return commonv1.MessageKind_MESSAGE_KIND_UNSPECIFIED
	}
}

func messageStatusToProto(status domainconversation.MessageStatus) commonv1.MessageStatus {
	switch status {
	case domainconversation.MessageStatusPending:
		return commonv1.MessageStatus_MESSAGE_STATUS_PENDING
	case domainconversation.MessageStatusSent:
		return commonv1.MessageStatus_MESSAGE_STATUS_SENT
	case domainconversation.MessageStatusDelivered:
		return commonv1.MessageStatus_MESSAGE_STATUS_DELIVERED
	case domainconversation.MessageStatusRead:
		return commonv1.MessageStatus_MESSAGE_STATUS_READ
	case domainconversation.MessageStatusFailed:
		return commonv1.MessageStatus_MESSAGE_STATUS_FAILED
	case domainconversation.MessageStatusDeleted:
		return commonv1.MessageStatus_MESSAGE_STATUS_DELETED
	default:
		return commonv1.MessageStatus_MESSAGE_STATUS_UNSPECIFIED
	}
}

func attachmentKindFromProto(kind commonv1.AttachmentKind) domainconversation.AttachmentKind {
	switch kind {
	case commonv1.AttachmentKind_ATTACHMENT_KIND_IMAGE:
		return domainconversation.AttachmentKindImage
	case commonv1.AttachmentKind_ATTACHMENT_KIND_VIDEO:
		return domainconversation.AttachmentKindVideo
	case commonv1.AttachmentKind_ATTACHMENT_KIND_DOCUMENT:
		return domainconversation.AttachmentKindDocument
	case commonv1.AttachmentKind_ATTACHMENT_KIND_VOICE:
		return domainconversation.AttachmentKindVoice
	case commonv1.AttachmentKind_ATTACHMENT_KIND_STICKER:
		return domainconversation.AttachmentKindSticker
	case commonv1.AttachmentKind_ATTACHMENT_KIND_GIF:
		return domainconversation.AttachmentKindGIF
	case commonv1.AttachmentKind_ATTACHMENT_KIND_AVATAR:
		return domainconversation.AttachmentKindAvatar
	case commonv1.AttachmentKind_ATTACHMENT_KIND_FILE:
		return domainconversation.AttachmentKindFile
	default:
		return domainconversation.AttachmentKindUnspecified
	}
}

func attachmentKindToProto(kind domainconversation.AttachmentKind) commonv1.AttachmentKind {
	switch kind {
	case domainconversation.AttachmentKindImage:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_IMAGE
	case domainconversation.AttachmentKindVideo:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_VIDEO
	case domainconversation.AttachmentKindDocument:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_DOCUMENT
	case domainconversation.AttachmentKindVoice:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_VOICE
	case domainconversation.AttachmentKindSticker:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_STICKER
	case domainconversation.AttachmentKindGIF:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_GIF
	case domainconversation.AttachmentKindAvatar:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_AVATAR
	case domainconversation.AttachmentKindFile:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_FILE
	default:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_UNSPECIFIED
	}
}

func presenceStateFromProto(state commonv1.PresenceState) domainpresence.PresenceState {
	switch state {
	case commonv1.PresenceState_PRESENCE_STATE_OFFLINE:
		return domainpresence.PresenceStateOffline
	case commonv1.PresenceState_PRESENCE_STATE_ONLINE:
		return domainpresence.PresenceStateOnline
	case commonv1.PresenceState_PRESENCE_STATE_AWAY:
		return domainpresence.PresenceStateAway
	case commonv1.PresenceState_PRESENCE_STATE_BUSY:
		return domainpresence.PresenceStateBusy
	case commonv1.PresenceState_PRESENCE_STATE_INVISIBLE:
		return domainpresence.PresenceStateInvisible
	default:
		return domainpresence.PresenceStateUnspecified
	}
}

func presenceStateToProto(state domainpresence.PresenceState) commonv1.PresenceState {
	switch state {
	case domainpresence.PresenceStateOffline:
		return commonv1.PresenceState_PRESENCE_STATE_OFFLINE
	case domainpresence.PresenceStateOnline:
		return commonv1.PresenceState_PRESENCE_STATE_ONLINE
	case domainpresence.PresenceStateAway:
		return commonv1.PresenceState_PRESENCE_STATE_AWAY
	case domainpresence.PresenceStateBusy:
		return commonv1.PresenceState_PRESENCE_STATE_BUSY
	case domainpresence.PresenceStateInvisible:
		return commonv1.PresenceState_PRESENCE_STATE_INVISIBLE
	default:
		return commonv1.PresenceState_PRESENCE_STATE_UNSPECIFIED
	}
}

func accountKindToProto(kind domainidentity.AccountKind) commonv1.AccountKind {
	switch kind {
	case domainidentity.AccountKindUser:
		return commonv1.AccountKind_ACCOUNT_KIND_USER
	case domainidentity.AccountKindBot:
		return commonv1.AccountKind_ACCOUNT_KIND_BOT
	default:
		return commonv1.AccountKind_ACCOUNT_KIND_UNSPECIFIED
	}
}

func searchScopeFromProto(scope commonv1.SearchScope) domainsearch.SearchScope {
	switch scope {
	case commonv1.SearchScope_SEARCH_SCOPE_USERS:
		return domainsearch.SearchScopeUsers
	case commonv1.SearchScope_SEARCH_SCOPE_CONVERSATIONS:
		return domainsearch.SearchScopeConversations
	case commonv1.SearchScope_SEARCH_SCOPE_MESSAGES:
		return domainsearch.SearchScopeMessages
	case commonv1.SearchScope_SEARCH_SCOPE_MEDIA:
		return domainsearch.SearchScopeMedia
	default:
		return domainsearch.SearchScopeUnspecified
	}
}

func searchScopeToProto(scope domainsearch.SearchScope) commonv1.SearchScope {
	switch scope {
	case domainsearch.SearchScopeUsers:
		return commonv1.SearchScope_SEARCH_SCOPE_USERS
	case domainsearch.SearchScopeConversations:
		return commonv1.SearchScope_SEARCH_SCOPE_CONVERSATIONS
	case domainsearch.SearchScopeMessages:
		return commonv1.SearchScope_SEARCH_SCOPE_MESSAGES
	case domainsearch.SearchScopeMedia:
		return commonv1.SearchScope_SEARCH_SCOPE_MEDIA
	default:
		return commonv1.SearchScope_SEARCH_SCOPE_UNSPECIFIED
	}
}

func mediaPurposeToKind(purpose commonv1.MediaPurpose) domainmedia.MediaKind {
	switch purpose {
	case commonv1.MediaPurpose_MEDIA_PURPOSE_MESSAGE_ATTACHMENT:
		return domainmedia.MediaKindFile
	case commonv1.MediaPurpose_MEDIA_PURPOSE_PROFILE_AVATAR:
		return domainmedia.MediaKindAvatar
	case commonv1.MediaPurpose_MEDIA_PURPOSE_CHAT_AVATAR:
		return domainmedia.MediaKindAvatar
	case commonv1.MediaPurpose_MEDIA_PURPOSE_BOT_AVATAR:
		return domainmedia.MediaKindAvatar
	case commonv1.MediaPurpose_MEDIA_PURPOSE_STICKER_ASSET:
		return domainmedia.MediaKindSticker
	default:
		return domainmedia.MediaKindFile
	}
}

func mediaObject(asset domainmedia.MediaAsset) *mediav1.MediaObject {
	return &mediav1.MediaObject{
		MediaId:     asset.ID,
		OwnerUserId: asset.OwnerAccountID,
		Purpose:     mediaPurposeFromAsset(asset),
		FileName:    asset.FileName,
		MimeType:    asset.ContentType,
		SizeBytes:   asset.SizeBytes,
		Sha256Hex:   asset.SHA256Hex,
		StorageKey:  asset.ObjectKey,
		CreatedAt:   protoTime(asset.CreatedAt),
		CompletedAt: protoTime(asset.ReadyAt),
		DeletedAt:   protoTime(asset.DeletedAt),
	}
}

func mediaKindToProto(kind domainmedia.MediaKind) commonv1.AttachmentKind {
	switch kind {
	case domainmedia.MediaKindImage:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_IMAGE
	case domainmedia.MediaKindVideo:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_VIDEO
	case domainmedia.MediaKindDocument:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_DOCUMENT
	case domainmedia.MediaKindVoice:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_VOICE
	case domainmedia.MediaKindSticker:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_STICKER
	case domainmedia.MediaKindGIF:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_GIF
	case domainmedia.MediaKindAvatar:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_AVATAR
	case domainmedia.MediaKindFile:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_FILE
	default:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_UNSPECIFIED
	}
}

func mediaPurposeFromKind(kind domainmedia.MediaKind) commonv1.MediaPurpose {
	switch kind {
	case domainmedia.MediaKindAvatar:
		return commonv1.MediaPurpose_MEDIA_PURPOSE_PROFILE_AVATAR
	case domainmedia.MediaKindSticker:
		return commonv1.MediaPurpose_MEDIA_PURPOSE_STICKER_ASSET
	default:
		return commonv1.MediaPurpose_MEDIA_PURPOSE_MESSAGE_ATTACHMENT
	}
}

func authDevice(device domainidentity.Device) *authv1.Device {
	return &authv1.Device{
		DeviceId:   device.ID,
		UserId:     device.AccountID,
		DeviceName: device.Name,
		Platform:   identityPlatformToProto(device.Platform),
		Status:     identityDeviceStatusToProto(device.Status),
		PublicKey:  protoPublicKey(device.PublicKey),
		PushToken:  device.PushToken,
		CreatedAt:  protoTime(device.CreatedAt),
		LastSeenAt: protoTime(device.LastSeenAt),
		RevokedAt:  protoTime(device.RevokedAt),
	}
}

func authSession(session domainidentity.Session) *authv1.Session {
	return &authv1.Session{
		SessionId:      session.ID,
		UserId:         session.AccountID,
		DeviceId:       session.DeviceID,
		DeviceName:     session.DeviceName,
		DevicePlatform: identityPlatformToProto(session.DevicePlatform),
		IpAddress:      session.IPAddress,
		UserAgent:      session.UserAgent,
		CreatedAt:      protoTime(session.CreatedAt),
		LastSeenAt:     protoTime(session.LastSeenAt),
		RevokedAt:      protoTime(session.RevokedAt),
		Current:        session.Current,
		Revoked:        !session.RevokedAt.IsZero() || session.Status == domainidentity.SessionStatusRevoked,
	}
}

func authTargets(targets []domainidentity.LoginTarget) []*authv1.LoginTarget {
	result := make([]*authv1.LoginTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, &authv1.LoginTarget{
			Channel:         loginChannelToProto(target.Channel),
			DestinationMask: target.DestinationMask,
			Primary:         target.Primary,
			Verified:        target.Verified,
		})
	}

	return result
}

func loginChannelFromProto(channel authv1.LoginDeliveryChannel) domainidentity.LoginDeliveryChannel {
	switch channel {
	case authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_SMS:
		return domainidentity.LoginDeliveryChannelSMS
	case authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL:
		return domainidentity.LoginDeliveryChannelEmail
	case authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_PUSH:
		return domainidentity.LoginDeliveryChannelPush
	case authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_MANUAL:
		return domainidentity.LoginDeliveryChannelManual
	default:
		return domainidentity.LoginDeliveryChannelUnspecified
	}
}

func loginChannelToProto(channel domainidentity.LoginDeliveryChannel) authv1.LoginDeliveryChannel {
	switch channel {
	case domainidentity.LoginDeliveryChannelSMS:
		return authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_SMS
	case domainidentity.LoginDeliveryChannelEmail:
		return authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL
	case domainidentity.LoginDeliveryChannelPush:
		return authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_PUSH
	case domainidentity.LoginDeliveryChannelManual:
		return authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_MANUAL
	default:
		return authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_UNSPECIFIED
	}
}

func loginFactorToProto(factor domainidentity.LoginFactor) authv1.LoginFactor {
	switch factor {
	case domainidentity.LoginFactorCode:
		return authv1.LoginFactor_LOGIN_FACTOR_CODE
	case domainidentity.LoginFactorPassword:
		return authv1.LoginFactor_LOGIN_FACTOR_PASSWORD
	case domainidentity.LoginFactorRecoveryPassword:
		return authv1.LoginFactor_LOGIN_FACTOR_RECOVERY_PASSWORD
	case domainidentity.LoginFactorBotToken:
		return authv1.LoginFactor_LOGIN_FACTOR_BOT_TOKEN
	default:
		return authv1.LoginFactor_LOGIN_FACTOR_UNSPECIFIED
	}
}

func visibilityFromProto(value commonv1.Visibility) domainuser.Visibility {
	switch value {
	case commonv1.Visibility_VISIBILITY_EVERYONE:
		return domainuser.VisibilityEveryone
	case commonv1.Visibility_VISIBILITY_CONTACTS:
		return domainuser.VisibilityContacts
	case commonv1.Visibility_VISIBILITY_NOBODY:
		return domainuser.VisibilityNobody
	case commonv1.Visibility_VISIBILITY_CUSTOM:
		return domainuser.VisibilityCustom
	default:
		return domainuser.VisibilityUnspecified
	}
}

func visibilityToProto(value domainuser.Visibility) commonv1.Visibility {
	switch value {
	case domainuser.VisibilityEveryone:
		return commonv1.Visibility_VISIBILITY_EVERYONE
	case domainuser.VisibilityContacts:
		return commonv1.Visibility_VISIBILITY_CONTACTS
	case domainuser.VisibilityNobody:
		return commonv1.Visibility_VISIBILITY_NOBODY
	case domainuser.VisibilityCustom:
		return commonv1.Visibility_VISIBILITY_CUSTOM
	default:
		return commonv1.Visibility_VISIBILITY_UNSPECIFIED
	}
}

func privacyToProto(privacy domainuser.Privacy) *usersv1.PrivacySettings {
	if privacy.AccountID == "" {
		return nil
	}

	return &usersv1.PrivacySettings{
		PhoneVisibility:     visibilityToProto(privacy.PhoneVisibility),
		LastSeenVisibility:  visibilityToProto(privacy.LastSeenVisibility),
		MessagePrivacy:      visibilityToProto(privacy.MessagePrivacy),
		BirthdayVisibility:  visibilityToProto(privacy.BirthdayVisibility),
		AllowContactSync:    privacy.AllowContactSync,
		AllowUnknownSenders: privacy.AllowUnknownSenders,
		AllowUsernameSearch: privacy.AllowUsernameSearch,
	}
}

func contactSourceToProto(source domainuser.ContactSource) usersv1.ContactSource {
	switch source {
	case domainuser.ContactSourceManual:
		return usersv1.ContactSource_CONTACT_SOURCE_MANUAL
	case domainuser.ContactSourceImported:
		return usersv1.ContactSource_CONTACT_SOURCE_IMPORTED
	case domainuser.ContactSourceSynced:
		return usersv1.ContactSource_CONTACT_SOURCE_SYNCED
	case domainuser.ContactSourceInvited:
		return usersv1.ContactSource_CONTACT_SOURCE_INVITED
	default:
		return usersv1.ContactSource_CONTACT_SOURCE_UNSPECIFIED
	}
}

func userContact(contact domainuser.Contact) *usersv1.Contact {
	return &usersv1.Contact{
		ContactUserId: contact.ContactAccountID,
		DisplayName:   contact.DisplayName,
		Username:      contact.Username,
		PhoneHash:     contact.PhoneHash,
		Source:        contactSourceToProto(contact.Source),
		Starred:       contact.Starred,
		AddedAt:       protoTime(contact.AddedAt),
	}
}

func userBlock(block domainuser.BlockEntry) *usersv1.BlockEntry {
	return &usersv1.BlockEntry{
		BlockedUserId: block.BlockedAccountID,
		Reason:        block.Reason,
		BlockedAt:     protoTime(block.BlockedAt),
	}
}

func protoPublicKey(value string) *commonv1.PublicKeyBundle {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	decoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		decoded = []byte(value)
	}

	return &commonv1.PublicKeyBundle{
		PublicKey: decoded,
	}
}

func publicKeyFromProto(bundle *commonv1.PublicKeyBundle) string {
	if bundle == nil || len(bundle.PublicKey) == 0 {
		return ""
	}

	return base64.RawStdEncoding.EncodeToString(bundle.PublicKey)
}

func protoPayload(payload domainconversation.EncryptedPayload) *commonv1.EncryptedPayload {
	if payload.KeyID == "" && payload.Algorithm == "" && len(payload.Nonce) == 0 && len(payload.Ciphertext) == 0 && len(payload.AAD) == 0 && len(payload.Metadata) == 0 {
		return nil
	}

	return &commonv1.EncryptedPayload{
		KeyId:      payload.KeyID,
		Algorithm:  payload.Algorithm,
		Nonce:      payload.Nonce,
		Ciphertext: payload.Ciphertext,
		Aad:        payload.AAD,
		Metadata:   payload.Metadata,
	}
}

func payloadFromProto(payload *commonv1.EncryptedPayload) domainconversation.EncryptedPayload {
	if payload == nil {
		return domainconversation.EncryptedPayload{}
	}

	return domainconversation.EncryptedPayload{
		KeyID:      strings.TrimSpace(payload.KeyId),
		Algorithm:  strings.TrimSpace(payload.Algorithm),
		Nonce:      append([]byte(nil), payload.Nonce...),
		Ciphertext: append([]byte(nil), payload.Ciphertext...),
		AAD:        append([]byte(nil), payload.Aad...),
		Metadata:   payload.Metadata,
	}
}

func encryptedPayloadProto(payload domaine2ee.SenderKeyPayload) *commonv1.EncryptedPayload {
	if strings.TrimSpace(payload.Algorithm) == "" && len(payload.Ciphertext) == 0 {
		return nil
	}

	return &commonv1.EncryptedPayload{
		KeyId:      payload.KeyID,
		Algorithm:  payload.Algorithm,
		Nonce:      append([]byte(nil), payload.Nonce...),
		Ciphertext: append([]byte(nil), payload.Ciphertext...),
		Aad:        append([]byte(nil), payload.AAD...),
		Metadata:   cloneStringMap(payload.Metadata),
	}
}

func encryptedPayloadFromProto(payload *commonv1.EncryptedPayload) domaine2ee.SenderKeyPayload {
	if payload == nil {
		return domaine2ee.SenderKeyPayload{}
	}

	return domaine2ee.SenderKeyPayload{
		KeyID:      strings.TrimSpace(payload.GetKeyId()),
		Algorithm:  strings.TrimSpace(payload.GetAlgorithm()),
		Nonce:      append([]byte(nil), payload.GetNonce()...),
		Ciphertext: append([]byte(nil), payload.GetCiphertext()...),
		AAD:        append([]byte(nil), payload.GetAad()...),
		Metadata:   cloneStringMap(payload.GetMetadata()),
	}
}

func attachmentFromProto(attachment *commonv1.AttachmentRef) domainconversation.AttachmentRef {
	if attachment == nil {
		return domainconversation.AttachmentRef{}
	}

	duration := time.Duration(0)
	if attachment.Duration != nil {
		duration = attachment.Duration.AsDuration()
	}

	return domainconversation.AttachmentRef{
		MediaID:   strings.TrimSpace(attachment.MediaId),
		Kind:      attachmentKindFromProto(attachment.Kind),
		FileName:  strings.TrimSpace(attachment.FileName),
		MimeType:  strings.TrimSpace(attachment.MimeType),
		SizeBytes: attachment.SizeBytes,
		SHA256Hex: strings.TrimSpace(attachment.Sha256Hex),
		Width:     attachment.Width,
		Height:    attachment.Height,
		Duration:  duration,
		Caption:   strings.TrimSpace(attachment.Caption),
	}
}

func attachmentToProto(attachment domainconversation.AttachmentRef) *commonv1.AttachmentRef {
	return &commonv1.AttachmentRef{
		MediaId:   attachment.MediaID,
		Kind:      attachmentKindToProto(attachment.Kind),
		FileName:  attachment.FileName,
		MimeType:  attachment.MimeType,
		SizeBytes: attachment.SizeBytes,
		Sha256Hex: attachment.SHA256Hex,
		Width:     attachment.Width,
		Height:    attachment.Height,
		Duration:  protoDuration(attachment.Duration),
		Caption:   attachment.Caption,
	}
}

func referenceFromProto(reference *commonv1.MessageReference) domainconversation.MessageReference {
	if reference == nil {
		return domainconversation.MessageReference{}
	}

	return domainconversation.MessageReference{
		ConversationID:  strings.TrimSpace(reference.ConversationId),
		MessageID:       strings.TrimSpace(reference.MessageId),
		SenderAccountID: strings.TrimSpace(reference.SenderUserId),
		MessageKind:     messageKindFromProto(reference.MessageKind),
		Snippet:         strings.TrimSpace(reference.Snippet),
	}
}

func referenceToProto(reference domainconversation.MessageReference) *commonv1.MessageReference {
	if reference.MessageID == "" {
		return nil
	}

	return &commonv1.MessageReference{
		ConversationId: reference.ConversationID,
		MessageId:      reference.MessageID,
		SenderUserId:   reference.SenderAccountID,
		MessageKind:    messageKindToProto(reference.MessageKind),
		Snippet:        reference.Snippet,
	}
}

func eventTypeToProto(eventType domainconversation.EventType) commonv1.EventType {
	switch eventType {
	case domainconversation.EventTypeMessageCreated:
		return commonv1.EventType_EVENT_TYPE_MESSAGE_CREATED
	case domainconversation.EventTypeMessageEdited:
		return commonv1.EventType_EVENT_TYPE_MESSAGE_EDITED
	case domainconversation.EventTypeMessageDeleted:
		return commonv1.EventType_EVENT_TYPE_MESSAGE_DELETED
	case domainconversation.EventTypeMessageReactionAdded, domainconversation.EventTypeMessageReactionUpdated:
		return commonv1.EventType_EVENT_TYPE_MESSAGE_REACTION_ADDED
	case domainconversation.EventTypeMessageReactionRemoved:
		return commonv1.EventType_EVENT_TYPE_MESSAGE_REACTION_REMOVED
	case domainconversation.EventTypeConversationCreated:
		return commonv1.EventType_EVENT_TYPE_CONVERSATION_CREATED
	case domainconversation.EventTypeConversationUpdated:
		return commonv1.EventType_EVENT_TYPE_CONVERSATION_UPDATED
	case domainconversation.EventTypeConversationMembers:
		return commonv1.EventType_EVENT_TYPE_CONVERSATION_MEMBERS_CHANGED
	case domainconversation.EventTypeUserUpdated:
		return commonv1.EventType_EVENT_TYPE_USER_UPDATED
	case domainconversation.EventTypeAdminActionRecorded:
		return commonv1.EventType_EVENT_TYPE_ADMIN_ACTION_RECORDED
	default:
		return commonv1.EventType_EVENT_TYPE_UNSPECIFIED
	}
}

func profile(account domainidentity.Account, snapshot domainpresence.Snapshot) *usersv1.UserProfile {
	return &usersv1.UserProfile{
		UserId:           account.ID,
		Username:         account.Username,
		DisplayName:      account.DisplayName,
		Bio:              account.Bio,
		Phone:            account.Phone,
		Email:            account.Email,
		Verified:         false,
		CustomBadgeEmoji: account.CustomBadgeEmoji,
		Presence:         presenceStateToProto(snapshot.State),
		CustomStatus:     snapshot.CustomStatus,
		LastSeenAt:       protoTime(snapshot.LastSeenAt),
		CreatedAt:        protoTime(account.CreatedAt),
		UpdatedAt:        protoTime(account.UpdatedAt),
		AccountKind:      accountKindToProto(account.Kind),
	}
}

func userProfile(
	account domainidentity.Account,
	snapshot domainpresence.Snapshot,
	privacy domainuser.Privacy,
	relation domainuser.Relation,
	self bool,
) *usersv1.UserProfile {
	result := profile(account, snapshot)
	result.IsContact = relation.IsContact
	result.IsBlocked = relation.IsBlocked
	if self {
		result.Privacy = privacyToProto(privacy)
		return result
	}

	result.Email = ""
	if !canViewVisibility(privacy.PhoneVisibility, relation) {
		result.Phone = ""
	}
	if !canViewVisibility(privacy.LastSeenVisibility, relation) {
		result.LastSeenAt = nil
	}
	result.Privacy = nil
	return result
}

func canViewVisibility(visibility domainuser.Visibility, relation domainuser.Relation) bool {
	switch visibility {
	case domainuser.VisibilityEveryone:
		return true
	case domainuser.VisibilityContacts:
		return relation.IsContact
	default:
		return false
	}
}

func inviteProto(invite domainconversation.ConversationInvite) *conversationv1.Invite {
	if invite.ID == "" {
		return nil
	}

	allowedRoles := make([]commonv1.MemberRole, 0, len(invite.AllowedRoles))
	for _, role := range invite.AllowedRoles {
		allowedRoles = append(allowedRoles, memberRoleToProto(role))
	}

	return &conversationv1.Invite{
		InviteId:        invite.ID,
		ConversationId:  invite.ConversationID,
		Code:            invite.Code,
		CreatedByUserId: invite.CreatedByAccountID,
		AllowedRoles:    allowedRoles,
		ExpiresAt:       protoTime(invite.ExpiresAt),
		MaxUses:         invite.MaxUses,
		UseCount:        invite.UseCount,
		Revoked:         invite.Revoked,
		RevokedAt:       protoTime(invite.RevokedAt),
	}
}

func searchHit(hit domainsearch.Hit) *searchv1.SearchHit {
	return &searchv1.SearchHit{
		HitId:          hit.HitID,
		Scope:          searchScopeToProto(hit.Scope),
		Title:          hit.Title,
		Subtitle:       hit.Subtitle,
		Snippet:        hit.Snippet,
		TargetId:       hit.TargetID,
		ConversationId: hit.ConversationID,
		MessageId:      hit.MessageID,
		MediaId:        hit.MediaID,
		UserId:         hit.UserID,
		AccountKind:    accountKindToProto(domainidentity.AccountKind(hit.AccountKind)),
		Metadata:       hit.Metadata,
		UpdatedAt:      protoTime(hit.UpdatedAt),
	}
}
