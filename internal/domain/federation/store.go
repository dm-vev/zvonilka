package federation

import "context"

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
	BundlesAfter(
		ctx context.Context,
		peerID string,
		linkID string,
		direction BundleDirection,
		afterCursor uint64,
		limit int,
	) ([]Bundle, error)
	AcknowledgeBundles(ctx context.Context, params AcknowledgeBundlesParams) ([]Bundle, error)

	SaveCursor(ctx context.Context, cursor ReplicationCursor) (ReplicationCursor, error)
	CursorByPeerAndLink(ctx context.Context, peerID string, linkID string) (ReplicationCursor, error)
}
