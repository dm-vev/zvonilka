package identity

import (
	"testing"
	"time"
)

func TestIdempotencyCacheEvictsExpiredEntriesOnAnyOperation(t *testing.T) {
	t.Parallel()

	cache := newIdempotencyCache()
	now := time.Date(2026, time.March, 23, 23, 45, 0, 0, time.UTC)

	cache.storeCreateAccountResult(
		"create-key",
		"create-fingerprint",
		createAccountCacheResult{
			account: Account{ID: "acc-1"},
		},
		now,
	)
	cache.storeBeginLoginResult(
		"begin-key",
		"begin-fingerprint",
		beginLoginCacheResult{
			challenge: LoginChallenge{ID: "chal-1"},
		},
		now,
	)
	cache.storeRegisterDeviceResult(
		"register-key",
		"register-fingerprint",
		registerDeviceCacheResult{
			device:  Device{ID: "dev-1"},
			session: Session{ID: "sess-1"},
		},
		now,
	)

	if got := len(cache.createAccount); got != 1 {
		t.Fatalf("expected one create-account cache entry, got %d", got)
	}
	if got := len(cache.beginLogin); got != 1 {
		t.Fatalf("expected one begin-login cache entry, got %d", got)
	}
	if got := len(cache.registerDevice); got != 1 {
		t.Fatalf("expected one register-device cache entry, got %d", got)
	}

	later := now.Add(defaultIdempotencyTTL + time.Minute)
	_, _, err := cache.submitJoinRequestResult("missing-key", "submit-fingerprint", later)
	if err != nil {
		t.Fatalf("lookup through idempotency cache: %v", err)
	}

	if got := len(cache.createAccount); got != 0 {
		t.Fatalf("expected expired create-account cache entry to be evicted, got %d", got)
	}
	if got := len(cache.beginLogin); got != 0 {
		t.Fatalf("expected expired begin-login cache entry to be evicted, got %d", got)
	}
	if got := len(cache.registerDevice); got != 0 {
		t.Fatalf("expected expired register-device cache entry to be evicted, got %d", got)
	}
}

func TestIdempotencyCacheRetainsLiveEntriesDuringCleanup(t *testing.T) {
	t.Parallel()

	cache := newIdempotencyCache()
	now := time.Date(2026, time.March, 24, 0, 0, 0, 0, time.UTC)

	cache.storeCreateAccountResult(
		"expired-create-key",
		"expired-create-fingerprint",
		createAccountCacheResult{
			account: Account{ID: "acc-expired"},
		},
		now,
	)
	cache.storeBeginLoginResult(
		"live-begin-key",
		"live-begin-fingerprint",
		beginLoginCacheResult{
			challenge: LoginChallenge{ID: "chal-live"},
		},
		now.Add(12*time.Hour),
	)

	later := now.Add(defaultIdempotencyTTL + time.Minute)
	_, _, err := cache.revokeSessionResult("missing-key", "revoke-fingerprint", later)
	if err != nil {
		t.Fatalf("lookup through idempotency cache: %v", err)
	}

	if got := len(cache.createAccount); got != 0 {
		t.Fatalf("expected expired create-account cache entry to be evicted, got %d", got)
	}
	if got := len(cache.beginLogin); got != 1 {
		t.Fatalf("expected live begin-login cache entry to remain, got %d", got)
	}
	if got := len(cache.expirations); got != 1 {
		t.Fatalf("expected one live expiration entry, got %d", got)
	}
}
