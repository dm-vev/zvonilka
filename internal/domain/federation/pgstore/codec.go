package pgstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/federation"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func nullTime(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: value.UTC(), Valid: true}
}

func scanTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}

	return value.Time.UTC()
}

func encodeCapabilities(values []federation.Capability) (string, error) {
	raw, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("encode federation capabilities: %w", err)
	}

	return string(raw), nil
}

func decodeCapabilities(raw string) ([]federation.Capability, error) {
	if raw == "" {
		return nil, nil
	}

	var values []federation.Capability
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("decode federation capabilities: %w", err)
	}

	return values, nil
}

func encodeConversationKinds(values []federation.ConversationKind) (string, error) {
	raw, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("encode federation conversation kinds: %w", err)
	}

	return string(raw), nil
}

func decodeConversationKinds(raw string) ([]federation.ConversationKind, error) {
	if raw == "" {
		return nil, nil
	}

	var values []federation.ConversationKind
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("decode federation conversation kinds: %w", err)
	}

	return values, nil
}

func scanPeer(row rowScanner) (federation.Peer, error) {
	var (
		peer       federation.Peer
		rawCaps    string
		lastSeenAt sql.NullTime
	)

	if err := row.Scan(
		&peer.ID,
		&peer.ServerName,
		&peer.BaseURL,
		&rawCaps,
		&peer.Trusted,
		&peer.State,
		&peer.VerificationFingerprint,
		&peer.SharedSecret,
		&peer.SharedSecretHash,
		&peer.SigningFingerprint,
		&peer.SigningSecret,
		&peer.PreviousSigningSecret,
		&peer.SigningKeyVersion,
		&peer.CreatedAt,
		&peer.UpdatedAt,
		&lastSeenAt,
	); err != nil {
		return federation.Peer{}, err
	}

	capabilities, err := decodeCapabilities(rawCaps)
	if err != nil {
		return federation.Peer{}, err
	}

	peer.Capabilities = capabilities
	peer.CreatedAt = peer.CreatedAt.UTC()
	peer.UpdatedAt = peer.UpdatedAt.UTC()
	peer.LastSeenAt = scanTime(lastSeenAt)

	return peer, nil
}

func scanLink(row rowScanner) (federation.Link, error) {
	var (
		link          federation.Link
		rawKinds      string
		lastHealthyAt sql.NullTime
	)

	if err := row.Scan(
		&link.ID,
		&link.PeerID,
		&link.Name,
		&link.Endpoint,
		&link.TransportKind,
		&link.DeliveryClass,
		&link.DiscoveryMode,
		&link.MediaPolicy,
		&link.State,
		&link.MaxBundleBytes,
		&link.MaxFragmentBytes,
		&rawKinds,
		&link.CreatedAt,
		&link.UpdatedAt,
		&lastHealthyAt,
		&link.LastError,
	); err != nil {
		return federation.Link{}, err
	}

	kinds, err := decodeConversationKinds(rawKinds)
	if err != nil {
		return federation.Link{}, err
	}

	link.AllowedConversationKinds = kinds
	link.CreatedAt = link.CreatedAt.UTC()
	link.UpdatedAt = link.UpdatedAt.UTC()
	link.LastHealthyAt = scanTime(lastHealthyAt)

	return link, nil
}

func scanBundle(row rowScanner) (federation.Bundle, error) {
	var (
		bundle    federation.Bundle
		expiresAt sql.NullTime
		ackedAt   sql.NullTime
	)

	if err := row.Scan(
		&bundle.ID,
		&bundle.PeerID,
		&bundle.LinkID,
		&bundle.DedupKey,
		&bundle.Direction,
		&bundle.CursorFrom,
		&bundle.CursorTo,
		&bundle.EventCount,
		&bundle.PayloadType,
		&bundle.Payload,
		&bundle.Compression,
		&bundle.IntegrityHash,
		&bundle.AuthTag,
		&bundle.State,
		&bundle.CreatedAt,
		&bundle.AvailableAt,
		&expiresAt,
		&ackedAt,
	); err != nil {
		return federation.Bundle{}, err
	}

	bundle.CreatedAt = bundle.CreatedAt.UTC()
	bundle.AvailableAt = bundle.AvailableAt.UTC()
	bundle.ExpiresAt = scanTime(expiresAt)
	bundle.AckedAt = scanTime(ackedAt)

	return bundle, nil
}

func scanFragment(row rowScanner) (federation.BundleFragment, error) {
	var (
		fragment       federation.BundleFragment
		leaseExpiresAt sql.NullTime
		ackedAt        sql.NullTime
	)

	if err := row.Scan(
		&fragment.ID,
		&fragment.PeerID,
		&fragment.LinkID,
		&fragment.BundleID,
		&fragment.DedupKey,
		&fragment.Direction,
		&fragment.CursorFrom,
		&fragment.CursorTo,
		&fragment.EventCount,
		&fragment.PayloadType,
		&fragment.Compression,
		&fragment.IntegrityHash,
		&fragment.AuthTag,
		&fragment.FragmentIndex,
		&fragment.FragmentCount,
		&fragment.Payload,
		&fragment.State,
		&fragment.LeaseToken,
		&leaseExpiresAt,
		&fragment.AttemptCount,
		&fragment.CreatedAt,
		&fragment.AvailableAt,
		&ackedAt,
	); err != nil {
		return federation.BundleFragment{}, err
	}

	fragment.CreatedAt = fragment.CreatedAt.UTC()
	fragment.AvailableAt = fragment.AvailableAt.UTC()
	fragment.LeaseExpiresAt = scanTime(leaseExpiresAt)
	fragment.AckedAt = scanTime(ackedAt)

	return fragment, nil
}

func scanCursor(row rowScanner) (federation.ReplicationCursor, error) {
	var cursor federation.ReplicationCursor

	if err := row.Scan(
		&cursor.PeerID,
		&cursor.LinkID,
		&cursor.LastReceivedCursor,
		&cursor.LastInboundCursor,
		&cursor.LastOutboundCursor,
		&cursor.LastAckedCursor,
		&cursor.UpdatedAt,
	); err != nil {
		return federation.ReplicationCursor{}, err
	}

	cursor.UpdatedAt = cursor.UpdatedAt.UTC()

	return cursor, nil
}
