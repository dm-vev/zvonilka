package e2ee_test

import (
	"context"
	"strings"
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

func TestRotateDeviceKeysExpiresPendingArtifacts(t *testing.T) {
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

	now := time.Now().UTC()
	_, err = service.UploadDevicePreKeys(ctx, domaine2ee.UploadDevicePreKeysParams{
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		SignedPreKey: domaine2ee.SignedPreKey{
			Key:       domaine2ee.PublicKey{KeyID: "spk-a1", Algorithm: "x25519", PublicKey: []byte("spk-a1"), CreatedAt: now},
			Signature: []byte("sig-a1"),
		},
		OneTimePreKeys: []domaine2ee.OneTimePreKey{
			{Key: domaine2ee.PublicKey{KeyID: "otk-a1", Algorithm: "x25519", PublicKey: []byte("otk-a1"), CreatedAt: now}},
		},
	})
	if err != nil {
		t.Fatalf("upload alice prekeys: %v", err)
	}
	_, err = service.UploadDevicePreKeys(ctx, domaine2ee.UploadDevicePreKeysParams{
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		SignedPreKey: domaine2ee.SignedPreKey{
			Key:       domaine2ee.PublicKey{KeyID: "spk-b1", Algorithm: "x25519", PublicKey: []byte("spk-b1"), CreatedAt: now},
			Signature: []byte("sig-b1"),
		},
		OneTimePreKeys: []domaine2ee.OneTimePreKey{
			{Key: domaine2ee.PublicKey{KeyID: "otk-b1", Algorithm: "x25519", PublicKey: []byte("otk-b1"), CreatedAt: now}},
		},
	})
	if err != nil {
		t.Fatalf("upload bob prekeys: %v", err)
	}

	groupService, err := conversation.NewService(chats)
	if err != nil {
		t.Fatalf("new conversation service: %v", err)
	}
	group, _, err := groupService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-a",
		Kind:             conversation.ConversationKindGroup,
		Title:            "Rotate group",
		MemberAccountIDs: []string{"acc-b"},
		CreatedAt:        now,
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	created, err := service.CreateDirectSessions(ctx, domaine2ee.CreateDirectSessionsParams{
		InitiatorAccountID: "acc-a",
		InitiatorDeviceID:  "dev-a",
		TargetAccountID:    "acc-b",
		InitiatorEphemeral: domaine2ee.PublicKey{KeyID: "eph-a1", Algorithm: "x25519", PublicKey: []byte("eph-a1"), CreatedAt: now},
		Bootstrap:          domaine2ee.BootstrapPayload{Algorithm: "x3dh-v1", Ciphertext: []byte("cipher")},
	})
	if err != nil {
		t.Fatalf("create direct session: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected one direct session, got %d", len(created))
	}

	published, err := service.PublishGroupSenderKeys(ctx, domaine2ee.PublishGroupSenderKeysParams{
		ConversationID:  group.ID,
		SenderAccountID: "acc-a",
		SenderDeviceID:  "dev-a",
		SenderKeyID:     "sender-key-a1",
		Recipients: []domaine2ee.RecipientSenderKey{
			{
				RecipientAccountID: "acc-b",
				RecipientDeviceID:  "dev-b",
				Payload:            domaine2ee.SenderKeyPayload{Algorithm: "sender-key-v1", Ciphertext: []byte("payload")},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish sender key: %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("expected one sender-key distribution, got %d", len(published))
	}

	_, expiredDirect, expiredGroup, err := service.RotateDeviceKeys(ctx, domaine2ee.RotateDeviceKeysParams{
		AccountID:                    "acc-a",
		DeviceID:                     "dev-a",
		SignedPreKey:                 domaine2ee.SignedPreKey{Key: domaine2ee.PublicKey{KeyID: "spk-a2", Algorithm: "x25519", PublicKey: []byte("spk-a2"), CreatedAt: now.Add(time.Minute)}, Signature: []byte("sig-a2")},
		OneTimePreKeys:               []domaine2ee.OneTimePreKey{{Key: domaine2ee.PublicKey{KeyID: "otk-a2", Algorithm: "x25519", PublicKey: []byte("otk-a2"), CreatedAt: now.Add(time.Minute)}}},
		ReplaceOneTimePreKey:         true,
		ExpirePendingDirectSessions:  true,
		ExpirePendingGroupSenderKeys: true,
	})
	if err != nil {
		t.Fatalf("rotate device keys: %v", err)
	}
	if expiredDirect != 1 {
		t.Fatalf("expected one expired direct session, got %d", expiredDirect)
	}
	if expiredGroup != 1 {
		t.Fatalf("expected one expired group sender key, got %d", expiredGroup)
	}

	listed, err := service.ListDeviceSessions(ctx, domaine2ee.ListDeviceSessionsParams{
		AccountID:           "acc-b",
		DeviceID:            "dev-b",
		IncludeAcknowledged: true,
	})
	if err != nil {
		t.Fatalf("list direct sessions: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected expired direct sessions to disappear, got %+v", listed)
	}

	distributions, err := service.ListGroupSenderKeys(ctx, domaine2ee.ListGroupSenderKeysParams{
		ConversationID:      group.ID,
		RecipientAccountID:  "acc-b",
		RecipientDeviceID:   "dev-b",
		IncludeAcknowledged: true,
	})
	if err != nil {
		t.Fatalf("list group sender keys: %v", err)
	}
	if len(distributions) != 0 {
		t.Fatalf("expected expired sender-key distributions to disappear, got %+v", distributions)
	}
}

func TestSetAndListDeviceTrust(t *testing.T) {
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

	trust, err := service.SetDeviceTrust(ctx, domaine2ee.SetDeviceTrustParams{
		ObserverAccountID: "acc-a",
		ObserverDeviceID:  "dev-a",
		TargetAccountID:   "acc-b",
		TargetDeviceID:    "dev-b",
		State:             domaine2ee.DeviceTrustStateTrusted,
		Note:              "verified out of band",
	})
	if err != nil {
		t.Fatalf("set device trust: %v", err)
	}
	if trust.State != domaine2ee.DeviceTrustStateTrusted {
		t.Fatalf("unexpected trust state: %+v", trust)
	}
	if trust.KeyFingerprint == "" {
		t.Fatal("expected key fingerprint")
	}

	listed, err := service.ListDeviceTrusts(ctx, domaine2ee.ListDeviceTrustsParams{
		ObserverAccountID: "acc-a",
		ObserverDeviceID:  "dev-a",
		TargetAccountID:   "acc-b",
	})
	if err != nil {
		t.Fatalf("list device trusts: %v", err)
	}
	if len(listed) != 1 || listed[0].TargetDeviceID != "dev-b" {
		t.Fatalf("unexpected trust list: %+v", listed)
	}
}

func TestGetConversationKeyCoverage(t *testing.T) {
	ctx := context.Background()
	directory := identitytest.NewMemoryStore()
	chats := conversationtest.NewMemoryStore()
	e2eeStore := teststore.NewMemoryStore()
	service, err := domaine2ee.NewService(e2eeStore, directory, chats)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	now := time.Now().UTC()
	seedIdentity(t, ctx, directory, "acc-a", "dev-a", "device-key-a")
	seedIdentity(t, ctx, directory, "acc-b", "dev-b", "device-key-b")
	seedIdentity(t, ctx, directory, "acc-c", "dev-c", "device-key-c")

	_, err = service.UploadDevicePreKeys(ctx, domaine2ee.UploadDevicePreKeysParams{
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		SignedPreKey: domaine2ee.SignedPreKey{
			Key:       domaine2ee.PublicKey{KeyID: "spk-b", Algorithm: "x25519", PublicKey: []byte("spk-b"), CreatedAt: now},
			Signature: []byte("sig-b"),
		},
	})
	if err != nil {
		t.Fatalf("upload bob prekeys: %v", err)
	}

	conversationService, err := conversation.NewService(chats)
	if err != nil {
		t.Fatalf("new conversation service: %v", err)
	}
	direct, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-a",
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{"acc-b"},
		CreatedAt:        now,
	})
	if err != nil {
		t.Fatalf("create direct: %v", err)
	}
	group, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-a",
		Kind:             conversation.ConversationKindGroup,
		Title:            "Coverage group",
		MemberAccountIDs: []string{"acc-b", "acc-c"},
		CreatedAt:        now,
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	sessions, err := service.CreateDirectSessions(ctx, domaine2ee.CreateDirectSessionsParams{
		InitiatorAccountID: "acc-a",
		InitiatorDeviceID:  "dev-a",
		TargetAccountID:    "acc-b",
		InitiatorEphemeral: domaine2ee.PublicKey{KeyID: "eph", Algorithm: "x25519", PublicKey: []byte("eph"), CreatedAt: now},
		Bootstrap:          domaine2ee.BootstrapPayload{Algorithm: "x3dh-v1", Ciphertext: []byte("cipher")},
	})
	if err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	_, err = service.AcknowledgeDirectSession(ctx, domaine2ee.AcknowledgeDirectSessionParams{
		SessionID:          sessions[0].ID,
		RecipientAccountID: "acc-b",
		RecipientDeviceID:  "dev-b",
	})
	if err != nil {
		t.Fatalf("ack session: %v", err)
	}

	_, err = service.PublishGroupSenderKeys(ctx, domaine2ee.PublishGroupSenderKeysParams{
		ConversationID:  group.ID,
		SenderAccountID: "acc-a",
		SenderDeviceID:  "dev-a",
		SenderKeyID:     "sender-key-coverage",
		Recipients: []domaine2ee.RecipientSenderKey{
			{
				RecipientAccountID: "acc-b",
				RecipientDeviceID:  "dev-b",
				Payload:            domaine2ee.SenderKeyPayload{Algorithm: "sender-key-v1", Ciphertext: []byte("payload-b")},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish sender keys: %v", err)
	}

	directCoverage, err := service.GetConversationKeyCoverage(ctx, domaine2ee.GetConversationKeyCoverageParams{
		ConversationID:  direct.ID,
		SenderAccountID: "acc-a",
		SenderDeviceID:  "dev-a",
	})
	if err != nil {
		t.Fatalf("direct coverage: %v", err)
	}
	if len(directCoverage) != 1 || directCoverage[0].State != domaine2ee.ConversationKeyCoverageStateReady {
		t.Fatalf("unexpected direct coverage: %+v", directCoverage)
	}
	if directCoverage[0].TrustState != domaine2ee.DeviceTrustStateUnspecified || !directCoverage[0].VerificationRequired || directCoverage[0].KeyFingerprint == "" {
		t.Fatalf("unexpected direct verification hints: %+v", directCoverage[0])
	}

	_, err = service.SetDeviceTrust(ctx, domaine2ee.SetDeviceTrustParams{
		ObserverAccountID: "acc-a",
		ObserverDeviceID:  "dev-a",
		TargetAccountID:   "acc-b",
		TargetDeviceID:    "dev-b",
		State:             domaine2ee.DeviceTrustStateTrusted,
	})
	if err != nil {
		t.Fatalf("set direct trust: %v", err)
	}

	directCoverage, err = service.GetConversationKeyCoverage(ctx, domaine2ee.GetConversationKeyCoverageParams{
		ConversationID:  direct.ID,
		SenderAccountID: "acc-a",
		SenderDeviceID:  "dev-a",
	})
	if err != nil {
		t.Fatalf("direct coverage after trust: %v", err)
	}
	if directCoverage[0].TrustState != domaine2ee.DeviceTrustStateTrusted || directCoverage[0].VerificationRequired {
		t.Fatalf("unexpected direct trust coverage after trust: %+v", directCoverage[0])
	}

	groupCoverage, err := service.GetConversationKeyCoverage(ctx, domaine2ee.GetConversationKeyCoverageParams{
		ConversationID:  group.ID,
		SenderAccountID: "acc-a",
		SenderDeviceID:  "dev-a",
		SenderKeyID:     "sender-key-coverage",
	})
	if err != nil {
		t.Fatalf("group coverage: %v", err)
	}
	if len(groupCoverage) != 2 {
		t.Fatalf("expected two group coverage entries, got %d", len(groupCoverage))
	}
	if groupCoverage[0].State == domaine2ee.ConversationKeyCoverageStateMissing && groupCoverage[1].State == domaine2ee.ConversationKeyCoverageStateMissing {
		t.Fatalf("expected at least one non-missing group coverage entry: %+v", groupCoverage)
	}
	seenUntrusted := false
	for _, entry := range groupCoverage {
		if entry.KeyFingerprint == "" {
			t.Fatalf("expected key fingerprint in coverage entry: %+v", entry)
		}
		if entry.VerificationRequired {
			seenUntrusted = true
		}
	}
	if !seenUntrusted {
		t.Fatalf("expected verification-required group coverage: %+v", groupCoverage)
	}
}

func TestGetAndVerifyDeviceSafetyNumber(t *testing.T) {
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

	code, err := service.GetDeviceVerificationCode(ctx, domaine2ee.GetDeviceVerificationCodeParams{
		ObserverAccountID: "acc-a",
		ObserverDeviceID:  "dev-a",
		TargetAccountID:   "acc-b",
		TargetDeviceID:    "dev-b",
	})
	if err != nil {
		t.Fatalf("get device verification code: %v", err)
	}
	if code.SafetyNumber == "" || code.TargetKeyFingerprint == "" {
		t.Fatalf("unexpected verification code: %+v", code)
	}

	trust, err := service.VerifyDeviceSafetyNumber(ctx, domaine2ee.VerifyDeviceSafetyNumberParams{
		ObserverAccountID: "acc-a",
		ObserverDeviceID:  "dev-a",
		TargetAccountID:   "acc-b",
		TargetDeviceID:    "dev-b",
		SafetyNumber:      strings.ReplaceAll(code.SafetyNumber, "-", ""),
		Note:              "verified via qr",
	})
	if err != nil {
		t.Fatalf("verify safety number: %v", err)
	}
	if trust.State != domaine2ee.DeviceTrustStateTrusted {
		t.Fatalf("unexpected trust after verification: %+v", trust)
	}
}

func TestListVerificationRequiredDevices(t *testing.T) {
	ctx := context.Background()
	directory := identitytest.NewMemoryStore()
	chats := conversationtest.NewMemoryStore()
	e2eeStore := teststore.NewMemoryStore()
	service, err := domaine2ee.NewService(e2eeStore, directory, chats)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	now := time.Now().UTC()
	seedIdentity(t, ctx, directory, "acc-a", "dev-a", "device-key-a")
	seedIdentity(t, ctx, directory, "acc-b", "dev-b", "device-key-b")
	seedIdentity(t, ctx, directory, "acc-c", "dev-c", "device-key-c")

	conversationService, err := conversation.NewService(chats)
	if err != nil {
		t.Fatalf("new conversation service: %v", err)
	}
	if _, _, err = conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-a",
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{"acc-b"},
		CreatedAt:        now,
	}); err != nil {
		t.Fatalf("create direct: %v", err)
	}
	group, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-a",
		Kind:             conversation.ConversationKindGroup,
		MemberAccountIDs: []string{"acc-b", "acc-c"},
		Title:            "verify queue",
		CreatedAt:        now,
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err = service.SetDeviceTrust(ctx, domaine2ee.SetDeviceTrustParams{
		ObserverAccountID: "acc-a",
		ObserverDeviceID:  "dev-a",
		TargetAccountID:   "acc-c",
		TargetDeviceID:    "dev-c",
		State:             domaine2ee.DeviceTrustStateCompromised,
	}); err != nil {
		t.Fatalf("set compromised trust: %v", err)
	}

	devices, err := service.ListVerificationRequiredDevices(ctx, domaine2ee.ListVerificationRequiredDevicesParams{
		ObserverAccountID: "acc-a",
		ObserverDeviceID:  "dev-a",
	})
	if err != nil {
		t.Fatalf("list verification required devices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected two devices requiring verification, got %+v", devices)
	}
	if devices[0].AccountID != "acc-b" || !devices[0].DirectConversation {
		t.Fatalf("expected direct conversation device first, got %+v", devices[0])
	}
	if devices[1].AccountID != "acc-c" || devices[1].TrustState != domaine2ee.DeviceTrustStateCompromised || len(devices[1].ConversationIDs) != 1 || devices[1].ConversationIDs[0] != group.ID {
		t.Fatalf("unexpected group verification queue entry: %+v", devices[1])
	}
}

func TestValidateConversationPayloadRejectsCompromisedTrust(t *testing.T) {
	ctx := context.Background()
	directory := identitytest.NewMemoryStore()
	chats := conversationtest.NewMemoryStore()
	e2eeStore := teststore.NewMemoryStore()
	service, err := domaine2ee.NewService(e2eeStore, directory, chats)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	now := time.Now().UTC()
	seedIdentity(t, ctx, directory, "acc-a", "dev-a", "device-key-a")
	seedIdentity(t, ctx, directory, "acc-b", "dev-b", "device-key-b")
	if _, err = service.UploadDevicePreKeys(ctx, domaine2ee.UploadDevicePreKeysParams{
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		SignedPreKey: domaine2ee.SignedPreKey{
			Key:       domaine2ee.PublicKey{KeyID: "spk-b", Algorithm: "x25519", PublicKey: []byte("spk-b"), CreatedAt: now},
			Signature: []byte("sig-b"),
		},
		OneTimePreKeys: []domaine2ee.OneTimePreKey{
			{Key: domaine2ee.PublicKey{KeyID: "otk-b", Algorithm: "x25519", PublicKey: []byte("otk-b"), CreatedAt: now}},
		},
	}); err != nil {
		t.Fatalf("upload bob prekeys: %v", err)
	}

	conversationService, err := conversation.NewService(chats)
	if err != nil {
		t.Fatalf("new conversation service: %v", err)
	}
	direct, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-a",
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{"acc-b"},
		CreatedAt:        now,
		Settings: conversation.ConversationSettings{
			RequireEncryptedMessages: true,
		},
	})
	if err != nil {
		t.Fatalf("create direct: %v", err)
	}

	sessions, err := service.CreateDirectSessions(ctx, domaine2ee.CreateDirectSessionsParams{
		InitiatorAccountID: "acc-a",
		InitiatorDeviceID:  "dev-a",
		TargetAccountID:    "acc-b",
		InitiatorEphemeral: domaine2ee.PublicKey{KeyID: "eph", Algorithm: "x25519", PublicKey: []byte("eph"), CreatedAt: now},
		Bootstrap:          domaine2ee.BootstrapPayload{Algorithm: "x3dh-v1", Ciphertext: []byte("cipher")},
	})
	if err != nil {
		t.Fatalf("create direct sessions: %v", err)
	}
	if _, err = service.AcknowledgeDirectSession(ctx, domaine2ee.AcknowledgeDirectSessionParams{
		SessionID:          sessions[0].ID,
		RecipientAccountID: "acc-b",
		RecipientDeviceID:  "dev-b",
	}); err != nil {
		t.Fatalf("ack direct session: %v", err)
	}
	if _, err = service.SetDeviceTrust(ctx, domaine2ee.SetDeviceTrustParams{
		ObserverAccountID: "acc-a",
		ObserverDeviceID:  "dev-a",
		TargetAccountID:   "acc-b",
		TargetDeviceID:    "dev-b",
		State:             domaine2ee.DeviceTrustStateCompromised,
	}); err != nil {
		t.Fatalf("set trust: %v", err)
	}

	err = service.ValidateConversationPayload(ctx, domaine2ee.ValidateConversationPayloadParams{
		ConversationID:  direct.ID,
		SenderAccountID: "acc-a",
		SenderDeviceID:  "dev-a",
		PayloadKeyID:    sessions[0].ID,
		PayloadMetadata: map[string]string{"direct_session_id": sessions[0].ID},
	})
	if err != domaine2ee.ErrForbidden {
		t.Fatalf("expected forbidden for compromised trust, got %v", err)
	}
}

func TestValidateConversationPayloadRejectsMissingGroupCoverage(t *testing.T) {
	ctx := context.Background()
	directory := identitytest.NewMemoryStore()
	chats := conversationtest.NewMemoryStore()
	e2eeStore := teststore.NewMemoryStore()
	service, err := domaine2ee.NewService(e2eeStore, directory, chats)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	now := time.Now().UTC()
	seedIdentity(t, ctx, directory, "acc-a", "dev-a", "device-key-a")
	seedIdentity(t, ctx, directory, "acc-b", "dev-b", "device-key-b")
	seedIdentity(t, ctx, directory, "acc-c", "dev-c", "device-key-c")

	conversationService, err := conversation.NewService(chats)
	if err != nil {
		t.Fatalf("new conversation service: %v", err)
	}
	group, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-a",
		Kind:             conversation.ConversationKindGroup,
		MemberAccountIDs: []string{"acc-b", "acc-c"},
		CreatedAt:        now,
		Settings: conversation.ConversationSettings{
			RequireEncryptedMessages: true,
		},
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err = service.PublishGroupSenderKeys(ctx, domaine2ee.PublishGroupSenderKeysParams{
		ConversationID:  group.ID,
		SenderAccountID: "acc-a",
		SenderDeviceID:  "dev-a",
		SenderKeyID:     "sender-key-1",
		Recipients: []domaine2ee.RecipientSenderKey{
			{
				RecipientAccountID: "acc-b",
				RecipientDeviceID:  "dev-b",
				Payload:            domaine2ee.SenderKeyPayload{Algorithm: "sender-key-v1", Ciphertext: []byte("cipher")},
			},
		},
	}); err != nil {
		t.Fatalf("publish sender keys: %v", err)
	}

	err = service.ValidateConversationPayload(ctx, domaine2ee.ValidateConversationPayloadParams{
		ConversationID:  group.ID,
		SenderAccountID: "acc-a",
		SenderDeviceID:  "dev-a",
		PayloadKeyID:    "sender-key-1",
		PayloadMetadata: map[string]string{"sender_key_id": "sender-key-1"},
	})
	if err != domaine2ee.ErrConflict {
		t.Fatalf("expected conflict for missing group coverage, got %v", err)
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
