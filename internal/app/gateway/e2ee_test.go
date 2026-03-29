package gateway

import (
	"context"
	"testing"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	e2eev1 "github.com/dm-vev/zvonilka/gen/proto/contracts/e2ee/v1"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"google.golang.org/grpc/metadata"
)

func TestUploadAndFetchPreKeysRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-e2ee",
		DisplayName: "Alice E2EE",
		Email:       "alice-e2ee@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-e2ee",
		DisplayName: "Bob E2EE",
		Email:       "bob-e2ee@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	bobBegin, err := api.BeginLogin(ctx, &authv1.BeginLoginRequest{
		Identifier:      &authv1.BeginLoginRequest_Username{Username: bob.Username},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
		DeviceName:      "bob-phone",
		DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
	})
	if err != nil {
		t.Fatalf("begin bob login: %v", err)
	}
	bobVerify, err := api.VerifyLoginCode(ctx, &authv1.VerifyLoginCodeRequest{
		ChallengeId:    bobBegin.ChallengeId,
		Code:           sender.code(bobBegin.Targets[0].DestinationMask),
		DeviceName:     "bob-phone",
		DevicePlatform: commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		DeviceKey: &commonv1.PublicKeyBundle{
			PublicKey: []byte("bob-device-key"),
		},
	})
	if err != nil {
		t.Fatalf("verify bob login: %v", err)
	}
	bobCtx := metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+bobVerify.Tokens.AccessToken))

	upload, err := api.UploadDevicePreKeys(bobCtx, &e2eev1.UploadDevicePreKeysRequest{
		SignedPrekey: &e2eev1.SignedPreKey{
			Key: &commonv1.PublicKeyBundle{
				KeyId:     "spk-1",
				Algorithm: "x25519",
				PublicKey: []byte("signed-prekey"),
			},
			Signature: []byte("sig"),
		},
		OneTimePrekeys: []*e2eev1.PreKey{
			{Key: &commonv1.PublicKeyBundle{KeyId: "otk-1", Algorithm: "x25519", PublicKey: []byte("otk-1")}},
			{Key: &commonv1.PublicKeyBundle{KeyId: "otk-2", Algorithm: "x25519", PublicKey: []byte("otk-2")}},
		},
	})
	if err != nil {
		t.Fatalf("upload prekeys: %v", err)
	}
	if upload.Bundle == nil || upload.Bundle.SignedPrekey == nil {
		t.Fatalf("expected uploaded bundle, got %+v", upload)
	}

	aliceBegin, err := api.BeginLogin(ctx, &authv1.BeginLoginRequest{
		Identifier:      &authv1.BeginLoginRequest_Username{Username: alice.Username},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
		DeviceName:      "alice-phone",
		DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
	})
	if err != nil {
		t.Fatalf("begin alice login: %v", err)
	}
	aliceVerify, err := api.VerifyLoginCode(ctx, &authv1.VerifyLoginCodeRequest{
		ChallengeId:    aliceBegin.ChallengeId,
		Code:           sender.code(aliceBegin.Targets[0].DestinationMask),
		DeviceName:     "alice-phone",
		DevicePlatform: commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		DeviceKey: &commonv1.PublicKeyBundle{
			PublicKey: []byte("alice-device-key"),
		},
	})
	if err != nil {
		t.Fatalf("verify alice login: %v", err)
	}
	aliceCtx := metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+aliceVerify.Tokens.AccessToken))

	fetched, err := api.GetAccountPreKeyBundles(aliceCtx, &e2eev1.GetAccountPreKeyBundlesRequest{
		UserId:                bob.ID,
		ConsumeOneTimePrekeys: true,
	})
	if err != nil {
		t.Fatalf("fetch prekey bundles: %v", err)
	}
	if len(fetched.Bundles) != 1 {
		t.Fatalf("expected one fetched bundle, got %d", len(fetched.Bundles))
	}
	if fetched.Bundles[0].IdentityKey == nil || string(fetched.Bundles[0].IdentityKey.PublicKey) != "bob-device-key" {
		t.Fatalf("unexpected identity key bundle: %+v", fetched.Bundles[0].IdentityKey)
	}
	if fetched.Bundles[0].OneTimePrekey == nil || fetched.Bundles[0].OneTimePrekey.Key == nil {
		t.Fatalf("expected claimed one-time prekey, got %+v", fetched.Bundles[0].OneTimePrekey)
	}
	if fetched.Bundles[0].OneTimePrekeysAvailable != 1 {
		t.Fatalf("expected one remaining one-time prekey, got %d", fetched.Bundles[0].OneTimePrekeysAvailable)
	}
}

func TestCreateListAndAcknowledgeDirectSessionsRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-session",
		DisplayName: "Alice Session",
		Email:       "alice-session@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-session",
		DisplayName: "Bob Session",
		Email:       "bob-session@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	login := func(username string, deviceName string, publicKey string) (context.Context, string) {
		begin, beginErr := api.BeginLogin(ctx, &authv1.BeginLoginRequest{
			Identifier:      &authv1.BeginLoginRequest_Username{Username: username},
			DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
			DeviceName:      deviceName,
			DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		})
		if beginErr != nil {
			t.Fatalf("begin login for %s: %v", username, beginErr)
		}
		verify, verifyErr := api.VerifyLoginCode(ctx, &authv1.VerifyLoginCodeRequest{
			ChallengeId:    begin.ChallengeId,
			Code:           sender.code(begin.Targets[0].DestinationMask),
			DeviceName:     deviceName,
			DevicePlatform: commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
			DeviceKey:      &commonv1.PublicKeyBundle{PublicKey: []byte(publicKey)},
		})
		if verifyErr != nil {
			t.Fatalf("verify login for %s: %v", username, verifyErr)
		}
		return metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+verify.Tokens.AccessToken)), verify.Device.DeviceId
	}

	aliceCtx, _ := login(alice.Username, "alice-phone", "alice-device-key")
	bobCtx, _ := login(bob.Username, "bob-phone", "bob-device-key")

	_, err = api.UploadDevicePreKeys(bobCtx, &e2eev1.UploadDevicePreKeysRequest{
		SignedPrekey: &e2eev1.SignedPreKey{
			Key: &commonv1.PublicKeyBundle{
				KeyId:     "spk-bob",
				Algorithm: "x25519",
				PublicKey: []byte("signed-prekey-bob"),
			},
			Signature: []byte("sig-bob"),
		},
		OneTimePrekeys: []*e2eev1.PreKey{
			{Key: &commonv1.PublicKeyBundle{KeyId: "otk-bob", Algorithm: "x25519", PublicKey: []byte("otk-bob")}},
		},
	})
	if err != nil {
		t.Fatalf("upload bob prekeys: %v", err)
	}

	created, err := api.CreateDirectSessions(aliceCtx, &e2eev1.CreateDirectSessionsRequest{
		UserId: bob.ID,
		InitiatorEphemeralKey: &commonv1.PublicKeyBundle{
			KeyId:     "eph-a1",
			Algorithm: "x25519",
			PublicKey: []byte("alice-ephemeral"),
		},
		Bootstrap: &e2eev1.SessionBootstrapPayload{
			Algorithm:  "x3dh-v1",
			Nonce:      []byte("nonce"),
			Ciphertext: []byte("ciphertext"),
			Metadata:   map[string]string{"conversation_id": "conv-direct"},
		},
	})
	if err != nil {
		t.Fatalf("create direct sessions rpc: %v", err)
	}
	if len(created.Sessions) != 1 {
		t.Fatalf("expected one created session, got %d", len(created.Sessions))
	}

	listed, err := api.ListDeviceSessions(bobCtx, &e2eev1.ListDeviceSessionsRequest{})
	if err != nil {
		t.Fatalf("list device sessions rpc: %v", err)
	}
	if len(listed.Sessions) != 1 {
		t.Fatalf("expected one listed session, got %d", len(listed.Sessions))
	}
	if listed.Sessions[0].Bootstrap == nil || listed.Sessions[0].Bootstrap.Algorithm != "x3dh-v1" {
		t.Fatalf("unexpected listed bootstrap: %+v", listed.Sessions[0].Bootstrap)
	}

	acked, err := api.AcknowledgeDirectSession(bobCtx, &e2eev1.AcknowledgeDirectSessionRequest{
		SessionId: listed.Sessions[0].SessionId,
	})
	if err != nil {
		t.Fatalf("ack direct session rpc: %v", err)
	}
	if acked.Session == nil || acked.Session.State != e2eev1.DirectSessionState_DIRECT_SESSION_STATE_ACKNOWLEDGED {
		t.Fatalf("unexpected acknowledged session: %+v", acked.Session)
	}
}

func TestPublishListAndAcknowledgeGroupSenderKeysRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-group-keys",
		DisplayName: "Alice Group Keys",
		Email:       "alice-group-keys@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-group-keys",
		DisplayName: "Bob Group Keys",
		Email:       "bob-group-keys@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	login := func(username string, deviceName string, publicKey string) (context.Context, string) {
		begin, beginErr := api.BeginLogin(ctx, &authv1.BeginLoginRequest{
			Identifier:      &authv1.BeginLoginRequest_Username{Username: username},
			DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
			DeviceName:      deviceName,
			DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		})
		if beginErr != nil {
			t.Fatalf("begin login for %s: %v", username, beginErr)
		}
		verify, verifyErr := api.VerifyLoginCode(ctx, &authv1.VerifyLoginCodeRequest{
			ChallengeId:    begin.ChallengeId,
			Code:           sender.code(begin.Targets[0].DestinationMask),
			DeviceName:     deviceName,
			DevicePlatform: commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
			DeviceKey:      &commonv1.PublicKeyBundle{PublicKey: []byte(publicKey)},
		})
		if verifyErr != nil {
			t.Fatalf("verify login for %s: %v", username, verifyErr)
		}
		return metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+verify.Tokens.AccessToken)), verify.Device.DeviceId
	}

	aliceCtx, _ := login(alice.Username, "alice-group-phone", "alice-group-device")
	bobCtx, bobDeviceID := login(bob.Username, "bob-group-phone", "bob-group-device")

	group, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:        commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:       "Encrypted Group",
		MemberUserIds: []string{bob.ID},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	published, err := api.PublishGroupSenderKeys(aliceCtx, &e2eev1.PublishGroupSenderKeysRequest{
		ConversationId: group.Conversation.ConversationId,
		SenderKeyId:    "sender-key-1",
		Recipients: []*e2eev1.RecipientSenderKey{
			{
				RecipientUserId:   bob.ID,
				RecipientDeviceId: bobDeviceID,
				Payload: &e2eev1.SenderKeyPayload{
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
	if len(published.Distributions) != 1 {
		t.Fatalf("expected one published distribution, got %d", len(published.Distributions))
	}

	listed, err := api.ListGroupSenderKeys(bobCtx, &e2eev1.ListGroupSenderKeysRequest{
		ConversationId: group.Conversation.ConversationId,
	})
	if err != nil {
		t.Fatalf("list group sender keys: %v", err)
	}
	if len(listed.Distributions) != 1 || listed.Distributions[0].SenderKeyId != "sender-key-1" {
		t.Fatalf("unexpected listed distributions: %+v", listed.Distributions)
	}

	acked, err := api.AcknowledgeGroupSenderKey(bobCtx, &e2eev1.AcknowledgeGroupSenderKeyRequest{
		DistributionId: listed.Distributions[0].DistributionId,
	})
	if err != nil {
		t.Fatalf("ack group sender key: %v", err)
	}
	if acked.Distribution == nil || acked.Distribution.State != e2eev1.GroupSenderKeyState_GROUP_SENDER_KEY_STATE_ACKNOWLEDGED {
		t.Fatalf("unexpected acknowledged distribution: %+v", acked.Distribution)
	}
}
