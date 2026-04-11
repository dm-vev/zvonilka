package federation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const defaultBridgeFragmentLimit = 256

// PullBridgeFragments returns queued outbound fragments for one constrained link.
func (s *Service) PullBridgeFragments(
	ctx context.Context,
	peerServerName string,
	linkName string,
	limit int,
) ([]BundleFragment, Link, ReplicationCursor, bool, string, error) {
	if err := s.validateContext(ctx, "pull federation bridge fragments"); err != nil {
		return nil, Link{}, ReplicationCursor{}, false, "", err
	}
	if limit <= 0 {
		limit = defaultBridgeFragmentLimit
	}

	peer, link, err := s.bridgeLinkByServerAndName(ctx, peerServerName, linkName)
	if err != nil {
		return nil, Link{}, ReplicationCursor{}, false, "", err
	}

	fragments, leaseToken, err := s.claimBridgeFragments(ctx, peer, link, limit)
	if err != nil {
		return nil, Link{}, ReplicationCursor{}, false, "", err
	}
	if len(fragments) == 0 {
		cursor, cursorErr := s.ReplicationCursorByPeerAndLink(ctx, peer.ID, link.ID)
		if cursorErr != nil {
			return nil, Link{}, ReplicationCursor{}, false, "", cursorErr
		}
		bundles, bundleErr := s.store.BundlesAfter(
			ctx,
			peer.ID,
			link.ID,
			BundleDirectionOutbound,
			cursor.LastAckedCursor,
			limit,
		)
		if bundleErr != nil {
			return nil, Link{}, ReplicationCursor{}, false, "", fmt.Errorf(
				"list outbound federation bundles for bridge %s/%s: %w",
				peer.ServerName,
				link.Name,
				bundleErr,
			)
		}
		for _, bundle := range bundles {
			if _, ensureErr := s.ensureOutboundFragments(ctx, link, bundle); ensureErr != nil {
				return nil, Link{}, ReplicationCursor{}, false, "", ensureErr
			}
		}
		fragments, leaseToken, err = s.claimBridgeFragments(ctx, peer, link, limit)
		if err != nil {
			return nil, Link{}, ReplicationCursor{}, false, "", err
		}
	}

	hasMore := false
	if len(fragments) > 0 {
		hasMore, err = s.store.HasClaimableFragments(ctx, peer.ID, link.ID, s.currentTime())
		if err != nil {
			return nil, Link{}, ReplicationCursor{}, false, "", fmt.Errorf(
				"check remaining federation bridge fragments for %s/%s: %w",
				peer.ServerName,
				link.Name,
				err,
			)
		}
	}
	cursor, err := s.ReplicationCursorByPeerAndLink(ctx, peer.ID, link.ID)
	if err != nil {
		return nil, Link{}, ReplicationCursor{}, false, "", err
	}

	return fragments, link, cursor, hasMore, leaseToken, nil
}

// SubmitBridgeFragments stores inbound fragments and assembles complete bundles.
func (s *Service) SubmitBridgeFragments(
	ctx context.Context,
	peerServerName string,
	linkName string,
	fragments []SaveFragmentParams,
) ([]BundleFragment, []Bundle, ReplicationCursor, error) {
	if err := s.validateContext(ctx, "submit federation bridge fragments"); err != nil {
		return nil, nil, ReplicationCursor{}, err
	}

	peer, link, err := s.bridgeLinkByServerAndName(ctx, peerServerName, linkName)
	if err != nil {
		return nil, nil, ReplicationCursor{}, err
	}

	accepted := make([]BundleFragment, 0, len(fragments))
	bundleIDs := make([]string, 0, len(fragments))
	seenBundles := make(map[string]struct{}, len(fragments))
	for _, params := range fragments {
		params.PeerID = peer.ID
		params.LinkID = link.ID
		params.Direction = BundleDirectionInbound
		fragment, saveErr := s.saveFragment(ctx, params)
		if saveErr != nil {
			return nil, nil, ReplicationCursor{}, saveErr
		}
		accepted = append(accepted, fragment)
		if _, ok := seenBundles[fragment.BundleID]; ok {
			continue
		}
		seenBundles[fragment.BundleID] = struct{}{}
		bundleIDs = append(bundleIDs, fragment.BundleID)
	}

	assembled := make([]Bundle, 0, len(bundleIDs))
	for _, bundleID := range bundleIDs {
		bundle, complete, assembleErr := s.assembleInboundBundleIfComplete(ctx, peer.ID, link.ID, bundleID)
		if assembleErr != nil {
			return nil, nil, ReplicationCursor{}, assembleErr
		}
		if !complete {
			continue
		}
		assembled = append(assembled, bundle)
	}

	cursor, err := s.ReplicationCursorByPeerAndLink(ctx, peer.ID, link.ID)
	if err != nil {
		return nil, nil, ReplicationCursor{}, err
	}

	return accepted, assembled, cursor, nil
}

// AcknowledgeBridgeFragments marks outbound fragments as delivered to the transport adapter.
func (s *Service) AcknowledgeBridgeFragments(
	ctx context.Context,
	peerServerName string,
	linkName string,
	fragmentIDs []string,
	leaseToken string,
	acknowledgedAt time.Time,
) ([]BundleFragment, ReplicationCursor, error) {
	if err := s.validateContext(ctx, "acknowledge federation bridge fragments"); err != nil {
		return nil, ReplicationCursor{}, err
	}

	peer, link, err := s.bridgeLinkByServerAndName(ctx, peerServerName, linkName)
	if err != nil {
		return nil, ReplicationCursor{}, err
	}
	if acknowledgedAt.IsZero() {
		acknowledgedAt = s.currentTime()
	}
	leaseToken = strings.TrimSpace(leaseToken)
	if len(fragmentIDs) > 0 && leaseToken == "" {
		return nil, ReplicationCursor{}, ErrInvalidInput
	}

	updated, err := s.store.AcknowledgeFragments(ctx, AcknowledgeFragmentsParams{
		PeerID:         peer.ID,
		LinkID:         link.ID,
		FragmentIDs:    append([]string(nil), fragmentIDs...),
		LeaseToken:     leaseToken,
		AcknowledgedAt: acknowledgedAt,
	})
	if err != nil {
		return nil, ReplicationCursor{}, fmt.Errorf(
			"acknowledge federation fragments for %s/%s: %w",
			peer.ServerName,
			link.Name,
			err,
		)
	}

	cursor, err := s.ReplicationCursorByPeerAndLink(ctx, peer.ID, link.ID)
	if err != nil {
		return nil, ReplicationCursor{}, err
	}

	readyBundleIDs := make([]string, 0)
	seenBundles := make(map[string]struct{}, len(updated))
	var upToCursor uint64
	for _, fragment := range updated {
		if _, ok := seenBundles[fragment.BundleID]; ok {
			continue
		}
		seenBundles[fragment.BundleID] = struct{}{}

		fragmentsForBundle, loadErr := s.store.FragmentsByBundle(ctx, fragment.BundleID, BundleDirectionOutbound)
		if loadErr != nil {
			return nil, ReplicationCursor{}, fmt.Errorf("load fragments for bundle %s: %w", fragment.BundleID, loadErr)
		}
		if !allFragmentsAcknowledged(fragmentsForBundle) {
			continue
		}

		readyBundleIDs = append(readyBundleIDs, fragment.BundleID)
		if fragment.CursorTo > upToCursor {
			upToCursor = fragment.CursorTo
		}
	}
	if len(readyBundleIDs) > 0 {
		cursor, err = s.AcknowledgeBundles(ctx, AcknowledgeBundlesParams{
			PeerID:         peer.ID,
			LinkID:         link.ID,
			UpToCursor:     upToCursor,
			BundleIDs:      readyBundleIDs,
			AcknowledgedAt: acknowledgedAt,
		})
		if err != nil {
			return nil, ReplicationCursor{}, err
		}
	}

	return updated, cursor, nil
}

func (s *Service) claimBridgeFragments(
	ctx context.Context,
	peer Peer,
	link Link,
	limit int,
) ([]BundleFragment, string, error) {
	now := s.currentTime()
	leaseToken, err := randomToken(18)
	if err != nil {
		return nil, "", fmt.Errorf("generate federation bridge lease token: %w", err)
	}

	fragments, err := s.store.ClaimFragments(ctx, ClaimFragmentsParams{
		PeerID:         peer.ID,
		LinkID:         link.ID,
		Limit:          limit,
		ClaimedAt:      now,
		LeaseToken:     leaseToken,
		LeaseExpiresAt: now.Add(s.bridgeFragmentLeaseTTL),
	})
	if err != nil {
		return nil, "", fmt.Errorf(
			"claim federation bridge fragments for %s/%s: %w",
			peer.ServerName,
			link.Name,
			err,
		)
	}
	if len(fragments) == 0 {
		return nil, "", nil
	}

	return fragments, leaseToken, nil
}

func (s *Service) saveFragment(ctx context.Context, params SaveFragmentParams) (BundleFragment, error) {
	if err := s.validateContext(ctx, "save federation fragment"); err != nil {
		return BundleFragment{}, err
	}

	params.PeerID = strings.TrimSpace(params.PeerID)
	params.LinkID = strings.TrimSpace(params.LinkID)
	params.BundleID = strings.TrimSpace(params.BundleID)
	params.DedupKey = strings.TrimSpace(params.DedupKey)
	if params.PeerID == "" || params.LinkID == "" || params.BundleID == "" || params.DedupKey == "" {
		return BundleFragment{}, ErrInvalidInput
	}

	link, err := s.store.LinkByID(ctx, params.LinkID)
	if err != nil {
		return BundleFragment{}, fmt.Errorf("load federation link %s before fragment save: %w", params.LinkID, err)
	}
	if link.PeerID != params.PeerID {
		return BundleFragment{}, ErrConflict
	}
	if link.MaxFragmentBytes > 0 && len(params.Payload) > link.MaxFragmentBytes {
		return BundleFragment{}, ErrInvalidInput
	}

	fragmentID, err := newID("frag")
	if err != nil {
		return BundleFragment{}, fmt.Errorf("generate federation fragment id: %w", err)
	}
	now := s.currentTime()
	fragment, err := NormalizeBundleFragment(BundleFragment{
		ID:            fragmentID,
		PeerID:        params.PeerID,
		LinkID:        params.LinkID,
		BundleID:      params.BundleID,
		DedupKey:      params.DedupKey,
		Direction:     params.Direction,
		CursorFrom:    params.CursorFrom,
		CursorTo:      params.CursorTo,
		EventCount:    params.EventCount,
		PayloadType:   params.PayloadType,
		Compression:   params.Compression,
		IntegrityHash: params.IntegrityHash,
		AuthTag:       params.AuthTag,
		FragmentIndex: params.FragmentIndex,
		FragmentCount: params.FragmentCount,
		Payload:       append([]byte(nil), params.Payload...),
		AvailableAt:   params.AvailableAt,
	}, now)
	if err != nil {
		return BundleFragment{}, err
	}
	if fragment.Direction == BundleDirectionInbound {
		existing, loadErr := s.store.FragmentsByBundle(ctx, fragment.BundleID, BundleDirectionInbound)
		if loadErr != nil {
			return BundleFragment{}, fmt.Errorf("load inbound fragments for bundle %s before save: %w", fragment.BundleID, loadErr)
		}
		if containsFragmentDedupKey(existing, fragment.DedupKey) {
			saved, saveErr := s.store.SaveFragment(ctx, fragment)
			if saveErr != nil {
				return BundleFragment{}, fmt.Errorf("save federation fragment %s: %w", fragment.BundleID, saveErr)
			}

			return saved, nil
		}
		if isInboundFragmentBundleTerminal(existing) && !containsFragmentDedupKey(existing, fragment.DedupKey) {
			return BundleFragment{}, ErrConflict
		}
		cursor, cursorErr := s.ReplicationCursorByPeerAndLink(ctx, fragment.PeerID, fragment.LinkID)
		if cursorErr != nil {
			return BundleFragment{}, fmt.Errorf(
				"load federation cursor for inbound fragment %s/%s: %w",
				fragment.PeerID,
				fragment.LinkID,
				cursorErr,
			)
		}
		if fragment.CursorTo <= cursor.LastReceivedCursor {
			return BundleFragment{}, ErrConflict
		}
		if validateErr := validateInboundFragmentCandidate(existing, fragment); validateErr != nil {
			if quarantineErr := s.quarantineInboundFragments(ctx, existing, validateErr); quarantineErr != nil {
				return BundleFragment{}, quarantineErr
			}
			return BundleFragment{}, validateErr
		}
	}

	saved, err := s.store.SaveFragment(ctx, fragment)
	if err != nil {
		return BundleFragment{}, fmt.Errorf("save federation fragment %s: %w", fragment.BundleID, err)
	}

	return saved, nil
}

func validateInboundFragmentCandidate(existing []BundleFragment, candidate BundleFragment) error {
	if len(existing) == 0 {
		return nil
	}

	first := existing[0]
	if candidate.FragmentCount != first.FragmentCount {
		return ErrConflict
	}
	if candidate.PeerID != first.PeerID ||
		candidate.LinkID != first.LinkID ||
		candidate.BundleID != first.BundleID ||
		candidate.Direction != first.Direction ||
		candidate.CursorFrom != first.CursorFrom ||
		candidate.CursorTo != first.CursorTo ||
		candidate.EventCount != first.EventCount ||
		candidate.PayloadType != first.PayloadType ||
		candidate.Compression != first.Compression {
		return ErrConflict
	}
	if candidate.IntegrityHash != first.IntegrityHash || candidate.AuthTag != first.AuthTag {
		return ErrUnauthorized
	}
	for _, fragment := range existing {
		if fragment.FragmentIndex == candidate.FragmentIndex {
			return ErrConflict
		}
	}

	return nil
}

func (s *Service) ensureOutboundFragments(ctx context.Context, link Link, bundle Bundle) ([]BundleFragment, error) {
	existing, err := s.store.FragmentsByBundle(ctx, bundle.ID, BundleDirectionOutbound)
	if err != nil {
		return nil, fmt.Errorf("load federation fragments for bundle %s: %w", bundle.ID, err)
	}
	if len(existing) > 0 {
		return existing, nil
	}

	maxBytes := link.MaxFragmentBytes
	if maxBytes <= 0 {
		maxBytes = link.MaxBundleBytes
	}
	if maxBytes <= 0 {
		maxBytes = 256
	}

	parts := splitPayload(bundle.Payload, maxBytes)
	fragments := make([]BundleFragment, 0, len(parts))
	for idx, payload := range parts {
		fragment, saveErr := s.saveFragment(ctx, SaveFragmentParams{
			PeerID:        bundle.PeerID,
			LinkID:        bundle.LinkID,
			BundleID:      bundle.ID,
			DedupKey:      fragmentDedupKey(bundle.DedupKey, idx),
			Direction:     BundleDirectionOutbound,
			CursorFrom:    bundle.CursorFrom,
			CursorTo:      bundle.CursorTo,
			EventCount:    bundle.EventCount,
			PayloadType:   bundle.PayloadType,
			Compression:   bundle.Compression,
			IntegrityHash: bundle.IntegrityHash,
			AuthTag:       bundle.AuthTag,
			FragmentIndex: idx,
			FragmentCount: len(parts),
			Payload:       payload,
			AvailableAt:   bundle.AvailableAt,
		})
		if saveErr != nil {
			return nil, saveErr
		}
		fragments = append(fragments, fragment)
	}

	return fragments, nil
}

func (s *Service) assembleInboundBundleIfComplete(
	ctx context.Context,
	peerID string,
	linkID string,
	bundleID string,
) (Bundle, bool, error) {
	fragments, err := s.store.FragmentsByBundle(ctx, bundleID, BundleDirectionInbound)
	if err != nil {
		return Bundle{}, false, fmt.Errorf("load inbound fragments for bundle %s: %w", bundleID, err)
	}
	if len(fragments) == 0 {
		return Bundle{}, false, nil
	}
	if isInboundFragmentBundleTerminal(fragments) {
		return Bundle{}, false, nil
	}
	first := fragments[0]

	complete, completeErr := fragmentsComplete(fragments)
	if completeErr != nil {
		if quarantineErr := s.quarantineInboundFragments(ctx, fragments, completeErr); quarantineErr != nil {
			return Bundle{}, false, quarantineErr
		}
		return Bundle{}, false, completeErr
	}
	if !complete {
		return Bundle{}, false, nil
	}

	payload := make([]byte, 0)
	for _, fragment := range fragments {
		payload = append(payload, fragment.Payload...)
	}

	bundle, err := s.AcceptInboundBundle(ctx, SaveBundleParams{
		PeerID:        peerID,
		LinkID:        linkID,
		DedupKey:      assembledBundleDedupKey(first.BundleID),
		CursorFrom:    first.CursorFrom,
		CursorTo:      first.CursorTo,
		EventCount:    first.EventCount,
		PayloadType:   first.PayloadType,
		Payload:       payload,
		Compression:   first.Compression,
		IntegrityHash: first.IntegrityHash,
		AuthTag:       first.AuthTag,
		AvailableAt:   first.AvailableAt,
	})
	if err != nil {
		if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalidInput) {
			if quarantineErr := s.quarantineInboundFragments(ctx, fragments, err); quarantineErr != nil {
				return Bundle{}, false, quarantineErr
			}
		}
		return Bundle{}, false, err
	}

	for _, fragment := range fragments {
		fragment.State = FragmentStateAssembled
		fragment.AckedAt = s.currentTime()
		if _, err := s.store.SaveFragment(ctx, fragment); err != nil {
			return Bundle{}, false, fmt.Errorf("mark fragment %s assembled: %w", fragment.ID, err)
		}
	}

	return bundle, true, nil
}

func (s *Service) bridgeLinkByServerAndName(
	ctx context.Context,
	peerServerName string,
	linkName string,
) (Peer, Link, error) {
	peerServerName = strings.TrimSpace(strings.ToLower(peerServerName))
	linkName = strings.TrimSpace(strings.ToLower(linkName))
	if peerServerName == "" || linkName == "" {
		return Peer{}, Link{}, ErrInvalidInput
	}

	peer, err := s.PeerByServerName(ctx, peerServerName)
	if err != nil {
		return Peer{}, Link{}, err
	}
	link, err := s.LinkByPeerAndName(ctx, peer.ID, linkName)
	if err != nil {
		return Peer{}, Link{}, err
	}
	if !isBridgeTransport(link.TransportKind) {
		return Peer{}, Link{}, ErrForbidden
	}
	if _, err := s.authorizeLinkForPeer(ctx, peer.ID, link.ID, false); err != nil {
		return Peer{}, Link{}, err
	}

	return peer, link, nil
}

func isBridgeTransport(kind TransportKind) bool {
	switch kind {
	case TransportKindMeshtastic, TransportKindMeshCore, TransportKindCustomDTN:
		return true
	default:
		return false
	}
}

func splitPayload(payload []byte, maxBytes int) [][]byte {
	if maxBytes <= 0 {
		maxBytes = len(payload)
	}
	if len(payload) == 0 {
		return [][]byte{{}}
	}

	parts := make([][]byte, 0, (len(payload)+maxBytes-1)/maxBytes)
	for start := 0; start < len(payload); start += maxBytes {
		end := start + maxBytes
		if end > len(payload) {
			end = len(payload)
		}
		parts = append(parts, append([]byte(nil), payload[start:end]...))
	}

	return parts
}

func fragmentsComplete(fragments []BundleFragment) (bool, error) {
	if len(fragments) == 0 {
		return false, nil
	}

	ordered := append([]BundleFragment(nil), fragments...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].FragmentIndex < ordered[j].FragmentIndex
	})

	expectedCount := ordered[0].FragmentCount
	if expectedCount <= 0 || len(fragments) != expectedCount {
		return false, validateFragmentSet(ordered)
	}
	if err := validateFragmentSet(ordered); err != nil {
		return false, err
	}

	return true, nil
}

func allFragmentsAcknowledged(fragments []BundleFragment) bool {
	if len(fragments) == 0 {
		return false
	}
	for _, fragment := range fragments {
		if fragment.State != FragmentStateAcknowledged && fragment.State != FragmentStateAssembled {
			return false
		}
	}

	return true
}

func validateFragmentSet(fragments []BundleFragment) error {
	if len(fragments) == 0 {
		return nil
	}

	first := fragments[0]
	seenIndexes := make(map[int]struct{}, len(fragments))
	for _, fragment := range fragments {
		if fragment.State == FragmentStateFailed || fragment.State == FragmentStateAssembled {
			return nil
		}
		if fragment.FragmentCount != first.FragmentCount {
			return ErrConflict
		}
		if fragment.PeerID != first.PeerID ||
			fragment.LinkID != first.LinkID ||
			fragment.BundleID != first.BundleID ||
			fragment.Direction != first.Direction ||
			fragment.CursorFrom != first.CursorFrom ||
			fragment.CursorTo != first.CursorTo ||
			fragment.EventCount != first.EventCount ||
			fragment.PayloadType != first.PayloadType ||
			fragment.Compression != first.Compression ||
			fragment.IntegrityHash != first.IntegrityHash ||
			fragment.AuthTag != first.AuthTag {
			return ErrConflict
		}
		if fragment.State != FragmentStateAccepted {
			return nil
		}
		if _, ok := seenIndexes[fragment.FragmentIndex]; ok {
			return ErrConflict
		}
		seenIndexes[fragment.FragmentIndex] = struct{}{}
	}

	return nil
}

func isInboundFragmentBundleTerminal(fragments []BundleFragment) bool {
	for _, fragment := range fragments {
		if fragment.State == FragmentStateFailed || fragment.State == FragmentStateAssembled {
			return true
		}
	}

	return false
}

func containsFragmentDedupKey(fragments []BundleFragment, dedupKey string) bool {
	dedupKey = strings.TrimSpace(dedupKey)
	for _, fragment := range fragments {
		if fragment.DedupKey == dedupKey {
			return true
		}
	}

	return false
}

func (s *Service) quarantineInboundFragments(
	ctx context.Context,
	fragments []BundleFragment,
	quarantineErr error,
) error {
	if len(fragments) == 0 {
		return nil
	}

	now := s.currentTime()
	return s.store.WithinTx(ctx, func(store Store) error {
		for _, fragment := range fragments {
			if fragment.Direction != BundleDirectionInbound {
				continue
			}
			if fragment.State == FragmentStateFailed || fragment.State == FragmentStateAssembled {
				continue
			}

			fragment.State = FragmentStateFailed
			fragment.AckedAt = now
			if _, err := store.SaveFragment(ctx, fragment); err != nil {
				return fmt.Errorf("quarantine federation fragment %s: %w", fragment.ID, err)
			}
		}

		link, err := store.LinkByID(ctx, fragments[0].LinkID)
		if err != nil {
			return fmt.Errorf("load federation link %s before fragment quarantine: %w", fragments[0].LinkID, err)
		}
		if quarantineErr != nil {
			link.LastError = strings.TrimSpace(quarantineErr.Error())
		}
		if link.State == LinkStateActive {
			link.State = LinkStateDegraded
		}
		link.UpdatedAt = now
		if _, err := store.SaveLink(ctx, link); err != nil {
			return fmt.Errorf("save federation link %s after fragment quarantine: %w", link.ID, err)
		}

		return nil
	})
}

func fragmentDedupKey(bundleDedupKey string, fragmentIndex int) string {
	return fmt.Sprintf("%s:frag:%06d", strings.TrimSpace(bundleDedupKey), fragmentIndex)
}

func assembledBundleDedupKey(bundleID string) string {
	return "bridge_bundle:" + strings.TrimSpace(bundleID)
}
