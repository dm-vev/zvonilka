package federationmeshtastic

import (
	"context"
	"testing"
	"time"

	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	transport "github.com/dm-vev/zvonilka/internal/platform/federation/transport"
	"github.com/stretchr/testify/require"
)

type stubBridgeClient struct {
	pulledFragments []*federationv1.BundleFragment
	pulledLink      *federationv1.Link
	ackedIDs        []string
	ackedLeaseToken string
	submitted       [][]*federationv1.BundleFragment
	submitPeer      []string
	submitLink      []string
}

func (s *stubBridgeClient) PullFragments(
	_ context.Context,
	_ string,
	_ string,
	_ int,
) (*federationv1.PullBridgeFragmentsResponse, error) {
	return &federationv1.PullBridgeFragmentsResponse{
		Fragments:  s.pulledFragments,
		Link:       s.pulledLink,
		LeaseToken: "lease-token-1",
	}, nil
}

func (s *stubBridgeClient) SubmitFragments(
	_ context.Context,
	peerServerName string,
	linkName string,
	fragments []*federationv1.BundleFragment,
) (*federationv1.SubmitBridgeFragmentsResponse, error) {
	s.submitPeer = append(s.submitPeer, peerServerName)
	s.submitLink = append(s.submitLink, linkName)
	copied := make([]*federationv1.BundleFragment, 0, len(fragments))
	for _, fragment := range fragments {
		copied = append(copied, cloneFragment(fragment))
	}
	s.submitted = append(s.submitted, copied)
	return &federationv1.SubmitBridgeFragmentsResponse{}, nil
}

func (s *stubBridgeClient) AcknowledgeFragments(
	_ context.Context,
	_ string,
	_ string,
	fragmentIDs []string,
	leaseToken string,
) (*federationv1.AcknowledgeBridgeFragmentsResponse, error) {
	s.ackedIDs = append([]string(nil), fragmentIDs...)
	s.ackedLeaseToken = leaseToken
	return &federationv1.AcknowledgeBridgeFragmentsResponse{}, nil
}

func (s *stubBridgeClient) Close() error { return nil }

type stubAdapter struct {
	sent     []*federationv1.BundleFragment
	received []transport.ReceivedFragment
}

func (s *stubAdapter) Send(
	_ context.Context,
	_ string,
	_ string,
	_ *federationv1.Link,
	fragments []*federationv1.BundleFragment,
) ([]string, error) {
	s.sent = append([]*federationv1.BundleFragment(nil), fragments...)
	ids := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		ids = append(ids, fragment.GetFragmentId())
	}
	return ids, nil
}

func (s *stubAdapter) Receive(_ context.Context, _ int) ([]transport.ReceivedFragment, error) {
	return append([]transport.ReceivedFragment(nil), s.received...), nil
}

func (s *stubAdapter) Close() error { return nil }

func TestWorkerProcessesPulledAndReceivedFragments(t *testing.T) {
	t.Parallel()

	bridge := &stubBridgeClient{
		pulledLink: &federationv1.Link{LinkId: "link-1", Name: "mesh"},
		pulledFragments: []*federationv1.BundleFragment{{
			FragmentId:    "frag-out-1",
			BundleId:      "bundle-out-1",
			DedupKey:      "bundle-out-1:frag:000000",
			IntegrityHash: "hash-out-1",
			AuthTag:       "auth-out-1",
		}},
	}
	adapter := &stubAdapter{
		received: []transport.ReceivedFragment{{
			PeerServerName: "mesh.example",
			LinkName:       "mesh",
			Fragment: &federationv1.BundleFragment{
				BundleId:      "bundle-in-1",
				DedupKey:      "bundle-in-1:frag:000000",
				CursorFrom:    10,
				CursorTo:      10,
				EventCount:    1,
				PayloadType:   "bundle",
				Compression:   federationv1.CompressionKind_COMPRESSION_KIND_NONE,
				IntegrityHash: "hash-in-1",
				AuthTag:       "auth-in-1",
				FragmentIndex: 0,
				FragmentCount: 1,
				Payload:       []byte("hello"),
			},
		}},
	}

	worker, err := newWorker(bridge, adapter, settings{
		PeerServerName: "mesh.example",
		LinkName:       "mesh",
		PollInterval:   time.Second,
		BatchSize:      10,
	})
	require.NoError(t, err)

	err = worker.ProcessOnceForTests(context.Background())
	require.NoError(t, err)
	require.Len(t, adapter.sent, 1)
	require.Equal(t, []string{"frag-out-1"}, bridge.ackedIDs)
	require.Equal(t, "lease-token-1", bridge.ackedLeaseToken)
	require.Len(t, bridge.submitted, 1)
	require.Equal(t, "mesh.example", bridge.submitPeer[0])
	require.Equal(t, "mesh", bridge.submitLink[0])
	require.Equal(t, []byte("hello"), bridge.submitted[0][0].GetPayload())
}
