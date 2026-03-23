package identity

import (
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

type idempotencyCache struct {
	mu                 sync.Mutex
	submitJoinRequest  map[string]idempotencyEntry[JoinRequest]
	approveJoinRequest map[string]idempotencyEntry[approveJoinRequestCacheResult]
	rejectJoinRequest  map[string]idempotencyEntry[JoinRequest]
	createAccount      map[string]idempotencyEntry[createAccountCacheResult]
	beginLogin         map[string]idempotencyEntry[beginLoginCacheResult]
	verifyLogin        map[string]idempotencyEntry[LoginResult]
	authenticateBot    map[string]idempotencyEntry[LoginResult]
	registerDevice     map[string]idempotencyEntry[registerDeviceCacheResult]
	revokeSession      map[string]idempotencyEntry[Session]
	revokeAllSessions  map[string]idempotencyEntry[uint32]
}

func newIdempotencyCache() *idempotencyCache {
	return &idempotencyCache{
		submitJoinRequest:  make(map[string]idempotencyEntry[JoinRequest]),
		approveJoinRequest: make(map[string]idempotencyEntry[approveJoinRequestCacheResult]),
		rejectJoinRequest:  make(map[string]idempotencyEntry[JoinRequest]),
		createAccount:      make(map[string]idempotencyEntry[createAccountCacheResult]),
		beginLogin:         make(map[string]idempotencyEntry[beginLoginCacheResult]),
		verifyLogin:        make(map[string]idempotencyEntry[LoginResult]),
		authenticateBot:    make(map[string]idempotencyEntry[LoginResult]),
		registerDevice:     make(map[string]idempotencyEntry[registerDeviceCacheResult]),
		revokeSession:      make(map[string]idempotencyEntry[Session]),
		revokeAllSessions:  make(map[string]idempotencyEntry[uint32]),
	}
}

func (c *idempotencyCache) submitJoinRequestResult(
	key string,
	fingerprint string,
	now time.Time,
) (JoinRequest, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return lookupIdempotencyResult(c.submitJoinRequest, key, fingerprint, now)
}

func (c *idempotencyCache) storeSubmitJoinRequestResult(
	key string,
	fingerprint string,
	joinRequest JoinRequest,
	now time.Time,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyResult(c.submitJoinRequest, key, fingerprint, joinRequest, now)
}

func (c *idempotencyCache) approveJoinRequestResult(
	key string,
	fingerprint string,
	now time.Time,
) (approveJoinRequestCacheResult, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result, ok, err := lookupIdempotencyResult(c.approveJoinRequest, key, fingerprint, now)
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
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyResult(c.approveJoinRequest, key, fingerprint, cloneApproveJoinRequestCacheResult(result), now)
}

func (c *idempotencyCache) rejectJoinRequestResult(
	key string,
	fingerprint string,
	now time.Time,
) (JoinRequest, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return lookupIdempotencyResult(c.rejectJoinRequest, key, fingerprint, now)
}

func (c *idempotencyCache) storeRejectJoinRequestResult(
	key string,
	fingerprint string,
	joinRequest JoinRequest,
	now time.Time,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyResult(c.rejectJoinRequest, key, fingerprint, joinRequest, now)
}

func (c *idempotencyCache) createAccountResult(
	key string,
	fingerprint string,
	now time.Time,
) (createAccountCacheResult, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result, ok, err := lookupIdempotencyResult(c.createAccount, key, fingerprint, now)
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
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyResult(c.createAccount, key, fingerprint, cloneCreateAccountCacheResult(result), now)
}

func (c *idempotencyCache) beginLoginResult(
	key string,
	fingerprint string,
	now time.Time,
) (beginLoginCacheResult, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result, ok, err := lookupIdempotencyResult(c.beginLogin, key, fingerprint, now)
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
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyResult(c.beginLogin, key, fingerprint, cloneBeginLoginCacheResult(result), now)
}

func (c *idempotencyCache) verifyLoginResult(
	key string,
	fingerprint string,
	now time.Time,
) (LoginResult, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return lookupIdempotencyResult(c.verifyLogin, key, fingerprint, now)
}

func (c *idempotencyCache) storeVerifyLoginResult(
	key string,
	fingerprint string,
	result LoginResult,
	now time.Time,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyResult(c.verifyLogin, key, fingerprint, result, now)
}

func (c *idempotencyCache) authenticateBotResult(
	key string,
	fingerprint string,
	now time.Time,
) (LoginResult, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return lookupIdempotencyResult(c.authenticateBot, key, fingerprint, now)
}

func (c *idempotencyCache) storeAuthenticateBotResult(
	key string,
	fingerprint string,
	result LoginResult,
	now time.Time,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyResult(c.authenticateBot, key, fingerprint, result, now)
}

func (c *idempotencyCache) registerDeviceResult(
	key string,
	fingerprint string,
	now time.Time,
) (Device, Session, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result, ok, err := lookupIdempotencyResult(c.registerDevice, key, fingerprint, now)
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
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyResult(c.registerDevice, key, fingerprint, result, now)
}

func (c *idempotencyCache) revokeSessionResult(
	key string,
	fingerprint string,
	now time.Time,
) (Session, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return lookupIdempotencyResult(c.revokeSession, key, fingerprint, now)
}

func (c *idempotencyCache) storeRevokeSessionResult(
	key string,
	fingerprint string,
	session Session,
	now time.Time,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyResult(c.revokeSession, key, fingerprint, session, now)
}

func (c *idempotencyCache) revokeAllSessionsResult(
	key string,
	fingerprint string,
	now time.Time,
) (uint32, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return lookupIdempotencyResult(c.revokeAllSessions, key, fingerprint, now)
}

func (c *idempotencyCache) storeRevokeAllSessionsResult(
	key string,
	fingerprint string,
	revoked uint32,
	now time.Time,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyResult(c.revokeAllSessions, key, fingerprint, revoked, now)
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
