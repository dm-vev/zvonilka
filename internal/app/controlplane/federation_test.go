package controlplane

import (
	"context"
	"testing"
	"time"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	domainfederation "github.com/dm-vev/zvonilka/internal/domain/federation"
	federationtest "github.com/dm-vev/zvonilka/internal/domain/federation/teststore"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func newTestFederationAPI(t *testing.T) (*api, *recordingSender) {
	t.Helper()

	api, sender := newTestAdminAPI(t)
	federationService, err := domainfederation.NewService(
		federationtest.NewMemoryStore(),
		domainfederation.WithNow(func() time.Time {
			return time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC)
		}),
	)
	require.NoError(t, err)

	api.federation = federationService
	return api, sender
}

func federationPeerContext(serverName string, sharedSecret string) context.Context {
	return metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs(
			"authorization", "Bearer "+sharedSecret,
			federationServerNameMetadataKey, serverName,
		),
	)
}

func TestFederationServicePeerAndLinkLifecycle(t *testing.T) {
	t.Parallel()

	api, sender := newTestFederationAPI(t)
	admin := mustCreateAccount(t, api, domainidentity.CreateAccountParams{
		Username:    "federation-admin",
		DisplayName: "Federation Admin",
		Email:       "federation-admin@example.com",
		Roles:       []domainidentity.Role{domainidentity.RoleAdmin},
		AccountKind: domainidentity.AccountKindUser,
		CreatedBy:   "bootstrap",
	})
	adminCtx := mustLoginAccount(t, api, sender, admin.Username)

	createdPeer, err := api.CreatePeer(adminCtx, &federationv1.CreatePeerRequest{
		ServerName: "alpha.example",
		BaseUrl:    "https://alpha.example",
		Trusted:    true,
		Capabilities: []commonv1.FederationCapability{
			commonv1.FederationCapability_FEDERATION_CAPABILITY_EVENT_REPLICATION,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, createdPeer.GetSharedSecret())
	require.NotEmpty(t, createdPeer.GetSigningSecret())
	require.NotNil(t, createdPeer.GetPeer())
	require.Equal(t, federationv1.PeerState_PEER_STATE_ACTIVE, createdPeer.GetPeer().GetState())
	require.NotEmpty(t, createdPeer.GetPeer().GetSigningFingerprint())
	require.Equal(t, uint64(1), createdPeer.GetPeer().GetSigningKeyVersion())

	listedPeers, err := api.ListPeers(adminCtx, &federationv1.ListPeersRequest{})
	require.NoError(t, err)
	require.Len(t, listedPeers.GetPeers(), 1)

	createdLink, err := api.CreateLink(adminCtx, &federationv1.CreateLinkRequest{
		PeerId:           createdPeer.GetPeer().GetPeerId(),
		Name:             "primary",
		TransportKind:    federationv1.TransportKind_TRANSPORT_KIND_HTTPS,
		DeliveryClass:    federationv1.DeliveryClass_DELIVERY_CLASS_REALTIME,
		DiscoveryMode:    federationv1.DiscoveryMode_DISCOVERY_MODE_MANUAL,
		MediaPolicy:      federationv1.MediaPolicy_MEDIA_POLICY_REFERENCE_PROXY,
		MaxBundleBytes:   4096,
		MaxFragmentBytes: 1024,
	})
	require.NoError(t, err)
	require.NotEmpty(t, createdPeer.GetSigningSecret())
	require.NotNil(t, createdLink.GetLink())
	require.Equal(t, createdPeer.GetPeer().GetPeerId(), createdLink.GetLink().GetPeerId())

	paused, err := api.PauseLink(adminCtx, &federationv1.PauseLinkRequest{LinkId: createdLink.GetLink().GetLinkId()})
	require.NoError(t, err)
	require.Equal(t, federationv1.LinkState_LINK_STATE_PAUSED, paused.GetLink().GetState())

	resumed, err := api.ResumeLink(adminCtx, &federationv1.ResumeLinkRequest{LinkId: createdLink.GetLink().GetLinkId()})
	require.NoError(t, err)
	require.Equal(t, federationv1.LinkState_LINK_STATE_ACTIVE, resumed.GetLink().GetState())

	cursor, err := api.GetReplicationCursor(adminCtx, &federationv1.GetReplicationCursorRequest{
		PeerId: createdPeer.GetPeer().GetPeerId(),
		LinkId: createdLink.GetLink().GetLinkId(),
	})
	require.NoError(t, err)
	require.Equal(t, uint64(0), cursor.GetCursor().GetLastReceivedCursor())
	require.Equal(t, uint64(0), cursor.GetCursor().GetLastInboundCursor())
	require.Equal(t, uint64(0), cursor.GetCursor().GetLastOutboundCursor())

	deleted, err := api.DeleteLink(adminCtx, &federationv1.DeleteLinkRequest{LinkId: createdLink.GetLink().GetLinkId()})
	require.NoError(t, err)
	require.Equal(t, federationv1.LinkState_LINK_STATE_DELETED, deleted.GetLink().GetState())
}

func TestFederationServiceBundleReplicationFlow(t *testing.T) {
	t.Parallel()

	api, sender := newTestFederationAPI(t)
	admin := mustCreateAccount(t, api, domainidentity.CreateAccountParams{
		Username:    "bundle-admin",
		DisplayName: "Bundle Admin",
		Email:       "bundle-admin@example.com",
		Roles:       []domainidentity.Role{domainidentity.RoleAdmin},
		AccountKind: domainidentity.AccountKindUser,
		CreatedBy:   "bootstrap",
	})
	adminCtx := mustLoginAccount(t, api, sender, admin.Username)

	createdPeer, err := api.CreatePeer(adminCtx, &federationv1.CreatePeerRequest{
		ServerName: "beta.example",
		BaseUrl:    "https://beta.example",
		Trusted:    true,
		Capabilities: []commonv1.FederationCapability{
			commonv1.FederationCapability_FEDERATION_CAPABILITY_EVENT_REPLICATION,
		},
	})
	require.NoError(t, err)

	createdLink, err := api.CreateLink(adminCtx, &federationv1.CreateLinkRequest{
		PeerId:           createdPeer.GetPeer().GetPeerId(),
		Name:             "primary",
		TransportKind:    federationv1.TransportKind_TRANSPORT_KIND_HTTPS,
		DeliveryClass:    federationv1.DeliveryClass_DELIVERY_CLASS_REALTIME,
		DiscoveryMode:    federationv1.DiscoveryMode_DISCOVERY_MODE_MANUAL,
		MediaPolicy:      federationv1.MediaPolicy_MEDIA_POLICY_REFERENCE_PROXY,
		MaxBundleBytes:   4096,
		MaxFragmentBytes: 1024,
	})
	require.NoError(t, err)

	outbound, err := api.federation.QueueOutboundBundle(context.Background(), domainfederation.SaveBundleParams{
		PeerID:      createdPeer.GetPeer().GetPeerId(),
		LinkID:      createdLink.GetLink().GetLinkId(),
		DedupKey:    "outbound-1",
		CursorFrom:  1,
		CursorTo:    3,
		EventCount:  2,
		PayloadType: "bundle",
		Payload:     []byte("hello beta"),
	})
	require.NoError(t, err)

	peerCtx := federationPeerContext(createdPeer.GetPeer().GetServerName(), createdPeer.GetSharedSecret())

	pulled, err := api.PullBundles(peerCtx, &federationv1.PullBundlesRequest{
		ServerName:  createdPeer.GetPeer().GetServerName(),
		LinkName:    createdLink.GetLink().GetName(),
		AfterCursor: 0,
		Limit:       10,
	})
	require.NoError(t, err)
	require.Len(t, pulled.GetBundles(), 1)
	require.Equal(t, outbound.ID, pulled.GetBundles()[0].GetBundleId())
	require.NotEmpty(t, pulled.GetBundles()[0].GetIntegrityHash())
	require.NotEmpty(t, pulled.GetBundles()[0].GetAuthTag())
	require.Equal(t, uint64(3), pulled.GetNextCursor())
	require.False(t, pulled.GetHasMore())

	signedInbound, err := api.federation.SignBundle(context.Background(), createdPeer.GetPeer().GetPeerId(), createdLink.GetLink().GetLinkId(), domainfederation.Bundle{
		ID:          "remote-bundle-1",
		DedupKey:    "inbound-1",
		CursorFrom:  10,
		CursorTo:    11,
		EventCount:  1,
		PayloadType: "bundle",
		Payload:     []byte("hello local"),
		Compression: domainfederation.CompressionKindNone,
	})
	require.NoError(t, err)

	acked, err := api.AcknowledgeBundles(peerCtx, &federationv1.AcknowledgeBundlesRequest{
		ServerName: createdPeer.GetPeer().GetServerName(),
		LinkName:   createdLink.GetLink().GetName(),
		UpToCursor: 3,
		BundleIds:  []string{outbound.ID},
	})
	require.NoError(t, err)
	require.Equal(t, uint64(3), acked.GetCursor().GetLastAckedCursor())
	require.Equal(t, []string{outbound.ID}, acked.GetAcknowledgedBundleIds())

	pushed, err := api.PushBundles(peerCtx, &federationv1.PushBundlesRequest{
		ServerName: createdPeer.GetPeer().GetServerName(),
		LinkName:   createdLink.GetLink().GetName(),
		Bundles: []*federationv1.Bundle{
			{
				DedupKey:      "inbound-1",
				CursorFrom:    10,
				CursorTo:      11,
				EventCount:    1,
				PayloadType:   "bundle",
				Payload:       []byte("hello local"),
				IntegrityHash: signedInbound.IntegrityHash,
				AuthTag:       signedInbound.AuthTag,
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, pushed.GetAcceptedBundleIds(), 1)
	require.Equal(t, uint64(11), pushed.GetCursor().GetLastReceivedCursor())
	require.Equal(t, uint64(0), pushed.GetCursor().GetLastInboundCursor())

	cursor, err := api.GetReplicationCursor(adminCtx, &federationv1.GetReplicationCursorRequest{
		PeerId: createdPeer.GetPeer().GetPeerId(),
		LinkId: createdLink.GetLink().GetLinkId(),
	})
	require.NoError(t, err)
	require.Equal(t, uint64(11), cursor.GetCursor().GetLastReceivedCursor())
	require.Equal(t, uint64(0), cursor.GetCursor().GetLastInboundCursor())
	require.Equal(t, uint64(3), cursor.GetCursor().GetLastOutboundCursor())
	require.Equal(t, uint64(3), cursor.GetCursor().GetLastAckedCursor())
}

func TestFederationServiceRejectsInvalidPeerSecret(t *testing.T) {
	t.Parallel()

	api, sender := newTestFederationAPI(t)
	admin := mustCreateAccount(t, api, domainidentity.CreateAccountParams{
		Username:    "auth-admin",
		DisplayName: "Auth Admin",
		Email:       "auth-admin@example.com",
		Roles:       []domainidentity.Role{domainidentity.RoleAdmin},
		AccountKind: domainidentity.AccountKindUser,
		CreatedBy:   "bootstrap",
	})
	adminCtx := mustLoginAccount(t, api, sender, admin.Username)

	createdPeer, err := api.CreatePeer(adminCtx, &federationv1.CreatePeerRequest{
		ServerName: "gamma.example",
		BaseUrl:    "https://gamma.example",
		Trusted:    true,
	})
	require.NoError(t, err)

	createdLink, err := api.CreateLink(adminCtx, &federationv1.CreateLinkRequest{
		PeerId:           createdPeer.GetPeer().GetPeerId(),
		Name:             "primary",
		TransportKind:    federationv1.TransportKind_TRANSPORT_KIND_HTTPS,
		DeliveryClass:    federationv1.DeliveryClass_DELIVERY_CLASS_REALTIME,
		DiscoveryMode:    federationv1.DiscoveryMode_DISCOVERY_MODE_MANUAL,
		MediaPolicy:      federationv1.MediaPolicy_MEDIA_POLICY_REFERENCE_PROXY,
		MaxBundleBytes:   4096,
		MaxFragmentBytes: 1024,
	})
	require.NoError(t, err)

	_, err = api.PullBundles(
		federationPeerContext(createdPeer.GetPeer().GetServerName(), "wrong-secret"),
		&federationv1.PullBundlesRequest{
			ServerName: createdPeer.GetPeer().GetServerName(),
			LinkName:   createdLink.GetLink().GetName(),
		},
	)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unauthenticated, st.Code())
}

func TestFederationServiceRotatesPeerSigningKey(t *testing.T) {
	t.Parallel()

	api, sender := newTestFederationAPI(t)
	admin := mustCreateAccount(t, api, domainidentity.CreateAccountParams{
		Username:    "rotate-admin",
		DisplayName: "Rotate Admin",
		Email:       "rotate-admin@example.com",
		Roles:       []domainidentity.Role{domainidentity.RoleAdmin},
		AccountKind: domainidentity.AccountKindUser,
		CreatedBy:   "bootstrap",
	})
	adminCtx := mustLoginAccount(t, api, sender, admin.Username)

	createdPeer, err := api.CreatePeer(adminCtx, &federationv1.CreatePeerRequest{
		ServerName: "delta.example",
		BaseUrl:    "https://delta.example",
		Trusted:    true,
	})
	require.NoError(t, err)

	rotated, err := api.RotatePeerSigningKey(adminCtx, &federationv1.RotatePeerSigningKeyRequest{
		PeerId: createdPeer.GetPeer().GetPeerId(),
	})
	require.NoError(t, err)
	require.NotEmpty(t, rotated.GetSigningSecret())
	require.NotEqual(t, createdPeer.GetSigningSecret(), rotated.GetSigningSecret())
	require.Equal(t, uint64(2), rotated.GetPeer().GetSigningKeyVersion())
	require.NotEqual(t, createdPeer.GetPeer().GetSigningFingerprint(), rotated.GetPeer().GetSigningFingerprint())
}
