package federation

import (
	"context"
	"time"
)

// Store persists federation state.
type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error

	SavePeer(ctx context.Context, peer Peer) (Peer, error)
	PeerByID(ctx context.Context, peerID string) (Peer, error)
	PeerByServerName(ctx context.Context, serverName string) (Peer, error)
	PeersByState(ctx context.Context, state PeerState) ([]Peer, error)

	SaveLink(ctx context.Context, link Link) (Link, error)
	LinkByID(ctx context.Context, linkID string) (Link, error)
	LinkByPeerAndName(ctx context.Context, peerID string, name string) (Link, error)
	Links(ctx context.Context, peerID string, state LinkState) ([]Link, error)

	SaveBundle(ctx context.Context, bundle Bundle) (Bundle, error)
	BundleByDedupKey(ctx context.Context, dedupKey string) (Bundle, error)
	BundlesAfter(
		ctx context.Context,
		peerID string,
		linkID string,
		direction BundleDirection,
		afterCursor uint64,
		limit int,
	) ([]Bundle, error)
	AcknowledgeBundles(ctx context.Context, params AcknowledgeBundlesParams) ([]Bundle, error)

	SaveFragment(ctx context.Context, fragment BundleFragment) (BundleFragment, error)
	ClaimFragments(ctx context.Context, params ClaimFragmentsParams) ([]BundleFragment, error)
	HasClaimableFragments(ctx context.Context, peerID string, linkID string, claimedAt time.Time) (bool, error)
	Fragments(
		ctx context.Context,
		peerID string,
		linkID string,
		direction BundleDirection,
		state FragmentState,
		limit int,
	) ([]BundleFragment, error)
	FragmentsByBundle(ctx context.Context, bundleID string, direction BundleDirection) ([]BundleFragment, error)
	AcknowledgeFragments(ctx context.Context, params AcknowledgeFragmentsParams) ([]BundleFragment, error)

	SaveCursor(ctx context.Context, cursor ReplicationCursor) (ReplicationCursor, error)
	CursorByPeerAndLink(ctx context.Context, peerID string, linkID string) (ReplicationCursor, error)
}
