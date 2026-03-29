package e2ee_test

import (
	"context"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
	domaine2ee "github.com/dm-vev/zvonilka/internal/domain/e2ee"
	"github.com/dm-vev/zvonilka/internal/domain/e2ee/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestUploadAndClaimBundles(t *testing.T) {
	ctx := context.Background()
	directory := identitytest.NewMemoryStore()
	chats := conversationtest.NewMemoryStore()
	e2eeStore := teststore.NewMemoryStore()
	service, err := domaine2ee.NewService(e2eeStore, directory, chats)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	seedIdentity(t, ctx, directory, "acc-a", "dev-a", "device-key-a")
	seedIdentity(t, ctx, directory, "acc-b", "dev-b", "device-key-b")

	_, err = service.UploadDevicePreKeys(ctx, domaine2ee.UploadDevicePreKeysParams{
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		SignedPreKey: domaine2ee.SignedPreKey{
			Key: domaine2ee.PublicKey{
				KeyID:     "spk-1",
				Algorithm: "x25519",
				PublicKey: []byte("signed-prekey"),
				CreatedAt: time.Now().UTC(),
			},
			Signature: []byte("sig"),
		},
		OneTimePreKeys: []domaine2ee.OneTimePreKey{
			{Key: domaine2ee.PublicKey{KeyID: "otk-1", Algorithm: "x25519", PublicKey: []byte("otk-1"), CreatedAt: time.Now().UTC()}},
			{Key: domaine2ee.PublicKey{KeyID: "otk-2", Algorithm: "x25519", PublicKey: []byte("otk-2"), CreatedAt: time.Now().UTC()}},
		},
	})
	if err != nil {
		t.Fatalf("upload prekeys: %v", err)
	}

	bundles, err := service.GetAccountBundles(ctx, domaine2ee.FetchAccountBundlesParams{
		RequesterAccountID:   "acc-a",
		RequesterDeviceID:    "dev-a",
		TargetAccountID:      "acc-b",
		ConsumeOneTimePreKey: true,
	})
	if err != nil {
		t.Fatalf("claim bundles: %v", err)
	}
	if len(bundles) != 1 {
		t.Fatalf("expected one bundle, got %d", len(bundles))
	}
	if bundles[0].SignedPreKey.Key.KeyID != "spk-1" {
		t.Fatalf("unexpected signed prekey: %+v", bundles[0].SignedPreKey)
	}
	if bundles[0].OneTimePreKey.Key.KeyID == "" {
		t.Fatal("expected claimed one-time prekey")
	}
	if bundles[0].OneTimePreKeysAvail != 1 {
		t.Fatalf("expected remaining count 1, got %d", bundles[0].OneTimePreKeysAvail)
	}
}

func TestCreateListAndAcknowledgeDirectSessions(t *testing.T) {
	ctx := context.Background()
	directory := identitytest.NewMemoryStore()
	chats := conversationtest.NewMemoryStore()
	e2eeStore := teststore.NewMemoryStore()
	service, err := domaine2ee.NewService(e2eeStore, directory, chats)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	seedIdentity(t, ctx, directory, "acc-a", "dev-a", "device-key-a")
	seedIdentity(t, ctx, directory, "acc-b", "dev-b1", "device-key-b1")
	seedIdentity(t, ctx, directory, "acc-b", "dev-b2", "device-key-b2")

	for _, deviceID := range []string{"dev-b1", "dev-b2"} {
		_, err = service.UploadDevicePreKeys(ctx, domaine2ee.UploadDevicePreKeysParams{
			AccountID: "acc-b",
			DeviceID:  deviceID,
			SignedPreKey: domaine2ee.SignedPreKey{
				Key: domaine2ee.PublicKey{
					KeyID:     "spk-" + deviceID,
					Algorithm: "x25519",
					PublicKey: []byte("signed-" + deviceID),
					CreatedAt: time.Now().UTC(),
				},
				Signature: []byte("sig-" + deviceID),
			},
			OneTimePreKeys: []domaine2ee.OneTimePreKey{
				{Key: domaine2ee.PublicKey{KeyID: "otk-" + deviceID, Algorithm: "x25519", PublicKey: []byte("otk-" + deviceID), CreatedAt: time.Now().UTC()}},
			},
		})
		if err != nil {
			t.Fatalf("upload prekeys for %s: %v", deviceID, err)
		}
	}

	created, err := service.CreateDirectSessions(ctx, domaine2ee.CreateDirectSessionsParams{
		InitiatorAccountID: "acc-a",
		InitiatorDeviceID:  "dev-a",
		TargetAccountID:    "acc-b",
		InitiatorEphemeral: domaine2ee.PublicKey{
			KeyID:     "eph-1",
			Algorithm: "x25519",
			PublicKey: []byte("ephemeral"),
			CreatedAt: time.Now().UTC(),
		},
		Bootstrap: domaine2ee.BootstrapPayload{
			Algorithm:  "x3dh-v1",
			Nonce:      []byte("nonce"),
			Ciphertext: []byte("ciphertext"),
			Metadata:   map[string]string{"conversation_id": "conv-1"},
		},
	})
	if err != nil {
		t.Fatalf("create direct sessions: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected two created sessions, got %d", len(created))
	}
	if created[0].OneTimePreKey.Key.KeyID == "" && created[1].OneTimePreKey.Key.KeyID == "" {
		t.Fatal("expected at least one claimed one-time prekey")
	}

	listed, err := service.ListDeviceSessions(ctx, domaine2ee.ListDeviceSessionsParams{
		AccountID: "acc-b",
		DeviceID:  "dev-b1",
	})
	if err != nil {
		t.Fatalf("list device sessions: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one listed session, got %d", len(listed))
	}
	if listed[0].Bootstrap.Algorithm != "x3dh-v1" {
		t.Fatalf("unexpected bootstrap payload: %+v", listed[0].Bootstrap)
	}

	acknowledged, err := service.AcknowledgeDirectSession(ctx, domaine2ee.AcknowledgeDirectSessionParams{
		SessionID:          listed[0].ID,
		RecipientAccountID: "acc-b",
		RecipientDeviceID:  "dev-b1",
	})
	if err != nil {
		t.Fatalf("acknowledge direct session: %v", err)
	}
	if acknowledged.State != domaine2ee.DirectSessionStateAcknowledged {
		t.Fatalf("expected acknowledged state, got %s", acknowledged.State)
	}

	listed, err = service.ListDeviceSessions(ctx, domaine2ee.ListDeviceSessionsParams{
		AccountID:           "acc-b",
		DeviceID:            "dev-b1",
		IncludeAcknowledged: true,
	})
	if err != nil {
		t.Fatalf("list acknowledged session: %v", err)
	}
	if len(listed) != 1 || listed[0].State != domaine2ee.DirectSessionStateAcknowledged {
		t.Fatalf("unexpected acknowledged sessions: %+v", listed)
	}
}

func TestPublishListAndAcknowledgeGroupSenderKeys(t *testing.T) {
	ctx := context.Background()
	directory := identitytest.NewMemoryStore()
	chats := conversationtest.NewMemoryStore()
	e2eeStore := teststore.NewMemoryStore()
	service, err := domaine2ee.NewService(e2eeStore, directory, chats)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	seedIdentity(t, ctx, directory, "acc-a", "dev-a", "device-key-a")
	seedIdentity(t, ctx, directory, "acc-b", "dev-b", "device-key-b")

	conversationService, err := conversation.NewService(chats)
	if err != nil {
		t.Fatalf("new conversation service: %v", err)
	}
	group, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-a",
		Kind:             conversation.ConversationKindGroup,
		Title:            "Secret group",
		MemberAccountIDs: []string{"acc-b"},
		CreatedAt:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	published, err := service.PublishGroupSenderKeys(ctx, domaine2ee.PublishGroupSenderKeysParams{
		ConversationID:  group.ID,
		SenderAccountID: "acc-a",
		SenderDeviceID:  "dev-a",
		SenderKeyID:     "sender-key-1",
		Recipients: []domaine2ee.RecipientSenderKey{
			{
				RecipientAccountID: "acc-b",
				RecipientDeviceID:  "dev-b",
				Payload: domaine2ee.SenderKeyPayload{
					Algorithm:  "sender-key-v1",
					Nonce:      []byte("nonce"),
					Ciphertext: []byte("ciphertext"),
					Metadata:   map[string]string{"epoch": "1"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish group sender keys: %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("expected one distribution, got %d", len(published))
	}

	listed, err := service.ListGroupSenderKeys(ctx, domaine2ee.ListGroupSenderKeysParams{
		ConversationID:     group.ID,
		RecipientAccountID: "acc-b",
		RecipientDeviceID:  "dev-b",
	})
	if err != nil {
		t.Fatalf("list group sender keys: %v", err)
	}
	if len(listed) != 1 || listed[0].SenderKeyID != "sender-key-1" {
		t.Fatalf("unexpected listed distributions: %+v", listed)
	}

	acknowledged, err := service.AcknowledgeGroupSenderKey(ctx, domaine2ee.AcknowledgeGroupSenderKeyParams{
		DistributionID:     listed[0].ID,
		RecipientAccountID: "acc-b",
		RecipientDeviceID:  "dev-b",
	})
	if err != nil {
		t.Fatalf("ack group sender key: %v", err)
	}
	if acknowledged.State != domaine2ee.GroupSenderKeyStateAcknowledged {
		t.Fatalf("expected acknowledged state, got %s", acknowledged.State)
	}
}

func seedIdentity(t *testing.T, ctx context.Context, store identity.Store, accountID string, deviceID string, publicKey string) {
	t.Helper()
	if _, err := store.SaveAccount(ctx, identity.Account{
		ID:        accountID,
		Username:  accountID,
		Status:    identity.AccountStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save account: %v", err)
	}
	if _, err := store.SaveDevice(ctx, identity.Device{
		ID:        deviceID,
		AccountID: accountID,
		Name:      deviceID,
		Platform:  identity.DevicePlatformDesktop,
		Status:    identity.DeviceStatusActive,
		PublicKey: publicKey,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save device: %v", err)
	}
}
