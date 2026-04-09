package federation

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Service owns peer, link, bundle, and cursor lifecycle for federation.
type Service struct {
	store Store
	now   func() time.Time
}

// NewService constructs a federation service backed by the provided store.
func NewService(store Store, opts ...Option) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}

	service := &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}

	return service, nil
}

// CreatePeer persists a new federation peer and returns the onboarding secret.
func (s *Service) CreatePeer(ctx context.Context, params CreatePeerParams) (Peer, string, bool, error) {
	if err := s.validateContext(ctx, "create federation peer"); err != nil {
		return Peer{}, "", false, err
	}

	params.ServerName = strings.TrimSpace(strings.ToLower(params.ServerName))
	params.BaseURL = strings.TrimSpace(params.BaseURL)
	params.SharedSecret = strings.TrimSpace(params.SharedSecret)
	if params.ServerName == "" {
		return Peer{}, "", false, ErrInvalidInput
	}

	sharedSecret := params.SharedSecret
	generated := false
	if sharedSecret == "" {
		token, err := randomToken(24)
		if err != nil {
			return Peer{}, "", false, fmt.Errorf("generate federation shared secret: %w", err)
		}
		sharedSecret = token
		generated = true
	}

	peerID, err := newID("peer")
	if err != nil {
		return Peer{}, "", false, fmt.Errorf("generate federation peer id: %w", err)
	}

	peer, err := (Peer{
		ID:                      peerID,
		ServerName:              params.ServerName,
		BaseURL:                 params.BaseURL,
		Capabilities:            params.Capabilities,
		Trusted:                 params.Trusted,
		VerificationFingerprint: secretFingerprint(sharedSecret),
		SharedSecret:            sharedSecret,
		SharedSecretHash:        secretHash(sharedSecret),
	}).normalize(s.currentTime())
	if err != nil {
		return Peer{}, "", false, err
	}

	saved, err := s.store.SavePeer(ctx, peer)
	if err != nil {
		return Peer{}, "", false, fmt.Errorf("save federation peer %s: %w", peer.ServerName, err)
	}

	return saved, sharedSecret, generated, nil
}

// PeerByID resolves one peer by ID.
func (s *Service) PeerByID(ctx context.Context, peerID string) (Peer, error) {
	if err := s.validateContext(ctx, "load federation peer"); err != nil {
		return Peer{}, err
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return Peer{}, ErrInvalidInput
	}

	peer, err := s.store.PeerByID(ctx, peerID)
	if err != nil {
		return Peer{}, fmt.Errorf("load federation peer %s: %w", peerID, err)
	}

	return peer, nil
}

// PeerByServerName resolves one peer by stable server name.
func (s *Service) PeerByServerName(ctx context.Context, serverName string) (Peer, error) {
	if err := s.validateContext(ctx, "load federation peer"); err != nil {
		return Peer{}, err
	}
	serverName = strings.TrimSpace(strings.ToLower(serverName))
	if serverName == "" {
		return Peer{}, ErrInvalidInput
	}

	peer, err := s.store.PeerByServerName(ctx, serverName)
	if err != nil {
		return Peer{}, fmt.Errorf("load federation peer by server name %s: %w", serverName, err)
	}

	return peer, nil
}

// ListPeers lists peers optionally filtered by state.
func (s *Service) ListPeers(ctx context.Context, state PeerState) ([]Peer, error) {
	if err := s.validateContext(ctx, "list federation peers"); err != nil {
		return nil, err
	}

	peers, err := s.store.PeersByState(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("list federation peers: %w", err)
	}

	sort.Slice(peers, func(i, j int) bool {
		if peers[i].CreatedAt.Equal(peers[j].CreatedAt) {
			return peers[i].ID < peers[j].ID
		}
		return peers[i].CreatedAt.Before(peers[j].CreatedAt)
	})

	return peers, nil
}

// UpdatePeer persists mutable peer fields.
func (s *Service) UpdatePeer(ctx context.Context, params UpdatePeerParams) (Peer, error) {
	if err := s.validateContext(ctx, "update federation peer"); err != nil {
		return Peer{}, err
	}
	params.PeerID = strings.TrimSpace(params.PeerID)
	if params.PeerID == "" {
		return Peer{}, ErrInvalidInput
	}

	current, err := s.store.PeerByID(ctx, params.PeerID)
	if err != nil {
		return Peer{}, fmt.Errorf("load federation peer %s before update: %w", params.PeerID, err)
	}

	if params.ServerName != nil {
		current.ServerName = strings.TrimSpace(strings.ToLower(*params.ServerName))
	}
	if params.BaseURL != nil {
		current.BaseURL = strings.TrimSpace(*params.BaseURL)
	}
	if params.Capabilities != nil {
		current.Capabilities = *params.Capabilities
	}
	if params.Trusted != nil {
		current.Trusted = *params.Trusted
	}
	if params.State != nil {
		current.State = *params.State
	}
	current.UpdatedAt = s.currentTime()

	current, err = current.normalize(current.UpdatedAt)
	if err != nil {
		return Peer{}, err
	}

	saved, err := s.store.SavePeer(ctx, current)
	if err != nil {
		return Peer{}, fmt.Errorf("save federation peer %s: %w", current.ID, err)
	}

	return saved, nil
}

// AuthenticatePeer validates the presented peer secret and returns the current peer snapshot.
func (s *Service) AuthenticatePeer(ctx context.Context, peerID string, sharedSecret string) (Peer, error) {
	if err := s.validateContext(ctx, "authenticate federation peer"); err != nil {
		return Peer{}, err
	}

	peerID = strings.TrimSpace(peerID)
	sharedSecret = strings.TrimSpace(sharedSecret)
	if peerID == "" || sharedSecret == "" {
		return Peer{}, ErrInvalidInput
	}

	peer, err := s.store.PeerByID(ctx, peerID)
	if err != nil {
		return Peer{}, fmt.Errorf("load federation peer %s during authentication: %w", peerID, err)
	}
	return s.authenticatePeer(ctx, peer, sharedSecret)
}

// AuthenticatePeerByServerName validates the presented peer secret via the stable server name.
func (s *Service) AuthenticatePeerByServerName(
	ctx context.Context,
	serverName string,
	sharedSecret string,
) (Peer, error) {
	if err := s.validateContext(ctx, "authenticate federation peer"); err != nil {
		return Peer{}, err
	}

	serverName = strings.TrimSpace(strings.ToLower(serverName))
	sharedSecret = strings.TrimSpace(sharedSecret)
	if serverName == "" || sharedSecret == "" {
		return Peer{}, ErrInvalidInput
	}

	peer, err := s.store.PeerByServerName(ctx, serverName)
	if err != nil {
		return Peer{}, fmt.Errorf("load federation peer by server name %s during authentication: %w", serverName, err)
	}

	return s.authenticatePeer(ctx, peer, sharedSecret)
}

// CreateLink persists a new delivery link for a peer.
func (s *Service) CreateLink(ctx context.Context, params CreateLinkParams) (Link, error) {
	if err := s.validateContext(ctx, "create federation link"); err != nil {
		return Link{}, err
	}
	params.PeerID = strings.TrimSpace(params.PeerID)
	params.Name = strings.TrimSpace(strings.ToLower(params.Name))
	if params.PeerID == "" || params.Name == "" {
		return Link{}, ErrInvalidInput
	}

	peer, err := s.store.PeerByID(ctx, params.PeerID)
	if err != nil {
		return Link{}, fmt.Errorf("load federation peer %s before link create: %w", params.PeerID, err)
	}

	linkID, err := newID("link")
	if err != nil {
		return Link{}, fmt.Errorf("generate federation link id: %w", err)
	}

	endpoint := strings.TrimSpace(params.Endpoint)
	if endpoint == "" {
		endpoint = peer.BaseURL
	}

	link, err := (Link{
		ID:                       linkID,
		PeerID:                   params.PeerID,
		Name:                     params.Name,
		Endpoint:                 endpoint,
		TransportKind:            params.TransportKind,
		DeliveryClass:            params.DeliveryClass,
		DiscoveryMode:            params.DiscoveryMode,
		MediaPolicy:              params.MediaPolicy,
		MaxBundleBytes:           params.MaxBundleBytes,
		MaxFragmentBytes:         params.MaxFragmentBytes,
		AllowedConversationKinds: params.AllowedConversationKinds,
	}).normalize(s.currentTime())
	if err != nil {
		return Link{}, err
	}

	saved, err := s.store.SaveLink(ctx, link)
	if err != nil {
		return Link{}, fmt.Errorf("save federation link %s: %w", link.Name, err)
	}

	return saved, nil
}

// LinkByID resolves one link by ID.
func (s *Service) LinkByID(ctx context.Context, linkID string) (Link, error) {
	if err := s.validateContext(ctx, "load federation link"); err != nil {
		return Link{}, err
	}
	linkID = strings.TrimSpace(linkID)
	if linkID == "" {
		return Link{}, ErrInvalidInput
	}

	link, err := s.store.LinkByID(ctx, linkID)
	if err != nil {
		return Link{}, fmt.Errorf("load federation link %s: %w", linkID, err)
	}

	return link, nil
}

// LinkByPeerAndName resolves one link by peer and stable link name.
func (s *Service) LinkByPeerAndName(ctx context.Context, peerID string, linkName string) (Link, error) {
	if err := s.validateContext(ctx, "load federation link"); err != nil {
		return Link{}, err
	}
	peerID = strings.TrimSpace(peerID)
	linkName = strings.TrimSpace(strings.ToLower(linkName))
	if peerID == "" || linkName == "" {
		return Link{}, ErrInvalidInput
	}

	link, err := s.store.LinkByPeerAndName(ctx, peerID, linkName)
	if err != nil {
		return Link{}, fmt.Errorf("load federation link %s for peer %s: %w", linkName, peerID, err)
	}

	return link, nil
}

// ListLinks lists links optionally filtered by peer and state.
func (s *Service) ListLinks(ctx context.Context, peerID string, state LinkState) ([]Link, error) {
	if err := s.validateContext(ctx, "list federation links"); err != nil {
		return nil, err
	}

	links, err := s.store.Links(ctx, strings.TrimSpace(peerID), state)
	if err != nil {
		return nil, fmt.Errorf("list federation links: %w", err)
	}

	sort.Slice(links, func(i, j int) bool {
		if links[i].CreatedAt.Equal(links[j].CreatedAt) {
			return links[i].ID < links[j].ID
		}
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	return links, nil
}

// UpdateLink persists mutable link fields.
func (s *Service) UpdateLink(ctx context.Context, params UpdateLinkParams) (Link, error) {
	if err := s.validateContext(ctx, "update federation link"); err != nil {
		return Link{}, err
	}
	params.LinkID = strings.TrimSpace(params.LinkID)
	if params.LinkID == "" {
		return Link{}, ErrInvalidInput
	}

	current, err := s.store.LinkByID(ctx, params.LinkID)
	if err != nil {
		return Link{}, fmt.Errorf("load federation link %s before update: %w", params.LinkID, err)
	}

	if params.Name != nil {
		current.Name = strings.TrimSpace(strings.ToLower(*params.Name))
	}
	if params.Endpoint != nil {
		current.Endpoint = strings.TrimSpace(*params.Endpoint)
	}
	if params.TransportKind != nil {
		current.TransportKind = *params.TransportKind
	}
	if params.DeliveryClass != nil {
		current.DeliveryClass = *params.DeliveryClass
	}
	if params.DiscoveryMode != nil {
		current.DiscoveryMode = *params.DiscoveryMode
	}
	if params.MediaPolicy != nil {
		current.MediaPolicy = *params.MediaPolicy
	}
	if params.State != nil {
		current.State = *params.State
	}
	if params.MaxBundleBytes != nil {
		current.MaxBundleBytes = *params.MaxBundleBytes
	}
	if params.MaxFragmentBytes != nil {
		current.MaxFragmentBytes = *params.MaxFragmentBytes
	}
	if params.AllowedConversationKinds != nil {
		current.AllowedConversationKinds = *params.AllowedConversationKinds
	}
	if params.LastHealthyAt != nil {
		current.LastHealthyAt = params.LastHealthyAt.UTC()
	}
	if params.LastError != nil {
		current.LastError = strings.TrimSpace(*params.LastError)
	}
	current.UpdatedAt = s.currentTime()

	current, err = current.normalize(current.UpdatedAt)
	if err != nil {
		return Link{}, err
	}

	saved, err := s.store.SaveLink(ctx, current)
	if err != nil {
		return Link{}, fmt.Errorf("save federation link %s: %w", current.ID, err)
	}

	return saved, nil
}

// PauseLink transitions a link to paused.
func (s *Service) PauseLink(ctx context.Context, linkID string) (Link, error) {
	state := LinkStatePaused
	return s.UpdateLink(ctx, UpdateLinkParams{LinkID: linkID, State: &state})
}

// ResumeLink transitions a link back to active.
func (s *Service) ResumeLink(ctx context.Context, linkID string) (Link, error) {
	state := LinkStateActive
	return s.UpdateLink(ctx, UpdateLinkParams{LinkID: linkID, State: &state})
}

// DeleteLink soft-deletes a link.
func (s *Service) DeleteLink(ctx context.Context, linkID string) (Link, error) {
	state := LinkStateDeleted
	return s.UpdateLink(ctx, UpdateLinkParams{LinkID: linkID, State: &state})
}

// QueueOutboundBundle stores one outbound bundle and updates the replication cursor.
func (s *Service) QueueOutboundBundle(ctx context.Context, params SaveBundleParams) (Bundle, error) {
	return s.saveBundle(ctx, params, BundleDirectionOutbound)
}

// AcceptInboundBundle stores one inbound bundle and updates the replication cursor.
func (s *Service) AcceptInboundBundle(ctx context.Context, params SaveBundleParams) (Bundle, error) {
	return s.saveBundle(ctx, params, BundleDirectionInbound)
}

// PullOutboundBundles returns one page of outbound bundles after the supplied cursor.
func (s *Service) PullOutboundBundles(
	ctx context.Context,
	peerID string,
	linkID string,
	afterCursor uint64,
	limit int,
) ([]Bundle, ReplicationCursor, bool, error) {
	if err := s.validateContext(ctx, "pull federation bundles"); err != nil {
		return nil, ReplicationCursor{}, false, err
	}
	if strings.TrimSpace(peerID) == "" || strings.TrimSpace(linkID) == "" {
		return nil, ReplicationCursor{}, false, ErrInvalidInput
	}
	if limit <= 0 {
		limit = 100
	}

	if _, err := s.authorizeLinkForPeer(ctx, peerID, linkID, false); err != nil {
		return nil, ReplicationCursor{}, false, err
	}

	bundles, err := s.store.BundlesAfter(ctx, peerID, linkID, BundleDirectionOutbound, afterCursor, limit+1)
	if err != nil {
		return nil, ReplicationCursor{}, false, fmt.Errorf("list federation outbound bundles for %s/%s: %w", peerID, linkID, err)
	}
	hasMore := len(bundles) > limit
	if hasMore {
		bundles = bundles[:limit]
	}

	cursor, err := s.ReplicationCursorByPeerAndLink(ctx, peerID, linkID)
	if err != nil {
		return nil, ReplicationCursor{}, false, err
	}
	if err := s.markLinkHealthy(ctx, linkID); err != nil {
		return nil, ReplicationCursor{}, false, err
	}

	return bundles, cursor, hasMore, nil
}

func (s *Service) saveBundle(ctx context.Context, params SaveBundleParams, direction BundleDirection) (Bundle, error) {
	if err := s.validateContext(ctx, "save federation bundle"); err != nil {
		return Bundle{}, err
	}

	params.PeerID = strings.TrimSpace(params.PeerID)
	params.LinkID = strings.TrimSpace(params.LinkID)
	params.DedupKey = strings.TrimSpace(params.DedupKey)
	if params.PeerID == "" || params.LinkID == "" || params.DedupKey == "" {
		return Bundle{}, ErrInvalidInput
	}

	link, err := s.store.LinkByID(ctx, params.LinkID)
	if err != nil {
		return Bundle{}, fmt.Errorf("load federation link %s before bundle save: %w", params.LinkID, err)
	}
	if link.PeerID != params.PeerID {
		return Bundle{}, ErrConflict
	}
	if link.State == LinkStateDeleted {
		return Bundle{}, ErrConflict
	}

	bundleID, err := newID("bundle")
	if err != nil {
		return Bundle{}, fmt.Errorf("generate federation bundle id: %w", err)
	}

	now := s.currentTime()
	bundle, err := (Bundle{
		ID:          bundleID,
		PeerID:      params.PeerID,
		LinkID:      params.LinkID,
		DedupKey:    params.DedupKey,
		Direction:   direction,
		CursorFrom:  params.CursorFrom,
		CursorTo:    params.CursorTo,
		EventCount:  params.EventCount,
		PayloadType: params.PayloadType,
		Payload:     append([]byte(nil), params.Payload...),
		Compression: params.Compression,
		AvailableAt: params.AvailableAt,
		ExpiresAt:   params.ExpiresAt,
	}).normalize(now)
	if err != nil {
		return Bundle{}, err
	}

	var saved Bundle
	err = s.store.WithinTx(ctx, func(store Store) error {
		saveBundle, saveErr := store.SaveBundle(ctx, bundle)
		if saveErr != nil {
			return fmt.Errorf("save federation bundle %s: %w", bundle.ID, saveErr)
		}

		cursor, cursorErr := store.CursorByPeerAndLink(ctx, bundle.PeerID, bundle.LinkID)
		if cursorErr != nil && !errors.Is(cursorErr, ErrNotFound) {
			return fmt.Errorf("load federation cursor for %s/%s: %w", bundle.PeerID, bundle.LinkID, cursorErr)
		}
		if errors.Is(cursorErr, ErrNotFound) {
			cursor = ReplicationCursor{
				PeerID: bundle.PeerID,
				LinkID: bundle.LinkID,
			}
		}

		if direction == BundleDirectionInbound && bundle.CursorTo > cursor.LastReceivedCursor {
			cursor.LastReceivedCursor = bundle.CursorTo
		}
		if direction == BundleDirectionOutbound && bundle.CursorTo > cursor.LastOutboundCursor {
			cursor.LastOutboundCursor = bundle.CursorTo
		}
		cursor.UpdatedAt = now

		if _, saveCursorErr := store.SaveCursor(ctx, cursor); saveCursorErr != nil {
			return fmt.Errorf("save federation cursor for %s/%s: %w", cursor.PeerID, cursor.LinkID, saveCursorErr)
		}

		saved = saveBundle
		return nil
	})
	if err != nil {
		return Bundle{}, err
	}

	return saved, nil
}

// AdvanceInboundCursor marks inbound bundles as successfully applied locally.
func (s *Service) AdvanceInboundCursor(
	ctx context.Context,
	peerID string,
	linkID string,
	upToCursor uint64,
) (ReplicationCursor, error) {
	if err := s.validateContext(ctx, "advance federation inbound cursor"); err != nil {
		return ReplicationCursor{}, err
	}
	peerID = strings.TrimSpace(peerID)
	linkID = strings.TrimSpace(linkID)
	if peerID == "" || linkID == "" {
		return ReplicationCursor{}, ErrInvalidInput
	}
	if _, err := s.authorizeLinkForPeer(ctx, peerID, linkID, false); err != nil {
		return ReplicationCursor{}, err
	}

	current, err := s.ReplicationCursorByPeerAndLink(ctx, peerID, linkID)
	if err != nil {
		return ReplicationCursor{}, err
	}
	if upToCursor > current.LastReceivedCursor {
		return ReplicationCursor{}, ErrConflict
	}
	if upToCursor <= current.LastInboundCursor {
		return current, nil
	}

	current.LastInboundCursor = upToCursor
	current.UpdatedAt = s.currentTime()
	saved, err := s.store.SaveCursor(ctx, current)
	if err != nil {
		return ReplicationCursor{}, fmt.Errorf("save federation inbound cursor for %s/%s: %w", peerID, linkID, err)
	}

	return saved, nil
}

// BundlesAfter lists stored bundles after the provided cursor.
func (s *Service) BundlesAfter(
	ctx context.Context,
	peerID string,
	linkID string,
	direction BundleDirection,
	afterCursor uint64,
	limit int,
) ([]Bundle, error) {
	if err := s.validateContext(ctx, "list federation bundles"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(peerID) == "" || strings.TrimSpace(linkID) == "" {
		return nil, ErrInvalidInput
	}

	bundles, err := s.store.BundlesAfter(ctx, strings.TrimSpace(peerID), strings.TrimSpace(linkID), direction, afterCursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list federation bundles for %s/%s: %w", peerID, linkID, err)
	}

	return bundles, nil
}

// PushInboundBundles persists a batch of inbound bundles for one authenticated peer/link.
func (s *Service) PushInboundBundles(
	ctx context.Context,
	peerID string,
	linkID string,
	bundles []SaveBundleParams,
) ([]Bundle, ReplicationCursor, error) {
	if err := s.validateContext(ctx, "push federation bundles"); err != nil {
		return nil, ReplicationCursor{}, err
	}
	peerID = strings.TrimSpace(peerID)
	linkID = strings.TrimSpace(linkID)
	if peerID == "" || linkID == "" {
		return nil, ReplicationCursor{}, ErrInvalidInput
	}

	if _, err := s.authorizeLinkForPeer(ctx, peerID, linkID, false); err != nil {
		return nil, ReplicationCursor{}, err
	}

	accepted := make([]Bundle, 0, len(bundles))
	for _, params := range bundles {
		params.PeerID = peerID
		params.LinkID = linkID
		bundle, err := s.AcceptInboundBundle(ctx, params)
		if err != nil {
			return nil, ReplicationCursor{}, err
		}
		accepted = append(accepted, bundle)
	}

	cursor, err := s.ReplicationCursorByPeerAndLink(ctx, peerID, linkID)
	if err != nil {
		return nil, ReplicationCursor{}, err
	}
	if err := s.markLinkHealthy(ctx, linkID); err != nil {
		return nil, ReplicationCursor{}, err
	}

	return accepted, cursor, nil
}

// AcknowledgeBundles marks outbound bundles as acknowledged and advances the cursor.
func (s *Service) AcknowledgeBundles(ctx context.Context, params AcknowledgeBundlesParams) (ReplicationCursor, error) {
	if err := s.validateContext(ctx, "acknowledge federation bundles"); err != nil {
		return ReplicationCursor{}, err
	}

	params.PeerID = strings.TrimSpace(params.PeerID)
	params.LinkID = strings.TrimSpace(params.LinkID)
	if params.PeerID == "" || params.LinkID == "" {
		return ReplicationCursor{}, ErrInvalidInput
	}
	if _, err := s.authorizeLinkForPeer(ctx, params.PeerID, params.LinkID, false); err != nil {
		return ReplicationCursor{}, err
	}
	if params.AcknowledgedAt.IsZero() {
		params.AcknowledgedAt = s.currentTime()
	}

	var cursor ReplicationCursor
	err := s.store.WithinTx(ctx, func(store Store) error {
		if _, ackErr := store.AcknowledgeBundles(ctx, params); ackErr != nil {
			return fmt.Errorf("acknowledge federation bundles for %s/%s: %w", params.PeerID, params.LinkID, ackErr)
		}

		current, currentErr := store.CursorByPeerAndLink(ctx, params.PeerID, params.LinkID)
		if currentErr != nil && !errors.Is(currentErr, ErrNotFound) {
			return fmt.Errorf("load federation cursor for %s/%s: %w", params.PeerID, params.LinkID, currentErr)
		}
		if errors.Is(currentErr, ErrNotFound) {
			current = ReplicationCursor{
				PeerID: params.PeerID,
				LinkID: params.LinkID,
			}
		}
		if params.UpToCursor > current.LastAckedCursor {
			current.LastAckedCursor = params.UpToCursor
		}
		current.UpdatedAt = params.AcknowledgedAt.UTC()

		saved, saveErr := store.SaveCursor(ctx, current)
		if saveErr != nil {
			return fmt.Errorf("save federation cursor for %s/%s: %w", current.PeerID, current.LinkID, saveErr)
		}

		cursor = saved
		return nil
	})
	if err != nil {
		return ReplicationCursor{}, err
	}
	if err := s.markLinkHealthy(ctx, params.LinkID); err != nil {
		return ReplicationCursor{}, err
	}

	return cursor, nil
}

// ReplicationCursorByPeerAndLink resolves the durable replication cursor for one link.
func (s *Service) ReplicationCursorByPeerAndLink(ctx context.Context, peerID string, linkID string) (ReplicationCursor, error) {
	if err := s.validateContext(ctx, "load federation cursor"); err != nil {
		return ReplicationCursor{}, err
	}
	peerID = strings.TrimSpace(peerID)
	linkID = strings.TrimSpace(linkID)
	if peerID == "" || linkID == "" {
		return ReplicationCursor{}, ErrInvalidInput
	}

	cursor, err := s.store.CursorByPeerAndLink(ctx, peerID, linkID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ReplicationCursor{
				PeerID: peerID,
				LinkID: linkID,
			}, nil
		}

		return ReplicationCursor{}, fmt.Errorf("load federation cursor for %s/%s: %w", peerID, linkID, err)
	}

	return cursor, nil
}

func (s *Service) currentTime() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}

	return s.now().UTC()
}

func (s *Service) validateContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return fmt.Errorf("%s: %w", operation, ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if s == nil || s.store == nil {
		return fmt.Errorf("%s: %w", operation, ErrInvalidInput)
	}

	return nil
}

func (s *Service) authenticatePeer(ctx context.Context, peer Peer, sharedSecret string) (Peer, error) {
	if subtle.ConstantTimeCompare([]byte(peer.SharedSecretHash), []byte(secretHash(sharedSecret))) != 1 {
		return Peer{}, ErrUnauthorized
	}
	if !peer.Trusted {
		return Peer{}, ErrForbidden
	}
	switch peer.State {
	case PeerStateActive, PeerStateDegraded:
	default:
		return Peer{}, ErrForbidden
	}

	peer.LastSeenAt = s.currentTime()
	peer.UpdatedAt = peer.LastSeenAt
	saved, err := s.store.SavePeer(ctx, peer)
	if err != nil {
		return Peer{}, fmt.Errorf("touch federation peer %s after authentication: %w", peer.ID, err)
	}

	return saved, nil
}

func (s *Service) authorizeLinkForPeer(ctx context.Context, peerID string, linkID string, allowPaused bool) (Link, error) {
	link, err := s.store.LinkByID(ctx, linkID)
	if err != nil {
		return Link{}, fmt.Errorf("load federation link %s: %w", linkID, err)
	}
	if link.PeerID != peerID {
		return Link{}, ErrForbidden
	}

	switch link.State {
	case LinkStateActive, LinkStateDegraded:
	case LinkStatePaused:
		if !allowPaused {
			return Link{}, ErrForbidden
		}
	case LinkStateDeleted:
		return Link{}, ErrForbidden
	default:
		return Link{}, ErrForbidden
	}

	return link, nil
}

func (s *Service) markLinkHealthy(ctx context.Context, linkID string) error {
	link, err := s.store.LinkByID(ctx, linkID)
	if err != nil {
		return fmt.Errorf("load federation link %s before health update: %w", linkID, err)
	}
	now := s.currentTime()
	link.LastHealthyAt = now
	link.LastError = ""
	if link.State == LinkStateDegraded {
		link.State = LinkStateActive
	}
	link.UpdatedAt = now

	if _, err := s.store.SaveLink(ctx, link); err != nil {
		return fmt.Errorf("save federation link %s health: %w", link.ID, err)
	}

	return nil
}

func (s *Service) markLinkFailed(ctx context.Context, linkID string, lastError error) error {
	link, err := s.store.LinkByID(ctx, linkID)
	if err != nil {
		return fmt.Errorf("load federation link %s before failure update: %w", linkID, err)
	}
	now := s.currentTime()
	if lastError != nil {
		link.LastError = strings.TrimSpace(lastError.Error())
	}
	if link.State == LinkStateActive {
		link.State = LinkStateDegraded
	}
	link.UpdatedAt = now

	if _, err := s.store.SaveLink(ctx, link); err != nil {
		return fmt.Errorf("save federation link %s failure: %w", link.ID, err)
	}

	return nil
}
