package gateway

import (
	"context"
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
