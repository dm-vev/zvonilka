package teststore

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/federation"
)

// NewMemoryStore builds a concurrency-safe in-memory federation store for tests.
func NewMemoryStore() federation.Store {
	return &memoryStore{
		peersByID:          make(map[string]federation.Peer),
		peerIDsByServer:    make(map[string]string),
		linksByID:          make(map[string]federation.Link),
		linkIDsByPeerName:  make(map[string]string),
		bundlesByID:        make(map[string]federation.Bundle),
		bundleIDsByDedup:   make(map[string]string),
		fragmentsByID:      make(map[string]federation.BundleFragment),
		fragmentIDsByDedup: make(map[string]string),
		cursorsByKey:       make(map[string]federation.ReplicationCursor),
	}
}

type memoryStore struct {
	mu sync.RWMutex

	peersByID          map[string]federation.Peer
	peerIDsByServer    map[string]string
	linksByID          map[string]federation.Link
	linkIDsByPeerName  map[string]string
	bundlesByID        map[string]federation.Bundle
	bundleIDsByDedup   map[string]string
	fragmentsByID      map[string]federation.BundleFragment
	fragmentIDsByDedup map[string]string
	cursorsByKey       map[string]federation.ReplicationCursor
}

type txStore struct {
	*memoryStore
}

func (s *memoryStore) WithinTx(_ context.Context, fn func(federation.Store) error) error {
	if s == nil || fn == nil {
		return federation.ErrInvalidInput
	}

	s.mu.RLock()
	snapshot := s.cloneLocked()
	s.mu.RUnlock()

	tx := &txStore{memoryStore: snapshot}
	if err := fn(tx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.replaceLocked(snapshot)

	return nil
}

func (s *memoryStore) SavePeer(_ context.Context, peer federation.Peer) (federation.Peer, error) {
	peer, err := federation.NormalizePeer(peer, nowUTC())
	if err != nil {
		return federation.Peer{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, ok := s.peerIDsByServer[peer.ServerName]; ok && existingID != peer.ID {
		return federation.Peer{}, federation.ErrConflict
	}
	if previous, ok := s.peersByID[peer.ID]; ok {
		delete(s.peerIDsByServer, previous.ServerName)
	}

	s.peersByID[peer.ID] = clonePeer(peer)
	s.peerIDsByServer[peer.ServerName] = peer.ID

	return clonePeer(peer), nil
}

func (s *memoryStore) PeerByID(_ context.Context, peerID string) (federation.Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peer, ok := s.peersByID[peerID]
	if !ok {
		return federation.Peer{}, federation.ErrNotFound
	}

	return clonePeer(peer), nil
}

func (s *memoryStore) PeerByServerName(_ context.Context, serverName string) (federation.Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peerID, ok := s.peerIDsByServer[serverName]
	if !ok {
		return federation.Peer{}, federation.ErrNotFound
	}
	peer, ok := s.peersByID[peerID]
	if !ok {
		return federation.Peer{}, federation.ErrNotFound
	}

	return clonePeer(peer), nil
}

func (s *memoryStore) PeersByState(_ context.Context, state federation.PeerState) ([]federation.Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peers := make([]federation.Peer, 0, len(s.peersByID))
	for _, peer := range s.peersByID {
		if state != federation.PeerStateUnspecified && peer.State != state {
			continue
		}
		peers = append(peers, clonePeer(peer))
	}

	sort.Slice(peers, func(i, j int) bool {
		if peers[i].CreatedAt.Equal(peers[j].CreatedAt) {
			return peers[i].ID < peers[j].ID
		}
		return peers[i].CreatedAt.Before(peers[j].CreatedAt)
	})

	return peers, nil
}

func (s *memoryStore) SaveLink(_ context.Context, link federation.Link) (federation.Link, error) {
	link, err := federation.NormalizeLink(link, nowUTC())
	if err != nil {
		return federation.Link{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.peersByID[link.PeerID]; !ok {
		return federation.Link{}, federation.ErrNotFound
	}

	indexKey := peerNameKey(link.PeerID, link.Name)
	if existingID, ok := s.linkIDsByPeerName[indexKey]; ok && existingID != link.ID {
		return federation.Link{}, federation.ErrConflict
	}
	if previous, ok := s.linksByID[link.ID]; ok {
		delete(s.linkIDsByPeerName, peerNameKey(previous.PeerID, previous.Name))
	}

	s.linksByID[link.ID] = cloneLink(link)
	s.linkIDsByPeerName[indexKey] = link.ID

	return cloneLink(link), nil
}

func (s *memoryStore) LinkByID(_ context.Context, linkID string) (federation.Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, ok := s.linksByID[linkID]
	if !ok {
		return federation.Link{}, federation.ErrNotFound
	}

	return cloneLink(link), nil
}

func (s *memoryStore) LinkByPeerAndName(_ context.Context, peerID string, name string) (federation.Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	linkID, ok := s.linkIDsByPeerName[peerNameKey(peerID, name)]
	if !ok {
		return federation.Link{}, federation.ErrNotFound
	}
	link, ok := s.linksByID[linkID]
	if !ok {
		return federation.Link{}, federation.ErrNotFound
	}

	return cloneLink(link), nil
}

func (s *memoryStore) Links(_ context.Context, peerID string, state federation.LinkState) ([]federation.Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]federation.Link, 0, len(s.linksByID))
	for _, link := range s.linksByID {
		if peerID != "" && link.PeerID != peerID {
			continue
		}
		if state != federation.LinkStateUnspecified && link.State != state {
			continue
		}
		links = append(links, cloneLink(link))
	}

	sort.Slice(links, func(i, j int) bool {
		if links[i].CreatedAt.Equal(links[j].CreatedAt) {
			return links[i].ID < links[j].ID
		}
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	return links, nil
}

func (s *memoryStore) SaveBundle(_ context.Context, bundle federation.Bundle) (federation.Bundle, error) {
	bundle, err := federation.NormalizeBundle(bundle, nowUTC())
	if err != nil {
		return federation.Bundle{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	link, ok := s.linksByID[bundle.LinkID]
	if !ok {
		return federation.Bundle{}, federation.ErrNotFound
	}
	if link.PeerID != bundle.PeerID {
		return federation.Bundle{}, federation.ErrConflict
	}

	if existingID, ok := s.bundleIDsByDedup[bundle.DedupKey]; ok {
		existing, ok := s.bundlesByID[existingID]
		if !ok {
			delete(s.bundleIDsByDedup, bundle.DedupKey)
		} else {
			if existing.PeerID != bundle.PeerID ||
				existing.LinkID != bundle.LinkID ||
				existing.Direction != bundle.Direction {
				return federation.Bundle{}, federation.ErrConflict
			}
			if existing.CursorFrom != bundle.CursorFrom ||
				existing.CursorTo != bundle.CursorTo ||
				existing.EventCount != bundle.EventCount ||
				existing.PayloadType != bundle.PayloadType ||
				existing.Compression != bundle.Compression ||
				existing.IntegrityHash != bundle.IntegrityHash ||
				existing.AuthTag != bundle.AuthTag ||
				!bytes.Equal(existing.Payload, bundle.Payload) {
				return federation.Bundle{}, federation.ErrConflict
			}
			return cloneBundle(existing), nil
		}
	}

	s.bundlesByID[bundle.ID] = cloneBundle(bundle)
	s.bundleIDsByDedup[bundle.DedupKey] = bundle.ID

	return cloneBundle(bundle), nil
}

func (s *memoryStore) BundleByDedupKey(_ context.Context, dedupKey string) (federation.Bundle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bundleID, ok := s.bundleIDsByDedup[dedupKey]
	if !ok {
		return federation.Bundle{}, federation.ErrNotFound
	}
	bundle, ok := s.bundlesByID[bundleID]
	if !ok {
		return federation.Bundle{}, federation.ErrNotFound
	}

	return cloneBundle(bundle), nil
}

func (s *memoryStore) BundlesAfter(
	_ context.Context,
	peerID string,
	linkID string,
	direction federation.BundleDirection,
	afterCursor uint64,
	limit int,
) ([]federation.Bundle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		return nil, nil
	}

	bundles := make([]federation.Bundle, 0)
	for _, bundle := range s.bundlesByID {
		if bundle.PeerID != peerID || bundle.LinkID != linkID || bundle.Direction != direction {
			continue
		}
		if bundle.CursorTo <= afterCursor {
			continue
		}
		bundles = append(bundles, cloneBundle(bundle))
	}

	sort.Slice(bundles, func(i, j int) bool {
		if bundles[i].CursorTo == bundles[j].CursorTo {
			if bundles[i].CreatedAt.Equal(bundles[j].CreatedAt) {
				return bundles[i].ID < bundles[j].ID
			}
			return bundles[i].CreatedAt.Before(bundles[j].CreatedAt)
		}
		return bundles[i].CursorTo < bundles[j].CursorTo
	})

	if len(bundles) > limit {
		bundles = bundles[:limit]
	}

	return bundles, nil
}

func (s *memoryStore) AcknowledgeBundles(
	_ context.Context,
	params federation.AcknowledgeBundlesParams,
) ([]federation.Bundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	updated := make([]federation.Bundle, 0)
	selected := make(map[string]struct{}, len(params.BundleIDs))
	for _, bundleID := range params.BundleIDs {
		if bundleID == "" {
			continue
		}
		selected[bundleID] = struct{}{}
	}

	for bundleID, bundle := range s.bundlesByID {
		if bundle.PeerID != params.PeerID || bundle.LinkID != params.LinkID {
			continue
		}
		if bundle.Direction != federation.BundleDirectionOutbound {
			continue
		}
		if _, ok := selected[bundleID]; !ok && bundle.CursorTo > params.UpToCursor {
			continue
		}

		bundle.State = federation.BundleStateAcknowledged
		bundle.AckedAt = params.AcknowledgedAt.UTC()
		s.bundlesByID[bundleID] = cloneBundle(bundle)
		updated = append(updated, cloneBundle(bundle))
	}

	return updated, nil
}

func (s *memoryStore) SaveFragment(
	_ context.Context,
	fragment federation.BundleFragment,
) (federation.BundleFragment, error) {
	fragment, err := federation.NormalizeBundleFragment(fragment, nowUTC())
	if err != nil {
		return federation.BundleFragment{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	link, ok := s.linksByID[fragment.LinkID]
	if !ok {
		return federation.BundleFragment{}, federation.ErrNotFound
	}
	if link.PeerID != fragment.PeerID {
		return federation.BundleFragment{}, federation.ErrConflict
	}

	if existingID, ok := s.fragmentIDsByDedup[fragment.DedupKey]; ok {
		existing, ok := s.fragmentsByID[existingID]
		if !ok {
			delete(s.fragmentIDsByDedup, fragment.DedupKey)
		} else {
			if existing.PeerID != fragment.PeerID ||
				existing.LinkID != fragment.LinkID ||
				existing.BundleID != fragment.BundleID ||
				existing.Direction != fragment.Direction {
				return federation.BundleFragment{}, federation.ErrConflict
			}
			if existing.CursorFrom != fragment.CursorFrom ||
				existing.CursorTo != fragment.CursorTo ||
				existing.EventCount != fragment.EventCount ||
				existing.PayloadType != fragment.PayloadType ||
				existing.Compression != fragment.Compression ||
				existing.IntegrityHash != fragment.IntegrityHash ||
				existing.AuthTag != fragment.AuthTag ||
				existing.FragmentIndex != fragment.FragmentIndex ||
				existing.FragmentCount != fragment.FragmentCount ||
				!bytes.Equal(existing.Payload, fragment.Payload) {
				return federation.BundleFragment{}, federation.ErrConflict
			}
			if existing.Direction == federation.BundleDirectionInbound &&
				(existing.State == federation.FragmentStateAssembled || existing.State == federation.FragmentStateFailed) {
				return cloneFragment(existing), nil
			}

			if existing.State != fragment.State ||
				!existing.AckedAt.Equal(fragment.AckedAt) ||
				existing.LeaseToken != fragment.LeaseToken ||
				!existing.LeaseExpiresAt.Equal(fragment.LeaseExpiresAt) ||
				existing.AttemptCount != fragment.AttemptCount {
				existing.State = fragment.State
				existing.LeaseToken = fragment.LeaseToken
				existing.LeaseExpiresAt = fragment.LeaseExpiresAt
				existing.AttemptCount = fragment.AttemptCount
				existing.AvailableAt = fragment.AvailableAt
				existing.AckedAt = fragment.AckedAt
				s.fragmentsByID[existingID] = cloneFragment(existing)
				s.fragmentIDsByDedup[fragment.DedupKey] = existingID
				return cloneFragment(existing), nil
			}

			return cloneFragment(existing), nil
		}
	}

	s.fragmentsByID[fragment.ID] = cloneFragment(fragment)
	s.fragmentIDsByDedup[fragment.DedupKey] = fragment.ID

	return cloneFragment(fragment), nil
}

func (s *memoryStore) ClaimFragments(
	_ context.Context,
	params federation.ClaimFragmentsParams,
) ([]federation.BundleFragment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if params.Limit <= 0 {
		return nil, nil
	}
	if params.PeerID == "" || params.LinkID == "" || params.LeaseToken == "" || params.LeaseExpiresAt.IsZero() {
		return nil, federation.ErrInvalidInput
	}
	if params.ClaimedAt.IsZero() {
		params.ClaimedAt = nowUTC()
	}

	fragments := make([]federation.BundleFragment, 0, len(s.fragmentsByID))
	for _, fragment := range s.fragmentsByID {
		if fragment.PeerID != params.PeerID || fragment.LinkID != params.LinkID {
			continue
		}
		if fragment.Direction != federation.BundleDirectionOutbound {
			continue
		}
		switch fragment.State {
		case federation.FragmentStateQueued:
			if fragment.AvailableAt.After(params.ClaimedAt) {
				continue
			}
		case federation.FragmentStateClaimed:
			if fragment.LeaseExpiresAt.IsZero() || fragment.LeaseExpiresAt.After(params.ClaimedAt) {
				continue
			}
		default:
			continue
		}

		fragments = append(fragments, cloneFragment(fragment))
	}

	sort.Slice(fragments, func(i, j int) bool {
		if fragments[i].AvailableAt.Equal(fragments[j].AvailableAt) {
			if fragments[i].CursorTo == fragments[j].CursorTo {
				if fragments[i].BundleID == fragments[j].BundleID {
					return fragments[i].FragmentIndex < fragments[j].FragmentIndex
				}
				return fragments[i].BundleID < fragments[j].BundleID
			}
			return fragments[i].CursorTo < fragments[j].CursorTo
		}
		return fragments[i].AvailableAt.Before(fragments[j].AvailableAt)
	})
	if len(fragments) > params.Limit {
		fragments = fragments[:params.Limit]
	}

	for idx, fragment := range fragments {
		fragment.State = federation.FragmentStateClaimed
		fragment.LeaseToken = params.LeaseToken
		fragment.LeaseExpiresAt = params.LeaseExpiresAt.UTC()
		fragment.AttemptCount++
		s.fragmentsByID[fragment.ID] = cloneFragment(fragment)
		fragments[idx] = cloneFragment(fragment)
	}

	return fragments, nil
}

func (s *memoryStore) HasClaimableFragments(
	_ context.Context,
	peerID string,
	linkID string,
	claimedAt time.Time,
) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if claimedAt.IsZero() {
		claimedAt = nowUTC()
	}
	for _, fragment := range s.fragmentsByID {
		if fragment.PeerID != peerID || fragment.LinkID != linkID {
			continue
		}
		if fragment.Direction != federation.BundleDirectionOutbound {
			continue
		}
		switch fragment.State {
		case federation.FragmentStateQueued:
			if !fragment.AvailableAt.After(claimedAt) {
				return true, nil
			}
		case federation.FragmentStateClaimed:
			if !fragment.LeaseExpiresAt.IsZero() && !fragment.LeaseExpiresAt.After(claimedAt) {
				return true, nil
			}
		}
	}

	return false, nil
}

func (s *memoryStore) Fragments(
	_ context.Context,
	peerID string,
	linkID string,
	direction federation.BundleDirection,
	state federation.FragmentState,
	limit int,
) ([]federation.BundleFragment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		return nil, nil
	}

	fragments := make([]federation.BundleFragment, 0, len(s.fragmentsByID))
	for _, fragment := range s.fragmentsByID {
		if fragment.PeerID != peerID || fragment.LinkID != linkID || fragment.Direction != direction {
			continue
		}
		if state != federation.FragmentStateUnspecified && fragment.State != state {
			continue
		}
		fragments = append(fragments, cloneFragment(fragment))
	}

	sort.Slice(fragments, func(i, j int) bool {
		if fragments[i].AvailableAt.Equal(fragments[j].AvailableAt) {
			if fragments[i].CursorTo == fragments[j].CursorTo {
				if fragments[i].BundleID == fragments[j].BundleID {
					return fragments[i].FragmentIndex < fragments[j].FragmentIndex
				}
				return fragments[i].BundleID < fragments[j].BundleID
			}
			return fragments[i].CursorTo < fragments[j].CursorTo
		}
		return fragments[i].AvailableAt.Before(fragments[j].AvailableAt)
	})

	if len(fragments) > limit {
		fragments = fragments[:limit]
	}

	return fragments, nil
}

func (s *memoryStore) FragmentsByBundle(
	_ context.Context,
	bundleID string,
	direction federation.BundleDirection,
) ([]federation.BundleFragment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fragments := make([]federation.BundleFragment, 0)
	for _, fragment := range s.fragmentsByID {
		if fragment.BundleID != bundleID || fragment.Direction != direction {
			continue
		}
		fragments = append(fragments, cloneFragment(fragment))
	}

	sort.Slice(fragments, func(i, j int) bool {
		if fragments[i].FragmentIndex == fragments[j].FragmentIndex {
			return fragments[i].ID < fragments[j].ID
		}
		return fragments[i].FragmentIndex < fragments[j].FragmentIndex
	})

	return fragments, nil
}

func (s *memoryStore) AcknowledgeFragments(
	_ context.Context,
	params federation.AcknowledgeFragmentsParams,
) ([]federation.BundleFragment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	updated := make([]federation.BundleFragment, 0)
	selected := make(map[string]struct{}, len(params.FragmentIDs))
	for _, fragmentID := range params.FragmentIDs {
		if fragmentID == "" {
			continue
		}
		selected[fragmentID] = struct{}{}
	}

	for fragmentID, fragment := range s.fragmentsByID {
		if fragment.PeerID != params.PeerID || fragment.LinkID != params.LinkID {
			continue
		}
		if fragment.Direction != federation.BundleDirectionOutbound {
			continue
		}
		if fragment.State != federation.FragmentStateClaimed || fragment.LeaseToken != params.LeaseToken {
			continue
		}
		if _, ok := selected[fragmentID]; !ok {
			continue
		}

		fragment.State = federation.FragmentStateAcknowledged
		fragment.LeaseToken = ""
		fragment.LeaseExpiresAt = time.Time{}
		fragment.AckedAt = params.AcknowledgedAt.UTC()
		s.fragmentsByID[fragmentID] = cloneFragment(fragment)
		updated = append(updated, cloneFragment(fragment))
	}

	sort.Slice(updated, func(i, j int) bool {
		if updated[i].BundleID == updated[j].BundleID {
			return updated[i].FragmentIndex < updated[j].FragmentIndex
		}
		return updated[i].BundleID < updated[j].BundleID
	})

	return updated, nil
}

func (s *memoryStore) SaveCursor(
	_ context.Context,
	cursor federation.ReplicationCursor,
) (federation.ReplicationCursor, error) {
	cursor, err := federation.NormalizeReplicationCursor(cursor, nowUTC())
	if err != nil {
		return federation.ReplicationCursor{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cursorsByKey[cursorKey(cursor.PeerID, cursor.LinkID)] = cloneCursor(cursor)
	return cloneCursor(cursor), nil
}

func (s *memoryStore) CursorByPeerAndLink(
	_ context.Context,
	peerID string,
	linkID string,
) (federation.ReplicationCursor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cursor, ok := s.cursorsByKey[cursorKey(peerID, linkID)]
	if !ok {
		return federation.ReplicationCursor{}, federation.ErrNotFound
	}

	return cloneCursor(cursor), nil
}

func (s *memoryStore) cloneLocked() *memoryStore {
	clone := &memoryStore{
		peersByID:          make(map[string]federation.Peer, len(s.peersByID)),
		peerIDsByServer:    make(map[string]string, len(s.peerIDsByServer)),
		linksByID:          make(map[string]federation.Link, len(s.linksByID)),
		linkIDsByPeerName:  make(map[string]string, len(s.linkIDsByPeerName)),
		bundlesByID:        make(map[string]federation.Bundle, len(s.bundlesByID)),
		bundleIDsByDedup:   make(map[string]string, len(s.bundleIDsByDedup)),
		fragmentsByID:      make(map[string]federation.BundleFragment, len(s.fragmentsByID)),
		fragmentIDsByDedup: make(map[string]string, len(s.fragmentIDsByDedup)),
		cursorsByKey:       make(map[string]federation.ReplicationCursor, len(s.cursorsByKey)),
	}

	for key, value := range s.peersByID {
		clone.peersByID[key] = clonePeer(value)
	}
	for key, value := range s.peerIDsByServer {
		clone.peerIDsByServer[key] = value
	}
	for key, value := range s.linksByID {
		clone.linksByID[key] = cloneLink(value)
	}
	for key, value := range s.linkIDsByPeerName {
		clone.linkIDsByPeerName[key] = value
	}
	for key, value := range s.bundlesByID {
		clone.bundlesByID[key] = cloneBundle(value)
	}
	for key, value := range s.bundleIDsByDedup {
		clone.bundleIDsByDedup[key] = value
	}
	for key, value := range s.fragmentsByID {
		clone.fragmentsByID[key] = cloneFragment(value)
	}
	for key, value := range s.fragmentIDsByDedup {
		clone.fragmentIDsByDedup[key] = value
	}
	for key, value := range s.cursorsByKey {
		clone.cursorsByKey[key] = cloneCursor(value)
	}

	return clone
}

func (s *memoryStore) replaceLocked(snapshot *memoryStore) {
	s.peersByID = snapshot.peersByID
	s.peerIDsByServer = snapshot.peerIDsByServer
	s.linksByID = snapshot.linksByID
	s.linkIDsByPeerName = snapshot.linkIDsByPeerName
	s.bundlesByID = snapshot.bundlesByID
	s.bundleIDsByDedup = snapshot.bundleIDsByDedup
	s.fragmentsByID = snapshot.fragmentsByID
	s.fragmentIDsByDedup = snapshot.fragmentIDsByDedup
	s.cursorsByKey = snapshot.cursorsByKey
}

func peerNameKey(peerID string, name string) string {
	return peerID + ":" + name
}

func cursorKey(peerID string, linkID string) string {
	return peerID + ":" + linkID
}

func nowUTC() (now time.Time) {
	return time.Now().UTC()
}

func clonePeer(peer federation.Peer) federation.Peer {
	peer.Capabilities = append([]federation.Capability(nil), peer.Capabilities...)
	return peer
}

func cloneLink(link federation.Link) federation.Link {
	link.AllowedConversationKinds = append([]federation.ConversationKind(nil), link.AllowedConversationKinds...)
	link.AllowedEventFamilies = append([]federation.EventFamily(nil), link.AllowedEventFamilies...)
	link.AllowedMessageKinds = append([]federation.MessageKind(nil), link.AllowedMessageKinds...)
	return link
}

func cloneBundle(bundle federation.Bundle) federation.Bundle {
	bundle.Payload = append([]byte(nil), bundle.Payload...)
	return bundle
}

func cloneFragment(fragment federation.BundleFragment) federation.BundleFragment {
	fragment.Payload = append([]byte(nil), fragment.Payload...)
	return fragment
}

func cloneCursor(cursor federation.ReplicationCursor) federation.ReplicationCursor {
	return cursor
}

var _ federation.Store = (*memoryStore)(nil)
