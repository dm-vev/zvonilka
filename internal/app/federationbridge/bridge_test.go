package federationbridge

import (
	"context"
	"testing"
	"time"

	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	domainfederation "github.com/dm-vev/zvonilka/internal/domain/federation"
	federationtest "github.com/dm-vev/zvonilka/internal/domain/federation/teststore"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func newTestBridgeAPI(t *testing.T) (*api, domainfederation.Peer, domainfederation.Link) {
	t.Helper()

	service, err := domainfederation.NewService(
		federationtest.NewMemoryStore(),
		domainfederation.WithNow(func() time.Time {
			return time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC)
		}),
	)
	require.NoError(t, err)

	peer, _, _, _, _, err := service.CreatePeer(context.Background(), domainfederation.CreatePeerParams{
		ServerName:   "mesh.example",
		BaseURL:      "bridge://mesh.example",
		Trusted:      true,
		SharedSecret: "peer-secret",
		Capabilities: []domainfederation.Capability{domainfederation.CapabilityEventReplication},
	})
	require.NoError(t, err)

	link, err := service.CreateLink(context.Background(), domainfederation.CreateLinkParams{
		PeerID:           peer.ID,
		Name:             "mesh",
		Endpoint:         "bridge://meshtastic.local",
		TransportKind:    domainfederation.TransportKindMeshtastic,
		DeliveryClass:    domainfederation.DeliveryClassUltraConstrained,
		DiscoveryMode:    domainfederation.DiscoveryModeBridgeAnnounced,
		MediaPolicy:      domainfederation.MediaPolicyDisabled,
		MaxBundleBytes:   1024,
		MaxFragmentBytes: 4,
	})
	require.NoError(t, err)

	return &api{
		federation:   service,
		sharedSecret: "bridge-secret",
	}, peer, link
}

func bridgeContext(secret string) context.Context {
	return metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("authorization", "Bearer "+secret),
	)
}

func TestBridgeServiceFragmentFlow(t *testing.T) {
	t.Parallel()

	api, peer, link := newTestBridgeAPI(t)

	_, err := api.federation.QueueOutboundBundle(context.Background(), domainfederation.SaveBundleParams{
		PeerID:      peer.ID,
		LinkID:      link.ID,
		DedupKey:    "bridge-outbound-1",
		CursorFrom:  1,
		CursorTo:    2,
		EventCount:  2,
		PayloadType: "bundle",
		Payload:     []byte("abcdefgh"),
	})
	require.NoError(t, err)

	pulled, err := api.PullBridgeFragments(bridgeContext("bridge-secret"), &federationv1.PullBridgeFragmentsRequest{
		PeerServerName: peer.ServerName,
		LinkName:       link.Name,
		Limit:          10,
	})
	require.NoError(t, err)
	require.Len(t, pulled.GetFragments(), 2)
	require.Equal(t, link.ID, pulled.GetLink().GetLinkId())
	require.NotEmpty(t, pulled.GetLeaseToken())
	require.Equal(t, federationv1.FragmentState_FRAGMENT_STATE_CLAIMED, pulled.GetFragments()[0].GetState())

	acked, err := api.AcknowledgeBridgeFragments(bridgeContext("bridge-secret"), &federationv1.AcknowledgeBridgeFragmentsRequest{
		PeerServerName: peer.ServerName,
		LinkName:       link.Name,
		LeaseToken:     pulled.GetLeaseToken(),
		FragmentIds: []string{
			pulled.GetFragments()[0].GetFragmentId(),
			pulled.GetFragments()[1].GetFragmentId(),
		},
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), acked.GetCursor().GetLastAckedCursor())

	signedInbound, err := api.federation.SignBundle(context.Background(), peer.ID, link.ID, domainfederation.Bundle{
		ID:          "remote-bundle-1",
		DedupKey:    "remote-bundle-1",
		CursorFrom:  10,
		CursorTo:    10,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("mesh payload"),
		Compression: domainfederation.CompressionKindNone,
	})
	require.NoError(t, err)

	submitted, err := api.SubmitBridgeFragments(bridgeContext("bridge-secret"), &federationv1.SubmitBridgeFragmentsRequest{
		PeerServerName: peer.ServerName,
		LinkName:       link.Name,
		Fragments: []*federationv1.BundleFragment{
			{
				BundleId:      "remote-bundle-1",
				DedupKey:      "remote-bundle-1:frag:000000",
				CursorFrom:    10,
				CursorTo:      10,
				EventCount:    1,
				PayloadType:   "bundle",
				Compression:   federationv1.CompressionKind_COMPRESSION_KIND_NONE,
				IntegrityHash: signedInbound.IntegrityHash,
				AuthTag:       signedInbound.AuthTag,
				FragmentIndex: 0,
				FragmentCount: 3,
				Payload:       []byte("mesh"),
			},
			{
				BundleId:      "remote-bundle-1",
				DedupKey:      "remote-bundle-1:frag:000001",
				CursorFrom:    10,
				CursorTo:      10,
				EventCount:    1,
				PayloadType:   "bundle",
				Compression:   federationv1.CompressionKind_COMPRESSION_KIND_NONE,
				IntegrityHash: signedInbound.IntegrityHash,
				AuthTag:       signedInbound.AuthTag,
				FragmentIndex: 1,
				FragmentCount: 3,
				Payload:       []byte(" pay"),
			},
			{
				BundleId:      "remote-bundle-1",
				DedupKey:      "remote-bundle-1:frag:000002",
				CursorFrom:    10,
				CursorTo:      10,
				EventCount:    1,
				PayloadType:   "bundle",
				Compression:   federationv1.CompressionKind_COMPRESSION_KIND_NONE,
				IntegrityHash: signedInbound.IntegrityHash,
				AuthTag:       signedInbound.AuthTag,
				FragmentIndex: 2,
				FragmentCount: 3,
				Payload:       []byte("load"),
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, submitted.GetAcceptedFragmentIds(), 3)
	require.Len(t, submitted.GetAssembledBundleIds(), 1)
	require.Equal(t, uint64(10), submitted.GetCursor().GetLastReceivedCursor())
}

func TestBridgeServiceRejectsInvalidSecret(t *testing.T) {
	t.Parallel()

	api, peer, link := newTestBridgeAPI(t)

	_, err := api.PullBridgeFragments(bridgeContext("wrong-secret"), &federationv1.PullBridgeFragmentsRequest{
		PeerServerName: peer.ServerName,
		LinkName:       link.Name,
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unauthenticated, st.Code())
}

func TestBridgeServiceRejectsTamperedInboundFragments(t *testing.T) {
	t.Parallel()

	api, peer, link := newTestBridgeAPI(t)

	signedInbound, err := api.federation.SignBundle(context.Background(), peer.ID, link.ID, domainfederation.Bundle{
		ID:          "remote-bundle-tampered",
		DedupKey:    "remote-bundle-tampered",
		CursorFrom:  10,
		CursorTo:    11,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("pingpong"),
		Compression: domainfederation.CompressionKindNone,
	})
	require.NoError(t, err)

	_, err = api.SubmitBridgeFragments(bridgeContext("bridge-secret"), &federationv1.SubmitBridgeFragmentsRequest{
		PeerServerName: peer.ServerName,
		LinkName:       link.Name,
		Fragments: []*federationv1.BundleFragment{{
			BundleId:      "remote-bundle-tampered",
			DedupKey:      "remote-bundle-tampered:frag:000000",
			CursorFrom:    10,
			CursorTo:      11,
			EventCount:    1,
			PayloadType:   "bundle",
			Compression:   federationv1.CompressionKind_COMPRESSION_KIND_NONE,
			IntegrityHash: signedInbound.IntegrityHash,
			AuthTag:       signedInbound.AuthTag,
			FragmentIndex: 0,
			FragmentCount: 2,
			Payload:       []byte("ping"),
		}},
	})
	require.NoError(t, err)

	_, err = api.SubmitBridgeFragments(bridgeContext("bridge-secret"), &federationv1.SubmitBridgeFragmentsRequest{
		PeerServerName: peer.ServerName,
		LinkName:       link.Name,
		Fragments: []*federationv1.BundleFragment{{
			BundleId:      "remote-bundle-tampered",
			DedupKey:      "remote-bundle-tampered:frag:000001",
			CursorFrom:    10,
			CursorTo:      11,
			EventCount:    1,
			PayloadType:   "bundle",
			Compression:   federationv1.CompressionKind_COMPRESSION_KIND_NONE,
			IntegrityHash: signedInbound.IntegrityHash,
			AuthTag:       signedInbound.AuthTag,
			FragmentIndex: 1,
			FragmentCount: 2,
			Payload:       []byte("fail"),
		}},
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unauthenticated, st.Code())
}
