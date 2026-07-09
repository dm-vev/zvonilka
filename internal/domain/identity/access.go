package identity

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RefreshSession rotates the bearer token pair for an active session.
func (s *Service) RefreshSession(ctx context.Context, params RefreshSessionParams) (LoginResult, error) {
	if err := s.validateContext(ctx, "refresh session"); err != nil {
		return LoginResult{}, err
	}
	if params.IdempotencyKey != "" {
		fingerprint := refreshSessionFingerprint(params)
		if result, ok, err := s.idempotency.refreshSessionResult(params.IdempotencyKey, fingerprint, s.currentTime()); err != nil {
			return LoginResult{}, err
		} else if ok {
			return result, nil
		}
	}

	params.RefreshToken = strings.TrimSpace(params.RefreshToken)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.RefreshToken == "" {
		return LoginResult{}, ErrInvalidInput
	}

	now := params.RequestedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var result LoginResult
	err := s.store.WithinTx(ctx, func(tx Store) error {
		credential, loadErr := tx.SessionCredentialByTokenHash(
			ctx,
			hashSecret(params.RefreshToken),
			SessionCredentialKindRefresh,
		)
		if loadErr != nil {
			if isNotFound(loadErr) {
				return ErrUnauthorized
			}
			return fmt.Errorf("load refresh credential: %w", loadErr)
		}
		if !now.Before(credential.ExpiresAt) {
			return ErrExpiredToken
		}
		if params.DeviceID != "" && credential.DeviceID != params.DeviceID {
			return ErrUnauthorized
		}

		account, loadErr := s.lockAccount(ctx, tx, credential.AccountID)
		if loadErr != nil {
			return fmt.Errorf("lock account %s for refresh: %w", credential.AccountID, loadErr)
		}
		if account.Status != AccountStatusActive {
			return ErrForbidden
		}

		session, loadErr := tx.SessionByID(ctx, credential.SessionID)
		if loadErr != nil {
			return fmt.Errorf("load session %s for refresh: %w", credential.SessionID, loadErr)
		}
		if session.AccountID != account.ID || session.Status != SessionStatusActive {
			return ErrUnauthorized
		}

		device, loadErr := tx.DeviceByID(ctx, credential.DeviceID)
		if loadErr != nil {
			return fmt.Errorf("load device %s for refresh: %w", credential.DeviceID, loadErr)
		}
		if device.AccountID != account.ID || device.SessionID != session.ID {
			return ErrUnauthorized
		}
		if device.Status != DeviceStatusActive && device.Status != DeviceStatusUnverified {
			return ErrForbidden
		}

		account.LastAuthAt = now
		account.UpdatedAt = now
		if touchErr := s.touchAccount(ctx, tx, account); touchErr != nil {
			return fmt.Errorf("touch account %s during refresh: %w", account.ID, touchErr)
		}

		tokens, tokenErr := s.rotateCredentials(ctx, tx, account, device, session, now)
		if tokenErr != nil {
			return tokenErr
		}

		result = LoginResult{
			Tokens:  tokens,
			Session: session,
			Device:  device,
		}
		return nil
	})
	if err != nil {
		return LoginResult{}, err
	}

	if params.IdempotencyKey != "" {
		s.idempotency.storeRefreshSessionResult(
			params.IdempotencyKey,
			refreshSessionFingerprint(params),
			result,
			s.currentTime(),
		)
	}

	return result, nil
}

// AuthenticateAccessToken resolves an active access token into the bound account, device, and session.
func (s *Service) AuthenticateAccessToken(ctx context.Context, accessToken string) (AuthContext, error) {
	if err := s.validateContext(ctx, "authenticate access token"); err != nil {
		return AuthContext{}, err
	}

	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return AuthContext{}, ErrUnauthorized
	}

	credential, err := s.store.SessionCredentialByTokenHash(
		ctx,
		hashSecret(accessToken),
		SessionCredentialKindAccess,
	)
	if err != nil {
		if isNotFound(err) {
			return AuthContext{}, ErrUnauthorized
		}
		return AuthContext{}, fmt.Errorf("load access credential: %w", err)
	}

	now := s.currentTime()
	if !now.Before(credential.ExpiresAt) {
		return AuthContext{}, ErrExpiredToken
	}

	account, err := s.store.AccountByID(ctx, credential.AccountID)
	if err != nil {
		return AuthContext{}, fmt.Errorf("load account %s for access token: %w", credential.AccountID, err)
	}
	if account.Status != AccountStatusActive {
		return AuthContext{}, ErrForbidden
	}

	session, err := s.store.SessionByID(ctx, credential.SessionID)
	if err != nil {
		return AuthContext{}, fmt.Errorf("load session %s for access token: %w", credential.SessionID, err)
	}
	if session.AccountID != account.ID || session.Status != SessionStatusActive {
		return AuthContext{}, ErrUnauthorized
	}

	device, err := s.store.DeviceByID(ctx, credential.DeviceID)
	if err != nil {
		return AuthContext{}, fmt.Errorf("load device %s for access token: %w", credential.DeviceID, err)
	}
	if device.AccountID != account.ID || device.SessionID != session.ID {
		return AuthContext{}, ErrUnauthorized
	}
	if device.Status != DeviceStatusActive && device.Status != DeviceStatusUnverified {
		return AuthContext{}, ErrForbidden
	}

	return AuthContext{
		Account: account,
		Device:  device,
		Session: session,
	}, nil
}

func (s *Service) rotateCredentials(
	ctx context.Context,
	store Store,
	account Account,
	device Device,
	session Session,
	now time.Time,
) (Tokens, error) {
	accessToken, err := randomToken(32)
	if err != nil {
		return Tokens{}, fmt.Errorf("generate access token for account %s: %w", account.ID, err)
	}

	refreshToken, err := randomToken(32)
	if err != nil {
		return Tokens{}, fmt.Errorf("generate refresh token for account %s: %w", account.ID, err)
	}

	accessCredential := SessionCredential{
		SessionID: session.ID,
		AccountID: account.ID,
		DeviceID:  device.ID,
		Kind:      SessionCredentialKindAccess,
		TokenHash: hashSecret(accessToken),
		ExpiresAt: now.Add(s.accessTokenTTL),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := store.SaveSessionCredential(ctx, accessCredential); err != nil {
		return Tokens{}, fmt.Errorf("save access credential for session %s: %w", session.ID, err)
	}

	refreshCredential := SessionCredential{
		SessionID: session.ID,
		AccountID: account.ID,
		DeviceID:  device.ID,
		Kind:      SessionCredentialKindRefresh,
		TokenHash: hashSecret(refreshToken),
		ExpiresAt: now.Add(s.refreshTokenTTL),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := store.SaveSessionCredential(ctx, refreshCredential); err != nil {
		return Tokens{}, fmt.Errorf("save refresh credential for session %s: %w", session.ID, err)
	}

	return Tokens{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		TokenType:        "Bearer",
		ExpiresAt:        accessCredential.ExpiresAt,
		RefreshExpiresAt: refreshCredential.ExpiresAt,
	}, nil
}
