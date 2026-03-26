package gateway

import (
	"context"
	"time"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	syncv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/sync/v1"
	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
)

// GetSyncState returns the sync cursors for the authenticated device.
func (a *api) GetSyncState(
	ctx context.Context,
	req *syncv1.GetSyncStateRequest,
) (*syncv1.GetSyncStateResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	deviceID, err := syncDeviceID(authContext, req.GetDeviceId())
	if err != nil {
		return nil, err
	}

	state, pending, err := a.conversation.GetSyncState(ctx, domainconversation.GetSyncStateParams{
		DeviceID: deviceID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &syncv1.GetSyncStateResponse{
		State:                  syncStateProto(state),
		PendingConversationIds: pending,
	}, nil
}

// PullEvents returns a page of sync events for the authenticated device.
func (a *api) PullEvents(
	ctx context.Context,
	req *syncv1.PullEventsRequest,
) (*syncv1.PullEventsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetIncludePresence() || req.GetIncludeModeration() {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}

	deviceID, err := syncDeviceID(authContext, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	limit := pageSize(req.GetPage())

	events, state, err := a.conversation.PullEvents(ctx, domainconversation.PullEventsParams{
		DeviceID:        deviceID,
		FromSequence:    req.GetFromSequence(),
		Limit:           limit + 1,
		ConversationIDs: req.GetConversationIds(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	hasMore := false
	if len(events) > limit {
		hasMore = true
		events = events[:limit]
	}

	eventRows := make([]*commonv1.EventEnvelope, 0, len(events))
	for _, event := range events {
		eventRows = append(eventRows, eventProto(event))
	}

	nextSequence := req.GetFromSequence()
	if len(events) > 0 {
		nextSequence = events[len(events)-1].Sequence
	}

	return &syncv1.PullEventsResponse{
		Events:       eventRows,
		NextSequence: nextSequence,
		HasMore:      hasMore,
		State:        syncStateProto(state),
	}, nil
}

// AcknowledgeEvents advances the ack cursor for the authenticated device.
func (a *api) AcknowledgeEvents(
	ctx context.Context,
	req *syncv1.AcknowledgeEventsRequest,
) (*syncv1.AcknowledgeEventsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	deviceID, err := syncDeviceID(authContext, req.GetDeviceId())
	if err != nil {
		return nil, err
	}

	state, err := a.conversation.AcknowledgeEvents(ctx, domainconversation.AcknowledgeEventsParams{
		DeviceID:      deviceID,
		AckedSequence: req.GetAckedSequence(),
		EventIDs:      req.GetEventIds(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &syncv1.AcknowledgeEventsResponse{State: syncStateProto(state)}, nil
}

// SubscribeEvents streams sync events by polling the durable event log.
func (a *api) SubscribeEvents(
	req *syncv1.SubscribeEventsRequest,
	stream syncv1.SyncService_SubscribeEventsServer,
) error {
	authContext, err := a.requireAuth(stream.Context())
	if err != nil {
		return err
	}

	deviceID, err := syncDeviceID(authContext, req.GetDeviceId())
	if err != nil {
		return err
	}

	fromSequence := req.GetFromSequence()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		default:
		}

		events, _, pullErr := a.conversation.PullEvents(stream.Context(), domainconversation.PullEventsParams{
			DeviceID:        deviceID,
			FromSequence:    fromSequence,
			Limit:           defaultPageSize,
			ConversationIDs: req.GetConversationIds(),
		})
		if pullErr != nil {
			return grpcError(pullErr)
		}

		if len(events) == 0 {
			select {
			case <-stream.Context().Done():
				return nil
			case <-ticker.C:
				continue
			}
		}

		for _, event := range events {
			if sendErr := stream.Send(&syncv1.SubscribeEventsResponse{Event: eventProto(event)}); sendErr != nil {
				return sendErr
			}
			fromSequence = event.Sequence
		}
	}
}

func syncStateProto(state domainconversation.SyncState) *commonv1.SyncState {
	return &commonv1.SyncState{
		LastAppliedSequence:    state.LastAppliedSequence,
		LastAckedSequence:      state.LastAckedSequence,
		ServerTime:             protoTime(state.ServerTime),
		ConversationWatermarks: state.ConversationWatermarks,
	}
}

func syncDeviceID(authContext domainidentity.AuthContext, requested string) (string, error) {
	if requested == "" || requested == authContext.Device.ID {
		return authContext.Device.ID, nil
	}

	return "", grpcError(domainconversation.ErrForbidden)
}
