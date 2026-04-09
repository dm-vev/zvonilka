package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/federation"
)

const (
	peerColumnList = `
id, server_name, base_url, capabilities, trusted, state, verification_fingerprint, shared_secret, shared_secret_hash,
created_at, updated_at, last_seen_at`
	linkColumnList = `
id, peer_id, name, endpoint, transport_kind, delivery_class, discovery_mode, media_policy, state,
max_bundle_bytes, max_fragment_bytes, allowed_conversation_kinds, created_at, updated_at, last_healthy_at, last_error`
	bundleColumnList = `
id, peer_id, link_id, dedup_key, direction, cursor_from, cursor_to, event_count, payload_type, payload,
compression, state, created_at, available_at, expires_at, acked_at`
)

func (s *Store) SavePeer(ctx context.Context, peer federation.Peer) (federation.Peer, error) {
	if err := s.requireStore(); err != nil {
		return federation.Peer{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return federation.Peer{}, err
	}

	peer, err := federation.NormalizePeer(peer, time.Now().UTC())
	if err != nil {
		return federation.Peer{}, err
	}

	rawCaps, err := encodeCapabilities(peer.Capabilities)
	if err != nil {
		return federation.Peer{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, server_name, base_url, capabilities, trusted, state, verification_fingerprint, shared_secret, shared_secret_hash,
	created_at, updated_at, last_seen_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (id) DO UPDATE SET
	server_name = EXCLUDED.server_name,
	base_url = EXCLUDED.base_url,
	capabilities = EXCLUDED.capabilities,
	trusted = EXCLUDED.trusted,
	state = EXCLUDED.state,
	verification_fingerprint = EXCLUDED.verification_fingerprint,
	shared_secret = EXCLUDED.shared_secret,
	shared_secret_hash = EXCLUDED.shared_secret_hash,
	created_at = EXCLUDED.created_at,
	updated_at = EXCLUDED.updated_at,
	last_seen_at = EXCLUDED.last_seen_at
RETURNING `+peerColumnList+`
`, s.table("federation_peers"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		peer.ID,
		peer.ServerName,
		peer.BaseURL,
		rawCaps,
		peer.Trusted,
		peer.State,
		peer.VerificationFingerprint,
		peer.SharedSecret,
		peer.SharedSecretHash,
		peer.CreatedAt.UTC(),
		peer.UpdatedAt.UTC(),
		nullTime(peer.LastSeenAt),
	)

	saved, err := scanPeer(row)
	if err != nil {
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return federation.Peer{}, mappedErr
		}
		return federation.Peer{}, fmt.Errorf("save federation peer %s: %w", peer.ID, err)
	}

	return saved, nil
}

func (s *Store) PeerByID(ctx context.Context, peerID string) (federation.Peer, error) {
	if err := s.requireStore(); err != nil {
		return federation.Peer{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return federation.Peer{}, err
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return federation.Peer{}, federation.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, peerColumnList, s.table("federation_peers"))
	row := s.conn().QueryRowContext(ctx, query, peerID)
	peer, err := scanPeer(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return federation.Peer{}, federation.ErrNotFound
		}
		return federation.Peer{}, fmt.Errorf("load federation peer %s: %w", peerID, err)
	}

	return peer, nil
}

func (s *Store) PeerByServerName(ctx context.Context, serverName string) (federation.Peer, error) {
	if err := s.requireStore(); err != nil {
		return federation.Peer{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return federation.Peer{}, err
	}
	serverName = strings.TrimSpace(strings.ToLower(serverName))
	if serverName == "" {
		return federation.Peer{}, federation.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE server_name = $1`, peerColumnList, s.table("federation_peers"))
	row := s.conn().QueryRowContext(ctx, query, serverName)
	peer, err := scanPeer(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return federation.Peer{}, federation.ErrNotFound
		}
		return federation.Peer{}, fmt.Errorf("load federation peer by server name %s: %w", serverName, err)
	}

	return peer, nil
}

func (s *Store) PeersByState(ctx context.Context, state federation.PeerState) ([]federation.Peer, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT %s FROM %s`, peerColumnList, s.table("federation_peers"))
	args := make([]any, 0, 1)
	if state != federation.PeerStateUnspecified {
		query += ` WHERE state = $1`
		args = append(args, state)
	}
	query += ` ORDER BY created_at ASC, id ASC`

	rows, err := s.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list federation peers: %w", err)
	}
	defer rows.Close()

	peers := make([]federation.Peer, 0)
	for rows.Next() {
		peer, scanErr := scanPeer(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan federation peer: %w", scanErr)
		}
		peers = append(peers, peer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate federation peers: %w", err)
	}

	return peers, nil
}

func (s *Store) SaveLink(ctx context.Context, link federation.Link) (federation.Link, error) {
	if err := s.requireStore(); err != nil {
		return federation.Link{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return federation.Link{}, err
	}

	link, err := federation.NormalizeLink(link, time.Now().UTC())
	if err != nil {
		return federation.Link{}, err
	}

	rawKinds, err := encodeConversationKinds(link.AllowedConversationKinds)
	if err != nil {
		return federation.Link{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, peer_id, name, endpoint, transport_kind, delivery_class, discovery_mode, media_policy, state,
	max_bundle_bytes, max_fragment_bytes, allowed_conversation_kinds, created_at, updated_at, last_healthy_at, last_error
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
)
ON CONFLICT (id) DO UPDATE SET
	peer_id = EXCLUDED.peer_id,
	name = EXCLUDED.name,
	endpoint = EXCLUDED.endpoint,
	transport_kind = EXCLUDED.transport_kind,
	delivery_class = EXCLUDED.delivery_class,
	discovery_mode = EXCLUDED.discovery_mode,
	media_policy = EXCLUDED.media_policy,
	state = EXCLUDED.state,
	max_bundle_bytes = EXCLUDED.max_bundle_bytes,
	max_fragment_bytes = EXCLUDED.max_fragment_bytes,
	allowed_conversation_kinds = EXCLUDED.allowed_conversation_kinds,
	created_at = EXCLUDED.created_at,
	updated_at = EXCLUDED.updated_at,
	last_healthy_at = EXCLUDED.last_healthy_at,
	last_error = EXCLUDED.last_error
RETURNING `+linkColumnList+`
`, s.table("federation_links"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		link.ID,
		link.PeerID,
		link.Name,
		link.Endpoint,
		link.TransportKind,
		link.DeliveryClass,
		link.DiscoveryMode,
		link.MediaPolicy,
		link.State,
		link.MaxBundleBytes,
		link.MaxFragmentBytes,
		rawKinds,
		link.CreatedAt.UTC(),
		link.UpdatedAt.UTC(),
		nullTime(link.LastHealthyAt),
		link.LastError,
	)

	saved, err := scanLink(row)
	if err != nil {
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return federation.Link{}, mappedErr
		}
		return federation.Link{}, fmt.Errorf("save federation link %s: %w", link.ID, err)
	}

	return saved, nil
}

func (s *Store) LinkByID(ctx context.Context, linkID string) (federation.Link, error) {
	if err := s.requireStore(); err != nil {
		return federation.Link{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return federation.Link{}, err
	}
	linkID = strings.TrimSpace(linkID)
	if linkID == "" {
		return federation.Link{}, federation.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, linkColumnList, s.table("federation_links"))
	row := s.conn().QueryRowContext(ctx, query, linkID)
	link, err := scanLink(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return federation.Link{}, federation.ErrNotFound
		}
		return federation.Link{}, fmt.Errorf("load federation link %s: %w", linkID, err)
	}

	return link, nil
}

func (s *Store) LinkByPeerAndName(ctx context.Context, peerID string, name string) (federation.Link, error) {
	if err := s.requireStore(); err != nil {
		return federation.Link{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return federation.Link{}, err
	}
	peerID = strings.TrimSpace(peerID)
	name = strings.TrimSpace(strings.ToLower(name))
	if peerID == "" || name == "" {
		return federation.Link{}, federation.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE peer_id = $1 AND name = $2`, linkColumnList, s.table("federation_links"))
	row := s.conn().QueryRowContext(ctx, query, peerID, name)
	link, err := scanLink(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return federation.Link{}, federation.ErrNotFound
		}
		return federation.Link{}, fmt.Errorf("load federation link %s for peer %s: %w", name, peerID, err)
	}

	return link, nil
}

func (s *Store) Links(ctx context.Context, peerID string, state federation.LinkState) ([]federation.Link, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE 1=1`, linkColumnList, s.table("federation_links"))
	args := make([]any, 0, 2)
	index := 1
	if peerID = strings.TrimSpace(peerID); peerID != "" {
		query += fmt.Sprintf(" AND peer_id = $%d", index)
		args = append(args, peerID)
		index++
	}
	if state != federation.LinkStateUnspecified {
		query += fmt.Sprintf(" AND state = $%d", index)
		args = append(args, state)
		index++
	}
	query += ` ORDER BY created_at ASC, id ASC`

	rows, err := s.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list federation links: %w", err)
	}
	defer rows.Close()

	links := make([]federation.Link, 0)
	for rows.Next() {
		link, scanErr := scanLink(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan federation link: %w", scanErr)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate federation links: %w", err)
	}

	return links, nil
}

func (s *Store) SaveBundle(ctx context.Context, bundle federation.Bundle) (federation.Bundle, error) {
	if err := s.requireStore(); err != nil {
		return federation.Bundle{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return federation.Bundle{}, err
	}

	bundle, err := federation.NormalizeBundle(bundle, time.Now().UTC())
	if err != nil {
		return federation.Bundle{}, err
	}

	insertQuery := fmt.Sprintf(`
INSERT INTO %s (
	id, peer_id, link_id, dedup_key, direction, cursor_from, cursor_to, event_count, payload_type, payload,
	compression, state, created_at, available_at, expires_at, acked_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
)
ON CONFLICT (dedup_key) DO NOTHING
RETURNING `+bundleColumnList+`
`, s.table("federation_bundles"))

	row := s.conn().QueryRowContext(
		ctx,
		insertQuery,
		bundle.ID,
		bundle.PeerID,
		bundle.LinkID,
		bundle.DedupKey,
		bundle.Direction,
		bundle.CursorFrom,
		bundle.CursorTo,
		bundle.EventCount,
		bundle.PayloadType,
		bundle.Payload,
		bundle.Compression,
		bundle.State,
		bundle.CreatedAt.UTC(),
		bundle.AvailableAt.UTC(),
		nullTime(bundle.ExpiresAt),
		nullTime(bundle.AckedAt),
	)

	saved, err := scanBundle(row)
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return federation.Bundle{}, mappedErr
		}
		return federation.Bundle{}, fmt.Errorf("save federation bundle %s: %w", bundle.ID, err)
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE dedup_key = $1`, bundleColumnList, s.table("federation_bundles"))
	row = s.conn().QueryRowContext(ctx, query, bundle.DedupKey)
	saved, err = scanBundle(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return federation.Bundle{}, federation.ErrNotFound
		}
		return federation.Bundle{}, fmt.Errorf("load existing federation bundle by dedup key %s: %w", bundle.DedupKey, err)
	}
	if saved.PeerID != bundle.PeerID || saved.LinkID != bundle.LinkID || saved.Direction != bundle.Direction {
		return federation.Bundle{}, federation.ErrConflict
	}

	return saved, nil
}

func (s *Store) BundlesAfter(
	ctx context.Context,
	peerID string,
	linkID string,
	direction federation.BundleDirection,
	afterCursor uint64,
	limit int,
) ([]federation.Bundle, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, nil
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE peer_id = $1 AND link_id = $2 AND direction = $3 AND cursor_to > $4
ORDER BY cursor_to ASC, created_at ASC, id ASC
LIMIT $5
`, bundleColumnList, s.table("federation_bundles"))

	rows, err := s.conn().QueryContext(ctx, query, peerID, linkID, direction, afterCursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list federation bundles after cursor for %s/%s: %w", peerID, linkID, err)
	}
	defer rows.Close()

	bundles := make([]federation.Bundle, 0)
	for rows.Next() {
		bundle, scanErr := scanBundle(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan federation bundle: %w", scanErr)
		}
		bundles = append(bundles, bundle)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate federation bundles for %s/%s: %w", peerID, linkID, err)
	}

	return bundles, nil
}

func (s *Store) AcknowledgeBundles(
	ctx context.Context,
	params federation.AcknowledgeBundlesParams,
) ([]federation.Bundle, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}

	args := []any{params.PeerID, params.LinkID, params.UpToCursor, params.AcknowledgedAt.UTC()}
	query := fmt.Sprintf(`
UPDATE %s
SET state = $5, acked_at = $4
WHERE peer_id = $1 AND link_id = $2 AND direction = $6 AND (cursor_to <= $3`,
		s.table("federation_bundles"),
	)
	args = append(args, federation.BundleStateAcknowledged, federation.BundleDirectionOutbound)

	if len(params.BundleIDs) > 0 {
		start := len(args) + 1
		holders := make([]string, 0, len(params.BundleIDs))
		for _, bundleID := range params.BundleIDs {
			bundleID = strings.TrimSpace(bundleID)
			if bundleID == "" {
				continue
			}
			holders = append(holders, fmt.Sprintf("$%d", start))
			args = append(args, bundleID)
			start++
		}
		if len(holders) > 0 {
			query += ` OR id IN (` + strings.Join(holders, ", ") + `)`
		}
	}
	query += `) RETURNING ` + bundleColumnList

	rows, err := s.conn().QueryContext(ctx, query, args...)
	if err != nil {
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return nil, mappedErr
		}
		return nil, fmt.Errorf("acknowledge federation bundles for %s/%s: %w", params.PeerID, params.LinkID, err)
	}
	defer rows.Close()

	bundles := make([]federation.Bundle, 0)
	for rows.Next() {
		bundle, scanErr := scanBundle(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan acknowledged federation bundle: %w", scanErr)
		}
		bundles = append(bundles, bundle)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate acknowledged federation bundles for %s/%s: %w", params.PeerID, params.LinkID, err)
	}

	return bundles, nil
}

func (s *Store) SaveCursor(
	ctx context.Context,
	cursor federation.ReplicationCursor,
) (federation.ReplicationCursor, error) {
	if err := s.requireStore(); err != nil {
		return federation.ReplicationCursor{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return federation.ReplicationCursor{}, err
	}

	cursor, err := federation.NormalizeReplicationCursor(cursor, time.Now().UTC())
	if err != nil {
		return federation.ReplicationCursor{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	peer_id, link_id, last_received_cursor, last_inbound_cursor, last_outbound_cursor, last_acked_cursor, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (peer_id, link_id) DO UPDATE SET
	last_received_cursor = EXCLUDED.last_received_cursor,
	last_inbound_cursor = EXCLUDED.last_inbound_cursor,
	last_outbound_cursor = EXCLUDED.last_outbound_cursor,
	last_acked_cursor = EXCLUDED.last_acked_cursor,
	updated_at = EXCLUDED.updated_at
RETURNING peer_id, link_id, last_received_cursor, last_inbound_cursor, last_outbound_cursor, last_acked_cursor, updated_at
`, s.table("federation_replication_cursors"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		cursor.PeerID,
		cursor.LinkID,
		cursor.LastReceivedCursor,
		cursor.LastInboundCursor,
		cursor.LastOutboundCursor,
		cursor.LastAckedCursor,
		cursor.UpdatedAt.UTC(),
	)

	saved, err := scanCursor(row)
	if err != nil {
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return federation.ReplicationCursor{}, mappedErr
		}
		return federation.ReplicationCursor{}, fmt.Errorf("save federation cursor for %s/%s: %w", cursor.PeerID, cursor.LinkID, err)
	}

	return saved, nil
}

func (s *Store) CursorByPeerAndLink(
	ctx context.Context,
	peerID string,
	linkID string,
) (federation.ReplicationCursor, error) {
	if err := s.requireStore(); err != nil {
		return federation.ReplicationCursor{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return federation.ReplicationCursor{}, err
	}
	peerID = strings.TrimSpace(peerID)
	linkID = strings.TrimSpace(linkID)
	if peerID == "" || linkID == "" {
		return federation.ReplicationCursor{}, federation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT peer_id, link_id, last_received_cursor, last_inbound_cursor, last_outbound_cursor, last_acked_cursor, updated_at
FROM %s
WHERE peer_id = $1 AND link_id = $2
`, s.table("federation_replication_cursors"))

	row := s.conn().QueryRowContext(ctx, query, peerID, linkID)
	cursor, err := scanCursor(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return federation.ReplicationCursor{}, federation.ErrNotFound
		}
		return federation.ReplicationCursor{}, fmt.Errorf("load federation cursor for %s/%s: %w", peerID, linkID, err)
	}

	return cursor, nil
}
