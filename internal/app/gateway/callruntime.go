package gateway

import (
	"context"

	callv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/call/v1"
	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	callruntimev1 "github.com/dm-vev/zvonilka/internal/genproto/callruntime/v1"
	platformrtc "github.com/dm-vev/zvonilka/internal/platform/rtc"
)

type callRuntimeAPI struct {
	callruntimev1.UnimplementedCallRuntimeServiceServer

	local *platformrtc.Manager
}

func newCallRuntimeAPI(local *platformrtc.Manager) *callRuntimeAPI {
	return &callRuntimeAPI{local: local}
}

func (a *callRuntimeAPI) EnsureSession(
	ctx context.Context,
	req *callruntimev1.EnsureSessionRequest,
) (*callruntimev1.EnsureSessionResponse, error) {
	if a == nil || a.local == nil || req.GetCall() == nil {
		return nil, grpcError(domaincall.ErrInvalidInput)
	}

	session, err := a.local.EnsureSession(ctx, domaincall.Call{
		ID:              req.GetCall().GetCallId(),
		ConversationID:  req.GetCall().GetConversationId(),
		ActiveSessionID: req.GetCall().GetActiveSessionId(),
	})
	if err != nil {
		return nil, grpcError(err)
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

func (a *callRuntimeAPI) JoinSession(
	ctx context.Context,
	req *callruntimev1.JoinSessionRequest,
) (*callruntimev1.JoinSessionResponse, error) {
	if a == nil || a.local == nil {
		return nil, grpcError(domaincall.ErrInvalidInput)
	}

	join, err := a.local.JoinSession(
		ctx,
		req.GetSessionId(),
		runtimeParticipantFromProto(req.GetParticipant()),
	)
	if err != nil {
		return nil, grpcError(err)
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

func (a *callRuntimeAPI) PublishDescription(
	ctx context.Context,
	req *callruntimev1.PublishDescriptionRequest,
) (*callruntimev1.PublishDescriptionResponse, error) {
	if a == nil || a.local == nil || req.GetDescription() == nil {
		return nil, grpcError(domaincall.ErrInvalidInput)
	}

	signals, err := a.local.PublishDescription(
		ctx,
		req.GetSessionId(),
		runtimeParticipantFromProto(req.GetParticipant()),
		domaincall.SessionDescription{
			Type: req.GetDescription().GetType(),
			SDP:  req.GetDescription().GetSdp(),
		},
	)
	if err != nil {
		return nil, grpcError(err)
	}

	return &callruntimev1.PublishDescriptionResponse{Signals: runtimeSignalsProto(signals)}, nil
}

func (a *callRuntimeAPI) PublishCandidate(
	ctx context.Context,
	req *callruntimev1.PublishCandidateRequest,
) (*callruntimev1.PublishCandidateResponse, error) {
	if a == nil || a.local == nil || req.GetCandidate() == nil {
		return nil, grpcError(domaincall.ErrInvalidInput)
	}

	signals, err := a.local.PublishCandidate(
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
		return nil, grpcError(err)
	}

	return &callruntimev1.PublishCandidateResponse{Signals: runtimeSignalsProto(signals)}, nil
}

func (a *callRuntimeAPI) UpdateParticipant(
	ctx context.Context,
	req *callruntimev1.UpdateParticipantRequest,
) (*callruntimev1.UpdateParticipantResponse, error) {
	if a == nil || a.local == nil {
		return nil, grpcError(domaincall.ErrInvalidInput)
	}

	err := a.local.UpdateParticipant(ctx, req.GetSessionId(), runtimeParticipantFromProto(req.GetParticipant()))
	if err != nil {
		return nil, grpcError(err)
	}

	return &callruntimev1.UpdateParticipantResponse{}, nil
}

func (a *callRuntimeAPI) AcknowledgeAdaptation(
	ctx context.Context,
	req *callruntimev1.AcknowledgeAdaptationRequest,
) (*callruntimev1.AcknowledgeAdaptationResponse, error) {
	if a == nil || a.local == nil {
		return nil, grpcError(domaincall.ErrInvalidInput)
	}

	err := a.local.AcknowledgeAdaptation(
		ctx,
		req.GetSessionId(),
		runtimeParticipantFromProto(req.GetParticipant()),
		req.GetAdaptationRevision(),
		req.GetAppliedProfile(),
	)
	if err != nil {
		return nil, grpcError(err)
	}

	return &callruntimev1.AcknowledgeAdaptationResponse{}, nil
}

func (a *callRuntimeAPI) SessionStats(
	ctx context.Context,
	req *callruntimev1.SessionStatsRequest,
) (*callruntimev1.SessionStatsResponse, error) {
	if a == nil || a.local == nil {
		return nil, grpcError(domaincall.ErrInvalidInput)
	}

	stats, err := a.local.SessionStats(ctx, req.GetSessionId())
	if err != nil {
		return nil, grpcError(err)
	}

	result := make([]*callruntimev1.RuntimeStat, 0, len(stats))
	for _, item := range stats {
		result = append(result, &callruntimev1.RuntimeStat{
			AccountId:      item.AccountID,
			DeviceId:       item.DeviceID,
			TransportStats: callTransportStatsProto(item.Transport),
		})
	}

	return &callruntimev1.SessionStatsResponse{Stats: result}, nil
}

func (a *callRuntimeAPI) LeaveSession(
	ctx context.Context,
	req *callruntimev1.LeaveSessionRequest,
) (*callruntimev1.LeaveSessionResponse, error) {
	if a == nil || a.local == nil {
		return nil, grpcError(domaincall.ErrInvalidInput)
	}

	err := a.local.LeaveSession(ctx, req.GetSessionId(), req.GetAccountId(), req.GetDeviceId())
	if err != nil {
		return nil, grpcError(err)
	}

	return &callruntimev1.LeaveSessionResponse{}, nil
}

func (a *callRuntimeAPI) CloseSession(
	ctx context.Context,
	req *callruntimev1.CloseSessionRequest,
) (*callruntimev1.CloseSessionResponse, error) {
	if a == nil || a.local == nil {
		return nil, grpcError(domaincall.ErrInvalidInput)
	}

	err := a.local.CloseSession(ctx, req.GetSessionId())
	if err != nil {
		return nil, grpcError(err)
	}

	return &callruntimev1.CloseSessionResponse{}, nil
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
		Media:     callMediaStateFromProto(value.GetMediaState()),
	}
}

func runtimeSignalsProto(values []domaincall.RuntimeSignal) []*callruntimev1.RuntimeSignal {
	result := make([]*callruntimev1.RuntimeSignal, 0, len(values))
	for _, value := range values {
		signal := &callruntimev1.RuntimeSignal{
			TargetAccountId: value.TargetAccountID,
			TargetDeviceId:  value.TargetDeviceID,
			SessionId:       value.SessionID,
			Metadata:        copyStringMap(value.Metadata),
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

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}

	return result
}
