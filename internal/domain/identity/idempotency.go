package identity

import (
	"strconv"
	"strings"
	"sync"
)

type idempotencyCache struct {
	mu                sync.Mutex
	beginLogin        map[string]idempotencyEntry[beginLoginCacheResult]
	verifyLogin       map[string]idempotencyEntry[LoginResult]
	registerDevice    map[string]idempotencyEntry[registerDeviceCacheResult]
	revokeSession     map[string]Session
	revokeAllSessions map[string]uint32
}

type idempotencyEntry[T any] struct {
	fingerprint string
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

func newIdempotencyCache() *idempotencyCache {
	return &idempotencyCache{
		beginLogin:        make(map[string]idempotencyEntry[beginLoginCacheResult]),
		verifyLogin:       make(map[string]idempotencyEntry[LoginResult]),
		registerDevice:    make(map[string]idempotencyEntry[registerDeviceCacheResult]),
		revokeSession:     make(map[string]Session),
		revokeAllSessions: make(map[string]uint32),
	}
}

func (c *idempotencyCache) beginLoginResult(
	key string,
	fingerprint string,
) (beginLoginCacheResult, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return lookupIdempotencyEntry(c.beginLogin, key, fingerprint)
}

func (c *idempotencyCache) storeBeginLoginResult(
	key string,
	fingerprint string,
	result beginLoginCacheResult,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyEntry(c.beginLogin, key, fingerprint, result)
}

func (c *idempotencyCache) verifyLoginResult(
	key string,
	fingerprint string,
) (LoginResult, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return lookupIdempotencyEntry(c.verifyLogin, key, fingerprint)
}

func (c *idempotencyCache) storeVerifyLoginResult(
	key string,
	fingerprint string,
	result LoginResult,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyEntry(c.verifyLogin, key, fingerprint, result)
}

func (c *idempotencyCache) registerDeviceResult(
	key string,
	fingerprint string,
) (Device, Session, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.registerDevice[key]
	if !ok {
		return Device{}, Session{}, false, nil
	}
	if entry.fingerprint != fingerprint {
		return Device{}, Session{}, false, ErrConflict
	}

	return entry.value.device, entry.value.session, true, nil
}

func (c *idempotencyCache) storeRegisterDeviceResult(
	key string,
	fingerprint string,
	result registerDeviceCacheResult,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	storeIdempotencyEntry(c.registerDevice, key, fingerprint, result)
}

func (c *idempotencyCache) revokeSessionResult(key string) (Session, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	session, ok := c.revokeSession[key]
	if !ok {
		return Session{}, false
	}

	return session, true
}

func (c *idempotencyCache) storeRevokeSessionResult(key string, session Session) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.revokeSession[key] = session
}

func (c *idempotencyCache) revokeAllSessionsResult(key string) (uint32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	revoked, ok := c.revokeAllSessions[key]
	if !ok {
		return 0, false
	}

	return revoked, true
}

func (c *idempotencyCache) storeRevokeAllSessionsResult(key string, revoked uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.revokeAllSessions[key] = revoked
}

func beginLoginFingerprint(params BeginLoginParams) string {
	return idempotencyFingerprint(
		"begin-login",
		normalizeUsername(params.Username),
		normalizeEmail(params.Email),
		normalizePhone(params.Phone),
		string(params.Delivery),
		params.DeviceName,
		string(params.Platform),
		params.ClientVersion,
		params.Locale,
	)
}

func verifyLoginFingerprint(params VerifyLoginCodeParams) string {
	return idempotencyFingerprint(
		"verify-login",
		params.ChallengeID,
		params.Code,
		params.TwoFactorCode,
		params.RecoveryPassword,
		strconv.FormatBool(params.EnablePasswordRecovery),
		params.DeviceName,
		string(params.Platform),
		params.PublicKey,
		params.PushToken,
	)
}

func registerDeviceFingerprint(params RegisterDeviceParams) string {
	return idempotencyFingerprint(
		"register-device",
		params.SessionID,
		params.DeviceName,
		string(params.Platform),
		params.PublicKey,
		params.PushToken,
	)
}

func idempotencyFingerprint(parts ...string) string {
	return hashSecret(strings.Join(parts, "\x1f"))
}

func cloneLoginTargets(targets []LoginTarget) []LoginTarget {
	if len(targets) == 0 {
		return nil
	}

	return append([]LoginTarget(nil), targets...)
}

func lookupIdempotencyEntry[T any](
	cache map[string]idempotencyEntry[T],
	key string,
	fingerprint string,
) (T, bool, error) {
	entry, ok := cache[key]
	if !ok {
		var zero T
		return zero, false, nil
	}
	if entry.fingerprint != fingerprint {
		var zero T
		return zero, false, ErrConflict
	}

	return entry.value, true, nil
}

func storeIdempotencyEntry[T any](
	cache map[string]idempotencyEntry[T],
	key string,
	fingerprint string,
	value T,
) {
	cache[key] = idempotencyEntry[T]{
		fingerprint: fingerprint,
		value:       value,
	}
}
