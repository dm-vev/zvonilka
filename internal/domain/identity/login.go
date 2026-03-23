package identity

import (
	"context"
	"errors"
	"fmt"
)

// BeginLogin starts a code-based login challenge for a human account.
//
// The challenge is persisted before the code is sent so the request can be retried
// safely if delivery fails or the client retries with the same idempotency key.
func (s *Service) BeginLogin(ctx context.Context, params BeginLoginParams) (LoginChallenge, []LoginTarget, error) {
	if err := s.validateContext(ctx, "begin login"); err != nil {
		return LoginChallenge{}, nil, err
	}
	username, email, phone := s.normalizeAccountInput(params.Username, params.Email, params.Phone)
	fingerprint := beginLoginFingerprint(params)
	if params.IdempotencyKey != "" {
		if cached, ok, err := s.idempotency.beginLoginResult(params.IdempotencyKey, fingerprint, s.currentTime()); err != nil {
			return LoginChallenge{}, nil, err
		} else if ok {
			return cached.challenge, cached.targets, nil
		}
	}

	account, err := s.lookupAccountByIdentifier(ctx, username, email, phone)
	if err != nil {
		return LoginChallenge{}, nil, err
	}
	if account.Kind == AccountKindBot {
		return LoginChallenge{}, nil, ErrForbidden
	}
	if account.Status != AccountStatusActive {
		return LoginChallenge{}, nil, ErrForbidden
	}
	if !hasContactInformation(account) {
		return LoginChallenge{}, nil, ErrInvalidInput
	}

	targets, delivery, err := s.selectLoginTargets(account, params.Delivery)
	if err != nil {
		return LoginChallenge{}, nil, err
	}

	code, err := newSecret(6)
	if err != nil {
		return LoginChallenge{}, nil, fmt.Errorf("generate login code for account %s: %w", account.ID, err)
	}

	challengeID, err := newID("chal")
	if err != nil {
		return LoginChallenge{}, nil, fmt.Errorf("generate challenge ID for account %s: %w", account.ID, err)
	}

	now := params.RequestedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	challenge := LoginChallenge{
		ID:              challengeID,
		AccountID:       account.ID,
		AccountKind:     account.Kind,
		CodeHash:        hashSecret(code),
		DeliveryChannel: delivery,
		Targets:         append([]LoginTarget(nil), targets...),
		ExpiresAt:       now.Add(s.challengeTTL),
		CreatedAt:       now,
	}

	savedChallenge, err := s.store.SaveLoginChallenge(ctx, challenge)
	if err != nil {
		return LoginChallenge{}, nil, fmt.Errorf("save login challenge for account %s: %w", account.ID, err)
	}

	for _, target := range targets {
		if err := s.sender.SendLoginCode(ctx, target, code); err != nil {
			if deleteErr := s.store.DeleteLoginChallenge(ctx, savedChallenge.ID); deleteErr != nil {
				return LoginChallenge{}, nil, errors.Join(
					fmt.Errorf("send login code for account %s: %w", account.ID, err),
					fmt.Errorf("delete login challenge %s: %w", savedChallenge.ID, deleteErr),
				)
			}
			return LoginChallenge{}, nil, fmt.Errorf("send login code for account %s: %w", account.ID, err)
		}
	}

	if params.IdempotencyKey != "" {
		s.idempotency.storeBeginLoginResult(
			params.IdempotencyKey,
			fingerprint,
			beginLoginCacheResult{
				challenge: savedChallenge,
				targets:   targets,
			},
			s.currentTime(),
		)
	}

	return savedChallenge, targets, nil
}

// VerifyLoginCode completes a login challenge and issues a new session.
//
// The challenge is consumed before session issuance so the same code cannot be replayed
// if a later write fails. That keeps the login code single-use even across retries.
func (s *Service) VerifyLoginCode(ctx context.Context, params VerifyLoginCodeParams) (LoginResult, error) {
	if err := s.validateContext(ctx, "verify login code"); err != nil {
		return LoginResult{}, err
	}
	if params.IdempotencyKey != "" {
		fingerprint := verifyLoginFingerprint(params)
		if result, ok, err := s.idempotency.verifyLoginResult(params.IdempotencyKey, fingerprint, s.currentTime()); err != nil {
			return LoginResult{}, err
		} else if ok {
			return result, nil
		}
	}
	if params.ChallengeID == "" || params.Code == "" {
		return LoginResult{}, ErrInvalidInput
	}
	if params.PublicKey == "" {
		return LoginResult{}, ErrInvalidInput
	}

	challenge, err := s.store.LoginChallengeByID(ctx, params.ChallengeID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("load login challenge %s: %w", params.ChallengeID, err)
	}
	if challenge.Used {
		return LoginResult{}, ErrConflict
	}

	now := params.RequestedAt
	if now.IsZero() {
		now = s.currentTime()
	}
	if now.After(challenge.ExpiresAt) {
		_ = s.store.DeleteLoginChallenge(ctx, challenge.ID)
		return LoginResult{}, ErrExpiredChallenge
	}
	if hashSecret(params.Code) != challenge.CodeHash {
		return LoginResult{}, ErrInvalidCode
	}

	// Mark the challenge as consumed before any session or device writes happen.
	challenge.Used = true
	challenge.UsedAt = now
	if _, err := s.store.SaveLoginChallenge(ctx, challenge); err != nil {
		return LoginResult{}, fmt.Errorf("consume login challenge %s: %w", challenge.ID, err)
	}

	account, err := s.store.AccountByID(ctx, challenge.AccountID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("load account %s from challenge %s: %w", challenge.AccountID, challenge.ID, err)
	}

	result, err := s.issueSession(ctx, account, params.DeviceName, params.Platform, params.PublicKey, params.PushToken)
	if err != nil {
		return LoginResult{}, err
	}

	if params.IdempotencyKey != "" {
		s.idempotency.storeVerifyLoginResult(
			params.IdempotencyKey,
			verifyLoginFingerprint(params),
			result,
			s.currentTime(),
		)
	}

	return result, nil
}

// AuthenticateBot logs a bot account in using its issued bot token.
//
// Bot login reuses the same session issuance path as human login so the downstream
// device and session invariants stay identical.
func (s *Service) AuthenticateBot(ctx context.Context, params AuthenticateBotParams) (LoginResult, error) {
	if err := s.validateContext(ctx, "authenticate bot"); err != nil {
		return LoginResult{}, err
	}
	if params.BotToken == "" || params.PublicKey == "" {
		return LoginResult{}, ErrInvalidInput
	}
	fingerprint := authenticateBotFingerprint(params)
	if params.IdempotencyKey != "" {
		if result, ok, err := s.idempotency.authenticateBotResult(params.IdempotencyKey, fingerprint, s.currentTime()); err != nil {
			return LoginResult{}, err
		} else if ok {
			return result, nil
		}
	}

	tokenHash := hashSecret(params.BotToken)
	account, err := s.store.AccountByBotTokenHash(ctx, tokenHash)
	if err != nil {
		return LoginResult{}, fmt.Errorf("authenticate bot by token: %w", err)
	}
	if account.Kind != AccountKindBot {
		return LoginResult{}, ErrForbidden
	}

	result, err := s.issueSession(ctx, account, params.DeviceName, params.Platform, params.PublicKey, "")
	if err != nil {
		return LoginResult{}, err
	}

	if params.IdempotencyKey != "" {
		s.idempotency.storeAuthenticateBotResult(
			params.IdempotencyKey,
			fingerprint,
			result,
			s.currentTime(),
		)
	}

	return result, nil
}

// RegisterDevice attaches another trusted device to an active session.
//
// The new device inherits the owning account's display name when the client does not
// supply one, which keeps the UI default stable across first-party and retry flows.
func (s *Service) RegisterDevice(ctx context.Context, params RegisterDeviceParams) (Device, Session, error) {
	if err := s.validateContext(ctx, "register device"); err != nil {
		return Device{}, Session{}, err
	}
	if params.IdempotencyKey != "" {
		fingerprint := registerDeviceFingerprint(params)
		if device, session, ok, err := s.idempotency.registerDeviceResult(params.IdempotencyKey, fingerprint, s.currentTime()); err != nil {
			return Device{}, Session{}, err
		} else if ok {
			return device, session, nil
		}
	}
	if params.SessionID == "" || params.PublicKey == "" {
		return Device{}, Session{}, ErrInvalidInput
	}

	session, err := s.store.SessionByID(ctx, params.SessionID)
	if err != nil {
		return Device{}, Session{}, fmt.Errorf("load session %s: %w", params.SessionID, err)
	}
	if session.Status != SessionStatusActive {
		return Device{}, Session{}, ErrForbidden
	}

	account, err := s.store.AccountByID(ctx, session.AccountID)
	if err != nil {
		return Device{}, Session{}, fmt.Errorf("load account %s for session %s: %w", session.AccountID, session.ID, err)
	}
	if account.Status != AccountStatusActive {
		return Device{}, Session{}, ErrForbidden
	}

	if params.DeviceName == "" {
		params.DeviceName = account.DisplayName
		if params.DeviceName == "" {
			params.DeviceName = account.Username
		}
	}

	now := params.RequestedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	deviceID, err := newID("dev")
	if err != nil {
		return Device{}, Session{}, fmt.Errorf("generate device ID: %w", err)
	}

	device := Device{
		ID:         deviceID,
		AccountID:  account.ID,
		SessionID:  session.ID,
		Name:       params.DeviceName,
		Platform:   params.Platform,
		Status:     DeviceStatusActive,
		PublicKey:  params.PublicKey,
		PushToken:  params.PushToken,
		CreatedAt:  now,
		LastSeenAt: now,
	}

	savedDevice, err := s.store.SaveDevice(ctx, device)
	if err != nil {
		return Device{}, Session{}, fmt.Errorf("save device for session %s: %w", session.ID, err)
	}

	session.LastSeenAt = now
	savedSession, err := s.store.UpdateSession(ctx, session)
	if err != nil {
		if deleteErr := s.store.DeleteDevice(ctx, savedDevice.ID); deleteErr != nil {
			return Device{}, Session{}, errors.Join(
				fmt.Errorf("update session %s after device registration: %w", session.ID, err),
				fmt.Errorf("delete device %s: %w", savedDevice.ID, deleteErr),
			)
		}
		return Device{}, Session{}, fmt.Errorf("update session %s after device registration: %w", session.ID, err)
	}

	if params.IdempotencyKey != "" {
		s.idempotency.storeRegisterDeviceResult(
			params.IdempotencyKey,
			registerDeviceFingerprint(params),
			registerDeviceCacheResult{
				device:  savedDevice,
				session: savedSession,
			},
			s.currentTime(),
		)
	}

	return savedDevice, savedSession, nil
}
