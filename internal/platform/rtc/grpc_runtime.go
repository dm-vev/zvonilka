package rtc

import (
	"context"
	"fmt"
	"strings"
	"time"

	callv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/call/v1"
	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	callruntimev1 "github.com/dm-vev/zvonilka/internal/genproto/callruntime/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type grpcRuntimeClient struct {
	target string
	conn   *grpc.ClientConn
	client callruntimev1.CallRuntimeServiceClient
}

func newGRPCRuntimeClient(target string) (*grpcRuntimeClient, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, domaincall.ErrInvalidInput
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, err
	}

	return &grpcRuntimeClient{
		target: target,
		conn:   conn,
		client: callruntimev1.NewCallRuntimeServiceClient(conn),
	}, nil
}

func (c *grpcRuntimeClient) EnsureSession(ctx context.Context, callRow domaincall.Call) (domaincall.RuntimeSession, error) {
	resp, err := c.client.EnsureSession(ctx, &callruntimev1.EnsureSessionRequest{
		Call: &callruntimev1.RuntimeCall{
			CallId:          callRow.ID,
			ConversationId:  callRow.ConversationID,
			ActiveSessionId: callRow.ActiveSessionID,
		},
	})
	if err != nil {
		return domaincall.RuntimeSession{}, err
	}

	return domaincall.RuntimeSession{
		SessionID:       resp.GetSessionId(),
		RuntimeEndpoint: resp.GetRuntimeEndpoint(),
		IceUfrag:        resp.GetIceUfrag(),
		IcePwd:          resp.GetIcePwd(),
		DTLSFingerprint: resp.GetDtlsFingerprint(),
		CandidateHost:   resp.GetCandidateHost(),
		CandidatePort:   int(resp.GetCandidatePort()),
	}, nil
}

func (c *grpcRuntimeClient) JoinSession(ctx context.Context, sessionID string, participant domaincall.RuntimeParticipant) (domaincall.RuntimeJoin, error) {
	resp, err := c.client.JoinSession(ctx, &callruntimev1.JoinSessionRequest{
		SessionId:   sessionID,
		Participant: runtimeParticipantProto(participant),
	})
	if err != nil {
		return domaincall.RuntimeJoin{}, err
	}

	return domaincall.RuntimeJoin{
		SessionID:       resp.GetSessionId(),
		SessionToken:    resp.GetSessionToken(),
		RuntimeEndpoint: resp.GetRuntimeEndpoint(),
		ExpiresAt:       unixTime(resp.GetExpiresAtUnix()),
		IceUfrag:        resp.GetIceUfrag(),
		IcePwd:          resp.GetIcePwd(),
		DTLSFingerprint: resp.GetDtlsFingerprint(),
		CandidateHost:   resp.GetCandidateHost(),
		CandidatePort:   int(resp.GetCandidatePort()),
	}, nil
}

func (c *grpcRuntimeClient) PublishDescription(ctx context.Context, sessionID string, participant domaincall.RuntimeParticipant, description domaincall.SessionDescription) ([]domaincall.RuntimeSignal, error) {
	resp, err := c.client.PublishDescription(ctx, &callruntimev1.PublishDescriptionRequest{
		SessionId:   sessionID,
		Participant: runtimeParticipantProto(participant),
		Description: &callv1.SessionDescription{Type: description.Type, Sdp: description.SDP},
	})
	if err != nil {
		return nil, err
	}

	return runtimeSignalsFromProto(resp.GetSignals()), nil
}

func (c *grpcRuntimeClient) PublishCandidate(ctx context.Context, sessionID string, participant domaincall.RuntimeParticipant, candidate domaincall.Candidate) ([]domaincall.RuntimeSignal, error) {
	resp, err := c.client.PublishCandidate(ctx, &callruntimev1.PublishCandidateRequest{
		SessionId:   sessionID,
		Participant: runtimeParticipantProto(participant),
		Candidate: &callv1.IceCandidate{
			Candidate:        candidate.Candidate,
			SdpMid:           candidate.SDPMid,
			SdpMlineIndex:    candidate.SDPMLineIndex,
			UsernameFragment: candidate.UsernameFragment,
		},
	})
	if err != nil {
		return nil, err
	}

	return runtimeSignalsFromProto(resp.GetSignals()), nil
}

func (c *grpcRuntimeClient) UpdateParticipant(ctx context.Context, sessionID string, participant domaincall.RuntimeParticipant) error {
	_, err := c.client.UpdateParticipant(ctx, &callruntimev1.UpdateParticipantRequest{
		SessionId:   sessionID,
		Participant: runtimeParticipantProto(participant),
	})
	return err
}

func (c *grpcRuntimeClient) AcknowledgeAdaptation(ctx context.Context, sessionID string, participant domaincall.RuntimeParticipant, adaptationRevision uint64, appliedProfile string) error {
	_, err := c.client.AcknowledgeAdaptation(ctx, &callruntimev1.AcknowledgeAdaptationRequest{
		SessionId:          sessionID,
		Participant:        runtimeParticipantProto(participant),
		AdaptationRevision: adaptationRevision,
		AppliedProfile:     appliedProfile,
	})
	return err
}

func (c *grpcRuntimeClient) SessionStats(ctx context.Context, sessionID string) ([]domaincall.RuntimeStats, error) {
	resp, err := c.client.SessionStats(ctx, &callruntimev1.SessionStatsRequest{SessionId: sessionID})
	if err != nil {
		return nil, err
	}

	result := make([]domaincall.RuntimeStats, 0, len(resp.GetStats()))
	for _, item := range resp.GetStats() {
		result = append(result, domaincall.RuntimeStats{
			AccountID: item.GetAccountId(),
			DeviceID:  item.GetDeviceId(),
			Transport: runtimeTransportStatsFromProto(item.GetTransportStats()),
		})
	}

	return result, nil
}

func (c *grpcRuntimeClient) ExportSessionSnapshot(ctx context.Context, sessionID string) (sessionSnapshot, error) {
	resp, err := c.client.ExportSessionSnapshot(ctx, &callruntimev1.ExportSessionSnapshotRequest{SessionId: sessionID})
	if err != nil {
		return sessionSnapshot{}, err
	}

	return sessionSnapshotFromProto(resp.GetSnapshot()), nil
}

func (c *grpcRuntimeClient) SaveReplica(ctx context.Context, snapshot sessionSnapshot) error {
	_, err := c.client.SaveReplica(ctx, &callruntimev1.SaveReplicaRequest{
		Snapshot: sessionSnapshotProto(snapshot),
	})
	return err
}

func (c *grpcRuntimeClient) RestoreReplica(ctx context.Context, callID string, sessionID string) error {
	_, err := c.client.RestoreReplica(ctx, &callruntimev1.RestoreReplicaRequest{
		CallId:    callID,
		SessionId: sessionID,
	})
	return err
}

func (c *grpcRuntimeClient) LeaveSession(ctx context.Context, sessionID string, accountID string, deviceID string) error {
	_, err := c.client.LeaveSession(ctx, &callruntimev1.LeaveSessionRequest{
		SessionId: sessionID,
		AccountId: accountID,
		DeviceId:  deviceID,
	})
	return err
}

func (c *grpcRuntimeClient) CloseSession(ctx context.Context, sessionID string) error {
	_, err := c.client.CloseSession(ctx, &callruntimev1.CloseSessionRequest{SessionId: sessionID})
	return err
}

func (c *grpcRuntimeClient) Healthy(ctx context.Context) error {
	_, err := c.client.Health(ctx, &callruntimev1.HealthRequest{})
	return err
}

func (c *grpcRuntimeClient) Close(context.Context) error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func runtimeParticipantProto(value domaincall.RuntimeParticipant) *callruntimev1.RuntimeParticipant {
	return &callruntimev1.RuntimeParticipant{
		CallId:    value.CallID,
		AccountId: value.AccountID,
		DeviceId:  value.DeviceID,
		WithVideo: value.WithVideo,
		MediaState: &callv1.CallMediaState{
			AudioMuted:         value.Media.AudioMuted,
			VideoMuted:         value.Media.VideoMuted,
			CameraEnabled:      value.Media.CameraEnabled,
			ScreenShareEnabled: value.Media.ScreenShareEnabled,
		},
	}
}

func runtimeSignalsFromProto(values []*callruntimev1.RuntimeSignal) []domaincall.RuntimeSignal {
	result := make([]domaincall.RuntimeSignal, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		signal := domaincall.RuntimeSignal{
			TargetAccountID: value.GetTargetAccountId(),
			TargetDeviceID:  value.GetTargetDeviceId(),
			SessionID:       value.GetSessionId(),
			Metadata:        copyMetadata(value.GetMetadata()),
		}
		if description := value.GetDescription(); description != nil {
			signal.Description = &domaincall.SessionDescription{
				Type: description.GetType(),
				SDP:  description.GetSdp(),
			}
		}
		if candidate := value.GetIceCandidate(); candidate != nil {
			signal.IceCandidate = &domaincall.Candidate{
				Candidate:        candidate.GetCandidate(),
				SDPMid:           candidate.GetSdpMid(),
				SDPMLineIndex:    candidate.GetSdpMlineIndex(),
				UsernameFragment: candidate.GetUsernameFragment(),
			}
		}
		result = append(result, signal)
	}
	return result
}

func sessionSnapshotProto(value sessionSnapshot) *callruntimev1.SessionSnapshot {
	result := &callruntimev1.SessionSnapshot{
		CallId:         value.CallID,
		ConversationId: value.Conversation,
		Participants:   make([]*callruntimev1.SnapshotParticipant, 0, len(value.Participants)),
	}
	for _, participant := range value.Participants {
		result.Participants = append(result.Participants, &callruntimev1.SnapshotParticipant{
			AccountId: participant.AccountID,
			DeviceId:  participant.DeviceID,
			WithVideo: participant.WithVideo,
			MediaState: &callv1.CallMediaState{
				AudioMuted:         participant.Media.AudioMuted,
				VideoMuted:         participant.Media.VideoMuted,
				CameraEnabled:      participant.Media.CameraEnabled,
				ScreenShareEnabled: participant.Media.ScreenShareEnabled,
			},
			TransportStats: runtimeTransportStatsProto(participant.Transport),
			RelayTracks:    relayTrackBlueprintsProto(participant.Relay),
		})
	}
	return result
}

func sessionSnapshotFromProto(value *callruntimev1.SessionSnapshot) sessionSnapshot {
	if value == nil {
		return sessionSnapshot{}
	}

	result := sessionSnapshot{
		CallID:       value.GetCallId(),
		Conversation: value.GetConversationId(),
		Participants: make([]snapshotParticipant, 0, len(value.GetParticipants())),
	}
	for _, participant := range value.GetParticipants() {
		result.Participants = append(result.Participants, snapshotParticipant{
			AccountID: participant.GetAccountId(),
			DeviceID:  participant.GetDeviceId(),
			WithVideo: participant.GetWithVideo(),
			Media: domaincall.MediaState{
				AudioMuted:         participant.GetMediaState().GetAudioMuted(),
				VideoMuted:         participant.GetMediaState().GetVideoMuted(),
				CameraEnabled:      participant.GetMediaState().GetCameraEnabled(),
				ScreenShareEnabled: participant.GetMediaState().GetScreenShareEnabled(),
			},
			Transport: runtimeTransportStatsFromProto(participant.GetTransportStats()),
			Relay:     relayTrackBlueprintsFromProto(participant.GetRelayTracks()),
		})
	}

	return result
}

func runtimeTransportStatsFromProto(value *callv1.CallTransportStats) domaincall.TransportStats {
	if value == nil {
		return domaincall.TransportStats{}
	}

	return domaincall.TransportStats{
		PeerConnectionState:      value.GetPeerConnectionState(),
		IceConnectionState:       value.GetIceConnectionState(),
		SignalingState:           value.GetSignalingState(),
		Quality:                  value.GetQuality(),
		AdaptationRevision:       value.GetAdaptationRevision(),
		PendingAdaptation:        value.GetPendingAdaptation(),
		AckedAdaptationRevision:  value.GetAckedAdaptationRevision(),
		AppliedProfile:           value.GetAppliedProfile(),
		AppliedAt:                protoTimestamp(value.GetAppliedAt()),
		RecommendedProfile:       value.GetRecommendedProfile(),
		RecommendationReason:     value.GetRecommendationReason(),
		VideoFallbackRecommended: value.GetVideoFallbackRecommended(),
		ScreenSharePriority:      value.GetScreenSharePriority(),
		ReconnectRecommended:     value.GetReconnectRecommended(),
		SuppressCameraVideo:      value.GetSuppressCameraVideo(),
		SuppressOutgoingVideo:    value.GetSuppressOutgoingVideo(),
		SuppressIncomingVideo:    value.GetSuppressIncomingVideo(),
		SuppressOutgoingAudio:    value.GetSuppressOutgoingAudio(),
		SuppressIncomingAudio:    value.GetSuppressIncomingAudio(),
		ReconnectAttempt:         value.GetReconnectAttempt(),
		ReconnectBackoffUntil:    protoTimestamp(value.GetReconnectBackoffUntil()),
		QualityTrend:             value.GetQualityTrend(),
		DegradedTransitions:      value.GetDegradedTransitions(),
		RecoveredTransitions:     value.GetRecoveredTransitions(),
		LastQualityChangeAt:      protoTimestamp(value.GetLastQualityChangeAt()),
		PacketLossPct:            value.GetPacketLossPct(),
		JitterScore:              value.GetJitterScore(),
		QoSEscalation:            value.GetQosEscalation(),
		QoSTrend:                 value.GetQosTrend(),
		QoSBadStreak:             value.GetQosBadStreak(),
		LastQoSUpdatedAt:         protoTimestamp(value.GetLastQosUpdatedAt()),
		RelayTracks:              value.GetRelayTracks(),
		ScreenShareRelayTracks:   value.GetScreenShareRelayTracks(),
		RelayPackets:             value.GetRelayPackets(),
		RelayBytes:               value.GetRelayBytes(),
		RelayWriteErrors:         value.GetRelayWriteErrors(),
		ActiveSpeaker:            value.GetActiveSpeaker(),
		DominantSpeaker:          value.GetDominantSpeaker(),
		LastSpokeAt:              protoTimestamp(value.GetLastSpokeAt()),
		LastUpdatedAt:            protoTimestamp(value.GetLastUpdatedAt()),
	}
}

func protoTimestamp[T interface{ AsTime() time.Time }](value T) time.Time {
	var zero T
	if any(value) == any(zero) {
		return time.Time{}
	}
	return value.AsTime().UTC()
}

func unixTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.Unix(value, 0).UTC()
}

func copyMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func relayTrackBlueprintsProto(values []snapshotRelayTrack) []*callruntimev1.SnapshotRelayTrack {
	if len(values) == 0 {
		return nil
	}

	result := make([]*callruntimev1.SnapshotRelayTrack, 0, len(values))
	for _, value := range values {
		result = append(result, &callruntimev1.SnapshotRelayTrack{
			SourceAccountId: value.SourceAccountID,
			SourceDeviceId:  value.SourceDeviceID,
			TrackId:         value.TrackID,
			StreamId:        value.StreamID,
			Kind:            value.Kind,
			ScreenShare:     value.ScreenShare,
			CodecMimeType:   value.CodecMimeType,
			CodecClockRate:  value.CodecClockRate,
			CodecChannels:   value.CodecChannels,
		})
	}
	return result
}

func relayTrackBlueprintsFromProto(values []*callruntimev1.SnapshotRelayTrack) []snapshotRelayTrack {
	if len(values) == 0 {
		return nil
	}

	result := make([]snapshotRelayTrack, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		result = append(result, snapshotRelayTrack{
			SourceAccountID: value.GetSourceAccountId(),
			SourceDeviceID:  value.GetSourceDeviceId(),
			TrackID:         value.GetTrackId(),
			StreamID:        value.GetStreamId(),
			Kind:            value.GetKind(),
			ScreenShare:     value.GetScreenShare(),
			CodecMimeType:   value.GetCodecMimeType(),
			CodecClockRate:  value.GetCodecClockRate(),
			CodecChannels:   value.GetCodecChannels(),
		})
	}
	return result
}

func (c *grpcRuntimeClient) String() string {
	return fmt.Sprintf("grpcRuntimeClient(%s)", c.target)
}
