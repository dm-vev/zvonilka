package federation_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/federation"
	federationtest "github.com/dm-vev/zvonilka/internal/domain/federation/teststore"
	identity "github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

type recordedPush struct {
	serverName string
	linkName   string
	bundles    []federation.Bundle
}

type recordedAck struct {
	serverName string
	linkName   string
	upToCursor uint64
	bundleIDs  []string
}

type stubReplicationClient struct {
	pushes []recordedPush
	acks   []recordedAck
	pull   []federation.Bundle
}

func (c *stubReplicationClient) PushBundles(
	_ context.Context,
	serverName string,
	linkName string,
	bundles []federation.Bundle,
) error {
	copied := make([]federation.Bundle, 0, len(bundles))
	for _, bundle := range bundles {
		copied = append(copied, bundle)
	}
	c.pushes = append(c.pushes, recordedPush{
		serverName: serverName,
		linkName:   linkName,
		bundles:    copied,
	})
	return nil
}

func (c *stubReplicationClient) PullBundles(
	_ context.Context,
	_ string,
	_ string,
	_ uint64,
	_ int,
) ([]federation.Bundle, bool, error) {
	copied := make([]federation.Bundle, 0, len(c.pull))
	for _, bundle := range c.pull {
		copied = append(copied, bundle)
	}
	return copied, false, nil
}

func (c *stubReplicationClient) AcknowledgeBundles(
	_ context.Context,
	serverName string,
	linkName string,
	upToCursor uint64,
	bundleIDs []string,
) error {
	c.acks = append(c.acks, recordedAck{
		serverName: serverName,
		linkName:   linkName,
		upToCursor: upToCursor,
		bundleIDs:  append([]string(nil), bundleIDs...),
	})
	return nil
}

func (c *stubReplicationClient) Close() error {
	return nil
}

func TestWorkerReplicatesOutboundAndInboundBundles(t *testing.T) {
	t.Parallel()

	federationStore := federationtest.NewMemoryStore()
	service, err := federation.NewService(federationStore, federation.WithNow(func() time.Time {
		return time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC)
	}))
	require.NoError(t, err)

	peer, _, _, err := service.CreatePeer(context.Background(), federation.CreatePeerParams{
		ServerName:   "beta.example",
		BaseURL:      "https://beta.example",
		Trusted:      true,
		SharedSecret: "beta-secret",
		Capabilities: []federation.Capability{federation.CapabilityEventReplication},
	})
	require.NoError(t, err)

	link, err := service.CreateLink(context.Background(), federation.CreateLinkParams{
		PeerID:                   peer.ID,
		Name:                     "primary",
		TransportKind:            federation.TransportKindHTTPS,
		DeliveryClass:            federation.DeliveryClassRealtime,
		DiscoveryMode:            federation.DiscoveryModeManual,
		MediaPolicy:              federation.MediaPolicyReferenceProxy,
		MaxBundleBytes:           4096,
		MaxFragmentBytes:         1024,
		AllowedConversationKinds: []federation.ConversationKind{federation.ConversationKindGroup},
	})
	require.NoError(t, err)

	conversations := conversationtest.NewMemoryStore()
	identities := identitytest.NewMemoryStore()
	_, err = conversations.SaveConversation(context.Background(), conversation.Conversation{
		ID:   "conv-1",
		Kind: conversation.ConversationKindGroup,
	})
	require.NoError(t, err)

	_, err = conversations.SaveEvent(context.Background(), conversation.EventEnvelope{
		EventID:        "evt-1",
		EventType:      conversation.EventTypeConversationUpdated,
		ConversationID: "conv-1",
		ActorAccountID: "actor-1",
	})
	require.NoError(t, err)
	_, err = conversations.SaveEvent(context.Background(), conversation.EventEnvelope{
		EventID:        "evt-2",
		EventType:      conversation.EventTypeConversationUpdated,
		ConversationID: "conv-1",
		ActorAccountID: "actor-1",
	})
	require.NoError(t, err)

	remotePayload, err := json.Marshal([]conversation.EventEnvelope{
		{
			EventID:        "remote-evt-1",
			EventType:      conversation.EventTypeConversationCreated,
			ConversationID: "remote-conv-1",
			ActorAccountID: "remote-owner",
			ActorDeviceID:  "remote-device-1",
			Metadata: map[string]string{
				"kind":  string(conversation.ConversationKindGroup),
				"title": "Remote Room",
			},
			CreatedAt: time.Date(2026, time.April, 9, 12, 5, 0, 0, time.UTC),
		},
		{
			EventID:        "remote-evt-2",
			EventType:      conversation.EventTypeConversationMembers,
			ConversationID: "remote-conv-1",
			ActorAccountID: "remote-owner",
			ActorDeviceID:  "remote-device-1",
			Metadata: map[string]string{
				"change":            "added",
				"target_account_id": "remote-member-1",
			},
			CreatedAt: time.Date(2026, time.April, 9, 12, 5, 30, 0, time.UTC),
		},
		{
			EventID:        "remote-evt-3",
			EventType:      conversation.EventTypeMessageCreated,
			ConversationID: "remote-conv-1",
			ActorAccountID: "remote-owner",
			ActorDeviceID:  "remote-device-1",
			MessageID:      "remote-msg-1",
			PayloadType:    "message",
			Payload: conversation.EncryptedPayload{
				Ciphertext: []byte("ciphertext"),
			},
			Metadata: map[string]string{
				"message_id":   "remote-msg-1",
				"message_kind": string(conversation.MessageKindText),
			},
			CreatedAt: time.Date(2026, time.April, 9, 12, 6, 0, 0, time.UTC),
		},
		{
			EventID:        "remote-evt-4",
			EventType:      conversation.EventTypeMessageReactionAdded,
			ConversationID: "remote-conv-1",
			ActorAccountID: "remote-member-1",
			ActorDeviceID:  "remote-device-2",
			MessageID:      "remote-msg-1",
			PayloadType:    "message",
			Metadata: map[string]string{
				"message_id": "remote-msg-1",
				"action":     "reaction",
				"reaction":   "fire",
			},
			CreatedAt: time.Date(2026, time.April, 9, 12, 6, 5, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	remoteClient := &stubReplicationClient{
		pull: []federation.Bundle{{
			ID:          "remote-bundle-1",
			DedupKey:    "out:beta.example:primary:00000000000000000010:00000000000000000010",
			CursorFrom:  10,
			CursorTo:    10,
			EventCount:  1,
			PayloadType: "conversation.events.v1",
			Payload:     remotePayload,
			Compression: federation.CompressionKindNone,
		}},
	}
	remoteClient.pull[0], err = service.SignBundle(context.Background(), peer.ID, link.ID, remoteClient.pull[0])
	require.NoError(t, err)

	worker, err := federation.NewWorker(
		service,
		identities,
		conversations,
		func(context.Context, federation.Peer, federation.Link) (federation.ReplicationClient, error) {
			return remoteClient, nil
		},
		federation.WorkerSettings{
			LocalServerName: "alpha.example",
			PollInterval:    time.Second,
			BatchSize:       10,
		},
	)
	require.NoError(t, err)

	err = worker.ProcessOnceForTests(context.Background())
	require.NoError(t, err)

	require.Len(t, remoteClient.pushes, 1)
	require.Equal(t, "alpha.example", remoteClient.pushes[0].serverName)
	require.Equal(t, "primary", remoteClient.pushes[0].linkName)
	require.Len(t, remoteClient.pushes[0].bundles, 1)
	require.Equal(t, uint64(2), remoteClient.pushes[0].bundles[0].CursorTo)

	require.Len(t, remoteClient.acks, 1)
	require.Equal(t, "alpha.example", remoteClient.acks[0].serverName)
	require.Equal(t, "primary", remoteClient.acks[0].linkName)
	require.Equal(t, uint64(10), remoteClient.acks[0].upToCursor)
	require.Equal(t, []string{"remote-bundle-1"}, remoteClient.acks[0].bundleIDs)

	cursor, err := service.ReplicationCursorByPeerAndLink(context.Background(), peer.ID, link.ID)
	require.NoError(t, err)
	require.Equal(t, uint64(10), cursor.LastReceivedCursor)
	require.Equal(t, uint64(10), cursor.LastInboundCursor)
	require.Equal(t, uint64(2), cursor.LastOutboundCursor)
	require.Equal(t, uint64(2), cursor.LastAckedCursor)

	inbound, err := service.BundlesAfter(context.Background(), peer.ID, link.ID, federation.BundleDirectionInbound, 0, 10)
	require.NoError(t, err)
	require.Len(t, inbound, 1)
	require.Equal(t, "out:beta.example:primary:00000000000000000010:00000000000000000010", inbound[0].DedupKey)
	require.Equal(t, uint64(10), inbound[0].CursorTo)

	outbound, err := service.BundlesAfter(context.Background(), peer.ID, link.ID, federation.BundleDirectionOutbound, 0, 10)
	require.NoError(t, err)
	require.Len(t, outbound, 1)
	require.Equal(t, federation.BundleStateAcknowledged, outbound[0].State)

	remoteConversationID := "fedconv:beta.example:remote-conv-1"
	remoteOwnerID := "fedacct:beta.example:remote-owner"
	remoteOwnerDeviceID := "feddev:beta.example:remote-device-1"
	remoteMemberID := "fedacct:beta.example:remote-member-1"
	remoteMemberDeviceID := "feddev:beta.example:remote-device-2"
	remoteMessageID := "fedmsg:beta.example:remote-msg-1"

	remoteConversation, err := conversations.ConversationByID(context.Background(), remoteConversationID)
	require.NoError(t, err)
	require.Equal(t, "Remote Room", remoteConversation.Title)
	require.Equal(t, remoteOwnerID, remoteConversation.OwnerAccountID)

	ownerMember, err := conversations.ConversationMemberByConversationAndAccount(
		context.Background(),
		remoteConversationID,
		remoteOwnerID,
	)
	require.NoError(t, err)
	require.Equal(t, conversation.MemberRoleOwner, ownerMember.Role)

	addedMember, err := conversations.ConversationMemberByConversationAndAccount(
		context.Background(),
		remoteConversationID,
		remoteMemberID,
	)
	require.NoError(t, err)
	require.Equal(t, conversation.MemberRoleMember, addedMember.Role)

	message, err := conversations.MessageByID(context.Background(), remoteConversationID, remoteMessageID)
	require.NoError(t, err)
	require.Equal(t, remoteOwnerID, message.SenderAccountID)
	require.Equal(t, remoteOwnerDeviceID, message.SenderDeviceID)
	require.Len(t, message.Reactions, 1)
	require.Equal(t, remoteMemberID, message.Reactions[0].AccountID)
	require.Equal(t, "fire", message.Reactions[0].Reaction)

	ownerAccount, err := identities.AccountByID(context.Background(), remoteOwnerID)
	require.NoError(t, err)
	require.Equal(t, identity.AccountStatusActive, ownerAccount.Status)

	_, err = identities.DeviceByID(context.Background(), remoteOwnerDeviceID)
	require.NoError(t, err)
	_, err = identities.DeviceByID(context.Background(), remoteMemberDeviceID)
	require.NoError(t, err)

	appliedEvents, err := conversations.EventsAfterSequence(context.Background(), 2, 10, []string{remoteConversationID})
	require.NoError(t, err)
	require.Len(t, appliedEvents, 4)
	require.Equal(t, "fedevt:beta.example:remote-evt-1", appliedEvents[0].EventID)
	require.Equal(t, remoteConversationID, appliedEvents[0].ConversationID)
}

func TestWorkerProcessesBridgeLinksWithoutDirectClientDial(t *testing.T) {
	t.Parallel()

	federationStore := federationtest.NewMemoryStore()
	service, err := federation.NewService(federationStore, federation.WithNow(func() time.Time {
		return time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC)
	}))
	require.NoError(t, err)

	peer, _, _, err := service.CreatePeer(context.Background(), federation.CreatePeerParams{
		ServerName:   "mesh.example",
		BaseURL:      "bridge://mesh.example",
		Trusted:      true,
		SharedSecret: "mesh-secret",
		Capabilities: []federation.Capability{federation.CapabilityEventReplication},
	})
	require.NoError(t, err)

	link, err := service.CreateLink(context.Background(), federation.CreateLinkParams{
		PeerID:                   peer.ID,
		Name:                     "mesh",
		Endpoint:                 "bridge://meshtastic.local",
		TransportKind:            federation.TransportKindMeshtastic,
		DeliveryClass:            federation.DeliveryClassUltraConstrained,
		DiscoveryMode:            federation.DiscoveryModeBridgeAnnounced,
		MediaPolicy:              federation.MediaPolicyDisabled,
		MaxBundleBytes:           1024,
		MaxFragmentBytes:         128,
		AllowedConversationKinds: []federation.ConversationKind{federation.ConversationKindGroup},
	})
	require.NoError(t, err)

	conversations := conversationtest.NewMemoryStore()
	identities := identitytest.NewMemoryStore()

	_, err = conversations.SaveConversation(context.Background(), conversation.Conversation{
		ID:   "conv-bridge-1",
		Kind: conversation.ConversationKindGroup,
	})
	require.NoError(t, err)
	_, err = conversations.SaveEvent(context.Background(), conversation.EventEnvelope{
		EventID:        "evt-bridge-1",
		EventType:      conversation.EventTypeConversationUpdated,
		ConversationID: "conv-bridge-1",
		ActorAccountID: "actor-1",
	})
	require.NoError(t, err)

	remotePayload, err := json.Marshal([]conversation.EventEnvelope{
		{
			EventID:        "remote-bridge-evt-1",
			EventType:      conversation.EventTypeConversationCreated,
			ConversationID: "remote-bridge-conv-1",
			ActorAccountID: "remote-owner",
			ActorDeviceID:  "remote-device-1",
			Metadata: map[string]string{
				"kind":  string(conversation.ConversationKindGroup),
				"title": "Mesh Room",
			},
			CreatedAt: time.Date(2026, time.April, 9, 12, 5, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	signedInbound, err := service.SignBundle(context.Background(), peer.ID, link.ID, federation.Bundle{
		ID:          "bridge-inbound-bundle-1",
		DedupKey:    "bridge-inbound-1",
		CursorFrom:  10,
		CursorTo:    10,
		EventCount:  1,
		PayloadType: "conversation.events.v1",
		Payload:     remotePayload,
		Compression: federation.CompressionKindNone,
	})
	require.NoError(t, err)

	_, err = service.AcceptInboundBundle(context.Background(), federation.SaveBundleParams{
		PeerID:        peer.ID,
		LinkID:        link.ID,
		DedupKey:      "bridge-inbound-1",
		CursorFrom:    10,
		CursorTo:      10,
		EventCount:    1,
		PayloadType:   "conversation.events.v1",
		Payload:       remotePayload,
		Compression:   federation.CompressionKindNone,
		IntegrityHash: signedInbound.IntegrityHash,
		AuthTag:       signedInbound.AuthTag,
	})
	require.NoError(t, err)

	worker, err := federation.NewWorker(
		service,
		identities,
		conversations,
		func(context.Context, federation.Peer, federation.Link) (federation.ReplicationClient, error) {
			t.Fatal("bridge link should not construct direct replication client")
			return nil, nil
		},
		federation.WorkerSettings{
			LocalServerName: "alpha.example",
			PollInterval:    time.Second,
			BatchSize:       10,
		},
	)
	require.NoError(t, err)

	err = worker.ProcessOnceForTests(context.Background())
	require.NoError(t, err)

	cursor, err := service.ReplicationCursorByPeerAndLink(context.Background(), peer.ID, link.ID)
	require.NoError(t, err)
	require.Equal(t, uint64(10), cursor.LastReceivedCursor)
	require.Equal(t, uint64(10), cursor.LastInboundCursor)
	require.Equal(t, uint64(1), cursor.LastOutboundCursor)
	require.Equal(t, uint64(0), cursor.LastAckedCursor)

	outbound, err := service.BundlesAfter(context.Background(), peer.ID, link.ID, federation.BundleDirectionOutbound, 0, 10)
	require.NoError(t, err)
	require.Len(t, outbound, 1)
	require.Equal(t, federation.BundleStateQueued, outbound[0].State)

	remoteConversation, err := conversations.ConversationByID(context.Background(), "fedconv:mesh.example:remote-bridge-conv-1")
	require.NoError(t, err)
	require.Equal(t, "Mesh Room", remoteConversation.Title)
}
