package identity

import "sync"

type idempotencyCache struct {
	mu                sync.Mutex
	beginLogin        map[string]beginLoginCacheEntry
	verifyLogin       map[string]LoginResult
	registerDevice    map[string]registerDeviceCacheEntry
	revokeSession     map[string]Session
	revokeAllSessions map[string]uint32
}

type beginLoginCacheEntry struct {
	challenge LoginChallenge
	targets   []LoginTarget
}

type registerDeviceCacheEntry struct {
	device  Device
	session Session
}

func newIdempotencyCache() *idempotencyCache {
	return &idempotencyCache{
		beginLogin:        make(map[string]beginLoginCacheEntry),
		verifyLogin:       make(map[string]LoginResult),
		registerDevice:    make(map[string]registerDeviceCacheEntry),
		revokeSession:     make(map[string]Session),
		revokeAllSessions: make(map[string]uint32),
	}
}

func (c *idempotencyCache) beginLoginResult(key string) (LoginChallenge, []LoginTarget, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.beginLogin[key]
	if !ok {
		return LoginChallenge{}, nil, false
	}

	return entry.challenge, cloneLoginTargets(entry.targets), true
}

func (c *idempotencyCache) storeBeginLoginResult(
	key string,
	challenge LoginChallenge,
	targets []LoginTarget,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.beginLogin[key] = beginLoginCacheEntry{
		challenge: challenge,
		targets:   cloneLoginTargets(targets),
	}
}

func (c *idempotencyCache) verifyLoginResult(key string) (LoginResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result, ok := c.verifyLogin[key]
	if !ok {
		return LoginResult{}, false
	}

	return result, true
}

func (c *idempotencyCache) storeVerifyLoginResult(key string, result LoginResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.verifyLogin[key] = result
}

func (c *idempotencyCache) registerDeviceResult(key string) (Device, Session, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.registerDevice[key]
	if !ok {
		return Device{}, Session{}, false
	}

	return entry.device, entry.session, true
}

func (c *idempotencyCache) storeRegisterDeviceResult(key string, device Device, session Session) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.registerDevice[key] = registerDeviceCacheEntry{
		device:  device,
		session: session,
	}
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

func cloneLoginTargets(targets []LoginTarget) []LoginTarget {
	if len(targets) == 0 {
		return nil
	}

	return append([]LoginTarget(nil), targets...)
}
