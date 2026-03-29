package gateway

import (
	"context"
	"errors"
	"time"

	callv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/call/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

// StartCall creates a new direct ringing call.
func (a *api) StartCall(ctx context.Context, req *callv1.StartCallRequest) (*callv1.StartCallResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, events, err := a.call.StartCall(ctx, domaincall.StartParams{
		ConversationID: req.GetConversationId(),
		AccountID:      authContext.Account.ID,
		DeviceID:       authContext.Device.ID,
		WithVideo:      req.GetWithVideo(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.StartCallResponse{Call: callProto(callRow)}, nil
}

// GetCall returns one visible call.
func (a *api) GetCall(ctx context.Context, req *callv1.GetCallRequest) (*callv1.GetCallResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, err := a.call.GetCall(ctx, domaincall.GetParams{
		CallID:    req.GetCallId(),
		AccountID: authContext.Account.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &callv1.GetCallResponse{Call: callProto(callRow)}, nil
}

// GetCallDiagnostics returns one aggregated diagnostics report for a visible call.
func (a *api) GetCallDiagnostics(
	ctx context.Context,
	req *callv1.GetCallDiagnosticsRequest,
) (*callv1.GetCallDiagnosticsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	report, err := a.call.GetDiagnostics(ctx, domaincall.GetParams{
		CallID:    req.GetCallId(),
		AccountID: authContext.Account.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &callv1.GetCallDiagnosticsResponse{Diagnostics: callDiagnosticsProto(report)}, nil
}

// ListCalls returns calls for one conversation visible to the caller.
func (a *api) ListCalls(ctx context.Context, req *callv1.ListCallsRequest) (*callv1.ListCallsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := a.call.ListCalls(ctx, domaincall.ListParams{
		ConversationID: req.GetConversationId(),
		AccountID:      authContext.Account.ID,
		IncludeEnded:   req.GetIncludeEnded(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	offset, err := decodeOffset(req.GetPage(), "calls")
	if err != nil {
		return nil, grpcError(domaincall.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(rows) {
		end = len(rows)
	}

	page := rows
	if offset < len(rows) {
		page = rows[offset:end]
	} else {
		page = nil
	}

	result := make([]*callv1.Call, 0, len(page))
	for _, row := range page {
		result = append(result, callProto(row))
	}

	nextToken := ""
	if end < len(rows) {
		nextToken = offsetToken("calls", end)
	}

	return &callv1.ListCallsResponse{
		Calls: result,
		Page: &commonv1.PageResponse{
			NextPageToken: nextToken,
			TotalSize:     uint64(len(rows)),
		},
	}, nil
}

// AcceptCall accepts one incoming call invite.
func (a *api) AcceptCall(ctx context.Context, req *callv1.AcceptCallRequest) (*callv1.AcceptCallResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, events, err := a.call.AcceptCall(ctx, domaincall.AcceptParams{
		CallID:    req.GetCallId(),
		AccountID: authContext.Account.ID,
		DeviceID:  authContext.Device.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.AcceptCallResponse{Call: callProto(callRow)}, nil
}

// DeclineCall declines one incoming call invite.
func (a *api) DeclineCall(ctx context.Context, req *callv1.DeclineCallRequest) (*callv1.DeclineCallResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, events, err := a.call.DeclineCall(ctx, domaincall.DeclineParams{
		CallID:    req.GetCallId(),
		AccountID: authContext.Account.ID,
		DeviceID:  authContext.Device.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.DeclineCallResponse{Call: callProto(callRow)}, nil
}

// CancelCall cancels one ringing outbound call.
func (a *api) CancelCall(ctx context.Context, req *callv1.CancelCallRequest) (*callv1.CancelCallResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, events, err := a.call.CancelCall(ctx, domaincall.CancelParams{
		CallID:    req.GetCallId(),
		AccountID: authContext.Account.ID,
		DeviceID:  authContext.Device.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.CancelCallResponse{Call: callProto(callRow)}, nil
}

// EndCall ends one active call.
func (a *api) EndCall(ctx context.Context, req *callv1.EndCallRequest) (*callv1.EndCallResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, events, err := a.call.EndCall(ctx, domaincall.EndParams{
		CallID:    req.GetCallId(),
		AccountID: authContext.Account.ID,
		DeviceID:  authContext.Device.ID,
		Reason:    callEndReasonFromProto(req.GetReason()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.EndCallResponse{Call: callProto(callRow)}, nil
}

// JoinCall joins the authenticated device to the media session.
func (a *api) JoinCall(ctx context.Context, req *callv1.JoinCallRequest) (*callv1.JoinCallResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, details, events, err := a.call.JoinCall(ctx, domaincall.JoinParams{
		CallID:    req.GetCallId(),
		AccountID: authContext.Account.ID,
		DeviceID:  authContext.Device.ID,
		WithVideo: req.GetWithVideo(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.JoinCallResponse{
		Call:      callProto(callRow),
		Transport: joinDetailsProto(details),
	}, nil
}

// HandoffCall transfers one active call from another device of the same account to the current device.
func (a *api) HandoffCall(ctx context.Context, req *callv1.HandoffCallRequest) (*callv1.HandoffCallResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, participant, details, events, err := a.call.HandoffCall(ctx, domaincall.HandoffParams{
		CallID:       req.GetCallId(),
		AccountID:    authContext.Account.ID,
		FromDeviceID: req.GetFromDeviceId(),
		ToDeviceID:   authContext.Device.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.HandoffCallResponse{
		Call:        callProto(callRow),
		Participant: callParticipantProto(participant),
		Transport:   joinDetailsProto(details),
	}, nil
}

// PublishCallDescription publishes one SDP description for the joined device.
func (a *api) PublishCallDescription(
	ctx context.Context,
	req *callv1.PublishCallDescriptionRequest,
) (*callv1.PublishCallDescriptionResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	event, err := a.call.PublishDescription(ctx, domaincall.PublishDescriptionParams{
		CallID:    req.GetCallId(),
		SessionID: req.GetSessionId(),
		AccountID: authContext.Account.ID,
		DeviceID:  authContext.Device.ID,
		Description: domaincall.SessionDescription{
			Type: req.GetDescription().GetType(),
			SDP:  req.GetDescription().GetSdp(),
		},
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(event)

	return &callv1.PublishCallDescriptionResponse{Event: callEventProto(event)}, nil
}

// PublishCallIceCandidate publishes one ICE candidate for the joined device.
func (a *api) PublishCallIceCandidate(
	ctx context.Context,
	req *callv1.PublishCallIceCandidateRequest,
) (*callv1.PublishCallIceCandidateResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	event, err := a.call.PublishIceCandidate(ctx, domaincall.PublishCandidateParams{
		CallID:    req.GetCallId(),
		SessionID: req.GetSessionId(),
		AccountID: authContext.Account.ID,
		DeviceID:  authContext.Device.ID,
		IceCandidate: domaincall.Candidate{
			Candidate:        req.GetIceCandidate().GetCandidate(),
			SDPMid:           req.GetIceCandidate().GetSdpMid(),
			SDPMLineIndex:    req.GetIceCandidate().GetSdpMlineIndex(),
			UsernameFragment: req.GetIceCandidate().GetUsernameFragment(),
		},
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(event)

	return &callv1.PublishCallIceCandidateResponse{Event: callEventProto(event)}, nil
}

// LeaveCall leaves one call for the authenticated device.
func (a *api) LeaveCall(ctx context.Context, req *callv1.LeaveCallRequest) (*callv1.LeaveCallResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, events, err := a.call.LeaveCall(ctx, domaincall.LeaveParams{
		CallID:    req.GetCallId(),
		AccountID: authContext.Account.ID,
		DeviceID:  authContext.Device.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.LeaveCallResponse{Call: callProto(callRow)}, nil
}

// UpdateCallMediaState updates the caller's media flags for the active device.
func (a *api) UpdateCallMediaState(
	ctx context.Context,
	req *callv1.UpdateCallMediaStateRequest,
) (*callv1.UpdateCallMediaStateResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, participant, events, err := a.call.UpdateCallMediaState(ctx, domaincall.UpdateParams{
		CallID:    req.GetCallId(),
		AccountID: authContext.Account.ID,
		DeviceID:  authContext.Device.ID,
		Media:     callMediaStateFromProto(req.GetMediaState()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.UpdateCallMediaStateResponse{
		Call:        callProto(callRow),
		Participant: callParticipantProto(participant),
	}, nil
}

// RaiseCallHand updates the caller's hand state in one active group call.
func (a *api) RaiseCallHand(
	ctx context.Context,
	req *callv1.RaiseCallHandRequest,
) (*callv1.RaiseCallHandResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, participant, events, err := a.call.RaiseHand(ctx, domaincall.RaiseHandParams{
		CallID:    req.GetCallId(),
		AccountID: authContext.Account.ID,
		DeviceID:  authContext.Device.ID,
		Raised:    req.GetRaised(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.RaiseCallHandResponse{
		Call:        callProto(callRow),
		Participant: callParticipantProto(participant),
	}, nil
}

// ModerateCallParticipant applies host controls to one participant in an active group call.
func (a *api) ModerateCallParticipant(
	ctx context.Context,
	req *callv1.ModerateCallParticipantRequest,
) (*callv1.ModerateCallParticipantResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, participant, events, err := a.call.ModerateParticipant(ctx, domaincall.ModerateParticipantParams{
		CallID:         req.GetCallId(),
		AccountID:      authContext.Account.ID,
		DeviceID:       authContext.Device.ID,
		TargetDeviceID: req.GetTargetDeviceId(),
		HostMutedAudio: req.GetHostMutedAudio(),
		HostMutedVideo: req.GetHostMutedVideo(),
		LowerHand:      req.GetLowerHand(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.ModerateCallParticipantResponse{
		Call:        callProto(callRow),
		Participant: callParticipantProto(participant),
	}, nil
}

// MuteAllCallParticipants applies host mute flags to every joined group-call participant except the acting device.
func (a *api) MuteAllCallParticipants(
	ctx context.Context,
	req *callv1.MuteAllCallParticipantsRequest,
) (*callv1.MuteAllCallParticipantsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, participants, events, err := a.call.MuteAllParticipants(ctx, domaincall.MuteAllParticipantsParams{
		CallID:     req.GetCallId(),
		AccountID:  authContext.Account.ID,
		DeviceID:   authContext.Device.ID,
		MuteAudio:  req.GetMuteAudio(),
		MuteVideo:  req.GetMuteVideo(),
		LowerHands: req.GetLowerHands(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	result := make([]*callv1.CallParticipant, 0, len(participants))
	for _, participant := range participants {
		result = append(result, callParticipantProto(participant))
	}

	return &callv1.MuteAllCallParticipantsResponse{
		Call:         callProto(callRow),
		Participants: result,
	}, nil
}

// RemoveCallParticipant removes one participant from an active group call.
func (a *api) RemoveCallParticipant(
	ctx context.Context,
	req *callv1.RemoveCallParticipantRequest,
) (*callv1.RemoveCallParticipantResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, participant, events, err := a.call.RemoveParticipant(ctx, domaincall.RemoveParticipantParams{
		CallID:         req.GetCallId(),
		AccountID:      authContext.Account.ID,
		DeviceID:       authContext.Device.ID,
		TargetDeviceID: req.GetTargetDeviceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.RemoveCallParticipantResponse{
		Call:        callProto(callRow),
		Participant: callParticipantProto(participant),
	}, nil
}

// TransferCallHost transfers group-call host controls to another joined account.
func (a *api) TransferCallHost(
	ctx context.Context,
	req *callv1.TransferCallHostRequest,
) (*callv1.TransferCallHostResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, events, err := a.call.TransferHost(ctx, domaincall.TransferHostParams{
		CallID:          req.GetCallId(),
		AccountID:       authContext.Account.ID,
		DeviceID:        authContext.Device.ID,
		TargetAccountID: req.GetTargetUserId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.TransferCallHostResponse{Call: callProto(callRow)}, nil
}

// ListRaisedHands returns the current raised-hand queue for one visible group call.
func (a *api) ListRaisedHands(
	ctx context.Context,
	req *callv1.ListRaisedHandsRequest,
) (*callv1.ListRaisedHandsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := a.call.RaisedHands(ctx, domaincall.GetParams{
		CallID:    req.GetCallId(),
		AccountID: authContext.Account.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	result := make([]*callv1.CallParticipant, 0, len(rows))
	for _, participant := range rows {
		result = append(result, callParticipantProto(participant))
	}

	return &callv1.ListRaisedHandsResponse{Participants: result}, nil
}

// AcknowledgeCallAdaptation confirms application of a server-issued adaptation revision.
func (a *api) AcknowledgeCallAdaptation(
	ctx context.Context,
	req *callv1.AcknowledgeCallAdaptationRequest,
) (*callv1.AcknowledgeCallAdaptationResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	callRow, participant, events, err := a.call.AcknowledgeCallAdaptation(ctx, domaincall.AcknowledgeAdaptationParams{
		CallID:             req.GetCallId(),
		SessionID:          req.GetSessionId(),
		AccountID:          authContext.Account.ID,
		DeviceID:           authContext.Device.ID,
		AdaptationRevision: req.GetAdaptationRevision(),
		AppliedProfile:     req.GetAppliedProfile(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishCallEvents(events...)

	return &callv1.AcknowledgeCallAdaptationResponse{
		Call:        callProto(callRow),
		Participant: callParticipantProto(participant),
	}, nil
}

// GetIceConfig returns ICE/TURN settings for the current call.
func (a *api) GetIceConfig(ctx context.Context, req *callv1.GetIceConfigRequest) (*callv1.GetIceConfigResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	servers, expiresAt, endpoint, err := a.call.GetIceConfig(ctx, domaincall.IceParams{
		CallID:    req.GetCallId(),
		AccountID: authContext.Account.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	result := make([]*callv1.IceServer, 0, len(servers))
	for _, server := range servers {
		result = append(result, iceServerProto(server))
	}

	return &callv1.GetIceConfigResponse{
		IceServers:      result,
		ExpiresAt:       protoTime(expiresAt),
		RuntimeEndpoint: endpoint,
	}, nil
}

// SubscribeCallEvents streams durable call events with live in-process wakeups.
func (a *api) SubscribeCallEvents(
	req *callv1.SubscribeCallEventsRequest,
	stream callv1.CallService_SubscribeCallEventsServer,
) error {
	authContext, err := a.requireAuth(stream.Context())
	if err != nil {
		return err
	}

	fromSequence := req.GetFromSequence()
	updates, unsubscribe := a.subscribeCallEvents()
	defer unsubscribe()

	reconcileTicker := time.NewTicker(30 * time.Second)
	defer reconcileTicker.Stop()

	for {
		events, loadErr := a.call.Events(stream.Context(), domaincall.EventParams{
			FromSequence:   fromSequence,
			CallID:         req.GetCallId(),
			ConversationID: req.GetConversationId(),
			AccountID:      authContext.Account.ID,
			DeviceID:       authContext.Device.ID,
			Limit:          defaultPageSize,
		})
		if loadErr != nil {
			if stream.Context().Err() != nil {
				return nil
			}
			return grpcError(loadErr)
		}
		if len(events) > 0 {
			for _, event := range events {
				if sendErr := stream.Send(&callv1.SubscribeCallEventsResponse{
					Event: callEventProto(event),
				}); sendErr != nil {
					return sendErr
				}
				fromSequence = event.Sequence
			}
			continue
		}

		select {
		case <-stream.Context().Done():
			return nil
		case signal := <-updates:
			if signal.event == nil || signal.event.Sequence <= fromSequence {
				continue
			}
			if req.GetCallId() != "" && signal.event.CallID != req.GetCallId() {
				continue
			}
			if req.GetConversationId() != "" && signal.event.ConversationID != req.GetConversationId() {
				continue
			}
			if sendErr := stream.Send(&callv1.SubscribeCallEventsResponse{
				Event: callEventProto(*signal.event),
			}); sendErr != nil {
				return sendErr
			}
			fromSequence = signal.event.Sequence
		case <-reconcileTicker.C:
		}
	}
}

// SubscribeCallStats streams dedicated live call stats snapshots for one visible call.
func (a *api) SubscribeCallStats(
	req *callv1.SubscribeCallStatsRequest,
	stream callv1.CallService_SubscribeCallStatsServer,
) error {
	authContext, err := a.requireAuth(stream.Context())
	if err != nil {
		return err
	}

	interval := subscribeCallStatsInterval(req.GetIntervalMs())
	sendSnapshot := func() error {
		callRow, loadErr := a.call.GetCall(stream.Context(), domaincall.GetParams{
			CallID:    req.GetCallId(),
			AccountID: authContext.Account.ID,
		})
		if loadErr != nil {
			if stream.Context().Err() != nil {
				return nil
			}
			return grpcError(loadErr)
		}
		if sendErr := stream.Send(&callv1.SubscribeCallStatsResponse{
			Snapshot: callStatsSnapshotProto(callRow, time.Now().UTC()),
		}); sendErr != nil {
			return sendErr
		}
		if callRow.State == domaincall.StateEnded {
			return nil
		}
		return errCallStatsContinue
	}

	if err := sendSnapshot(); err != errCallStatsContinue {
		return err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-ticker.C:
			if err := sendSnapshot(); err != errCallStatsContinue {
				return err
			}
		}
	}
}

var errCallStatsContinue = errors.New("continue call stats stream")

func subscribeCallStatsInterval(intervalMs uint32) time.Duration {
	const (
		defaultInterval = time.Second
		minInterval     = 100 * time.Millisecond
		maxInterval     = 5 * time.Second
	)
	if intervalMs == 0 {
		return defaultInterval
	}
	interval := time.Duration(intervalMs) * time.Millisecond
	if interval < minInterval {
		return minInterval
	}
	if interval > maxInterval {
		return maxInterval
	}
	return interval
}
