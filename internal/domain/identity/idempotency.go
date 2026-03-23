package identity

import (
	"container/heap"
	"sync"
	"time"
)

const defaultIdempotencyTTL = 24 * time.Hour

type idempotencyEntry[T any] struct {
	fingerprint string
	expiresAt   time.Time
	value       T
}

type beginLoginCacheResult struct {
	challenge LoginChallenge
	targets   []LoginTarget
}

type registerDeviceCacheResult struct {
	device  Device
	session Session
}

type createAccountCacheResult struct {
	account  Account
	botToken string
}

type approveJoinRequestCacheResult struct {
	joinRequest JoinRequest
	account     Account
}

type idempotencyExpiration struct {
	key       string
	expiresAt time.Time
}

type idempotencyExpirationHeap []idempotencyExpiration

func (h idempotencyExpirationHeap) Len() int { return len(h) }

func (h idempotencyExpirationHeap) Less(i, j int) bool {
	if h[i].expiresAt.Equal(h[j].expiresAt) {
		return h[i].key < h[j].key
	}

	return h[i].expiresAt.Before(h[j].expiresAt)
}

func (h idempotencyExpirationHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *idempotencyExpirationHeap) Push(value any) {
	*h = append(*h, value.(idempotencyExpiration))
}

func (h *idempotencyExpirationHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	*h = old[:last]
	return value
}

type idempotencyBucket[T any] struct {
	mu          sync.Mutex
	entries     map[string]idempotencyEntry[T]
	expirations idempotencyExpirationHeap
}

func newIdempotencyBucket[T any]() *idempotencyBucket[T] {
	bucket := &idempotencyBucket[T]{
		entries: make(map[string]idempotencyEntry[T]),
	}
	heap.Init(&bucket.expirations)
	return bucket
}

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

func (b *idempotencyBucket[T]) len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.entries)
}

func (b *idempotencyBucket[T]) expirationLen() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.expirations.Len()
}

type idempotencyCache struct {
	submitJoinRequest  *idempotencyBucket[JoinRequest]
	approveJoinRequest *idempotencyBucket[approveJoinRequestCacheResult]
	rejectJoinRequest  *idempotencyBucket[JoinRequest]
	createAccount      *idempotencyBucket[createAccountCacheResult]
	beginLogin         *idempotencyBucket[beginLoginCacheResult]
	verifyLogin        *idempotencyBucket[LoginResult]
	authenticateBot    *idempotencyBucket[LoginResult]
	registerDevice     *idempotencyBucket[registerDeviceCacheResult]
	revokeSession      *idempotencyBucket[Session]
	revokeAllSessions  *idempotencyBucket[uint32]
}

func newIdempotencyCache() *idempotencyCache {
	return &idempotencyCache{
		submitJoinRequest:  newIdempotencyBucket[JoinRequest](),
		approveJoinRequest: newIdempotencyBucket[approveJoinRequestCacheResult](),
		rejectJoinRequest:  newIdempotencyBucket[JoinRequest](),
		createAccount:      newIdempotencyBucket[createAccountCacheResult](),
		beginLogin:         newIdempotencyBucket[beginLoginCacheResult](),
		verifyLogin:        newIdempotencyBucket[LoginResult](),
		authenticateBot:    newIdempotencyBucket[LoginResult](),
		registerDevice:     newIdempotencyBucket[registerDeviceCacheResult](),
		revokeSession:      newIdempotencyBucket[Session](),
		revokeAllSessions:  newIdempotencyBucket[uint32](),
	}
}

func (c *idempotencyCache) submitJoinRequestResult(
	key string,
	fingerprint string,
	now time.Time,
) (JoinRequest, bool, error) {
	return c.submitJoinRequest.get(key, fingerprint, now)
}

func (c *idempotencyCache) storeSubmitJoinRequestResult(
	key string,
	fingerprint string,
	joinRequest JoinRequest,
	now time.Time,
) {
	c.submitJoinRequest.put(key, fingerprint, joinRequest, now)
}

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

func (c *idempotencyCache) storeApproveJoinRequestResult(
	key string,
	fingerprint string,
	result approveJoinRequestCacheResult,
	now time.Time,
) {
	c.approveJoinRequest.put(key, fingerprint, cloneApproveJoinRequestCacheResult(result), now)
}

func (c *idempotencyCache) rejectJoinRequestResult(
	key string,
	fingerprint string,
	now time.Time,
) (JoinRequest, bool, error) {
	return c.rejectJoinRequest.get(key, fingerprint, now)
}

func (c *idempotencyCache) storeRejectJoinRequestResult(
	key string,
	fingerprint string,
	joinRequest JoinRequest,
	now time.Time,
) {
	c.rejectJoinRequest.put(key, fingerprint, joinRequest, now)
}

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

func (c *idempotencyCache) storeCreateAccountResult(
	key string,
	fingerprint string,
	result createAccountCacheResult,
	now time.Time,
) {
	c.createAccount.put(key, fingerprint, cloneCreateAccountCacheResult(result), now)
}

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

func (c *idempotencyCache) storeBeginLoginResult(
	key string,
	fingerprint string,
	result beginLoginCacheResult,
	now time.Time,
) {
	c.beginLogin.put(key, fingerprint, cloneBeginLoginCacheResult(result), now)
}

func (c *idempotencyCache) verifyLoginResult(
	key string,
	fingerprint string,
	now time.Time,
) (LoginResult, bool, error) {
	return c.verifyLogin.get(key, fingerprint, now)
}

func (c *idempotencyCache) storeVerifyLoginResult(
	key string,
	fingerprint string,
	result LoginResult,
	now time.Time,
) {
	c.verifyLogin.put(key, fingerprint, result, now)
}

func (c *idempotencyCache) authenticateBotResult(
	key string,
	fingerprint string,
	now time.Time,
) (LoginResult, bool, error) {
	return c.authenticateBot.get(key, fingerprint, now)
}

func (c *idempotencyCache) storeAuthenticateBotResult(
	key string,
	fingerprint string,
	result LoginResult,
	now time.Time,
) {
	c.authenticateBot.put(key, fingerprint, result, now)
}

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

func (c *idempotencyCache) storeRegisterDeviceResult(
	key string,
	fingerprint string,
	result registerDeviceCacheResult,
	now time.Time,
) {
	c.registerDevice.put(key, fingerprint, result, now)
}

func (c *idempotencyCache) revokeSessionResult(
	key string,
	fingerprint string,
	now time.Time,
) (Session, bool, error) {
	return c.revokeSession.get(key, fingerprint, now)
}

func (c *idempotencyCache) storeRevokeSessionResult(
	key string,
	fingerprint string,
	session Session,
	now time.Time,
) {
	c.revokeSession.put(key, fingerprint, session, now)
}

func (c *idempotencyCache) revokeAllSessionsResult(
	key string,
	fingerprint string,
	now time.Time,
) (uint32, bool, error) {
	return c.revokeAllSessions.get(key, fingerprint, now)
}

func (c *idempotencyCache) storeRevokeAllSessionsResult(
	key string,
	fingerprint string,
	revoked uint32,
	now time.Time,
) {
	c.revokeAllSessions.put(key, fingerprint, revoked, now)
}

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

func cloneBeginLoginCacheResult(result beginLoginCacheResult) beginLoginCacheResult {
	result.challenge = cloneLoginChallenge(result.challenge)
	result.targets = cloneLoginTargets(result.targets)
	return result
}

func cloneCreateAccountCacheResult(result createAccountCacheResult) createAccountCacheResult {
	result.account = cloneAccount(result.account)
	return result
}

func cloneApproveJoinRequestCacheResult(result approveJoinRequestCacheResult) approveJoinRequestCacheResult {
	result.account = cloneAccount(result.account)
	return result
}

func cloneAccount(account Account) Account {
	if len(account.Roles) == 0 {
		return account
	}

	account.Roles = append([]Role(nil), account.Roles...)
	return account
}

func cloneLoginChallenge(challenge LoginChallenge) LoginChallenge {
	if len(challenge.Targets) == 0 {
		return challenge
	}

	challenge.Targets = cloneLoginTargets(challenge.Targets)
	return challenge
}

func cloneLoginTargets(targets []LoginTarget) []LoginTarget {
	if len(targets) == 0 {
		return nil
	}

	return append([]LoginTarget(nil), targets...)
}
