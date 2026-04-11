package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/dm-vev/zvonilka/internal/domain/storage"
)

// SubmitJoinRequest stores a pending request for moderator review.
//
// The service canonicalizes the identifiers first and relies on the store to enforce
// uniqueness atomically, which keeps the write path race-free without a separate
// read-before-write availability check.
func (s *Service) SubmitJoinRequest(ctx context.Context, params SubmitJoinRequestParams) (JoinRequest, error) {
	if err := s.validateContext(ctx, "submit join request"); err != nil {
		return JoinRequest{}, err
	}
	fingerprint := submitJoinRequestFingerprint(params)
	if params.IdempotencyKey != "" {
		if joinRequest, ok, err := s.idempotency.submitJoinRequestResult(params.IdempotencyKey, fingerprint, s.currentTime()); err != nil {
			return JoinRequest{}, err
		} else if ok {
			return joinRequest, nil
		}
	}

	username, email, phone := s.normalizeAccountInput(params.Username, params.Email, params.Phone)
	if username == "" {
		return JoinRequest{}, ErrInvalidInput
	}
	if email == "" && phone == "" {
		return JoinRequest{}, ErrInvalidInput
	}

	now := params.RequestedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	requestID, err := newID("join")
	if err != nil {
		return JoinRequest{}, fmt.Errorf("generate join request ID: %w", err)
	}

	joinRequest := JoinRequest{
		ID:          requestID,
		Username:    username,
		DisplayName: trimmed(params.DisplayName),
		Email:       email,
		Phone:       phone,
		Note:        trimmed(params.Note),
		Status:      JoinRequestStatusPending,
		RequestedAt: now,
		ExpiresAt:   now.Add(s.joinRequestTTL),
	}

	saved, err := s.store.SaveJoinRequest(ctx, joinRequest)
	if err != nil {
		return JoinRequest{}, fmt.Errorf("save join request for username %s: %w", username, err)
	}

	if params.IdempotencyKey != "" {
		s.idempotency.storeSubmitJoinRequestResult(params.IdempotencyKey, fingerprint, saved, s.currentTime())
	}

	return saved, nil
}

// ApproveJoinRequest converts a join request into an active user account.
//
// The entire approval flow runs inside a store-managed transaction so account creation
// and join-request state changes commit or roll back together.
func (s *Service) ApproveJoinRequest(ctx context.Context, params ApproveJoinRequestParams) (JoinRequest, Account, error) {
	if err := s.validateContext(ctx, "approve join request"); err != nil {
		return JoinRequest{}, Account{}, err
	}
	if params.JoinRequestID == "" {
		return JoinRequest{}, Account{}, ErrInvalidInput
	}
	fingerprint := approveJoinRequestFingerprint(params)
	if params.IdempotencyKey != "" {
		if result, ok, err := s.idempotency.approveJoinRequestResult(params.IdempotencyKey, fingerprint, s.currentTime()); err != nil {
			return JoinRequest{}, Account{}, err
		} else if ok {
			return result.joinRequest, result.account, nil
		}
	}

	var (
		savedJoinRequest JoinRequest
		account          Account
		botToken         string
		expiredJoin      JoinRequest
	)

	err := s.store.WithinTx(ctx, func(tx Store) error {
		joinRequest, loadErr := s.loadPendingJoinRequest(ctx, tx, params.JoinRequestID, params.ReviewedBy)
		if loadErr != nil {
			if errors.Is(loadErr, ErrExpiredJoinRequest) {
				expiredJoin = joinRequest
				return storage.Commit(loadErr)
			}

			return loadErr
		}

		now := s.currentTime()
		account, botToken, loadErr = s.createAccount(
			ctx,
			tx,
			CreateAccountParams{
				Username:       joinRequest.Username,
				DisplayName:    joinRequest.DisplayName,
				Email:          joinRequest.Email,
				Phone:          joinRequest.Phone,
				Roles:          params.Roles,
				Note:           params.Note,
				InviteCode:     "",
				AccountKind:    AccountKindUser,
				CreatedBy:      params.ReviewedBy,
				IdempotencyKey: params.IdempotencyKey,
				RequestedAt:    now,
			},
			true,
		)
		if loadErr != nil {
			return loadErr
		}
		if botToken != "" {
			return fmt.Errorf("join request created unexpected bot token")
		}

		joinRequest.Status = JoinRequestStatusApproved
		joinRequest.ReviewedAt = now
		joinRequest.ReviewedBy = params.ReviewedBy
		joinRequest.DecisionReason = trimmed(params.DecisionReason)

		savedJoinRequest, loadErr = tx.SaveJoinRequest(ctx, joinRequest)
		if loadErr != nil {
			return loadErr
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, ErrExpiredJoinRequest) {
			return expiredJoin, Account{}, err
		}

		return JoinRequest{}, Account{}, err
	}

	s.indexAccount(ctx, account)

	if params.IdempotencyKey != "" {
		s.idempotency.storeApproveJoinRequestResult(
			params.IdempotencyKey,
			fingerprint,
			approveJoinRequestCacheResult{
				joinRequest: savedJoinRequest,
				account:     account,
			},
			s.currentTime(),
		)
	}

	return savedJoinRequest, account, nil
}

// RejectJoinRequest marks a join request as rejected.
//
// The rejection path also runs inside a transaction so late-expiry transitions can
// be committed while keeping the write path consistent.
func (s *Service) RejectJoinRequest(ctx context.Context, params RejectJoinRequestParams) (JoinRequest, error) {
	if err := s.validateContext(ctx, "reject join request"); err != nil {
		return JoinRequest{}, err
	}
	if params.JoinRequestID == "" {
		return JoinRequest{}, ErrInvalidInput
	}
	fingerprint := rejectJoinRequestFingerprint(params)
	if params.IdempotencyKey != "" {
		if joinRequest, ok, err := s.idempotency.rejectJoinRequestResult(params.IdempotencyKey, fingerprint, s.currentTime()); err != nil {
			return JoinRequest{}, err
		} else if ok {
			return joinRequest, nil
		}
	}

	var (
		savedJoinRequest JoinRequest
		expiredJoin      JoinRequest
	)

	err := s.store.WithinTx(ctx, func(tx Store) error {
		joinRequest, loadErr := s.loadPendingJoinRequest(ctx, tx, params.JoinRequestID, params.ReviewedBy)
		if loadErr != nil {
			if errors.Is(loadErr, ErrExpiredJoinRequest) {
				expiredJoin = joinRequest
				return storage.Commit(loadErr)
			}

			return loadErr
		}

		now := s.currentTime()
		joinRequest.Status = JoinRequestStatusRejected
		joinRequest.ReviewedAt = now
		joinRequest.ReviewedBy = params.ReviewedBy
		joinRequest.DecisionReason = trimmed(params.Reason)

		savedJoinRequest, loadErr = tx.SaveJoinRequest(ctx, joinRequest)
		if loadErr != nil {
			return loadErr
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, ErrExpiredJoinRequest) {
			return expiredJoin, err
		}

		return JoinRequest{}, err
	}

	if params.IdempotencyKey != "" {
		s.idempotency.storeRejectJoinRequestResult(params.IdempotencyKey, fingerprint, savedJoinRequest, s.currentTime())
	}

	return savedJoinRequest, nil
}

// CreateAccount creates a human or bot account directly.
//
// It shares the same internal write path as approval-driven account creation so both
// flows normalize identifiers, enforce uniqueness, and cache the result identically.
func (s *Service) CreateAccount(ctx context.Context, params CreateAccountParams) (Account, string, error) {
	if err := s.validateContext(ctx, "create account"); err != nil {
		return Account{}, "", err
	}

	var (
		account  Account
		botToken string
	)
	err := s.store.WithinTx(ctx, func(tx Store) error {
		var createErr error
		account, botToken, createErr = s.createAccount(ctx, tx, params, false)
		return createErr
	})
	if err != nil {
		return Account{}, "", err
	}

	s.indexAccount(ctx, account)

	if params.IdempotencyKey != "" {
		s.idempotency.storeCreateAccountResult(
			params.IdempotencyKey,
			createAccountFingerprint(params),
			createAccountCacheResult{
				account:  account,
				botToken: botToken,
			},
			s.currentTime(),
		)
	}

	return account, botToken, nil
}

// createAccount executes the shared account write path for direct creation and approval retries.
//
// The helper accepts the store to write against so callers can run it inside a transaction
// while keeping the domain logic in one place.
func (s *Service) createAccount(
	ctx context.Context,
	store Store,
	params CreateAccountParams,
	allowExisting bool,
) (Account, string, error) {
	fingerprint := createAccountFingerprint(params)
	// Replay the cached result first so retried writes do not consume new IDs or tokens.
	if params.IdempotencyKey != "" {
		if result, ok, err := s.idempotency.createAccountResult(params.IdempotencyKey, fingerprint, s.currentTime()); err != nil {
			return Account{}, "", err
		} else if ok {
			if _, err := store.AccountByID(ctx, result.account.ID); err == nil {
				return result.account, result.botToken, nil
			} else if !isNotFound(err) {
				return Account{}, "", fmt.Errorf("verify cached account %s: %w", result.account.ID, err)
			}
		}
	}

	// Canonicalize identifiers before validation so the store sees the same values that
	// all lookup paths will use later.
	username, email, phone := s.normalizeAccountInput(params.Username, params.Email, params.Phone)
	password := trimmed(params.Password)
	if username == "" {
		return Account{}, "", ErrInvalidInput
	}
	if params.AccountKind == AccountKindUnspecified {
		return Account{}, "", ErrInvalidInput
	}
	if params.AccountKind == AccountKindUser && email == "" && phone == "" && password == "" {
		return Account{}, "", ErrInvalidInput
	}
	if params.AccountKind == AccountKindBot && password != "" {
		return Account{}, "", ErrInvalidInput
	}

	now := params.RequestedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	accountID, err := newID("acc")
	if err != nil {
		return Account{}, "", fmt.Errorf("generate account ID: %w", err)
	}

	account := Account{
		ID:          accountID,
		Kind:        params.AccountKind,
		Username:    username,
		DisplayName: trimmed(params.DisplayName),
		Email:       email,
		Phone:       phone,
		Roles:       append([]Role(nil), params.Roles...),
		Status:      AccountStatusActive,
		CreatedBy:   trimmed(params.CreatedBy),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if account.DisplayName == "" {
		account.DisplayName = username
	}

	var botToken string
	if params.AccountKind == AccountKindBot {
		botToken, err = randomToken(32)
		if err != nil {
			return Account{}, "", fmt.Errorf("generate bot token for account %s: %w", accountID, err)
		}
		account.BotTokenHash = hashSecret(botToken)
	}

	// Save the account once the request is fully normalized. Any conflict after this point
	// is handled by the store or by the approval retry recovery path below.
	savedAccount, err := store.SaveAccount(ctx, account)
	if err != nil {
		if allowExisting && params.AccountKind == AccountKindUser && errors.Is(err, ErrConflict) {
			// Approval retries may hit an account that already exists because a previous
			// attempt created it and then failed during the join-request update.
			existingAccount, lookupErr := store.AccountByUsername(ctx, username)
			if lookupErr != nil {
				return Account{}, "", fmt.Errorf("load existing account %s after conflict: %w", username, lookupErr)
			}
			if !accountsMatchForRecovery(existingAccount, account) {
				return Account{}, "", ErrConflict
			}

			return existingAccount, "", nil
		}

		return Account{}, "", fmt.Errorf("save account %s: %w", username, err)
	}

	if params.AccountKind == AccountKindUser && password != "" {
		if err := s.saveAccountCredential(
			ctx,
			store,
			savedAccount.ID,
			AccountCredentialKindPassword,
			password,
			now,
		); err != nil {
			return Account{}, "", err
		}
	}

	return savedAccount, botToken, nil
}

// accountsMatchForRecovery reports whether a persisted account still matches the intended create payload.
//
// The approval retry path only recovers an existing account when the persisted values
// still match the normalized creation request. That prevents a stale orphan from being
// silently reused for a different approval payload.
func accountsMatchForRecovery(existing, expected Account) bool {
	if existing.Status != AccountStatusActive {
		return false
	}
	if existing.Kind != expected.Kind {
		return false
	}
	if existing.Username != expected.Username {
		return false
	}
	if existing.DisplayName != expected.DisplayName {
		return false
	}
	if existing.Email != expected.Email {
		return false
	}
	if existing.Phone != expected.Phone {
		return false
	}
	if existing.CreatedBy != expected.CreatedBy {
		return false
	}
	if rolesFingerprint(existing.Roles) != rolesFingerprint(expected.Roles) {
		return false
	}

	return true
}
