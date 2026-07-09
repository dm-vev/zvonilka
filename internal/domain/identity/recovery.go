package identity

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// BeginPasswordRecovery starts a recovery-specific challenge for a human account.
func (s *Service) BeginPasswordRecovery(
	ctx context.Context,
	params BeginPasswordRecoveryParams,
) (LoginChallenge, []LoginTarget, error) {
	if err := s.validateContext(ctx, "begin password recovery"); err != nil {
		return LoginChallenge{}, nil, err
	}

	username, email, phone := s.normalizeAccountInput(params.Username, params.Email, params.Phone)
	account, err := s.lookupAccountByIdentifier(ctx, username, email, phone)
	if err != nil {
		return LoginChallenge{}, nil, err
	}
	if account.Kind != AccountKindUser || account.Status != AccountStatusActive {
		return LoginChallenge{}, nil, ErrForbidden
	}
	if _, err := s.store.AccountCredentialByAccountID(ctx, account.ID, AccountCredentialKindRecovery); err != nil {
		if err == ErrNotFound {
			return LoginChallenge{}, nil, ErrForbidden
		}
		return LoginChallenge{}, nil, fmt.Errorf("load recovery credential for %s: %w", account.ID, err)
	}

	targets, delivery, err := s.selectLoginTargets(account, params.Delivery)
	if err != nil {
		return LoginChallenge{}, nil, err
	}

	code, err := newSecret(s.loginCodeLength)
	if err != nil {
		return LoginChallenge{}, nil, fmt.Errorf("generate recovery code for %s: %w", account.ID, err)
	}
	challengeID, err := newID("rchal")
	if err != nil {
		return LoginChallenge{}, nil, fmt.Errorf("generate recovery challenge ID for %s: %w", account.ID, err)
	}

	now := params.RequestedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	challenge := LoginChallenge{
		ID:              challengeID,
		AccountID:       account.ID,
		AccountKind:     account.Kind,
		Purpose:         LoginChallengePurposeRecovery,
		CodeHash:        hashSecret(code),
		DeliveryChannel: delivery,
		Targets:         append([]LoginTarget(nil), targets...),
		ExpiresAt:       now.Add(s.challengeTTL),
		CreatedAt:       now,
	}

	saved, err := s.store.SaveLoginChallenge(ctx, challenge)
	if err != nil {
		return LoginChallenge{}, nil, fmt.Errorf("save recovery challenge for %s: %w", account.ID, err)
	}
	for _, target := range targets {
		if err := s.sender.SendLoginCode(ctx, target, code); err != nil {
			if deleteErr := s.store.DeleteLoginChallenge(ctx, saved.ID); deleteErr != nil {
				return LoginChallenge{}, nil, errors.Join(err, deleteErr)
			}
			return LoginChallenge{}, nil, fmt.Errorf("send recovery code for %s: %w", account.ID, err)
		}
	}

	return saved, targets, nil
}

// CompletePasswordRecovery consumes a recovery challenge, rotates stored secrets, and issues a new session.
func (s *Service) CompletePasswordRecovery(
	ctx context.Context,
	params CompletePasswordRecoveryParams,
) (result LoginResult, recoveryEnabled bool, err error) {
	if err = s.validateContext(ctx, "complete password recovery"); err != nil {
		return LoginResult{}, false, err
	}
	if params.RecoveryChallengeID == "" || params.Code == "" || params.NewPassword == "" || params.PublicKey == "" {
		return LoginResult{}, false, ErrInvalidInput
	}

	challenge, err := s.store.LoginChallengeByID(ctx, params.RecoveryChallengeID)
	if err != nil {
		return LoginResult{}, false, fmt.Errorf("load recovery challenge %s: %w", params.RecoveryChallengeID, err)
	}
	if challenge.Purpose != LoginChallengePurposeRecovery {
		return LoginResult{}, false, ErrForbidden
	}
	if challenge.Used {
		return LoginResult{}, false, ErrConflict
	}

	now := params.RequestedAt
	if now.IsZero() {
		now = s.currentTime()
	}
	if !now.Before(challenge.ExpiresAt) {
		_ = s.store.DeleteLoginChallenge(ctx, challenge.ID)
		return LoginResult{}, false, ErrExpiredChallenge
	}
	if hashSecret(params.Code) != challenge.CodeHash {
		return LoginResult{}, false, ErrInvalidCode
	}

	challenge.Used = true
	challenge.UsedAt = now
	if _, err = s.store.SaveLoginChallenge(ctx, challenge); err != nil {
		return LoginResult{}, false, fmt.Errorf("consume recovery challenge %s: %w", challenge.ID, err)
	}
	challengeSaved := true
	defer func() {
		if err == nil || !challengeSaved {
			return
		}
		restored := challenge
		restored.Used = false
		restored.UsedAt = time.Time{}
		if _, restoreErr := s.store.SaveLoginChallenge(ctx, restored); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restore recovery challenge %s: %w", challenge.ID, restoreErr))
		}
	}()

	err = s.store.WithinTx(ctx, func(tx Store) error {
		account, loadErr := tx.AccountByID(ctx, challenge.AccountID)
		if loadErr != nil {
			return fmt.Errorf("load recovery account %s: %w", challenge.AccountID, loadErr)
		}
		if account.Status != AccountStatusActive {
			return ErrForbidden
		}

		if saveErr := s.saveAccountCredential(
			ctx,
			tx,
			account.ID,
			AccountCredentialKindPassword,
			params.NewPassword,
			now,
		); saveErr != nil {
			return saveErr
		}
		if params.NewRecoveryPassword != "" {
			if saveErr := s.saveAccountCredential(
				ctx,
				tx,
				account.ID,
				AccountCredentialKindRecovery,
				params.NewRecoveryPassword,
				now,
			); saveErr != nil {
				return saveErr
			}
			recoveryEnabled = true
		} else {
			_, recoveryErr := tx.AccountCredentialByAccountID(ctx, account.ID, AccountCredentialKindRecovery)
			recoveryEnabled = recoveryErr == nil
		}

		result, err = s.issueSession(ctx, tx, account, params.DeviceName, params.Platform, params.PublicKey, "")
		return err
	})
	if err != nil {
		return LoginResult{}, false, err
	}
	challengeSaved = false

	return result, recoveryEnabled, nil
}

func (s *Service) saveAccountCredential(
	ctx context.Context,
	store Store,
	accountID string,
	kind AccountCredentialKind,
	secret string,
	now time.Time,
) error {
	secret = trimmed(secret)
	if secret == "" {
		return ErrInvalidInput
	}

	credential := AccountCredential{
		AccountID:  accountID,
		Kind:       kind,
		SecretHash: hashSecret(secret),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if existing, err := store.AccountCredentialByAccountID(ctx, accountID, kind); err == nil {
		credential.CreatedAt = existing.CreatedAt
	} else if err != ErrNotFound {
		return fmt.Errorf("load account credential %s:%s: %w", accountID, kind, err)
	}

	if _, err := store.SaveAccountCredential(ctx, credential); err != nil {
		return fmt.Errorf("save account credential %s:%s: %w", accountID, kind, err)
	}

	return nil
}
