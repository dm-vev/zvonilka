package federation_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/federation"
	federationtest "github.com/dm-vev/zvonilka/internal/domain/federation/teststore"
)

func TestServiceBridgeFragmentLifecycle(t *testing.T) {
	t.Parallel()

	service, err := federation.NewService(
		federationtest.NewMemoryStore(),
		federation.WithNow(func() time.Time {
			return time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC)
		}),
	)
	require.NoError(t, err)

	peer, _, _, _, _, err := service.CreatePeer(context.Background(), federation.CreatePeerParams{
		ServerName:   "mesh.example",
		BaseURL:      "bridge://mesh.example",
		Trusted:      true,
		SharedSecret: "mesh-secret",
		Capabilities: []federation.Capability{federation.CapabilityEventReplication},
	})
	require.NoError(t, err)

	link, err := service.CreateLink(context.Background(), federation.CreateLinkParams{
		PeerID:           peer.ID,
		Name:             "mesh",
		Endpoint:         "bridge://meshtastic.local",
		TransportKind:    federation.TransportKindMeshtastic,
		DeliveryClass:    federation.DeliveryClassUltraConstrained,
		DiscoveryMode:    federation.DiscoveryModeBridgeAnnounced,
		MediaPolicy:      federation.MediaPolicyDisabled,
		MaxBundleBytes:   1024,
		MaxFragmentBytes: 4,
	})
	require.NoError(t, err)

	outbound, err := service.QueueOutboundBundle(context.Background(), federation.SaveBundleParams{
		PeerID:      peer.ID,
		LinkID:      link.ID,
		DedupKey:    "outbound-bridge-1",
		CursorFrom:  1,
		CursorTo:    3,
		EventCount:  3,
		PayloadType: "bundle",
		Payload:     []byte("abcdefghij"),
	})
	require.NoError(t, err)

	fragments, returnedLink, cursor, hasMore, leaseToken, err := service.PullBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		10,
	)
	require.NoError(t, err)
	require.False(t, hasMore)
	require.NotEmpty(t, leaseToken)
	require.Equal(t, link.ID, returnedLink.ID)
	require.Equal(t, uint64(3), cursor.LastOutboundCursor)
	require.Len(t, fragments, 3)
	require.Equal(t, outbound.ID, fragments[0].BundleID)
	require.Equal(t, federation.FragmentStateClaimed, fragments[0].State)
	require.Equal(t, 1, fragments[0].AttemptCount)
	require.Equal(t, []byte("abcd"), fragments[0].Payload)
	require.Equal(t, []byte("efgh"), fragments[1].Payload)
	require.Equal(t, []byte("ij"), fragments[2].Payload)

	acknowledged, cursor, err := service.AcknowledgeBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]string{fragments[0].ID},
		leaseToken,
		time.Date(2026, time.April, 9, 12, 1, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	require.Len(t, acknowledged, 1)
	require.Equal(t, uint64(0), cursor.LastAckedCursor)

	acknowledged, cursor, err = service.AcknowledgeBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]string{fragments[1].ID, fragments[2].ID},
		leaseToken,
		time.Date(2026, time.April, 9, 12, 2, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	require.Len(t, acknowledged, 2)
	require.Equal(t, uint64(3), cursor.LastAckedCursor)

	signedInbound, err := service.SignBundle(context.Background(), peer.ID, link.ID, federation.Bundle{
		ID:          "remote-bundle-1",
		DedupKey:    "remote-bundle-1",
		CursorFrom:  10,
		CursorTo:    11,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("hello mesh"),
		Compression: federation.CompressionKindNone,
	})
	require.NoError(t, err)

	accepted, assembled, cursor, err := service.SubmitBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]federation.SaveFragmentParams{
			{
				BundleID:      "remote-bundle-1",
				DedupKey:      "remote-bundle-1:frag:000000",
				CursorFrom:    10,
				CursorTo:      11,
				EventCount:    1,
				PayloadType:   "bundle",
				Compression:   federation.CompressionKindNone,
				IntegrityHash: signedInbound.IntegrityHash,
				AuthTag:       signedInbound.AuthTag,
				FragmentIndex: 0,
				FragmentCount: 3,
				Payload:       []byte("hell"),
			},
			{
				BundleID:      "remote-bundle-1",
				DedupKey:      "remote-bundle-1:frag:000001",
				CursorFrom:    10,
				CursorTo:      11,
				EventCount:    1,
				PayloadType:   "bundle",
				Compression:   federation.CompressionKindNone,
				IntegrityHash: signedInbound.IntegrityHash,
				AuthTag:       signedInbound.AuthTag,
				FragmentIndex: 1,
				FragmentCount: 3,
				Payload:       []byte("o me"),
			},
			{
				BundleID:      "remote-bundle-1",
				DedupKey:      "remote-bundle-1:frag:000002",
				CursorFrom:    10,
				CursorTo:      11,
				EventCount:    1,
				PayloadType:   "bundle",
				Compression:   federation.CompressionKindNone,
				IntegrityHash: signedInbound.IntegrityHash,
				AuthTag:       signedInbound.AuthTag,
				FragmentIndex: 2,
				FragmentCount: 3,
				Payload:       []byte("sh"),
			},
		},
	)
	require.NoError(t, err)
	require.Len(t, accepted, 3)
	require.Len(t, assembled, 1)
	require.Equal(t, []byte("hello mesh"), assembled[0].Payload)
	require.Equal(t, uint64(11), cursor.LastReceivedCursor)
}

func TestServiceBridgeFragmentLeaseExpiryAndStaleAck(t *testing.T) {
	t.Parallel()

	current := time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC)
	service, err := federation.NewService(
		federationtest.NewMemoryStore(),
		federation.WithNow(func() time.Time { return current }),
		federation.WithBridgeFragmentLeaseTTL(30*time.Second),
	)
	require.NoError(t, err)

	peer, _, _, _, _, err := service.CreatePeer(context.Background(), federation.CreatePeerParams{
		ServerName:   "mesh.example",
		BaseURL:      "bridge://mesh.example",
		Trusted:      true,
		SharedSecret: "mesh-secret",
		Capabilities: []federation.Capability{federation.CapabilityEventReplication},
	})
	require.NoError(t, err)

	link, err := service.CreateLink(context.Background(), federation.CreateLinkParams{
		PeerID:           peer.ID,
		Name:             "mesh",
		Endpoint:         "bridge://meshtastic.local",
		TransportKind:    federation.TransportKindMeshtastic,
		DeliveryClass:    federation.DeliveryClassUltraConstrained,
		DiscoveryMode:    federation.DiscoveryModeBridgeAnnounced,
		MediaPolicy:      federation.MediaPolicyDisabled,
		MaxBundleBytes:   1024,
		MaxFragmentBytes: 2,
	})
	require.NoError(t, err)

	_, err = service.QueueOutboundBundle(context.Background(), federation.SaveBundleParams{
		PeerID:      peer.ID,
		LinkID:      link.ID,
		DedupKey:    "outbound-bridge-lease-1",
		CursorFrom:  1,
		CursorTo:    2,
		EventCount:  2,
		PayloadType: "bundle",
		Payload:     []byte("abcd"),
	})
	require.NoError(t, err)

	firstPull, _, _, _, firstLease, err := service.PullBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		10,
	)
	require.NoError(t, err)
	require.Len(t, firstPull, 2)
	require.NotEmpty(t, firstLease)
	require.Equal(t, 1, firstPull[0].AttemptCount)

	current = current.Add(31 * time.Second)

	secondPull, _, _, _, secondLease, err := service.PullBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		10,
	)
	require.NoError(t, err)
	require.Len(t, secondPull, 2)
	require.NotEqual(t, firstLease, secondLease)
	require.Equal(t, firstPull[0].ID, secondPull[0].ID)
	require.Equal(t, 2, secondPull[0].AttemptCount)

	_, cursor, err := service.AcknowledgeBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]string{secondPull[0].ID, secondPull[1].ID},
		firstLease,
		current,
	)
	require.NoError(t, err)
	require.Equal(t, uint64(0), cursor.LastAckedCursor)

	_, cursor, err = service.AcknowledgeBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]string{secondPull[0].ID, secondPull[1].ID},
		secondLease,
		current,
	)
	require.NoError(t, err)
	require.Equal(t, uint64(2), cursor.LastAckedCursor)
}

func TestServiceRejectsConflictingDuplicateInboundFragment(t *testing.T) {
	t.Parallel()

	service, err := federation.NewService(
		federationtest.NewMemoryStore(),
		federation.WithNow(func() time.Time {
			return time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC)
		}),
	)
	require.NoError(t, err)

	peer, _, _, _, _, err := service.CreatePeer(context.Background(), federation.CreatePeerParams{
		ServerName:   "mesh.example",
		BaseURL:      "bridge://mesh.example",
		Trusted:      true,
		SharedSecret: "mesh-secret",
		Capabilities: []federation.Capability{federation.CapabilityEventReplication},
	})
	require.NoError(t, err)

	link, err := service.CreateLink(context.Background(), federation.CreateLinkParams{
		PeerID:           peer.ID,
		Name:             "mesh",
		Endpoint:         "bridge://meshtastic.local",
		TransportKind:    federation.TransportKindMeshtastic,
		DeliveryClass:    federation.DeliveryClassUltraConstrained,
		DiscoveryMode:    federation.DiscoveryModeBridgeAnnounced,
		MediaPolicy:      federation.MediaPolicyDisabled,
		MaxBundleBytes:   1024,
		MaxFragmentBytes: 8,
	})
	require.NoError(t, err)

	signedFirst, err := service.SignBundle(context.Background(), peer.ID, link.ID, federation.Bundle{
		ID:          "remote-bundle-1",
		DedupKey:    "remote-bundle-1",
		CursorFrom:  10,
		CursorTo:    10,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("hello"),
		Compression: federation.CompressionKindNone,
	})
	require.NoError(t, err)

	_, _, _, err = service.SubmitBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]federation.SaveFragmentParams{{
			BundleID:      "remote-bundle-1",
			DedupKey:      "remote-bundle-1:frag:000000",
			CursorFrom:    10,
			CursorTo:      10,
			EventCount:    1,
			PayloadType:   "bundle",
			Compression:   federation.CompressionKindNone,
			IntegrityHash: signedFirst.IntegrityHash,
			AuthTag:       signedFirst.AuthTag,
			FragmentIndex: 0,
			FragmentCount: 1,
			Payload:       []byte("hello"),
		}},
	)
	require.NoError(t, err)

	signedSecond, err := service.SignBundle(context.Background(), peer.ID, link.ID, federation.Bundle{
		ID:          "remote-bundle-1",
		DedupKey:    "remote-bundle-1",
		CursorFrom:  10,
		CursorTo:    10,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("tampered"),
		Compression: federation.CompressionKindNone,
	})
	require.NoError(t, err)

	_, _, _, err = service.SubmitBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]federation.SaveFragmentParams{{
			BundleID:      "remote-bundle-1",
			DedupKey:      "remote-bundle-1:frag:000000",
			CursorFrom:    10,
			CursorTo:      10,
			EventCount:    1,
			PayloadType:   "bundle",
			Compression:   federation.CompressionKindNone,
			IntegrityHash: signedSecond.IntegrityHash,
			AuthTag:       signedSecond.AuthTag,
			FragmentIndex: 0,
			FragmentCount: 1,
			Payload:       []byte("tampered"),
		}},
	)
	require.ErrorIs(t, err, federation.ErrConflict)
}

func TestServiceQuarantinesTamperedInboundFragments(t *testing.T) {
	t.Parallel()

	store := federationtest.NewMemoryStore()
	service, err := federation.NewService(
		store,
		federation.WithNow(func() time.Time {
			return time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC)
		}),
	)
	require.NoError(t, err)

	peer, _, _, _, _, err := service.CreatePeer(context.Background(), federation.CreatePeerParams{
		ServerName:   "mesh.example",
		BaseURL:      "bridge://mesh.example",
		Trusted:      true,
		SharedSecret: "mesh-secret",
		Capabilities: []federation.Capability{federation.CapabilityEventReplication},
	})
	require.NoError(t, err)

	link, err := service.CreateLink(context.Background(), federation.CreateLinkParams{
		PeerID:           peer.ID,
		Name:             "mesh",
		Endpoint:         "bridge://meshtastic.local",
		TransportKind:    federation.TransportKindMeshtastic,
		DeliveryClass:    federation.DeliveryClassUltraConstrained,
		DiscoveryMode:    federation.DiscoveryModeBridgeAnnounced,
		MediaPolicy:      federation.MediaPolicyDisabled,
		MaxBundleBytes:   1024,
		MaxFragmentBytes: 8,
	})
	require.NoError(t, err)

	signedInbound, err := service.SignBundle(context.Background(), peer.ID, link.ID, federation.Bundle{
		ID:          "remote-bundle-tampered",
		DedupKey:    "remote-bundle-tampered",
		CursorFrom:  10,
		CursorTo:    11,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("hello mesh"),
		Compression: federation.CompressionKindNone,
	})
	require.NoError(t, err)

	accepted, assembled, _, err := service.SubmitBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]federation.SaveFragmentParams{{
			BundleID:      "remote-bundle-tampered",
			DedupKey:      "remote-bundle-tampered:frag:000000",
			CursorFrom:    10,
			CursorTo:      11,
			EventCount:    1,
			PayloadType:   "bundle",
			Compression:   federation.CompressionKindNone,
			IntegrityHash: signedInbound.IntegrityHash,
			AuthTag:       signedInbound.AuthTag,
			FragmentIndex: 0,
			FragmentCount: 2,
			Payload:       []byte("hello "),
		}},
	)
	require.NoError(t, err)
	require.Len(t, accepted, 1)
	require.Empty(t, assembled)

	_, _, _, err = service.SubmitBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]federation.SaveFragmentParams{{
			BundleID:      "remote-bundle-tampered",
			DedupKey:      "remote-bundle-tampered:frag:000001",
			CursorFrom:    10,
			CursorTo:      11,
			EventCount:    1,
			PayloadType:   "bundle",
			Compression:   federation.CompressionKindNone,
			IntegrityHash: signedInbound.IntegrityHash,
			AuthTag:       signedInbound.AuthTag,
			FragmentIndex: 1,
			FragmentCount: 2,
			Payload:       []byte("tampered"),
		}},
	)
	require.ErrorIs(t, err, federation.ErrUnauthorized)

	fragments, err := store.FragmentsByBundle(
		context.Background(),
		"remote-bundle-tampered",
		federation.BundleDirectionInbound,
	)
	require.NoError(t, err)
	require.Len(t, fragments, 2)
	require.Equal(t, federation.FragmentStateFailed, fragments[0].State)
	require.Equal(t, federation.FragmentStateFailed, fragments[1].State)

	savedLink, err := service.LinkByID(context.Background(), link.ID)
	require.NoError(t, err)
	require.Equal(t, federation.LinkStateDegraded, savedLink.State)
	require.Contains(t, savedLink.LastError, federation.ErrUnauthorized.Error())

	_, _, _, err = service.SubmitBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]federation.SaveFragmentParams{{
			BundleID:      "remote-bundle-tampered",
			DedupKey:      "remote-bundle-tampered:frag:000001",
			CursorFrom:    10,
			CursorTo:      11,
			EventCount:    1,
			PayloadType:   "bundle",
			Compression:   federation.CompressionKindNone,
			IntegrityHash: signedInbound.IntegrityHash,
			AuthTag:       signedInbound.AuthTag,
			FragmentIndex: 1,
			FragmentCount: 2,
			Payload:       []byte("mesh"),
		}},
	)
	require.ErrorIs(t, err, federation.ErrConflict)
}

func TestServiceKeepsInboundFragmentsAssembledOnDuplicateReplay(t *testing.T) {
	t.Parallel()

	store := federationtest.NewMemoryStore()
	service, err := federation.NewService(
		store,
		federation.WithNow(func() time.Time {
			return time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC)
		}),
	)
	require.NoError(t, err)

	peer, _, _, _, _, err := service.CreatePeer(context.Background(), federation.CreatePeerParams{
		ServerName:   "mesh.example",
		BaseURL:      "bridge://mesh.example",
		Trusted:      true,
		SharedSecret: "mesh-secret",
		Capabilities: []federation.Capability{federation.CapabilityEventReplication},
	})
	require.NoError(t, err)

	link, err := service.CreateLink(context.Background(), federation.CreateLinkParams{
		PeerID:           peer.ID,
		Name:             "mesh",
		Endpoint:         "bridge://meshtastic.local",
		TransportKind:    federation.TransportKindMeshtastic,
		DeliveryClass:    federation.DeliveryClassUltraConstrained,
		DiscoveryMode:    federation.DiscoveryModeBridgeAnnounced,
		MediaPolicy:      federation.MediaPolicyDisabled,
		MaxBundleBytes:   1024,
		MaxFragmentBytes: 8,
	})
	require.NoError(t, err)

	signedInbound, err := service.SignBundle(context.Background(), peer.ID, link.ID, federation.Bundle{
		ID:          "remote-bundle-assembled",
		DedupKey:    "remote-bundle-assembled",
		CursorFrom:  20,
		CursorTo:    21,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("hello mesh"),
		Compression: federation.CompressionKindNone,
	})
	require.NoError(t, err)

	_, assembled, _, err := service.SubmitBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]federation.SaveFragmentParams{
			{
				BundleID:      "remote-bundle-assembled",
				DedupKey:      "remote-bundle-assembled:frag:000000",
				CursorFrom:    20,
				CursorTo:      21,
				EventCount:    1,
				PayloadType:   "bundle",
				Compression:   federation.CompressionKindNone,
				IntegrityHash: signedInbound.IntegrityHash,
				AuthTag:       signedInbound.AuthTag,
				FragmentIndex: 0,
				FragmentCount: 2,
				Payload:       []byte("hello "),
			},
			{
				BundleID:      "remote-bundle-assembled",
				DedupKey:      "remote-bundle-assembled:frag:000001",
				CursorFrom:    20,
				CursorTo:      21,
				EventCount:    1,
				PayloadType:   "bundle",
				Compression:   federation.CompressionKindNone,
				IntegrityHash: signedInbound.IntegrityHash,
				AuthTag:       signedInbound.AuthTag,
				FragmentIndex: 1,
				FragmentCount: 2,
				Payload:       []byte("mesh"),
			},
		},
	)
	require.NoError(t, err)
	require.Len(t, assembled, 1)

	accepted, assembled, _, err := service.SubmitBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]federation.SaveFragmentParams{{
			BundleID:      "remote-bundle-assembled",
			DedupKey:      "remote-bundle-assembled:frag:000001",
			CursorFrom:    20,
			CursorTo:      21,
			EventCount:    1,
			PayloadType:   "bundle",
			Compression:   federation.CompressionKindNone,
			IntegrityHash: signedInbound.IntegrityHash,
			AuthTag:       signedInbound.AuthTag,
			FragmentIndex: 1,
			FragmentCount: 2,
			Payload:       []byte("mesh"),
		}},
	)
	require.NoError(t, err)
	require.Len(t, accepted, 1)
	require.Empty(t, assembled)
	require.Equal(t, federation.FragmentStateAssembled, accepted[0].State)

	fragments, err := store.FragmentsByBundle(
		context.Background(),
		"remote-bundle-assembled",
		federation.BundleDirectionInbound,
	)
	require.NoError(t, err)
	require.Len(t, fragments, 2)
	require.Equal(t, federation.FragmentStateAssembled, fragments[0].State)
	require.Equal(t, federation.FragmentStateAssembled, fragments[1].State)
}

func TestServiceRejectsStaleInboundFragmentReplay(t *testing.T) {
	t.Parallel()

	store := federationtest.NewMemoryStore()
	service, err := federation.NewService(
		store,
		federation.WithNow(func() time.Time {
			return time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC)
		}),
	)
	require.NoError(t, err)

	peer, _, _, _, _, err := service.CreatePeer(context.Background(), federation.CreatePeerParams{
		ServerName:   "mesh.example",
		BaseURL:      "bridge://mesh.example",
		Trusted:      true,
		SharedSecret: "mesh-secret",
		Capabilities: []federation.Capability{federation.CapabilityEventReplication},
	})
	require.NoError(t, err)

	link, err := service.CreateLink(context.Background(), federation.CreateLinkParams{
		PeerID:           peer.ID,
		Name:             "mesh",
		Endpoint:         "bridge://meshtastic.local",
		TransportKind:    federation.TransportKindMeshtastic,
		DeliveryClass:    federation.DeliveryClassUltraConstrained,
		DiscoveryMode:    federation.DiscoveryModeBridgeAnnounced,
		MediaPolicy:      federation.MediaPolicyDisabled,
		MaxBundleBytes:   1024,
		MaxFragmentBytes: 8,
	})
	require.NoError(t, err)

	signedInbound, err := service.SignBundle(context.Background(), peer.ID, link.ID, federation.Bundle{
		ID:          "remote-bundle-assembled",
		DedupKey:    "remote-bundle-assembled",
		CursorFrom:  20,
		CursorTo:    21,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("hello mesh"),
		Compression: federation.CompressionKindNone,
	})
	require.NoError(t, err)

	_, assembled, cursor, err := service.SubmitBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]federation.SaveFragmentParams{
			{
				BundleID:      "remote-bundle-assembled",
				DedupKey:      "remote-bundle-assembled:frag:000000",
				CursorFrom:    20,
				CursorTo:      21,
				EventCount:    1,
				PayloadType:   "bundle",
				Compression:   federation.CompressionKindNone,
				IntegrityHash: signedInbound.IntegrityHash,
				AuthTag:       signedInbound.AuthTag,
				FragmentIndex: 0,
				FragmentCount: 2,
				Payload:       []byte("hello "),
			},
			{
				BundleID:      "remote-bundle-assembled",
				DedupKey:      "remote-bundle-assembled:frag:000001",
				CursorFrom:    20,
				CursorTo:      21,
				EventCount:    1,
				PayloadType:   "bundle",
				Compression:   federation.CompressionKindNone,
				IntegrityHash: signedInbound.IntegrityHash,
				AuthTag:       signedInbound.AuthTag,
				FragmentIndex: 1,
				FragmentCount: 2,
				Payload:       []byte("mesh"),
			},
		},
	)
	require.NoError(t, err)
	require.Len(t, assembled, 1)
	require.Equal(t, uint64(21), cursor.LastReceivedCursor)

	staleSigned, err := service.SignBundle(context.Background(), peer.ID, link.ID, federation.Bundle{
		ID:          "remote-bundle-stale",
		DedupKey:    "remote-bundle-stale",
		CursorFrom:  18,
		CursorTo:    21,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("stale"),
		Compression: federation.CompressionKindNone,
	})
	require.NoError(t, err)

	_, _, _, err = service.SubmitBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]federation.SaveFragmentParams{{
			BundleID:      "remote-bundle-stale",
			DedupKey:      "remote-bundle-stale:frag:000000",
			CursorFrom:    18,
			CursorTo:      21,
			EventCount:    1,
			PayloadType:   "bundle",
			Compression:   federation.CompressionKindNone,
			IntegrityHash: staleSigned.IntegrityHash,
			AuthTag:       staleSigned.AuthTag,
			FragmentIndex: 0,
			FragmentCount: 1,
			Payload:       []byte("stale"),
		}},
	)
	require.ErrorIs(t, err, federation.ErrConflict)

	fragments, err := store.FragmentsByBundle(context.Background(), "remote-bundle-stale", federation.BundleDirectionInbound)
	require.NoError(t, err)
	require.Empty(t, fragments)
}

func TestServiceQuarantinesInconsistentInboundFragmentMetadataBeforeAssembly(t *testing.T) {
	t.Parallel()

	store := federationtest.NewMemoryStore()
	service, err := federation.NewService(
		store,
		federation.WithNow(func() time.Time {
			return time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC)
		}),
	)
	require.NoError(t, err)

	peer, _, _, _, _, err := service.CreatePeer(context.Background(), federation.CreatePeerParams{
		ServerName:   "mesh.example",
		BaseURL:      "bridge://mesh.example",
		Trusted:      true,
		SharedSecret: "mesh-secret",
		Capabilities: []federation.Capability{federation.CapabilityEventReplication},
	})
	require.NoError(t, err)

	link, err := service.CreateLink(context.Background(), federation.CreateLinkParams{
		PeerID:           peer.ID,
		Name:             "mesh",
		Endpoint:         "bridge://meshtastic.local",
		TransportKind:    federation.TransportKindMeshtastic,
		DeliveryClass:    federation.DeliveryClassUltraConstrained,
		DiscoveryMode:    federation.DiscoveryModeBridgeAnnounced,
		MediaPolicy:      federation.MediaPolicyDisabled,
		MaxBundleBytes:   1024,
		MaxFragmentBytes: 8,
	})
	require.NoError(t, err)

	signedFirst, err := service.SignBundle(context.Background(), peer.ID, link.ID, federation.Bundle{
		ID:          "remote-bundle-early-mismatch",
		DedupKey:    "remote-bundle-early-mismatch",
		CursorFrom:  30,
		CursorTo:    31,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("hello mesh"),
		Compression: federation.CompressionKindNone,
	})
	require.NoError(t, err)

	accepted, assembled, _, err := service.SubmitBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]federation.SaveFragmentParams{{
			BundleID:      "remote-bundle-early-mismatch",
			DedupKey:      "remote-bundle-early-mismatch:frag:000000",
			CursorFrom:    30,
			CursorTo:      31,
			EventCount:    1,
			PayloadType:   "bundle",
			Compression:   federation.CompressionKindNone,
			IntegrityHash: signedFirst.IntegrityHash,
			AuthTag:       signedFirst.AuthTag,
			FragmentIndex: 0,
			FragmentCount: 2,
			Payload:       []byte("hello "),
		}},
	)
	require.NoError(t, err)
	require.Len(t, accepted, 1)
	require.Empty(t, assembled)

	signedSecond, err := service.SignBundle(context.Background(), peer.ID, link.ID, federation.Bundle{
		ID:          "remote-bundle-early-mismatch",
		DedupKey:    "remote-bundle-early-mismatch",
		CursorFrom:  30,
		CursorTo:    31,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("tampered"),
		Compression: federation.CompressionKindNone,
	})
	require.NoError(t, err)

	_, _, _, err = service.SubmitBridgeFragments(
		context.Background(),
		peer.ServerName,
		link.Name,
		[]federation.SaveFragmentParams{{
			BundleID:      "remote-bundle-early-mismatch",
			DedupKey:      "remote-bundle-early-mismatch:frag:000001",
			CursorFrom:    30,
			CursorTo:      31,
			EventCount:    1,
			PayloadType:   "bundle",
			Compression:   federation.CompressionKindNone,
			IntegrityHash: signedSecond.IntegrityHash,
			AuthTag:       signedSecond.AuthTag,
			FragmentIndex: 1,
			FragmentCount: 2,
			Payload:       []byte("tampered"),
		}},
	)
	require.ErrorIs(t, err, federation.ErrUnauthorized)

	fragments, err := store.FragmentsByBundle(
		context.Background(),
		"remote-bundle-early-mismatch",
		federation.BundleDirectionInbound,
	)
	require.NoError(t, err)
	require.Len(t, fragments, 1)
	require.Equal(t, federation.FragmentStateFailed, fragments[0].State)

	savedLink, err := service.LinkByID(context.Background(), link.ID)
	require.NoError(t, err)
	require.Equal(t, federation.LinkStateDegraded, savedLink.State)
	require.Contains(t, savedLink.LastError, federation.ErrUnauthorized.Error())
}
