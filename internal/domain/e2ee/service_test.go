package e2ee_test

import (
	"context"
	"testing"
	"time"

	domaine2ee "github.com/dm-vev/zvonilka/internal/domain/e2ee"
	"github.com/dm-vev/zvonilka/internal/domain/e2ee/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestUploadAndClaimBundles(t *testing.T) {
	ctx := context.Background()
	directory := identitytest.NewMemoryStore()
	e2eeStore := teststore.NewMemoryStore()
	service, err := domaine2ee.NewService(e2eeStore, directory)
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
