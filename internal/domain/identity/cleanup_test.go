package identity

import (
	"testing"
	"time"
)

func TestIdempotencyCacheCleanupIsBucketScoped(t *testing.T) {
	t.Parallel()

	cache := newIdempotencyCache()
	now := time.Date(2026, time.March, 23, 23, 45, 0, 0, time.UTC)

	cache.storeSubmitJoinRequestResult(
		"submit-key",
		"submit-fingerprint",
		JoinRequest{ID: "join-1"},
		now,
	)
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
			challenge: LoginChallenge{ID: "chal-live"},
		},
		now.Add(12*time.Hour),
	)

	if got := cache.submitJoinRequest.len(); got != 1 {
		t.Fatalf("expected one submit-join-request cache entry, got %d", got)
	}
	if got := cache.createAccount.len(); got != 1 {
		t.Fatalf("expected one create-account cache entry, got %d", got)
	}
	if got := cache.beginLogin.len(); got != 1 {
		t.Fatalf("expected one begin-login cache entry, got %d", got)
	}

	later := now.Add(defaultIdempotencyTTL + time.Minute)
	_, _, err := cache.submitJoinRequestResult("missing-key", "submit-fingerprint", later)
	if err != nil {
		t.Fatalf("lookup through idempotency cache: %v", err)
	}
	if got := cache.submitJoinRequest.len(); got != 0 {
		t.Fatalf("expected expired submit-join-request entry to be evicted, got %d", got)
	}
	if got := cache.submitJoinRequest.expirationLen(); got != 0 {
		t.Fatalf("expected submit-join-request expiration heap to be empty, got %d", got)
	}
	if got := cache.createAccount.len(); got != 1 {
		t.Fatalf("expected create-account cache to stay untouched, got %d", got)
	}
	if got := cache.createAccount.expirationLen(); got != 1 {
		t.Fatalf("expected create-account expiration heap to stay untouched, got %d", got)
	}
	if got := cache.beginLogin.len(); got != 1 {
		t.Fatalf("expected begin-login cache to stay untouched, got %d", got)
	}
	if got := cache.beginLogin.expirationLen(); got != 1 {
		t.Fatalf("expected begin-login expiration heap to stay untouched, got %d", got)
	}
}
