package transport

import (
	"context"

	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
)

// ReceivedFragment is one inbound transport fragment decoded from the constrained network.
type ReceivedFragment struct {
	PeerServerName string
	LinkName       string
	Fragment       *federationv1.BundleFragment
}

// Adapter exchanges bridge fragments with one constrained transport implementation.
type Adapter interface {
	Send(
		ctx context.Context,
		peerServerName string,
		linkName string,
		link *federationv1.Link,
		fragments []*federationv1.BundleFragment,
	) ([]string, error)
	Receive(ctx context.Context, limit int) ([]ReceivedFragment, error)
	Close() error
}
