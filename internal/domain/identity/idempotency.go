package identity

import (
	"container/heap"
	"sync"
	"time"
)

// defaultIdempotencyTTL bounds how long a cached idempotency response stays reusable.
const defaultIdempotencyTTL = 24 * time.Hour

// idempotencyEntry stores one cached response together with the request fingerprint and TTL.
//
// The fingerprint prevents a caller from reusing the same key for a different logical
// request, while the TTL bounds cache lifetime.
type idempotencyEntry[T any] struct {
	fingerprint string
	expiresAt   time.Time
	value       T
}

// beginLoginCacheResult keeps the cached login challenge together with the chosen targets.
type beginLoginCacheResult struct {
	challenge LoginChallenge
	targets   []LoginTarget
}

// registerDeviceCacheResult keeps the paired device and session for device-registration retries.
type registerDeviceCacheResult struct {
	device  Device
	session Session
}

// createAccountCacheResult keeps the created account and optional bot token.
type createAccountCacheResult struct {
	account  Account
	botToken string
}

// approveJoinRequestCacheResult keeps the approved join request and the resulting account.
type approveJoinRequestCacheResult struct {
	joinRequest JoinRequest
	account     Account
}

// idempotencyExpiration records when a key should be reaped from a bucket.
type idempotencyExpiration struct {
	key       string
	expiresAt time.Time
}

// idempotencyExpirationHeap keeps the oldest expiration at the head of the heap.
type idempotencyExpirationHeap []idempotencyExpiration

// Len implements heap.Interface.
func (h idempotencyExpirationHeap) Len() int { return len(h) }

// Less orders older expirations first and uses the key as a stable tie-breaker.
func (h idempotencyExpirationHeap) Less(i, j int) bool {
	if h[i].expiresAt.Equal(h[j].expiresAt) {
		return h[i].key < h[j].key
	}

	return h[i].expiresAt.Before(h[j].expiresAt)
}

// Swap implements heap.Interface.
func (h idempotencyExpirationHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Push appends an expiration to the heap.
func (h *idempotencyExpirationHeap) Push(value any) {
	*h = append(*h, value.(idempotencyExpiration))
}

// Pop removes the oldest expiration from the heap.
func (h *idempotencyExpirationHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	*h = old[:last]
	return value
}

// idempotencyBucket stores one logical idempotency namespace behind a single mutex.
//
// Each bucket owns both the cached values and a min-heap of expiration markers, which
// keeps cleanup local to the bucket instead of scanning unrelated request types.
type idempotencyBucket[T any] struct {
	mu          sync.Mutex
	entries     map[string]idempotencyEntry[T]
	expirations idempotencyExpirationHeap
}

// newIdempotencyBucket constructs an empty bucket with a ready-to-use expiration heap.
func newIdempotencyBucket[T any]() *idempotencyBucket[T] {
	bucket := &idempotencyBucket[T]{
		entries: make(map[string]idempotencyEntry[T]),
	}
	heap.Init(&bucket.expirations)
	return bucket
}

// get reads a cached value if the key is still valid and the fingerprint matches.
//
// Expired entries are removed on access so stale responses do not leak into later calls.
func (b *idempotencyBucket[T]) get(
	key string,
	fingerprint string,
	now time.Time,
) (T, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.cleanupExpiredLocked(now)
	return lookupIdempotencyResult(b.entries, key, fingerprint, now)
}

// put stores a value and records its expiration in the heap.
func (b *idempotencyBucket[T]) put(
	key string,
	fingerprint string,
	value T,
	now time.Time,
) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.cleanupExpiredLocked(now)
	storeIdempotencyResult(b.entries, key, fingerprint, value, now)
	heap.Push(&b.expirations, idempotencyExpiration{
		key:       key,
		expiresAt: now.Add(defaultIdempotencyTTL),
	})
}

// cleanupExpiredLocked drains expired entries from the heap until the head is still fresh.
func (b *idempotencyBucket[T]) cleanupExpiredLocked(now time.Time) {
	for b.expirations.Len() > 0 {
		next := b.expirations[0]
		if now.Before(next.expiresAt) {
			return
		}

		heap.Pop(&b.expirations)
		deleteExpiredIdempotencyEntry(b.entries, next.key, next.expiresAt)
	}
}

// len returns the number of live entries currently cached in the bucket.
func (b *idempotencyBucket[T]) len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.entries)
}

// expirationLen returns the number of queued expiration markers.
func (b *idempotencyBucket[T]) expirationLen() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.expirations.Len()
}

// idempotencyCache groups the per-operation buckets so unrelated writes do not contend.
type idempotencyCache struct {
	submitJoinRequest    *idempotencyBucket[JoinRequest]
	approveJoinRequest   *idempotencyBucket[approveJoinRequestCacheResult]
	rejectJoinRequest    *idempotencyBucket[JoinRequest]
	createAccount        *idempotencyBucket[createAccountCacheResult]
	beginLogin           *idempotencyBucket[beginLoginCacheResult]
	verifyLogin          *idempotencyBucket[LoginResult]
	authenticatePassword *idempotencyBucket[LoginResult]
	authenticateBot      *idempotencyBucket[LoginResult]
	refreshSession       *idempotencyBucket[LoginResult]
	registerDevice       *idempotencyBucket[registerDeviceCacheResult]
	revokeSession        *idempotencyBucket[Session]
	revokeAllSessions    *idempotencyBucket[uint32]
}

// newIdempotencyCache constructs the full cache set used by the service.
func newIdempotencyCache() *idempotencyCache {
	return &idempotencyCache{
		submitJoinRequest:    newIdempotencyBucket[JoinRequest](),
		approveJoinRequest:   newIdempotencyBucket[approveJoinRequestCacheResult](),
		rejectJoinRequest:    newIdempotencyBucket[JoinRequest](),
		createAccount:        newIdempotencyBucket[createAccountCacheResult](),
		beginLogin:           newIdempotencyBucket[beginLoginCacheResult](),
		verifyLogin:          newIdempotencyBucket[LoginResult](),
		authenticatePassword: newIdempotencyBucket[LoginResult](),
		authenticateBot:      newIdempotencyBucket[LoginResult](),
		refreshSession:       newIdempotencyBucket[LoginResult](),
		registerDevice:       newIdempotencyBucket[registerDeviceCacheResult](),
		revokeSession:        newIdempotencyBucket[Session](),
		revokeAllSessions:    newIdempotencyBucket[uint32](),
	}
}

// The methods below are thin typed adapters over the shared bucket logic.
//
// They keep the payload-specific cloning behavior local to the operation that needs it,
// while the bucket itself stays generic and unaware of the response shape.
// submitJoinRequestResult returns the cached join-request submission if it still matches.
func (c *idempotencyCache) submitJoinRequestResult(
	key string,
	fingerprint string,
	now time.Time,
) (JoinRequest, bool, error) {
	return c.submitJoinRequest.get(key, fingerprint, now)
}

// storeSubmitJoinRequestResult caches a successful join-request submission.
func (c *idempotencyCache) storeSubmitJoinRequestResult(
	key string,
	fingerprint string,
	joinRequest JoinRequest,
	now time.Time,
) {
	c.submitJoinRequest.put(key, fingerprint, joinRequest, now)
}

// approveJoinRequestResult returns the cached approval outcome if the fingerprint still matches.
func (c *idempotencyCache) approveJoinRequestResult(
	key string,
	fingerprint string,
	now time.Time,
) (approveJoinRequestCacheResult, bool, error) {
	result, ok, err := c.approveJoinRequest.get(key, fingerprint, now)
	if err != nil || !ok {
		return approveJoinRequestCacheResult{}, ok, err
	}

	return cloneApproveJoinRequestCacheResult(result), true, nil
}

// storeApproveJoinRequestResult caches a successful join-request approval.
func (c *idempotencyCache) storeApproveJoinRequestResult(
	key string,
	fingerprint string,
	result approveJoinRequestCacheResult,
	now time.Time,
) {
	c.approveJoinRequest.put(key, fingerprint, cloneApproveJoinRequestCacheResult(result), now)
}

// rejectJoinRequestResult returns the cached rejection outcome if it still matches.
func (c *idempotencyCache) rejectJoinRequestResult(
	key string,
	fingerprint string,
	now time.Time,
) (JoinRequest, bool, error) {
	return c.rejectJoinRequest.get(key, fingerprint, now)
}

// storeRejectJoinRequestResult caches a successful join-request rejection.
func (c *idempotencyCache) storeRejectJoinRequestResult(
	key string,
	fingerprint string,
	joinRequest JoinRequest,
	now time.Time,
) {
	c.rejectJoinRequest.put(key, fingerprint, joinRequest, now)
}

// createAccountResult returns the cached account creation outcome if the fingerprint still matches.
func (c *idempotencyCache) createAccountResult(
	key string,
	fingerprint string,
	now time.Time,
) (createAccountCacheResult, bool, error) {
	result, ok, err := c.createAccount.get(key, fingerprint, now)
	if err != nil || !ok {
		return createAccountCacheResult{}, ok, err
	}

	return cloneCreateAccountCacheResult(result), true, nil
}

// storeCreateAccountResult caches a successful account creation.
func (c *idempotencyCache) storeCreateAccountResult(
	key string,
	fingerprint string,
	result createAccountCacheResult,
	now time.Time,
) {
	c.createAccount.put(key, fingerprint, cloneCreateAccountCacheResult(result), now)
}

// beginLoginResult returns the cached login-start outcome if the fingerprint still matches.
func (c *idempotencyCache) beginLoginResult(
	key string,
	fingerprint string,
	now time.Time,
) (beginLoginCacheResult, bool, error) {
	result, ok, err := c.beginLogin.get(key, fingerprint, now)
	if err != nil || !ok {
		return beginLoginCacheResult{}, ok, err
	}

	return cloneBeginLoginCacheResult(result), true, nil
}

// storeBeginLoginResult caches a successful login-start challenge.
func (c *idempotencyCache) storeBeginLoginResult(
	key string,
	fingerprint string,
	result beginLoginCacheResult,
	now time.Time,
) {
	c.beginLogin.put(key, fingerprint, cloneBeginLoginCacheResult(result), now)
}

// verifyLoginResult returns the cached login completion outcome if the fingerprint still matches.
func (c *idempotencyCache) verifyLoginResult(
	key string,
	fingerprint string,
	now time.Time,
) (LoginResult, bool, error) {
	return c.verifyLogin.get(key, fingerprint, now)
}

// storeVerifyLoginResult caches a successful login completion.
func (c *idempotencyCache) storeVerifyLoginResult(
	key string,
	fingerprint string,
	result LoginResult,
	now time.Time,
) {
	c.verifyLogin.put(key, fingerprint, result, now)
}

// authenticatePasswordResult returns the cached password-auth outcome if the fingerprint still matches.
func (c *idempotencyCache) authenticatePasswordResult(
	key string,
	fingerprint string,
	now time.Time,
) (LoginResult, bool, error) {
	return c.authenticatePassword.get(key, fingerprint, now)
}

// storeAuthenticatePasswordResult caches a successful password authentication.
func (c *idempotencyCache) storeAuthenticatePasswordResult(
	key string,
	fingerprint string,
	result LoginResult,
	now time.Time,
) {
	c.authenticatePassword.put(key, fingerprint, result, now)
}

// authenticateBotResult returns the cached bot-auth outcome if the fingerprint still matches.
func (c *idempotencyCache) authenticateBotResult(
	key string,
	fingerprint string,
	now time.Time,
) (LoginResult, bool, error) {
	return c.authenticateBot.get(key, fingerprint, now)
}

// storeAuthenticateBotResult caches a successful bot authentication.
func (c *idempotencyCache) storeAuthenticateBotResult(
	key string,
	fingerprint string,
	result LoginResult,
	now time.Time,
) {
	c.authenticateBot.put(key, fingerprint, result, now)
}

// refreshSessionResult returns the cached refresh outcome if the fingerprint still matches.
func (c *idempotencyCache) refreshSessionResult(
	key string,
	fingerprint string,
	now time.Time,
) (LoginResult, bool, error) {
	return c.refreshSession.get(key, fingerprint, now)
}

// storeRefreshSessionResult caches a successful session-refresh outcome.
func (c *idempotencyCache) storeRefreshSessionResult(
	key string,
	fingerprint string,
	result LoginResult,
	now time.Time,
) {
	c.refreshSession.put(key, fingerprint, result, now)
}

// registerDeviceResult returns the cached device-registration outcome if the fingerprint still matches.
func (c *idempotencyCache) registerDeviceResult(
	key string,
	fingerprint string,
	now time.Time,
) (Device, Session, bool, error) {
	result, ok, err := c.registerDevice.get(key, fingerprint, now)
	if err != nil || !ok {
		return Device{}, Session{}, ok, err
	}

	return result.device, result.session, true, nil
}

// storeRegisterDeviceResult caches a successful device registration.
func (c *idempotencyCache) storeRegisterDeviceResult(
	key string,
	fingerprint string,
	result registerDeviceCacheResult,
	now time.Time,
) {
	c.registerDevice.put(key, fingerprint, result, now)
}

// revokeSessionResult returns the cached single-session revocation if it still matches.
func (c *idempotencyCache) revokeSessionResult(
	key string,
	fingerprint string,
	now time.Time,
) (Session, bool, error) {
	return c.revokeSession.get(key, fingerprint, now)
}

// storeRevokeSessionResult caches a successful single-session revocation.
func (c *idempotencyCache) storeRevokeSessionResult(
	key string,
	fingerprint string,
	session Session,
	now time.Time,
) {
	c.revokeSession.put(key, fingerprint, session, now)
}

// revokeAllSessionsResult returns the cached bulk-revocation count if it still matches.
func (c *idempotencyCache) revokeAllSessionsResult(
	key string,
	fingerprint string,
	now time.Time,
) (uint32, bool, error) {
	return c.revokeAllSessions.get(key, fingerprint, now)
}

// storeRevokeAllSessionsResult caches a successful bulk session revocation.
func (c *idempotencyCache) storeRevokeAllSessionsResult(
	key string,
	fingerprint string,
	revoked uint32,
	now time.Time,
) {
	c.revokeAllSessions.put(key, fingerprint, revoked, now)
}

// lookupIdempotencyResult enforces the request fingerprint and TTL contract for one bucket.
//
// A missing or expired entry is a cache miss. A mismatched fingerprint is a conflict.
func lookupIdempotencyResult[T any](
	entries map[string]idempotencyEntry[T],
	key string,
	fingerprint string,
	now time.Time,
) (T, bool, error) {
	var zero T

	entry, ok := entries[key]
	if !ok {
		return zero, false, nil
	}

	if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
		delete(entries, key)
		return zero, false, nil
	}

	if entry.fingerprint != fingerprint {
		return zero, false, ErrConflict
	}

	return entry.value, true, nil
}

// storeIdempotencyResult writes a value with a fresh TTL.
func storeIdempotencyResult[T any](
	entries map[string]idempotencyEntry[T],
	key string,
	fingerprint string,
	value T,
	now time.Time,
) {
	entries[key] = idempotencyEntry[T]{
		fingerprint: fingerprint,
		expiresAt:   now.Add(defaultIdempotencyTTL),
		value:       value,
	}
}

// deleteExpiredIdempotencyEntry removes an entry only if the expiration marker still matches.
//
// That guard prevents an older expiration record from deleting a fresh value that reused
// the same key.
func deleteExpiredIdempotencyEntry[T any](
	entries map[string]idempotencyEntry[T],
	key string,
	expiresAt time.Time,
) {
	entry, ok := entries[key]
	if !ok || !entry.expiresAt.Equal(expiresAt) {
		return
	}

	delete(entries, key)
}

// cloneBeginLoginCacheResult returns a defensive copy of the cached login challenge.
func cloneBeginLoginCacheResult(result beginLoginCacheResult) beginLoginCacheResult {
	result.challenge = cloneLoginChallenge(result.challenge)
	result.targets = cloneLoginTargets(result.targets)
	return result
}

// cloneCreateAccountCacheResult returns a defensive copy of the cached account result.
func cloneCreateAccountCacheResult(result createAccountCacheResult) createAccountCacheResult {
	result.account = cloneAccount(result.account)
	return result
}

// cloneApproveJoinRequestCacheResult returns a defensive copy of the cached approval result.
func cloneApproveJoinRequestCacheResult(result approveJoinRequestCacheResult) approveJoinRequestCacheResult {
	result.account = cloneAccount(result.account)
	return result
}

// cloneAccount copies mutable slices before a cached account is returned to callers.
func cloneAccount(account Account) Account {
	if len(account.Roles) == 0 {
		return account
	}

	account.Roles = append([]Role(nil), account.Roles...)
	return account
}

// cloneLoginChallenge copies the targets slice before returning a cached challenge.
func cloneLoginChallenge(challenge LoginChallenge) LoginChallenge {
	if len(challenge.Targets) == 0 {
		return challenge
	}

	challenge.Targets = cloneLoginTargets(challenge.Targets)
	return challenge
}

// cloneLoginTargets copies the target slice to keep cache state immutable from the outside.
func cloneLoginTargets(targets []LoginTarget) []LoginTarget {
	if len(targets) == 0 {
		return nil
	}

	return append([]LoginTarget(nil), targets...)
}
