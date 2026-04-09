package federation_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/federation"
	federationtest "github.com/dm-vev/zvonilka/internal/domain/federation/teststore"
)

func TestServicePeerLinkAndCursorLifecycle(t *testing.T) {
	t.Parallel()

	service, err := federation.NewService(
		federationtest.NewMemoryStore(),
		federation.WithNow(func() time.Time {
			return time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC)
		}),
	)
	require.NoError(t, err)

	peer, sharedSecret, generated, err := service.CreatePeer(context.Background(), federation.CreatePeerParams{
		ServerName: "alpha.example",
		BaseURL:    "https://alpha.example",
		Trusted:    true,
		Capabilities: []federation.Capability{
			federation.CapabilityEventReplication,
			federation.CapabilityMediaProxy,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, sharedSecret)
	require.True(t, generated)
	require.Equal(t, federation.PeerStateActive, peer.State)
	require.Equal(t, sharedSecret, peer.SharedSecret)

	authenticated, err := service.AuthenticatePeerByServerName(context.Background(), peer.ServerName, sharedSecret)
	require.NoError(t, err)
	require.Equal(t, peer.ID, authenticated.ID)

	link, err := service.CreateLink(context.Background(), federation.CreateLinkParams{
		PeerID:           peer.ID,
		Name:             "primary",
		TransportKind:    federation.TransportKindHTTPS,
		DeliveryClass:    federation.DeliveryClassRealtime,
		DiscoveryMode:    federation.DiscoveryModeManual,
		MediaPolicy:      federation.MediaPolicyReferenceProxy,
		MaxBundleBytes:   4096,
		MaxFragmentBytes: 1024,
	})
	require.NoError(t, err)
	require.Equal(t, peer.BaseURL, link.Endpoint)
	require.Equal(t, federation.LinkStateActive, link.State)

	resolvedLink, err := service.LinkByPeerAndName(context.Background(), peer.ID, link.Name)
	require.NoError(t, err)
	require.Equal(t, link.ID, resolvedLink.ID)

	outbound, err := service.QueueOutboundBundle(context.Background(), federation.SaveBundleParams{
		PeerID:      peer.ID,
		LinkID:      link.ID,
		DedupKey:    "out-1",
		CursorFrom:  1,
		CursorTo:    2,
		EventCount:  2,
		PayloadType: "bundle",
		Payload:     []byte("outbound"),
	})
	require.NoError(t, err)
	require.Equal(t, federation.BundleDirectionOutbound, outbound.Direction)

	inbound, err := service.AcceptInboundBundle(context.Background(), federation.SaveBundleParams{
		PeerID:      peer.ID,
		LinkID:      link.ID,
		DedupKey:    "in-1",
		CursorFrom:  10,
		CursorTo:    11,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("inbound"),
	})
	require.NoError(t, err)
	require.Equal(t, federation.BundleDirectionInbound, inbound.Direction)

	cursor, err := service.ReplicationCursorByPeerAndLink(context.Background(), peer.ID, link.ID)
	require.NoError(t, err)
	require.Equal(t, uint64(11), cursor.LastReceivedCursor)
	require.Equal(t, uint64(0), cursor.LastInboundCursor)
	require.Equal(t, uint64(2), cursor.LastOutboundCursor)
	require.Equal(t, uint64(0), cursor.LastAckedCursor)

	cursor, err = service.AdvanceInboundCursor(context.Background(), peer.ID, link.ID, 11)
	require.NoError(t, err)
	require.Equal(t, uint64(11), cursor.LastReceivedCursor)
	require.Equal(t, uint64(11), cursor.LastInboundCursor)

	cursor, err = service.AcknowledgeBundles(context.Background(), federation.AcknowledgeBundlesParams{
		PeerID:         peer.ID,
		LinkID:         link.ID,
		UpToCursor:     2,
		BundleIDs:      []string{outbound.ID},
		AcknowledgedAt: time.Date(2026, time.April, 9, 12, 1, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), cursor.LastAckedCursor)

	bundles, err := service.BundlesAfter(context.Background(), peer.ID, link.ID, federation.BundleDirectionOutbound, 0, 10)
	require.NoError(t, err)
	require.Len(t, bundles, 1)
	require.Equal(t, federation.BundleStateAcknowledged, bundles[0].State)
}
