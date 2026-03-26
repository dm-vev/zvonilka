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

	deviceID, err := syncDeviceID(authContext, req.GetDeviceId())
	if err != nil {
		return nil, err
	}
	limit := pageSize(req.GetPage())
	rawBatchLimit := syncPullBatchLimit(limit)
	scanSequence := req.GetFromSequence()
	nextSequence := scanSequence
	hasMore := false
	eventRows := make([]*commonv1.EventEnvelope, 0, limit)
	var state domainconversation.SyncState

	for batchCount := 0; batchCount < 8 && len(eventRows) < limit; batchCount++ {
		events, batchState, pullErr := a.conversation.PullEvents(ctx, domainconversation.PullEventsParams{
			DeviceID:        deviceID,
			FromSequence:    scanSequence,
			Limit:           rawBatchLimit,
			ConversationIDs: req.GetConversationIds(),
		})
		if pullErr != nil {
			return nil, grpcError(pullErr)
		}
		state = batchState
		if len(events) == 0 {
			hasMore = false
			break
		}

		scanSequence = events[len(events)-1].Sequence
		nextSequence = scanSequence
		hasMore = len(events) == rawBatchLimit

		for _, event := range events {
			if !includeSyncEvent(event, req.GetIncludePresence(), req.GetIncludeModeration()) {
				continue
			}
			if len(eventRows) >= limit {
				break
			}
			eventRows = append(eventRows, eventProto(event))
		}

		if len(events) < rawBatchLimit {
			break
		}
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

// SubscribeEvents streams sync events using durable-log replay with in-process wakeups.
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
	updates, unsubscribe := a.subscribeSyncNotifications()
	defer unsubscribe()

	reconcileTicker := time.NewTicker(30 * time.Second)
	defer reconcileTicker.Stop()

	for {
		events, _, pullErr := a.conversation.PullEvents(stream.Context(), domainconversation.PullEventsParams{
			DeviceID:        deviceID,
			FromSequence:    fromSequence,
			Limit:           defaultPageSize,
			ConversationIDs: req.GetConversationIds(),
		})
		if pullErr != nil {
			if stream.Context().Err() != nil {
				return nil
			}
			return grpcError(pullErr)
		}

		if len(events) == 0 {
			select {
			case <-stream.Context().Done():
				return nil
			case <-updates:
				continue
			case <-reconcileTicker.C:
				continue
			}
		}

		for _, event := range events {
			if !includeSyncEvent(event, req.GetIncludePresence(), req.GetIncludeModeration()) {
				fromSequence = event.Sequence
				continue
			}
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

func includeSyncEvent(
	event domainconversation.EventEnvelope,
	includePresence bool,
	includeModeration bool,
) bool {
	switch event.EventType {
	case domainconversation.EventTypeUserUpdated:
		if event.PayloadType == "presence" {
			return includePresence
		}
	case domainconversation.EventTypeAdminActionRecorded:
		if event.PayloadType == "moderation_action" {
			return includeModeration
		}
	}

	return true
}

func syncPullBatchLimit(limit int) int {
	switch {
	case limit <= 0:
		return 128
	case limit < 32:
		return 32
	case limit > 128:
		return limit
	default:
		return limit * 4
	}
}

func syncDeviceID(authContext domainidentity.AuthContext, requested string) (string, error) {
	if requested == "" || requested == authContext.Device.ID {
		return authContext.Device.ID, nil
	}

	return "", grpcError(domainconversation.ErrForbidden)
}
