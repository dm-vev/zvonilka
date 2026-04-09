package federation

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

const bundlePayloadTypeConversationEvents = "conversation.events.v1"

// ConversationStore exposes the conversation event log needed for federation replication.
type ConversationStore interface {
	WithinTx(ctx context.Context, fn func(conversation.Store) error) error
	ConversationByID(ctx context.Context, conversationID string) (conversation.Conversation, error)
	SaveConversation(ctx context.Context, conversationRow conversation.Conversation) (conversation.Conversation, error)
	TopicByConversationAndID(ctx context.Context, conversationID string, topicID string) (conversation.ConversationTopic, error)
	SaveTopic(ctx context.Context, topic conversation.ConversationTopic) (conversation.ConversationTopic, error)
	SaveConversationMember(
		ctx context.Context,
		member conversation.ConversationMember,
	) (conversation.ConversationMember, error)
	ConversationMemberByConversationAndAccount(
		ctx context.Context,
		conversationID string,
		accountID string,
	) (conversation.ConversationMember, error)
	SaveMessage(ctx context.Context, message conversation.Message) (conversation.Message, error)
	MessageByID(ctx context.Context, conversationID string, messageID string) (conversation.Message, error)
	SaveMessageReaction(
		ctx context.Context,
		reaction conversation.MessageReaction,
	) (conversation.MessageReaction, error)
	DeleteMessageReaction(ctx context.Context, messageID string, accountID string) error
	SaveEvent(ctx context.Context, event conversation.EventEnvelope) (conversation.EventEnvelope, error)
	EventsAfterSequence(
		ctx context.Context,
		fromSequence uint64,
		limit int,
		conversationIDs []string,
	) ([]conversation.EventEnvelope, error)
}

// ReplicationClient exchanges federation bundles with one remote peer.
type ReplicationClient interface {
	PushBundles(
		ctx context.Context,
		serverName string,
		linkName string,
		bundles []Bundle,
	) error
	PullBundles(
		ctx context.Context,
		serverName string,
		linkName string,
		afterCursor uint64,
		limit int,
	) ([]Bundle, bool, error)
	AcknowledgeBundles(
		ctx context.Context,
		serverName string,
		linkName string,
		upToCursor uint64,
		bundleIDs []string,
	) error
	Close() error
}

// ReplicationClientFactory constructs one transport client for a peer/link pair.
type ReplicationClientFactory func(ctx context.Context, peer Peer, link Link) (ReplicationClient, error)

// WorkerSettings control federation replication cadence.
type WorkerSettings struct {
	LocalServerName string
	PollInterval    time.Duration
	BatchSize       int
}

// Worker replicates local conversation events to configured federation peers.
type Worker struct {
	service       *Service
	identities    IdentityStore
	conversations ConversationStore
	newClient     ReplicationClientFactory
	settings      WorkerSettings
	now           func() time.Time
}

// NewWorker constructs a federation replication worker.
func NewWorker(
	service *Service,
	identities IdentityStore,
	conversations ConversationStore,
	newClient ReplicationClientFactory,
	settings WorkerSettings,
) (*Worker, error) {
	if service == nil || identities == nil || conversations == nil || newClient == nil {
		return nil, ErrInvalidInput
	}

	settings.LocalServerName = strings.TrimSpace(strings.ToLower(settings.LocalServerName))
	if settings.LocalServerName == "" {
		return nil, ErrInvalidInput
	}
	if settings.PollInterval <= 0 {
		settings.PollInterval = 3 * time.Second
	}
	if settings.BatchSize <= 0 {
		settings.BatchSize = 100
	}

	return &Worker{
		service:       service,
		identities:    identities,
		conversations: conversations,
		newClient:     newClient,
		settings:      settings,
		now:           func() time.Time { return time.Now().UTC() },
	}, nil
}

// Run executes the federation replication loop until ctx is canceled.
func (w *Worker) Run(ctx context.Context, logger *slog.Logger) error {
	if ctx == nil || logger == nil {
		return ErrInvalidInput
	}

	ticker := time.NewTicker(w.settings.PollInterval)
	defer ticker.Stop()

	for {
		if err := w.processOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.ErrorContext(ctx, "process federation replication batch", "err", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// ProcessOnceForTests runs one replication batch.
func (w *Worker) ProcessOnceForTests(ctx context.Context) error {
	return w.processOnce(ctx)
}

func (w *Worker) processOnce(ctx context.Context) error {
	if err := w.validateContext(ctx, "process federation replication"); err != nil {
		return err
	}

	peers, err := w.service.ListPeers(ctx, PeerStateUnspecified)
	if err != nil {
		return err
	}

	var result error
	for _, peer := range peers {
		if !isReplicablePeer(peer) {
			continue
		}

		links, err := w.service.ListLinks(ctx, peer.ID, LinkStateUnspecified)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}

		for _, link := range links {
			if !isReplicableLink(link) {
				continue
			}
			if err := w.processPeerLink(ctx, peer, link); err != nil {
				result = errors.Join(result, err)
			}
		}
	}

	return result
}

func (w *Worker) processPeerLink(ctx context.Context, peer Peer, link Link) error {
	if isBridgeTransport(link.TransportKind) {
		if err := w.processBridgeLink(ctx, peer, link); err != nil {
			_ = w.service.markLinkFailed(ctx, link.ID, err)
			return err
		}
		return nil
	}

	if _, err := w.applyPendingInboundBundles(ctx, peer, link); err != nil {
		_ = w.service.markLinkFailed(ctx, link.ID, err)
		return err
	}

	client, err := w.newClient(ctx, peer, link)
	if err != nil {
		_ = w.service.markLinkFailed(ctx, link.ID, err)
		return err
	}
	defer func() { _ = client.Close() }()

	if err := w.pushOutbound(ctx, client, peer, link); err != nil {
		_ = w.service.markLinkFailed(ctx, link.ID, err)
		return err
	}
	if err := w.pullInbound(ctx, client, peer, link); err != nil {
		_ = w.service.markLinkFailed(ctx, link.ID, err)
		return err
	}
	if err := w.service.markLinkHealthy(ctx, link.ID); err != nil {
		return err
	}

	return nil
}

func (w *Worker) processBridgeLink(ctx context.Context, peer Peer, link Link) error {
	if _, err := w.applyPendingInboundBundles(ctx, peer, link); err != nil {
		return err
	}

	cursor, err := w.service.ReplicationCursorByPeerAndLink(ctx, peer.ID, link.ID)
	if err != nil {
		return err
	}

	pending, err := w.service.BundlesAfter(
		ctx,
		peer.ID,
		link.ID,
		BundleDirectionOutbound,
		cursor.LastAckedCursor,
		1,
	)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		if _, err := w.queuePendingOutboundEvents(ctx, peer, link, cursor.LastOutboundCursor); err != nil {
			return err
		}
	}

	if err := w.service.markLinkHealthy(ctx, link.ID); err != nil {
		return err
	}

	return nil
}

func (w *Worker) pushOutbound(ctx context.Context, client ReplicationClient, peer Peer, link Link) error {
	cursor, err := w.service.ReplicationCursorByPeerAndLink(ctx, peer.ID, link.ID)
	if err != nil {
		return err
	}

	pending, err := w.service.BundlesAfter(
		ctx,
		peer.ID,
		link.ID,
		BundleDirectionOutbound,
		cursor.LastAckedCursor,
		w.settings.BatchSize,
	)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		pending, err = w.queuePendingOutboundEvents(ctx, peer, link, cursor.LastOutboundCursor)
		if err != nil {
			return err
		}
	}
	if len(pending) == 0 {
		return nil
	}

	if err := client.PushBundles(ctx, w.settings.LocalServerName, link.Name, pending); err != nil {
		return fmt.Errorf("push federation bundles to %s/%s: %w", peer.ServerName, link.Name, err)
	}

	bundleIDs := make([]string, 0, len(pending))
	var upToCursor uint64
	for _, bundle := range pending {
		bundleIDs = append(bundleIDs, bundle.ID)
		if bundle.CursorTo > upToCursor {
			upToCursor = bundle.CursorTo
		}
	}

	if _, err := w.service.AcknowledgeBundles(ctx, AcknowledgeBundlesParams{
		PeerID:         peer.ID,
		LinkID:         link.ID,
		UpToCursor:     upToCursor,
		BundleIDs:      bundleIDs,
		AcknowledgedAt: w.currentTime(),
	}); err != nil {
		return err
	}

	return nil
}

func (w *Worker) pullInbound(ctx context.Context, client ReplicationClient, peer Peer, link Link) error {
	cursor, err := w.service.ReplicationCursorByPeerAndLink(ctx, peer.ID, link.ID)
	if err != nil {
		return err
	}

	bundles, _, err := client.PullBundles(
		ctx,
		w.settings.LocalServerName,
		link.Name,
		cursor.LastInboundCursor,
		w.settings.BatchSize,
	)
	if err != nil {
		return fmt.Errorf("pull federation bundles from %s/%s: %w", peer.ServerName, link.Name, err)
	}
	if len(bundles) == 0 {
		return nil
	}

	params := make([]SaveBundleParams, 0, len(bundles))
	bundleIDs := make([]string, 0, len(bundles))
	var upToCursor uint64
	for _, bundle := range bundles {
		dedupKey := strings.TrimSpace(bundle.DedupKey)
		if dedupKey == "" {
			dedupKey = outboundDedupKey(peer.ServerName, link.Name, bundle.CursorFrom, bundle.CursorTo)
		}

		params = append(params, SaveBundleParams{
			PeerID:        peer.ID,
			LinkID:        link.ID,
			DedupKey:      dedupKey,
			CursorFrom:    bundle.CursorFrom,
			CursorTo:      bundle.CursorTo,
			EventCount:    bundle.EventCount,
			PayloadType:   bundle.PayloadType,
			Payload:       append([]byte(nil), bundle.Payload...),
			Compression:   bundle.Compression,
			IntegrityHash: bundle.IntegrityHash,
			AuthTag:       bundle.AuthTag,
			AvailableAt:   bundle.AvailableAt,
			ExpiresAt:     bundle.ExpiresAt,
		})
		if strings.TrimSpace(bundle.ID) != "" {
			bundleIDs = append(bundleIDs, bundle.ID)
		}
		if bundle.CursorTo > upToCursor {
			upToCursor = bundle.CursorTo
		}
	}

	if _, _, err := w.service.PushInboundBundles(ctx, peer.ID, link.ID, params); err != nil {
		return err
	}

	appliedCursor, err := w.applyPendingInboundBundles(ctx, peer, link)
	if err != nil {
		return err
	}
	if appliedCursor < upToCursor {
		return ErrConflict
	}
	if err := client.AcknowledgeBundles(ctx, w.settings.LocalServerName, link.Name, upToCursor, bundleIDs); err != nil {
		return fmt.Errorf("acknowledge federation bundles to %s/%s: %w", peer.ServerName, link.Name, err)
	}

	return nil
}

func (w *Worker) applyPendingInboundBundles(ctx context.Context, peer Peer, link Link) (uint64, error) {
	cursor, err := w.service.ReplicationCursorByPeerAndLink(ctx, peer.ID, link.ID)
	if err != nil {
		return 0, err
	}

	bundles, err := w.service.BundlesAfter(
		ctx,
		peer.ID,
		link.ID,
		BundleDirectionInbound,
		cursor.LastInboundCursor,
		w.settings.BatchSize,
	)
	if err != nil {
		return 0, err
	}
	if len(bundles) == 0 {
		return cursor.LastInboundCursor, nil
	}

	appliedCursor := cursor.LastInboundCursor
	for _, bundle := range bundles {
		if err := w.applyInboundBundle(ctx, peer, bundle); err != nil {
			return appliedCursor, err
		}
		if bundle.CursorTo > appliedCursor {
			appliedCursor = bundle.CursorTo
		}
	}

	cursor, err = w.service.AdvanceInboundCursor(ctx, peer.ID, link.ID, appliedCursor)
	if err != nil {
		return 0, err
	}

	return cursor.LastInboundCursor, nil
}

func (w *Worker) queuePendingOutboundEvents(
	ctx context.Context,
	peer Peer,
	link Link,
	afterCursor uint64,
) ([]Bundle, error) {
	events, err := w.conversations.EventsAfterSequence(ctx, afterCursor, w.settings.BatchSize, nil)
	if err != nil {
		return nil, fmt.Errorf("load conversation events after %d: %w", afterCursor, err)
	}
	if len(events) == 0 {
		return nil, nil
	}

	params, err := w.buildOutboundBundles(ctx, peer, link, events)
	if err != nil {
		return nil, err
	}
	queued := make([]Bundle, 0, len(params))
	for _, param := range params {
		param.PeerID = peer.ID
		param.LinkID = link.ID
		param.DedupKey = outboundDedupKey(
			w.settings.LocalServerName,
			link.Name,
			param.CursorFrom,
			param.CursorTo,
		)
		bundle, err := w.service.QueueOutboundBundle(ctx, param)
		if err != nil {
			return nil, err
		}
		queued = append(queued, bundle)
	}

	return queued, nil
}

func (w *Worker) buildOutboundBundles(
	ctx context.Context,
	peer Peer,
	link Link,
	events []conversation.EventEnvelope,
) ([]SaveBundleParams, error) {
	filtered := make([]conversation.EventEnvelope, 0, len(events))
	for _, event := range events {
		if originServer := strings.TrimSpace(strings.ToLower(event.Metadata[federationOriginServerMetadataKey])); originServer != "" &&
			originServer == strings.TrimSpace(strings.ToLower(peer.ServerName)) {
			continue
		}
		if !eventFamilyAllowed(link.AllowedEventFamilies, event) {
			continue
		}
		if event.ConversationID != "" && len(link.AllowedConversationKinds) > 0 {
			conversationRow, err := w.conversations.ConversationByID(ctx, event.ConversationID)
			if err != nil {
				return nil, fmt.Errorf("load conversation %s for federation bundle: %w", event.ConversationID, err)
			}
			if !conversationKindAllowed(link.AllowedConversationKinds, conversationRow.Kind) {
				continue
			}
		}
		filtered = append(filtered, event)
	}
	if len(filtered) == 0 {
		return emptyFederationEventBatch(events, link.MaxBundleBytes)
	}

	params := make([]SaveBundleParams, 0, len(filtered))
	start := 0
	for start < len(filtered) {
		end := start + 1
		payload, compression, err := marshalFederationEventBatch(filtered[start:end], link.MaxBundleBytes)
		if err != nil {
			if errors.Is(err, ErrConflict) && shouldSkipOversizedSingleEvent(link) {
				start = end
				continue
			}
			return nil, err
		}

		for end < len(filtered) {
			candidatePayload, candidateCompression, candidateErr := marshalFederationEventBatch(filtered[start:end+1], link.MaxBundleBytes)
			if candidateErr != nil {
				if errors.Is(candidateErr, ErrConflict) {
					break
				}
				return nil, candidateErr
			}

			payload = candidatePayload
			compression = candidateCompression
			end++
		}

		first := filtered[start]
		last := filtered[end-1]
		params = append(params, SaveBundleParams{
			CursorFrom:  first.Sequence,
			CursorTo:    last.Sequence,
			EventCount:  end - start,
			PayloadType: bundlePayloadTypeConversationEvents,
			Payload:     payload,
			Compression: compression,
		})
		start = end
	}
	if len(params) == 0 {
		return emptyFederationEventBatch(events, link.MaxBundleBytes)
	}

	return params, nil
}

func emptyFederationEventBatch(events []conversation.EventEnvelope, maxBytes int) ([]SaveBundleParams, error) {
	last := events[len(events)-1]
	payload, compression, err := marshalFederationEventBatch(nil, maxBytes)
	if err != nil {
		return nil, err
	}

	return []SaveBundleParams{{
		CursorFrom:  firstSequence(events),
		CursorTo:    last.Sequence,
		EventCount:  0,
		PayloadType: bundlePayloadTypeConversationEvents,
		Payload:     payload,
		Compression: compression,
	}}, nil
}

func marshalFederationEventBatch(
	events []conversation.EventEnvelope,
	maxBytes int,
) ([]byte, CompressionKind, error) {
	payload, err := json.Marshal(events)
	if err != nil {
		return nil, CompressionKindUnspecified, fmt.Errorf("marshal federation event batch: %w", err)
	}
	if maxBytes <= 0 || len(payload) <= maxBytes {
		return payload, CompressionKindNone, nil
	}

	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(payload); err != nil {
		return nil, CompressionKindUnspecified, fmt.Errorf("compress federation event batch: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, CompressionKindUnspecified, fmt.Errorf("finalize federation event compression: %w", err)
	}
	if compressed.Len() <= maxBytes {
		return compressed.Bytes(), CompressionKindGzip, nil
	}

	return nil, CompressionKindUnspecified, ErrConflict
}

func decodeFederationEventBatch(bundle Bundle) ([]conversation.EventEnvelope, error) {
	payload := append([]byte(nil), bundle.Payload...)
	switch bundle.Compression {
	case CompressionKindUnspecified, CompressionKindNone:
	case CompressionKindGzip:
		reader, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("open federation bundle %s gzip: %w", bundle.ID, err)
		}

		decompressed, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			return nil, fmt.Errorf("read federation bundle %s gzip: %w", bundle.ID, err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close federation bundle %s gzip: %w", bundle.ID, closeErr)
		}
		payload = decompressed
	default:
		return nil, ErrInvalidInput
	}

	var events []conversation.EventEnvelope
	if err := json.Unmarshal(payload, &events); err != nil {
		return nil, fmt.Errorf("decode federation bundle %s payload: %w", bundle.ID, err)
	}

	return events, nil
}

func conversationKindAllowed(
	allowed []ConversationKind,
	kind conversation.ConversationKind,
) bool {
	if len(allowed) == 0 {
		return true
	}

	normalizedKind := ConversationKind(strings.TrimSpace(strings.ToLower(string(kind))))
	for _, candidate := range allowed {
		if candidate == normalizedKind {
			return true
		}
	}

	return false
}

func eventFamilyAllowed(allowed []EventFamily, event conversation.EventEnvelope) bool {
	if len(allowed) == 0 {
		return true
	}

	family := eventFamilyForConversationEvent(event)
	if family == EventFamilyUnspecified {
		return false
	}
	for _, candidate := range allowed {
		if candidate == family {
			return true
		}
	}

	return false
}

func eventFamilyForConversationEvent(event conversation.EventEnvelope) EventFamily {
	switch event.EventType {
	case conversation.EventTypeConversationCreated, conversation.EventTypeConversationUpdated:
		return EventFamilyConversation
	case conversation.EventTypeConversationMembers:
		return EventFamilyMembership
	case conversation.EventTypeMessageCreated,
		conversation.EventTypeMessageEdited,
		conversation.EventTypeMessageDeleted,
		conversation.EventTypeMessagePinned,
		conversation.EventTypeMessageReactionAdded,
		conversation.EventTypeMessageReactionUpdated,
		conversation.EventTypeMessageReactionRemoved:
		return EventFamilyMessage
	case conversation.EventTypeMessageDelivered,
		conversation.EventTypeMessageRead,
		conversation.EventTypeSyncAcknowledged:
		return EventFamilyReceipt
	case conversation.EventTypeTopicCreated,
		conversation.EventTypeTopicUpdated,
		conversation.EventTypeTopicArchived,
		conversation.EventTypeTopicPinned,
		conversation.EventTypeTopicClosed:
		return EventFamilyTopic
	case conversation.EventTypeUserUpdated:
		return EventFamilyUser
	case conversation.EventTypeAdminActionRecorded:
		return EventFamilyAdminAction
	default:
		return EventFamilyUnspecified
	}
}

func shouldSkipOversizedSingleEvent(link Link) bool {
	return link.DeliveryClass == DeliveryClassUltraConstrained
}

func firstSequence(events []conversation.EventEnvelope) uint64 {
	if len(events) == 0 {
		return 0
	}
	return events[0].Sequence
}

func (w *Worker) applyInboundBundle(ctx context.Context, peer Peer, bundle Bundle) error {
	if bundle.PayloadType != bundlePayloadTypeConversationEvents {
		return ErrInvalidInput
	}

	events, err := decodeFederationEventBatch(bundle)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	normalizedEvents := make([]conversation.EventEnvelope, 0, len(events))
	for _, event := range events {
		normalizedEvent := normalizeInboundEvent(peer.ServerName, event, w.currentTime())
		if err := ensureFederatedShadowState(ctx, w.identities, normalizedEvent); err != nil {
			return err
		}
		normalizedEvents = append(normalizedEvents, normalizedEvent)
	}

	return w.conversations.WithinTx(ctx, func(tx conversation.Store) error {
		for _, event := range normalizedEvents {
			if err := applyFederatedConversationEvent(ctx, tx, peer.ServerName, event); err != nil {
				return err
			}
		}

		return nil
	})
}

func outboundDedupKey(serverName string, linkName string, cursorFrom uint64, cursorTo uint64) string {
	return fmt.Sprintf(
		"out:%s:%s:%020d:%020d",
		strings.TrimSpace(strings.ToLower(serverName)),
		strings.TrimSpace(strings.ToLower(linkName)),
		cursorFrom,
		cursorTo,
	)
}

func isReplicablePeer(peer Peer) bool {
	if !peer.Trusted {
		return false
	}
	switch peer.State {
	case PeerStateActive, PeerStateDegraded:
	default:
		return false
	}

	for _, capability := range peer.Capabilities {
		if capability == CapabilityEventReplication {
			return true
		}
	}

	return false
}

func isReplicableLink(link Link) bool {
	switch link.State {
	case LinkStateActive, LinkStateDegraded:
	default:
		return false
	}

	switch link.TransportKind {
	case TransportKindHTTPS,
		TransportKindMeshtastic,
		TransportKindMeshCore,
		TransportKindCustomDTN:
		return true
	default:
		return false
	}
}

func applyFederatedConversationEvent(
	ctx context.Context,
	store conversation.Store,
	serverName string,
	event conversation.EventEnvelope,
) error {
	normalizedEvent := event
	if normalizedEvent.CreatedAt.IsZero() {
		normalizedEvent.CreatedAt = time.Now().UTC()
	}

	conversationRow, err := ensureFederatedConversation(ctx, store, serverName, normalizedEvent)
	if err != nil {
		return err
	}

	topic, hasTopic, err := ensureFederatedTopic(ctx, store, normalizedEvent)
	if err != nil {
		return err
	}

	savedEvent, err := store.SaveEvent(ctx, normalizedEvent)
	if err != nil {
		return fmt.Errorf("save federated event %s: %w", normalizedEvent.EventID, err)
	}

	if savedEvent.Sequence > conversationRow.LastSequence {
		conversationRow.LastSequence = savedEvent.Sequence
	}
	if err := materializeFederatedEvent(ctx, store, normalizedEvent, savedEvent, conversationRow); err != nil {
		return err
	}
	if isConversationShellUpdate(normalizedEvent) {
		updateFederatedConversationMetadata(&conversationRow, normalizedEvent)
	}
	if isMessageVisibleEvent(normalizedEvent.EventType) && normalizedEvent.CreatedAt.After(conversationRow.LastMessageAt) {
		conversationRow.LastMessageAt = normalizedEvent.CreatedAt
	}
	conversationRow.UpdatedAt = maxTime(conversationRow.UpdatedAt, savedEvent.CreatedAt)

	if _, err := store.SaveConversation(ctx, conversationRow); err != nil {
		return fmt.Errorf("save federated conversation %s: %w", conversationRow.ID, err)
	}

	if hasTopic {
		if savedEvent.Sequence > topic.LastSequence {
			topic.LastSequence = savedEvent.Sequence
		}
		updateFederatedTopicMetadata(&topic, normalizedEvent)
		if isMessageVisibleEvent(normalizedEvent.EventType) && normalizedEvent.CreatedAt.After(topic.LastMessageAt) {
			topic.LastMessageAt = normalizedEvent.CreatedAt
		}
		topic.UpdatedAt = maxTime(topic.UpdatedAt, savedEvent.CreatedAt)
		if _, err := store.SaveTopic(ctx, topic); err != nil {
			return fmt.Errorf("save federated topic %s/%s: %w", topic.ConversationID, topic.ID, err)
		}
	}

	return nil
}

func ensureFederatedConversation(
	ctx context.Context,
	store conversation.Store,
	serverName string,
	event conversation.EventEnvelope,
) (conversation.Conversation, error) {
	conversationRow, err := store.ConversationByID(ctx, event.ConversationID)
	if err == nil {
		updateFederatedConversationMetadata(&conversationRow, event)
		if err := ensureFederatedConversationMember(
			ctx,
			store,
			conversationRow.ID,
			conversationRow.OwnerAccountID,
			conversation.MemberRoleOwner,
			conversationRow.CreatedAt,
		); err != nil {
			return conversation.Conversation{}, err
		}
		if err := ensureFederatedGeneralTopic(
			ctx,
			store,
			conversationRow.ID,
			conversationRow.OwnerAccountID,
			conversationRow.CreatedAt,
		); err != nil {
			return conversation.Conversation{}, err
		}
		return conversationRow, nil
	}
	if !errors.Is(err, conversation.ErrNotFound) {
		return conversation.Conversation{}, fmt.Errorf("load federated conversation %s: %w", event.ConversationID, err)
	}

	ownerID := strings.TrimSpace(event.ActorAccountID)
	if ownerID == "" {
		ownerID = "federation:" + strings.TrimSpace(strings.ToLower(serverName))
	}
	kind := federatedConversationKind(event.Metadata["kind"])
	if kind == conversation.ConversationKindUnspecified {
		kind = conversation.ConversationKindGroup
	}
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	conversationRow = conversation.Conversation{
		ID:             event.ConversationID,
		Kind:           kind,
		Title:          strings.TrimSpace(event.Metadata["title"]),
		OwnerAccountID: ownerID,
		Settings:       conversation.DefaultConversationSettings(),
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}
	if conversationRow.Title == "" {
		conversationRow.Title = event.ConversationID
	}

	saved, err := store.SaveConversation(ctx, conversationRow)
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("create federated conversation %s: %w", event.ConversationID, err)
	}
	if err := ensureFederatedConversationMember(ctx, store, saved.ID, saved.OwnerAccountID, conversation.MemberRoleOwner, createdAt); err != nil {
		return conversation.Conversation{}, err
	}
	if err := ensureFederatedGeneralTopic(ctx, store, saved.ID, saved.OwnerAccountID, createdAt); err != nil {
		return conversation.Conversation{}, err
	}

	return saved, nil
}

func ensureFederatedTopic(
	ctx context.Context,
	store conversation.Store,
	event conversation.EventEnvelope,
) (conversation.ConversationTopic, bool, error) {
	topicID := federatedTopicID(event)
	topic, err := store.TopicByConversationAndID(ctx, event.ConversationID, topicID)
	if err == nil {
		updateFederatedTopicMetadata(&topic, event)
		return topic, true, nil
	}
	if !errors.Is(err, conversation.ErrNotFound) {
		return conversation.ConversationTopic{}, false, fmt.Errorf(
			"load federated topic %s in conversation %s: %w",
			topicID,
			event.ConversationID,
			err,
		)
	}

	createdBy := strings.TrimSpace(event.ActorAccountID)
	if createdBy == "" {
		createdBy = "federation"
	}
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	topic = conversation.ConversationTopic{
		ConversationID:     event.ConversationID,
		ID:                 topicID,
		Title:              federatedTopicTitle(event),
		CreatedByAccountID: createdBy,
		IsGeneral:          topicID == "",
		CreatedAt:          createdAt,
		UpdatedAt:          createdAt,
	}
	updateFederatedTopicMetadata(&topic, event)

	saved, err := store.SaveTopic(ctx, topic)
	if err != nil {
		return conversation.ConversationTopic{}, false, fmt.Errorf(
			"create federated topic %s in conversation %s: %w",
			topicID,
			event.ConversationID,
			err,
		)
	}

	return saved, true, nil
}

func updateFederatedConversationMetadata(conversationRow *conversation.Conversation, event conversation.EventEnvelope) {
	if conversationRow == nil {
		return
	}

	if kind := federatedConversationKind(event.Metadata["kind"]); kind != conversation.ConversationKindUnspecified {
		conversationRow.Kind = kind
	}
	if title := strings.TrimSpace(event.Metadata["title"]); title != "" {
		conversationRow.Title = title
	}
}

func updateFederatedTopicMetadata(topic *conversation.ConversationTopic, event conversation.EventEnvelope) {
	if topic == nil {
		return
	}

	if title := federatedTopicTitle(event); title != "" {
		topic.Title = title
	}
	switch event.EventType {
	case conversation.EventTypeTopicArchived:
		topic.Archived = metadataBool(event.Metadata, "archived")
		if topic.Archived {
			topic.ArchivedAt = event.CreatedAt
		}
	case conversation.EventTypeTopicPinned:
		topic.Pinned = metadataBool(event.Metadata, "pinned")
	case conversation.EventTypeTopicClosed:
		topic.Closed = metadataBool(event.Metadata, "closed")
		if topic.Closed {
			topic.ClosedAt = event.CreatedAt
		}
	}
}

func federatedConversationKind(value string) conversation.ConversationKind {
	switch conversation.ConversationKind(strings.TrimSpace(strings.ToLower(value))) {
	case conversation.ConversationKindDirect,
		conversation.ConversationKindGroup,
		conversation.ConversationKindChannel,
		conversation.ConversationKindSavedMessages:
		return conversation.ConversationKind(strings.TrimSpace(strings.ToLower(value)))
	default:
		return conversation.ConversationKindUnspecified
	}
}

func federatedTopicID(event conversation.EventEnvelope) string {
	topicID := strings.TrimSpace(event.Metadata["topic_id"])
	if topicID != "" {
		return topicID
	}

	return strings.TrimSpace(event.Metadata["thread_id"])
}

func federatedTopicTitle(event conversation.EventEnvelope) string {
	title := strings.TrimSpace(event.Metadata["title"])
	if title != "" {
		return title
	}

	if topicID := federatedTopicID(event); topicID != "" {
		return "Topic"
	}

	return ""
}

func metadataBool(metadata map[string]string, key string) bool {
	return strings.EqualFold(strings.TrimSpace(metadata[key]), "true")
}

func isConversationShellUpdate(event conversation.EventEnvelope) bool {
	return event.EventType == conversation.EventTypeConversationCreated ||
		event.EventType == conversation.EventTypeConversationUpdated
}

func isMessageVisibleEvent(eventType conversation.EventType) bool {
	switch eventType {
	case conversation.EventTypeMessageCreated,
		conversation.EventTypeMessageEdited,
		conversation.EventTypeMessageDeleted,
		conversation.EventTypeMessagePinned,
		conversation.EventTypeMessageReactionAdded,
		conversation.EventTypeMessageReactionUpdated,
		conversation.EventTypeMessageReactionRemoved:
		return true
	default:
		return false
	}
}

func maxTime(left time.Time, right time.Time) time.Time {
	if right.After(left) {
		return right
	}

	return left
}

func (w *Worker) currentTime() time.Time {
	if w == nil || w.now == nil {
		return time.Now().UTC()
	}

	return w.now().UTC()
}

func (w *Worker) validateContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return fmt.Errorf("%s: %w", operation, ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if w == nil || w.service == nil || w.identities == nil || w.conversations == nil || w.newClient == nil {
		return fmt.Errorf("%s: %w", operation, ErrInvalidInput)
	}

	return nil
}
