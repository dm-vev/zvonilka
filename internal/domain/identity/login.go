package identity

import (
	"context"
	"errors"
	"fmt"
	"time"
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

	code, err := newSecret(s.loginCodeLength)
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
// The challenge is consumed before session issuance. If session issuance fails, the
// challenge is restored so the client can retry the same code once the downstream issue
// clears.
func (s *Service) VerifyLoginCode(ctx context.Context, params VerifyLoginCodeParams) (result LoginResult, err error) {
	if err = s.validateContext(ctx, "verify login code"); err != nil {
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
	if !now.Before(challenge.ExpiresAt) {
		_ = s.store.DeleteLoginChallenge(ctx, challenge.ID)
		return LoginResult{}, ErrExpiredChallenge
	}
	if hashSecret(params.Code) != challenge.CodeHash {
		return LoginResult{}, ErrInvalidCode
	}

	// Mark the challenge as consumed before any session or device writes happen.
	challenge.Used = true
	challenge.UsedAt = now
	if _, err = s.store.SaveLoginChallenge(ctx, challenge); err != nil {
		return LoginResult{}, fmt.Errorf("consume login challenge %s: %w", challenge.ID, err)
	}
	challengeSaved := true
	defer func() {
		if err == nil || !challengeSaved {
			return
		}

		restoredChallenge := challenge
		restoredChallenge.Used = false
		restoredChallenge.UsedAt = time.Time{}
		if _, restoreErr := s.store.SaveLoginChallenge(ctx, restoredChallenge); restoreErr != nil {
			err = errors.Join(
				err,
				fmt.Errorf("restore login challenge %s after failed login: %w", challenge.ID, restoreErr),
			)
		}
	}()

	err = s.store.WithinTx(ctx, func(tx Store) error {
		account, loadErr := tx.AccountByID(ctx, challenge.AccountID)
		if loadErr != nil {
			return fmt.Errorf("load account %s from challenge %s: %w", challenge.AccountID, challenge.ID, loadErr)
		}

		result, err = s.issueSession(ctx, tx, account, params.DeviceName, params.Platform, params.PublicKey, params.PushToken)
		return err
	})
	if err != nil {
		return LoginResult{}, err
	}
	challengeSaved = false

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

	var result LoginResult
	tokenHash := hashSecret(params.BotToken)
	err := s.store.WithinTx(ctx, func(tx Store) error {
		account, loadErr := tx.AccountByBotTokenHash(ctx, tokenHash)
		if loadErr != nil {
			return fmt.Errorf("authenticate bot by token: %w", loadErr)
		}
		if account.Kind != AccountKindBot {
			return ErrForbidden
		}

		var sessionErr error
		result, sessionErr = s.issueSession(ctx, tx, account, params.DeviceName, params.Platform, params.PublicKey, "")
		return sessionErr
	})
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

	var (
		session      Session
		account      Account
		savedDevice  Device
		savedSession Session
		err          error
	)

	err = s.store.WithinTx(ctx, func(tx Store) error {
		var loadErr error

		session, loadErr = tx.SessionByID(ctx, params.SessionID)
		if loadErr != nil {
			return fmt.Errorf("load session %s: %w", params.SessionID, loadErr)
		}
		if session.Status != SessionStatusActive {
			return ErrForbidden
		}

		account, loadErr = tx.AccountByID(ctx, session.AccountID)
		if loadErr != nil {
			return fmt.Errorf("load account %s for session %s: %w", session.AccountID, session.ID, loadErr)
		}
		if account.Status != AccountStatusActive {
			return ErrForbidden
		}

		if loadErr := s.touchAccount(ctx, tx, account); loadErr != nil {
			return fmt.Errorf("touch account %s before device registration: %w", account.ID, loadErr)
		}

		// Re-load the session after the account touch so a concurrent revoke cannot
		// continue from the stale active snapshot we observed before the boundary.
		session, loadErr = tx.SessionByID(ctx, params.SessionID)
		if loadErr != nil {
			return fmt.Errorf("reload session %s after account touch: %w", params.SessionID, loadErr)
		}
		if session.AccountID != account.ID {
			return ErrConflict
		}
		if session.Status != SessionStatusActive {
			return ErrForbidden
		}

		deviceName := params.DeviceName
		if deviceName == "" {
			deviceName = account.DisplayName
			if deviceName == "" {
				deviceName = account.Username
			}
		}

		now := params.RequestedAt
		if now.IsZero() {
			now = s.currentTime()
		}

		deviceID, idErr := newID("dev")
		if idErr != nil {
			return fmt.Errorf("generate device ID: %w", idErr)
		}

		device := Device{
			ID:         deviceID,
			AccountID:  account.ID,
			SessionID:  session.ID,
			Name:       deviceName,
			Platform:   params.Platform,
			Status:     DeviceStatusActive,
			PublicKey:  params.PublicKey,
			PushToken:  params.PushToken,
			CreatedAt:  now,
			LastSeenAt: now,
		}

		savedDevice, loadErr = tx.SaveDevice(ctx, device)
		if loadErr != nil {
			return fmt.Errorf("save device for session %s: %w", session.ID, loadErr)
		}

		session.LastSeenAt = now
		savedSession, loadErr = tx.UpdateSession(ctx, session)
		if loadErr != nil {
			return fmt.Errorf("update session %s after device registration: %w", session.ID, loadErr)
		}

		return nil
	})
	if err != nil {
		return Device{}, Session{}, err
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
