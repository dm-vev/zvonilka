package gateway

import (
	"context"
	"testing"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
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
