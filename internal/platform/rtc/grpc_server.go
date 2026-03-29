package rtc

import (
	"context"
	"strings"
	"time"

	callv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/call/v1"
	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	callruntimev1 "github.com/dm-vev/zvonilka/internal/genproto/callruntime/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type grpcRuntimeServer struct {
	callruntimev1.UnimplementedCallRuntimeServiceServer

	local *Manager
}

// NewGRPCRuntimeServer exposes one local RTC manager over the internal call runtime API.
func NewGRPCRuntimeServer(local *Manager) callruntimev1.CallRuntimeServiceServer {
	return &grpcRuntimeServer{local: local}
}

func (s *grpcRuntimeServer) EnsureSession(
	ctx context.Context,
	req *callruntimev1.EnsureSessionRequest,
) (*callruntimev1.EnsureSessionResponse, error) {
	if s == nil || s.local == nil || req.GetCall() == nil {
		return nil, domaincall.ErrInvalidInput
	}

	session, err := s.local.EnsureSession(ctx, domaincall.Call{
		ID:              req.GetCall().GetCallId(),
		ConversationID:  req.GetCall().GetConversationId(),
		ActiveSessionID: req.GetCall().GetActiveSessionId(),
	})
	if err != nil {
		return nil, err
	}

	return &callruntimev1.EnsureSessionResponse{
		SessionId:       session.SessionID,
		RuntimeEndpoint: session.RuntimeEndpoint,
		IceUfrag:        session.IceUfrag,
		IcePwd:          session.IcePwd,
		DtlsFingerprint: session.DTLSFingerprint,
		CandidateHost:   session.CandidateHost,
		CandidatePort:   int32(session.CandidatePort),
	}, nil
}

func (s *grpcRuntimeServer) JoinSession(
	ctx context.Context,
	req *callruntimev1.JoinSessionRequest,
) (*callruntimev1.JoinSessionResponse, error) {
	if s == nil || s.local == nil {
		return nil, domaincall.ErrInvalidInput
	}

	join, err := s.local.JoinSession(ctx, req.GetSessionId(), runtimeParticipantFromProto(req.GetParticipant()))
	if err != nil {
		return nil, err
	}

	return &callruntimev1.JoinSessionResponse{
		SessionId:       join.SessionID,
		SessionToken:    join.SessionToken,
		RuntimeEndpoint: join.RuntimeEndpoint,
		ExpiresAtUnix:   join.ExpiresAt.Unix(),
		IceUfrag:        join.IceUfrag,
		IcePwd:          join.IcePwd,
		DtlsFingerprint: join.DTLSFingerprint,
		CandidateHost:   join.CandidateHost,
		CandidatePort:   int32(join.CandidatePort),
	}, nil
}

func (s *grpcRuntimeServer) PublishDescription(
	ctx context.Context,
	req *callruntimev1.PublishDescriptionRequest,
) (*callruntimev1.PublishDescriptionResponse, error) {
	if s == nil || s.local == nil || req.GetDescription() == nil {
		return nil, domaincall.ErrInvalidInput
	}

	signals, err := s.local.PublishDescription(
		ctx,
		req.GetSessionId(),
		runtimeParticipantFromProto(req.GetParticipant()),
		domaincall.SessionDescription{
			Type: req.GetDescription().GetType(),
			SDP:  req.GetDescription().GetSdp(),
		},
	)
	if err != nil {
		return nil, err
	}

	return &callruntimev1.PublishDescriptionResponse{Signals: runtimeSignalsProto(signals)}, nil
}

func (s *grpcRuntimeServer) PublishCandidate(
	ctx context.Context,
	req *callruntimev1.PublishCandidateRequest,
) (*callruntimev1.PublishCandidateResponse, error) {
	if s == nil || s.local == nil || req.GetCandidate() == nil {
		return nil, domaincall.ErrInvalidInput
	}

	signals, err := s.local.PublishCandidate(
		ctx,
		req.GetSessionId(),
		runtimeParticipantFromProto(req.GetParticipant()),
		domaincall.Candidate{
			Candidate:        req.GetCandidate().GetCandidate(),
			SDPMid:           req.GetCandidate().GetSdpMid(),
			SDPMLineIndex:    req.GetCandidate().GetSdpMlineIndex(),
			UsernameFragment: req.GetCandidate().GetUsernameFragment(),
		},
	)
	if err != nil {
		return nil, err
	}

	return &callruntimev1.PublishCandidateResponse{Signals: runtimeSignalsProto(signals)}, nil
}

func (s *grpcRuntimeServer) UpdateParticipant(
	ctx context.Context,
	req *callruntimev1.UpdateParticipantRequest,
) (*callruntimev1.UpdateParticipantResponse, error) {
	if s == nil || s.local == nil {
		return nil, domaincall.ErrInvalidInput
	}

	if err := s.local.UpdateParticipant(ctx, req.GetSessionId(), runtimeParticipantFromProto(req.GetParticipant())); err != nil {
		return nil, err
	}

	return &callruntimev1.UpdateParticipantResponse{}, nil
}

func (s *grpcRuntimeServer) AcknowledgeAdaptation(
	ctx context.Context,
	req *callruntimev1.AcknowledgeAdaptationRequest,
) (*callruntimev1.AcknowledgeAdaptationResponse, error) {
	if s == nil || s.local == nil {
		return nil, domaincall.ErrInvalidInput
	}

	if err := s.local.AcknowledgeAdaptation(
		ctx,
		req.GetSessionId(),
		runtimeParticipantFromProto(req.GetParticipant()),
		req.GetAdaptationRevision(),
		req.GetAppliedProfile(),
	); err != nil {
		return nil, err
	}

	return &callruntimev1.AcknowledgeAdaptationResponse{}, nil
}

func (s *grpcRuntimeServer) SessionStats(
	ctx context.Context,
	req *callruntimev1.SessionStatsRequest,
) (*callruntimev1.SessionStatsResponse, error) {
	if s == nil || s.local == nil {
		return nil, domaincall.ErrInvalidInput
	}

	stats, err := s.local.SessionStats(ctx, req.GetSessionId())
	if err != nil {
		return nil, err
	}

	result := make([]*callruntimev1.RuntimeStat, 0, len(stats))
	for _, item := range stats {
		result = append(result, &callruntimev1.RuntimeStat{
			AccountId:      item.AccountID,
			DeviceId:       item.DeviceID,
			TransportStats: runtimeTransportStatsProto(item.Transport),
		})
	}

	return &callruntimev1.SessionStatsResponse{Stats: result}, nil
}

func (s *grpcRuntimeServer) ExportSessionSnapshot(
	ctx context.Context,
	req *callruntimev1.ExportSessionSnapshotRequest,
) (*callruntimev1.ExportSessionSnapshotResponse, error) {
	if s == nil || s.local == nil {
		return nil, domaincall.ErrInvalidInput
	}

	snapshot, err := s.local.ExportSessionSnapshot(ctx, req.GetSessionId())
	if err != nil {
		return nil, err
	}

	return &callruntimev1.ExportSessionSnapshotResponse{
		Snapshot: sessionSnapshotProto(snapshot),
	}, nil
}

func (s *grpcRuntimeServer) SaveReplica(
	ctx context.Context,
	req *callruntimev1.SaveReplicaRequest,
) (*callruntimev1.SaveReplicaResponse, error) {
	if s == nil || s.local == nil {
		return nil, domaincall.ErrInvalidInput
	}

	if err := s.local.SaveReplica(ctx, sessionSnapshotFromProto(req.GetSnapshot())); err != nil {
		return nil, err
	}

	return &callruntimev1.SaveReplicaResponse{}, nil
}

func (s *grpcRuntimeServer) RestoreReplica(
	ctx context.Context,
	req *callruntimev1.RestoreReplicaRequest,
) (*callruntimev1.RestoreReplicaResponse, error) {
	if s == nil || s.local == nil {
		return nil, domaincall.ErrInvalidInput
	}

	if err := s.local.RestoreReplica(ctx, req.GetCallId(), req.GetSessionId()); err != nil {
		return nil, err
	}

	return &callruntimev1.RestoreReplicaResponse{}, nil
}

func (s *grpcRuntimeServer) LeaveSession(
	ctx context.Context,
	req *callruntimev1.LeaveSessionRequest,
) (*callruntimev1.LeaveSessionResponse, error) {
	if s == nil || s.local == nil {
		return nil, domaincall.ErrInvalidInput
	}

	if err := s.local.LeaveSession(ctx, req.GetSessionId(), req.GetAccountId(), req.GetDeviceId()); err != nil {
		return nil, err
	}

	return &callruntimev1.LeaveSessionResponse{}, nil
}

func (s *grpcRuntimeServer) CloseSession(
	ctx context.Context,
	req *callruntimev1.CloseSessionRequest,
) (*callruntimev1.CloseSessionResponse, error) {
	if s == nil || s.local == nil {
		return nil, domaincall.ErrInvalidInput
	}

	if err := s.local.CloseSession(ctx, req.GetSessionId()); err != nil {
		return nil, err
	}

	return &callruntimev1.CloseSessionResponse{}, nil
}

func (s *grpcRuntimeServer) Health(context.Context, *callruntimev1.HealthRequest) (*callruntimev1.HealthResponse, error) {
	if s == nil || s.local == nil {
		return nil, domaincall.ErrInvalidInput
	}

	return &callruntimev1.HealthResponse{NodeId: strings.TrimSpace(s.local.endpoint)}, nil
}

func runtimeParticipantFromProto(value *callruntimev1.RuntimeParticipant) domaincall.RuntimeParticipant {
	if value == nil {
		return domaincall.RuntimeParticipant{}
	}

	return domaincall.RuntimeParticipant{
		CallID:    value.GetCallId(),
		AccountID: value.GetAccountId(),
		DeviceID:  value.GetDeviceId(),
		WithVideo: value.GetWithVideo(),
		Media: domaincall.MediaState{
			AudioMuted:         value.GetMediaState().GetAudioMuted(),
			VideoMuted:         value.GetMediaState().GetVideoMuted(),
			CameraEnabled:      value.GetMediaState().GetCameraEnabled(),
			ScreenShareEnabled: value.GetMediaState().GetScreenShareEnabled(),
		},
	}
}

func runtimeSignalsProto(values []domaincall.RuntimeSignal) []*callruntimev1.RuntimeSignal {
	result := make([]*callruntimev1.RuntimeSignal, 0, len(values))
	for _, value := range values {
		signal := &callruntimev1.RuntimeSignal{
			TargetAccountId: value.TargetAccountID,
			TargetDeviceId:  value.TargetDeviceID,
			SessionId:       value.SessionID,
			Metadata:        copyMetadata(value.Metadata),
		}
		if value.Description != nil {
			signal.Description = &callv1.SessionDescription{
				Type: value.Description.Type,
				Sdp:  value.Description.SDP,
			}
		}
		if value.IceCandidate != nil {
			signal.IceCandidate = &callv1.IceCandidate{
				Candidate:        value.IceCandidate.Candidate,
				SdpMid:           value.IceCandidate.SDPMid,
				SdpMlineIndex:    value.IceCandidate.SDPMLineIndex,
				UsernameFragment: value.IceCandidate.UsernameFragment,
			}
		}
		result = append(result, signal)
	}

	return result
}

func runtimeTransportStatsProto(value domaincall.TransportStats) *callv1.CallTransportStats {
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
		PacketLossPct:            value.PacketLossPct,
		JitterScore:              value.JitterScore,
		QosEscalation:            value.QoSEscalation,
		QosTrend:                 value.QoSTrend,
		QosBadStreak:             value.QoSBadStreak,
		LastQosUpdatedAt:         protoTime(value.LastQoSUpdatedAt),
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

func protoTime(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}
	return timestamppb.New(value.UTC())
}
