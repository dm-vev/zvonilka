package gateway

import (
	"context"
	"strings"
	"testing"
	"time"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	e2eev1 "github.com/dm-vev/zvonilka/gen/proto/contracts/e2ee/v1"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Encrypted Group",
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

func TestSendMessageRequiresDirectSessionReferenceRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-direct-msg",
		DisplayName: "Alice Direct Msg",
		Email:       "alice-direct-msg@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-direct-msg",
		DisplayName: "Bob Direct Msg",
		Email:       "bob-direct-msg@example.com",
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

	aliceCtx, _ := login(alice.Username, "alice-direct-phone", "alice-direct-device")
	bobCtx, _ := login(bob.Username, "bob-direct-phone", "bob-direct-device")

	_, err = api.UploadDevicePreKeys(bobCtx, &e2eev1.UploadDevicePreKeysRequest{
		SignedPrekey: &e2eev1.SignedPreKey{
			Key:       &commonv1.PublicKeyBundle{KeyId: "spk-bob-direct", Algorithm: "x25519", PublicKey: []byte("spk")},
			Signature: []byte("sig"),
		},
		OneTimePrekeys: []*e2eev1.PreKey{
			{Key: &commonv1.PublicKeyBundle{KeyId: "otk-bob-direct", Algorithm: "x25519", PublicKey: []byte("otk")}},
		},
	})
	if err != nil {
		t.Fatalf("upload bob direct prekeys: %v", err)
	}

	created, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{bob.ID},
		Settings: &conversationv1.ConversationSettings{
			RequireEncryptedMessages: true,
			RequireTrustedDevices:    true,
		},
	})
	if err != nil {
		t.Fatalf("create direct conversation: %v", err)
	}

	sessionResp, err := api.CreateDirectSessions(aliceCtx, &e2eev1.CreateDirectSessionsRequest{
		UserId: bob.ID,
		InitiatorEphemeralKey: &commonv1.PublicKeyBundle{
			KeyId:     "eph-direct",
			Algorithm: "x25519",
			PublicKey: []byte("eph"),
		},
		Bootstrap: &e2eev1.SessionBootstrapPayload{
			Algorithm:  "x3dh-v1",
			Nonce:      []byte("nonce"),
			Ciphertext: []byte("cipher"),
		},
	})
	if err != nil {
		t.Fatalf("create direct session: %v", err)
	}
	if len(sessionResp.Sessions) != 1 {
		t.Fatalf("expected one direct session, got %d", len(sessionResp.Sessions))
	}
	_, err = api.AcknowledgeDirectSession(bobCtx, &e2eev1.AcknowledgeDirectSessionRequest{
		SessionId: sessionResp.Sessions[0].SessionId,
	})
	if err != nil {
		t.Fatalf("ack direct session: %v", err)
	}

	_, err = api.SendMessage(aliceCtx, &conversationv1.SendMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		Draft: &commonv1.MessageDraft{
			ClientMessageId: "msg-direct-untrusted",
			Kind:            commonv1.MessageKind_MESSAGE_KIND_TEXT,
			Payload: &commonv1.EncryptedPayload{
				KeyId:      sessionResp.Sessions[0].SessionId,
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				Metadata: map[string]string{
					"direct_session_id": sessionResp.Sessions[0].SessionId,
				},
			},
		},
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected failed precondition for untrusted device policy, got %v", err)
	}
	_, err = api.SetDeviceTrust(aliceCtx, &e2eev1.SetDeviceTrustRequest{
		TargetUserId:   bob.ID,
		TargetDeviceId: sessionResp.Sessions[0].RecipientDeviceId,
		State:          e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_TRUSTED,
	})
	if err != nil {
		t.Fatalf("set trusted device: %v", err)
	}

	sent, err := api.SendMessage(aliceCtx, &conversationv1.SendMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		Draft: &commonv1.MessageDraft{
			ClientMessageId: "msg-direct-1",
			Kind:            commonv1.MessageKind_MESSAGE_KIND_TEXT,
			Payload: &commonv1.EncryptedPayload{
				KeyId:      sessionResp.Sessions[0].SessionId,
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				Metadata: map[string]string{
					"direct_session_id": sessionResp.Sessions[0].SessionId,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("send direct encrypted message: %v", err)
	}
	if sent.Message == nil || sent.Message.MessageId == "" {
		t.Fatalf("unexpected sent message: %+v", sent.Message)
	}
}

func TestSendMessageRequiresGroupSenderKeyReferenceRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-group-msg",
		DisplayName: "Alice Group Msg",
		Email:       "alice-group-msg@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-group-msg",
		DisplayName: "Bob Group Msg",
		Email:       "bob-group-msg@example.com",
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

	aliceCtx, aliceDeviceID := login(alice.Username, "alice-group-msg-phone", "alice-group-msg-device")
	bobCtx, bobDeviceID := login(bob.Username, "bob-group-msg-phone", "bob-group-msg-device")

	group, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Encrypted Sender Key Group",
		MemberUserIds: []string{bob.ID},
		Settings: &conversationv1.ConversationSettings{
			RequireEncryptedMessages: true,
		},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	published, err := api.PublishGroupSenderKeys(aliceCtx, &e2eev1.PublishGroupSenderKeysRequest{
		ConversationId: group.Conversation.ConversationId,
		SenderKeyId:    "sender-key-group-1",
		Recipients: []*e2eev1.RecipientSenderKey{
			{
				RecipientUserId:   bob.ID,
				RecipientDeviceId: bobDeviceID,
				Payload: &e2eev1.SenderKeyPayload{
					Algorithm:  "sender-key-v1",
					Nonce:      []byte("nonce"),
					Ciphertext: []byte("ciphertext"),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish group sender key: %v", err)
	}
	if len(published.Distributions) != 1 {
		t.Fatalf("expected one published distribution, got %+v", published.Distributions)
	}
	_, err = api.AcknowledgeGroupSenderKey(bobCtx, &e2eev1.AcknowledgeGroupSenderKeyRequest{
		DistributionId: published.Distributions[0].DistributionId,
	})
	if err != nil {
		t.Fatalf("ack published group sender key: %v", err)
	}

	sent, err := api.SendMessage(aliceCtx, &conversationv1.SendMessageRequest{
		ConversationId: group.Conversation.ConversationId,
		Draft: &commonv1.MessageDraft{
			ClientMessageId: "msg-group-1",
			Kind:            commonv1.MessageKind_MESSAGE_KIND_TEXT,
			Payload: &commonv1.EncryptedPayload{
				KeyId:      "sender-key-group-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				Metadata: map[string]string{
					"sender_key_id": "sender-key-group-1",
					"sender_device": aliceDeviceID,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("send group encrypted message: %v", err)
	}
	if sent.Message == nil || sent.Message.MessageId == "" {
		t.Fatalf("unexpected sent group message: %+v", sent.Message)
	}
}

func TestSendEncryptedMessageRejectsCompromisedTrustRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-direct-compromised",
		DisplayName: "Alice Direct Compromised",
		Email:       "alice-direct-compromised@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-direct-compromised",
		DisplayName: "Bob Direct Compromised",
		Email:       "bob-direct-compromised@example.com",
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

	aliceCtx, _ := login(alice.Username, "alice-compromised-phone", "alice-compromised-device")
	bobCtx, bobDeviceID := login(bob.Username, "bob-compromised-phone", "bob-compromised-device")

	_, err = api.UploadDevicePreKeys(bobCtx, &e2eev1.UploadDevicePreKeysRequest{
		SignedPrekey: &e2eev1.SignedPreKey{
			Key:       &commonv1.PublicKeyBundle{KeyId: "spk-bob-direct-compromised", Algorithm: "x25519", PublicKey: []byte("spk")},
			Signature: []byte("sig"),
		},
		OneTimePrekeys: []*e2eev1.PreKey{
			{Key: &commonv1.PublicKeyBundle{KeyId: "otk-bob-direct-compromised", Algorithm: "x25519", PublicKey: []byte("otk")}},
		},
	})
	if err != nil {
		t.Fatalf("upload bob prekeys: %v", err)
	}

	created, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{bob.ID},
		Settings:      &conversationv1.ConversationSettings{RequireEncryptedMessages: true},
	})
	if err != nil {
		t.Fatalf("create direct conversation: %v", err)
	}
	sessionResp, err := api.CreateDirectSessions(aliceCtx, &e2eev1.CreateDirectSessionsRequest{
		UserId:                bob.ID,
		InitiatorEphemeralKey: &commonv1.PublicKeyBundle{KeyId: "eph-direct-compromised", Algorithm: "x25519", PublicKey: []byte("eph")},
		Bootstrap:             &e2eev1.SessionBootstrapPayload{Algorithm: "x3dh-v1", Ciphertext: []byte("cipher")},
	})
	if err != nil {
		t.Fatalf("create direct session: %v", err)
	}
	_, err = api.AcknowledgeDirectSession(bobCtx, &e2eev1.AcknowledgeDirectSessionRequest{
		SessionId: sessionResp.Sessions[0].SessionId,
	})
	if err != nil {
		t.Fatalf("ack direct session: %v", err)
	}
	_, err = api.SetDeviceTrust(aliceCtx, &e2eev1.SetDeviceTrustRequest{
		TargetUserId:   bob.ID,
		TargetDeviceId: bobDeviceID,
		State:          e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_COMPROMISED,
	})
	if err != nil {
		t.Fatalf("set compromised trust: %v", err)
	}

	_, err = api.SendMessage(aliceCtx, &conversationv1.SendMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		Draft: &commonv1.MessageDraft{
			ClientMessageId: "msg-direct-compromised",
			Kind:            commonv1.MessageKind_MESSAGE_KIND_TEXT,
			Payload: &commonv1.EncryptedPayload{
				KeyId:      sessionResp.Sessions[0].SessionId,
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				Metadata:   map[string]string{"direct_session_id": sessionResp.Sessions[0].SessionId},
			},
		},
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied for compromised trust, got %v", err)
	}
}

func TestSendEncryptedGroupMessageRejectsMissingCoverageRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-group-missing-coverage",
		DisplayName: "Alice Group Missing Coverage",
		Email:       "alice-group-missing-coverage@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-group-missing-coverage",
		DisplayName: "Bob Group Missing Coverage",
		Email:       "bob-group-missing-coverage@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	carol, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "carol-group-missing-coverage",
		DisplayName: "Carol Group Missing Coverage",
		Email:       "carol-group-missing-coverage@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create carol: %v", err)
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

	aliceCtx, aliceDeviceID := login(alice.Username, "alice-group-missing-phone", "alice-group-missing-device")
	bobCtx, bobDeviceID := login(bob.Username, "bob-group-missing-phone", "bob-group-missing-device")
	_, _ = login(carol.Username, "carol-group-missing-phone", "carol-group-missing-device")

	group, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Encrypted Sender Key Group Missing",
		MemberUserIds: []string{bob.ID, carol.ID},
		Settings:      &conversationv1.ConversationSettings{RequireEncryptedMessages: true},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}
	published, err := api.PublishGroupSenderKeys(aliceCtx, &e2eev1.PublishGroupSenderKeysRequest{
		ConversationId: group.Conversation.ConversationId,
		SenderKeyId:    "sender-key-group-missing",
		Recipients: []*e2eev1.RecipientSenderKey{
			{
				RecipientUserId:   bob.ID,
				RecipientDeviceId: bobDeviceID,
				Payload:           &e2eev1.SenderKeyPayload{Algorithm: "sender-key-v1", Ciphertext: []byte("ciphertext")},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish group sender key: %v", err)
	}
	if len(published.Distributions) != 1 {
		t.Fatalf("expected one published distribution, got %+v", published.Distributions)
	}
	_, err = api.AcknowledgeGroupSenderKey(bobCtx, &e2eev1.AcknowledgeGroupSenderKeyRequest{
		DistributionId: published.Distributions[0].DistributionId,
	})
	if err != nil {
		t.Fatalf("ack published group sender key: %v", err)
	}

	_, err = api.SendMessage(aliceCtx, &conversationv1.SendMessageRequest{
		ConversationId: group.Conversation.ConversationId,
		Draft: &commonv1.MessageDraft{
			ClientMessageId: "msg-group-missing-coverage",
			Kind:            commonv1.MessageKind_MESSAGE_KIND_TEXT,
			Payload: &commonv1.EncryptedPayload{
				KeyId:      "sender-key-group-missing",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				Metadata: map[string]string{
					"sender_key_id": "sender-key-group-missing",
					"sender_device": aliceDeviceID,
				},
			},
		},
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected failed precondition for missing group coverage, got %v", err)
	}
}

func TestRotateE2EEKeysRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-rotate-e2ee",
		DisplayName: "Alice Rotate E2EE",
		Email:       "alice-rotate-e2ee@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-rotate-e2ee",
		DisplayName: "Bob Rotate E2EE",
		Email:       "bob-rotate-e2ee@example.com",
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

	aliceCtx, _ := login(alice.Username, "alice-rotate-phone", "alice-rotate-device")
	bobCtx, bobDeviceID := login(bob.Username, "bob-rotate-phone", "bob-rotate-device")

	_, err = api.UploadDevicePreKeys(bobCtx, &e2eev1.UploadDevicePreKeysRequest{
		SignedPrekey: &e2eev1.SignedPreKey{
			Key:       &commonv1.PublicKeyBundle{KeyId: "spk-bob-rotate", Algorithm: "x25519", PublicKey: []byte("spk-bob-rotate")},
			Signature: []byte("sig-bob-rotate"),
		},
		OneTimePrekeys: []*e2eev1.PreKey{
			{Key: &commonv1.PublicKeyBundle{KeyId: "otk-bob-rotate", Algorithm: "x25519", PublicKey: []byte("otk-bob-rotate")}},
		},
	})
	if err != nil {
		t.Fatalf("upload bob prekeys: %v", err)
	}

	group, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Rotate E2EE Group",
		MemberUserIds: []string{bob.ID},
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	_, err = api.CreateDirectSessions(aliceCtx, &e2eev1.CreateDirectSessionsRequest{
		UserId: bob.ID,
		InitiatorEphemeralKey: &commonv1.PublicKeyBundle{
			KeyId:     "eph-rotate",
			Algorithm: "x25519",
			PublicKey: []byte("eph-rotate"),
		},
		Bootstrap: &e2eev1.SessionBootstrapPayload{
			Algorithm:  "x3dh-v1",
			Ciphertext: []byte("cipher"),
		},
	})
	if err != nil {
		t.Fatalf("create direct session: %v", err)
	}

	_, err = api.PublishGroupSenderKeys(aliceCtx, &e2eev1.PublishGroupSenderKeysRequest{
		ConversationId: group.Conversation.ConversationId,
		SenderKeyId:    "sender-key-rotate-1",
		Recipients: []*e2eev1.RecipientSenderKey{
			{
				RecipientUserId:   bob.ID,
				RecipientDeviceId: bobDeviceID,
				Payload: &e2eev1.SenderKeyPayload{
					Algorithm:  "sender-key-v1",
					Ciphertext: []byte("ciphertext"),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish group sender key: %v", err)
	}

	rotated, err := api.RotateE2EEKeys(aliceCtx, &e2eev1.RotateE2EEKeysRequest{
		SignedPrekey: &e2eev1.SignedPreKey{
			Key:       &commonv1.PublicKeyBundle{KeyId: "spk-alice-rotate-2", Algorithm: "x25519", PublicKey: []byte("spk-alice-rotate-2")},
			Signature: []byte("sig-alice-rotate-2"),
		},
		OneTimePrekeys: []*e2eev1.PreKey{
			{Key: &commonv1.PublicKeyBundle{KeyId: "otk-alice-rotate-2", Algorithm: "x25519", PublicKey: []byte("otk-alice-rotate-2")}},
		},
		ReplaceOneTimePrekeys:        true,
		ExpirePendingDirectSessions:  true,
		ExpirePendingGroupSenderKeys: true,
	})
	if err != nil {
		t.Fatalf("rotate e2ee keys: %v", err)
	}
	if rotated.Bundle == nil || rotated.Bundle.SignedPrekey == nil || rotated.Bundle.SignedPrekey.Key.KeyId != "spk-alice-rotate-2" {
		t.Fatalf("unexpected rotated bundle: %+v", rotated.Bundle)
	}
	if rotated.ExpiredPendingDirectSessions == 0 {
		t.Fatalf("expected expired direct sessions, got %+v", rotated)
	}
	if rotated.ExpiredPendingGroupSenderKeys == 0 {
		t.Fatalf("expected expired group sender keys, got %+v", rotated)
	}
}

func TestSetAndListDeviceTrustRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-device-trust",
		DisplayName: "Alice Device Trust",
		Email:       "alice-device-trust@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-device-trust",
		DisplayName: "Bob Device Trust",
		Email:       "bob-device-trust@example.com",
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

	aliceCtx, _ := login(alice.Username, "alice-trust-phone", "alice-trust-device")
	_, bobDeviceID := login(bob.Username, "bob-trust-phone", "bob-trust-device")

	setResp, err := api.SetDeviceTrust(aliceCtx, &e2eev1.SetDeviceTrustRequest{
		TargetUserId:   bob.ID,
		TargetDeviceId: bobDeviceID,
		State:          e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_TRUSTED,
		Note:           "verified by safety number",
	})
	if err != nil {
		t.Fatalf("set device trust: %v", err)
	}
	if setResp.Trust == nil || setResp.Trust.State != e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_TRUSTED {
		t.Fatalf("unexpected trust row: %+v", setResp.Trust)
	}
	if setResp.Trust.KeyFingerprint == "" {
		t.Fatal("expected key fingerprint")
	}

	listResp, err := api.ListDeviceTrusts(aliceCtx, &e2eev1.ListDeviceTrustsRequest{
		TargetUserId: bob.ID,
	})
	if err != nil {
		t.Fatalf("list device trusts: %v", err)
	}
	if len(listResp.Trusts) != 1 || listResp.Trusts[0].TargetDeviceId != bobDeviceID {
		t.Fatalf("unexpected trust list: %+v", listResp.Trusts)
	}
}

func TestGetConversationKeyCoverageRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-coverage",
		DisplayName: "Alice Coverage",
		Email:       "alice-coverage@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-coverage",
		DisplayName: "Bob Coverage",
		Email:       "bob-coverage@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	carol, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "carol-coverage",
		DisplayName: "Carol Coverage",
		Email:       "carol-coverage@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create carol: %v", err)
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

	aliceCtx, _ := login(alice.Username, "alice-coverage-phone", "alice-coverage-device")
	bobCtx, bobDeviceID := login(bob.Username, "bob-coverage-phone", "bob-coverage-device")
	_, _ = login(carol.Username, "carol-coverage-phone", "carol-coverage-device")

	_, err = api.UploadDevicePreKeys(bobCtx, &e2eev1.UploadDevicePreKeysRequest{
		SignedPrekey: &e2eev1.SignedPreKey{
			Key:       &commonv1.PublicKeyBundle{KeyId: "spk-bob-coverage", Algorithm: "x25519", PublicKey: []byte("spk-bob-coverage")},
			Signature: []byte("sig-bob-coverage"),
		},
	})
	if err != nil {
		t.Fatalf("upload bob prekeys: %v", err)
	}

	direct, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{bob.ID},
	})
	if err != nil {
		t.Fatalf("create direct conversation: %v", err)
	}
	sessions, err := api.CreateDirectSessions(aliceCtx, &e2eev1.CreateDirectSessionsRequest{
		UserId: bob.ID,
		InitiatorEphemeralKey: &commonv1.PublicKeyBundle{
			KeyId:     "eph-coverage",
			Algorithm: "x25519",
			PublicKey: []byte("eph-coverage"),
		},
		Bootstrap: &e2eev1.SessionBootstrapPayload{
			Algorithm:  "x3dh-v1",
			Ciphertext: []byte("cipher"),
		},
	})
	if err != nil {
		t.Fatalf("create direct session: %v", err)
	}
	_, err = api.AcknowledgeDirectSession(bobCtx, &e2eev1.AcknowledgeDirectSessionRequest{
		SessionId: sessions.Sessions[0].SessionId,
	})
	if err != nil {
		t.Fatalf("ack direct session: %v", err)
	}

	directCoverage, err := api.GetConversationKeyCoverage(aliceCtx, &e2eev1.GetConversationKeyCoverageRequest{
		ConversationId: direct.Conversation.ConversationId,
	})
	if err != nil {
		t.Fatalf("get direct coverage: %v", err)
	}
	if len(directCoverage.Entries) != 1 || directCoverage.Entries[0].State != e2eev1.ConversationKeyCoverageState_CONVERSATION_KEY_COVERAGE_STATE_READY {
		t.Fatalf("unexpected direct coverage: %+v", directCoverage.Entries)
	}
	if directCoverage.Entries[0].KeyFingerprint == "" || !directCoverage.Entries[0].VerificationRequired {
		t.Fatalf("expected verification hints in direct coverage: %+v", directCoverage.Entries[0])
	}
	if _, err := api.SetDeviceTrust(aliceCtx, &e2eev1.SetDeviceTrustRequest{
		TargetUserId:   bob.ID,
		TargetDeviceId: bobDeviceID,
		State:          e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_UNTRUSTED,
	}); err != nil {
		t.Fatalf("set direct untrusted trust: %v", err)
	}

	group, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Coverage Group",
		MemberUserIds: []string{bob.ID, carol.ID},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}
	_, err = api.PublishGroupSenderKeys(aliceCtx, &e2eev1.PublishGroupSenderKeysRequest{
		ConversationId: group.Conversation.ConversationId,
		SenderKeyId:    "sender-key-coverage",
		Recipients: []*e2eev1.RecipientSenderKey{
			{
				RecipientUserId:   bob.ID,
				RecipientDeviceId: bobDeviceID,
				Payload: &e2eev1.SenderKeyPayload{
					Algorithm:  "sender-key-v1",
					Ciphertext: []byte("ciphertext"),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish sender key: %v", err)
	}

	groupCoverage, err := api.GetConversationKeyCoverage(aliceCtx, &e2eev1.GetConversationKeyCoverageRequest{
		ConversationId: group.Conversation.ConversationId,
		SenderKeyId:    "sender-key-coverage",
	})
	if err != nil {
		t.Fatalf("get group coverage: %v", err)
	}
	if len(groupCoverage.Entries) != 2 {
		t.Fatalf("expected two coverage entries, got %d", len(groupCoverage.Entries))
	}
	seenReady := false
	seenMissing := false
	for _, entry := range groupCoverage.Entries {
		if entry.State == e2eev1.ConversationKeyCoverageState_CONVERSATION_KEY_COVERAGE_STATE_PENDING {
			seenReady = true
		}
		if entry.State == e2eev1.ConversationKeyCoverageState_CONVERSATION_KEY_COVERAGE_STATE_MISSING {
			seenMissing = true
		}
	}
	if !seenReady || !seenMissing {
		t.Fatalf("unexpected group coverage states: %+v", groupCoverage.Entries)
	}
	for _, entry := range groupCoverage.Entries {
		if entry.KeyFingerprint == "" {
			t.Fatalf("expected key fingerprint in group coverage: %+v", entry)
		}
	}
}

func TestConversationResponsesIncludeVerificationRequiredOverlays(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-conversation-overlay",
		DisplayName: "Alice Conversation Overlay",
		Email:       "alice-conversation-overlay@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-conversation-overlay",
		DisplayName: "Bob Conversation Overlay",
		Email:       "bob-conversation-overlay@example.com",
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

	aliceCtx, _ := login(alice.Username, "alice-overlay-phone", "alice-overlay-device-key")
	_, bobDeviceID := login(bob.Username, "bob-overlay-phone", "bob-overlay-device-key")

	flaggedConversation, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{bob.ID},
	})
	if err != nil {
		t.Fatalf("create direct conversation: %v", err)
	}
	plainConversation, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:  commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title: "Plain Overlay Group",
	})
	if err != nil {
		t.Fatalf("create plain conversation: %v", err)
	}

	if flaggedConversation.Conversation.VerificationRequiredDevices != 1 {
		t.Fatalf("expected create response to expose one verification-required device, got %d", flaggedConversation.Conversation.VerificationRequiredDevices)
	}
	if flaggedConversation.Conversation.E2EeRequiredAction != conversationv1.ConversationE2EERequiredAction_CONVERSATION_E2EE_REQUIRED_ACTION_VERIFY_DEVICES {
		t.Fatalf("expected verify-devices action, got %s", flaggedConversation.Conversation.E2EeRequiredAction)
	}
	if plainConversation.Conversation.VerificationRequiredDevices != 0 {
		t.Fatalf("expected plain conversation to have no verification-required devices, got %d", plainConversation.Conversation.VerificationRequiredDevices)
	}
	if plainConversation.Conversation.E2EeRequiredAction != conversationv1.ConversationE2EERequiredAction_CONVERSATION_E2EE_REQUIRED_ACTION_NONE {
		t.Fatalf("expected no required action, got %s", plainConversation.Conversation.E2EeRequiredAction)
	}

	getConversation, err := api.GetConversation(aliceCtx, &conversationv1.GetConversationRequest{
		ConversationId: flaggedConversation.Conversation.ConversationId,
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if getConversation.Conversation.VerificationRequiredDevices != 1 {
		t.Fatalf("expected get conversation to expose one verification-required device, got %d", getConversation.Conversation.VerificationRequiredDevices)
	}

	listed, err := api.ListConversations(aliceCtx, &conversationv1.ListConversationsRequest{})
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	seenFlagged := false
	seenPlain := false
	for _, item := range listed.Conversations {
		switch item.ConversationId {
		case flaggedConversation.Conversation.ConversationId:
			seenFlagged = true
			if item.VerificationRequiredDevices != 1 {
				t.Fatalf("expected flagged conversation to require one verification, got %d", item.VerificationRequiredDevices)
			}
			if item.E2EeRequiredAction != conversationv1.ConversationE2EERequiredAction_CONVERSATION_E2EE_REQUIRED_ACTION_VERIFY_DEVICES {
				t.Fatalf("expected verify-devices action in list, got %s", item.E2EeRequiredAction)
			}
		case plainConversation.Conversation.ConversationId:
			seenPlain = true
			if item.VerificationRequiredDevices != 0 {
				t.Fatalf("expected plain conversation to stay clear in list, got %d", item.VerificationRequiredDevices)
			}
			if item.E2EeRequiredAction != conversationv1.ConversationE2EERequiredAction_CONVERSATION_E2EE_REQUIRED_ACTION_NONE {
				t.Fatalf("expected no action for plain conversation, got %s", item.E2EeRequiredAction)
			}
		}
	}
	if !seenFlagged || !seenPlain {
		t.Fatalf("expected both conversations in list, got %+v", listed.Conversations)
	}

	if _, err := api.SetDeviceTrust(aliceCtx, &e2eev1.SetDeviceTrustRequest{
		TargetUserId:   bob.ID,
		TargetDeviceId: bobDeviceID,
		State:          e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_TRUSTED,
	}); err != nil {
		t.Fatalf("set device trust: %v", err)
	}

	cleared, err := api.GetConversation(aliceCtx, &conversationv1.GetConversationRequest{
		ConversationId: flaggedConversation.Conversation.ConversationId,
	})
	if err != nil {
		t.Fatalf("get conversation after trust update: %v", err)
	}
	if cleared.Conversation.VerificationRequiredDevices != 0 {
		t.Fatalf("expected overlay to clear after trust update, got %d", cleared.Conversation.VerificationRequiredDevices)
	}
	if cleared.Conversation.E2EeRequiredAction != conversationv1.ConversationE2EERequiredAction_CONVERSATION_E2EE_REQUIRED_ACTION_NONE {
		t.Fatalf("expected required action to clear after trust update, got %s", cleared.Conversation.E2EeRequiredAction)
	}
}

func TestSubscribeE2EEUpdatesStreamsTrustChanges(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-e2ee-stream",
		DisplayName: "Alice E2EE Stream",
		Email:       "alice-e2ee-stream@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-e2ee-stream",
		DisplayName: "Bob E2EE Stream",
		Email:       "bob-e2ee-stream@example.com",
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

	aliceCtx, _ := login(alice.Username, "alice-e2ee-stream-phone", "alice-e2ee-stream-device")
	_, bobDeviceID := login(bob.Username, "bob-e2ee-stream-phone", "bob-e2ee-stream-device")

	stream := newTestSubscribeE2EEUpdatesStream(aliceCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- api.SubscribeE2EEUpdates(&e2eev1.SubscribeE2EEUpdatesRequest{}, stream)
	}()
	time.Sleep(50 * time.Millisecond)

	_, err = api.SetDeviceTrust(aliceCtx, &e2eev1.SetDeviceTrustRequest{
		TargetUserId:   bob.ID,
		TargetDeviceId: bobDeviceID,
		State:          e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_TRUSTED,
	})
	if err != nil {
		t.Fatalf("set device trust: %v", err)
	}

	select {
	case resp := <-stream.responses:
		if resp.Update == nil {
			t.Fatal("expected update payload")
		}
		if resp.Update.UpdateType != e2eev1.E2EEUpdateType_E2EE_UPDATE_TYPE_DEVICE_TRUST_UPDATED {
			t.Fatalf("unexpected update type: %+v", resp.Update)
		}
		if resp.Update.TargetUserId != bob.ID || resp.Update.TargetDeviceId != bobDeviceID {
			t.Fatalf("unexpected update target: %+v", resp.Update)
		}
		if resp.Update.CurrentTrustState != e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_TRUSTED || resp.Update.TargetKeyFingerprint == "" || resp.Update.VerificationRequired {
			t.Fatalf("unexpected trust update verification fields: %+v", resp.Update)
		}
		if len(resp.Update.ConversationIds) != 0 {
			t.Fatalf("expected no shared conversation ids before chats, got %+v", resp.Update.ConversationIds)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for e2ee update")
	}

	stream.cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe e2ee updates returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for stream shutdown")
	}
}

func TestGetAndVerifyDeviceSafetyNumberRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-e2ee-verify",
		DisplayName: "Alice E2EE Verify",
		Email:       "alice-e2ee-verify@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-e2ee-verify",
		DisplayName: "Bob E2EE Verify",
		Email:       "bob-e2ee-verify@example.com",
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

	aliceCtx, _ := login(alice.Username, "alice-verify-phone", "alice-verify-device")
	_, bobDeviceID := login(bob.Username, "bob-verify-phone", "bob-verify-device")

	codeResp, err := api.GetDeviceVerificationCode(aliceCtx, &e2eev1.GetDeviceVerificationCodeRequest{
		TargetUserId:   bob.ID,
		TargetDeviceId: bobDeviceID,
	})
	if err != nil {
		t.Fatalf("get device verification code: %v", err)
	}
	if codeResp.Verification == nil || codeResp.Verification.SafetyNumber == "" {
		t.Fatalf("unexpected verification payload: %+v", codeResp.Verification)
	}

	verifyResp, err := api.VerifyDeviceSafetyNumber(aliceCtx, &e2eev1.VerifyDeviceSafetyNumberRequest{
		TargetUserId:   bob.ID,
		TargetDeviceId: bobDeviceID,
		SafetyNumber:   strings.ReplaceAll(codeResp.Verification.SafetyNumber, "-", ""),
		Note:           "verified in person",
	})
	if err != nil {
		t.Fatalf("verify device safety number: %v", err)
	}
	if verifyResp.Trust == nil || verifyResp.Trust.State != e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_TRUSTED {
		t.Fatalf("unexpected verification trust: %+v", verifyResp.Trust)
	}
	codeResp, err = api.GetDeviceVerificationCode(aliceCtx, &e2eev1.GetDeviceVerificationCodeRequest{
		TargetUserId:   bob.ID,
		TargetDeviceId: bobDeviceID,
	})
	if err != nil {
		t.Fatalf("get verification code after verify: %v", err)
	}
	if codeResp.Verification == nil || codeResp.Verification.CurrentTrustState != e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_TRUSTED {
		t.Fatalf("unexpected verification state after trust: %+v", codeResp.Verification)
	}
}

func TestSubscribeE2EEUpdatesIncludesConversationIDsForTrustChanges(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-e2ee-stream-conversations",
		DisplayName: "Alice E2EE Stream Conversations",
		Email:       "alice-e2ee-stream-conversations@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-e2ee-stream-conversations",
		DisplayName: "Bob E2EE Stream Conversations",
		Email:       "bob-e2ee-stream-conversations@example.com",
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

	aliceCtx, _ := login(alice.Username, "alice-e2ee-stream-conversations-phone", "alice-e2ee-stream-conversations-device")
	_, bobDeviceID := login(bob.Username, "bob-e2ee-stream-conversations-phone", "bob-e2ee-stream-conversations-device")

	direct, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{bob.ID},
	})
	if err != nil {
		t.Fatalf("create direct conversation: %v", err)
	}

	stream := newTestSubscribeE2EEUpdatesStream(aliceCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- api.SubscribeE2EEUpdates(&e2eev1.SubscribeE2EEUpdatesRequest{}, stream)
	}()
	time.Sleep(50 * time.Millisecond)

	_, err = api.SetDeviceTrust(aliceCtx, &e2eev1.SetDeviceTrustRequest{
		TargetUserId:   bob.ID,
		TargetDeviceId: bobDeviceID,
		State:          e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_UNTRUSTED,
	})
	if err != nil {
		t.Fatalf("set device trust: %v", err)
	}

	select {
	case resp := <-stream.responses:
		if len(resp.Update.ConversationIds) != 1 || resp.Update.ConversationIds[0] != direct.Conversation.ConversationId {
			t.Fatalf("unexpected conversation ids in update: %+v", resp.Update)
		}
		if !resp.Update.VerificationRequired {
			t.Fatalf("expected verification required in trust update: %+v", resp.Update)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for trust update")
	}

	stream.cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe e2ee updates returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for stream shutdown")
	}
}

func TestListVerificationRequiredDevicesRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-e2ee-queue",
		DisplayName: "Alice E2EE Queue",
		Email:       "alice-e2ee-queue@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-e2ee-queue",
		DisplayName: "Bob E2EE Queue",
		Email:       "bob-e2ee-queue@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	carol, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "carol-e2ee-queue",
		DisplayName: "Carol E2EE Queue",
		Email:       "carol-e2ee-queue@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create carol: %v", err)
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

	aliceCtx, _ := login(alice.Username, "alice-queue-phone", "alice-queue-device")
	_, bobDeviceID := login(bob.Username, "bob-queue-phone", "bob-queue-device")
	_, carolDeviceID := login(carol.Username, "carol-queue-phone", "carol-queue-device")

	if _, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{bob.ID},
	}); err != nil {
		t.Fatalf("create direct conversation: %v", err)
	}
	group, err := api.CreateConversation(aliceCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "verification queue",
		MemberUserIds: []string{bob.ID, carol.ID},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}
	if _, err := api.SetDeviceTrust(aliceCtx, &e2eev1.SetDeviceTrustRequest{
		TargetUserId:   carol.ID,
		TargetDeviceId: carolDeviceID,
		State:          e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_COMPROMISED,
	}); err != nil {
		t.Fatalf("set compromised trust: %v", err)
	}

	resp, err := api.ListVerificationRequiredDevices(aliceCtx, &e2eev1.ListVerificationRequiredDevicesRequest{})
	if err != nil {
		t.Fatalf("list verification required devices: %v", err)
	}
	if len(resp.Devices) != 2 {
		t.Fatalf("expected two devices, got %+v", resp.Devices)
	}
	if resp.Devices[0].UserId != bob.ID || !resp.Devices[0].DirectConversation {
		t.Fatalf("expected bob direct entry first, got %+v", resp.Devices[0])
	}
	if resp.Devices[1].UserId != carol.ID || resp.Devices[1].TrustState != e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_COMPROMISED || len(resp.Devices[1].ConversationIds) != 1 || resp.Devices[1].ConversationIds[0] != group.Conversation.ConversationId {
		t.Fatalf("unexpected group verification entry: %+v", resp.Devices[1])
	}
	if resp.Devices[0].DeviceId != bobDeviceID || resp.Devices[1].KeyFingerprint == "" {
		t.Fatalf("unexpected verification device payloads: %+v", resp.Devices)
	}
}

type testSubscribeE2EEUpdatesStream struct {
	ctx       context.Context
	cancel    context.CancelFunc
	responses chan *e2eev1.SubscribeE2EEUpdatesResponse
}

func newTestSubscribeE2EEUpdatesStream(ctx context.Context) *testSubscribeE2EEUpdatesStream {
	streamCtx, cancel := context.WithCancel(ctx)
	return &testSubscribeE2EEUpdatesStream{
		ctx:       streamCtx,
		cancel:    cancel,
		responses: make(chan *e2eev1.SubscribeE2EEUpdatesResponse, 8),
	}
}

func (s *testSubscribeE2EEUpdatesStream) Context() context.Context { return s.ctx }
func (s *testSubscribeE2EEUpdatesStream) Send(resp *e2eev1.SubscribeE2EEUpdatesResponse) error {
	s.responses <- resp
	return nil
}
func (*testSubscribeE2EEUpdatesStream) SetHeader(metadata.MD) error  { return nil }
func (*testSubscribeE2EEUpdatesStream) SendHeader(metadata.MD) error { return nil }
func (*testSubscribeE2EEUpdatesStream) SetTrailer(metadata.MD)       {}
func (*testSubscribeE2EEUpdatesStream) SendMsg(any) error            { return nil }
func (*testSubscribeE2EEUpdatesStream) RecvMsg(any) error            { return nil }
