package identity

import (
	"context"
	"errors"
	"fmt"
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
// The approval path can recover an already-persisted account when a previous attempt
// rolled back too late. The reusedExisting flag tells the rollback branch whether it
// is still allowed to delete the account on a later join-request save failure.
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

	joinRequest, err := s.loadPendingJoinRequest(ctx, params.JoinRequestID, params.ReviewedBy)
	if err != nil {
		if errors.Is(err, ErrExpiredJoinRequest) {
			return joinRequest, Account{}, err
		}

		return JoinRequest{}, Account{}, err
	}

	now := s.currentTime()

	// Approval retries must reuse a still-persisted cached account, or recreate
	// it if rollback removed the previous one.
	account, botToken, reusedExisting, err := s.createAccount(
		ctx,
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
	if err != nil {
		return JoinRequest{}, Account{}, err
	}
	if botToken != "" {
		return JoinRequest{}, Account{}, fmt.Errorf("join request created unexpected bot token")
	}

	joinRequest.Status = JoinRequestStatusApproved
	joinRequest.ReviewedAt = now
	joinRequest.ReviewedBy = params.ReviewedBy
	joinRequest.DecisionReason = trimmed(params.DecisionReason)

	savedJoinRequest, err := s.store.SaveJoinRequest(ctx, joinRequest)
	if err != nil {
		if !reusedExisting {
			rollbackErr := s.store.DeleteAccount(ctx, account.ID)
			if rollbackErr != nil && !isNotFound(rollbackErr) {
				return JoinRequest{}, Account{}, fmt.Errorf(
					"update join request %s: %w: rollback account %s: %w",
					joinRequest.ID,
					err,
					account.ID,
					rollbackErr,
				)
			}
		}

		return JoinRequest{}, Account{}, fmt.Errorf("update join request %s: %w", joinRequest.ID, err)
	}

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
// Rejection only mutates the join-request row itself, so there is no account-side
// rollback path to manage.
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

	joinRequest, err := s.loadPendingJoinRequest(ctx, params.JoinRequestID, params.ReviewedBy)
	if err != nil {
		if errors.Is(err, ErrExpiredJoinRequest) {
			return joinRequest, err
		}

		return JoinRequest{}, err
	}

	now := s.currentTime()
	joinRequest.Status = JoinRequestStatusRejected
	joinRequest.ReviewedAt = now
	joinRequest.ReviewedBy = params.ReviewedBy
	joinRequest.DecisionReason = trimmed(params.Reason)

	savedJoinRequest, err := s.store.SaveJoinRequest(ctx, joinRequest)
	if err != nil {
		return JoinRequest{}, fmt.Errorf("update join request %s: %w", joinRequest.ID, err)
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

	account, botToken, _, err := s.createAccount(ctx, params, false)
	return account, botToken, err
}

// createAccount executes the shared account write path for direct creation and approval retries.
//
// The allowExisting flag is only enabled by approval flows so they can recover an active
// account that survived a partial rollback. Direct admin creation still fails on conflicts.
func (s *Service) createAccount(
	ctx context.Context,
	params CreateAccountParams,
	allowExisting bool,
) (Account, string, bool, error) {
	fingerprint := createAccountFingerprint(params)
	// Replay the cached result first so retried writes do not consume new IDs or tokens.
	if params.IdempotencyKey != "" {
		if result, ok, err := s.idempotency.createAccountResult(params.IdempotencyKey, fingerprint, s.currentTime()); err != nil {
			return Account{}, "", false, err
		} else if ok {
			if _, err := s.store.AccountByID(ctx, result.account.ID); err == nil {
				return result.account, result.botToken, false, nil
			} else if !isNotFound(err) {
				return Account{}, "", false, fmt.Errorf("verify cached account %s: %w", result.account.ID, err)
			}
		}
	}

	// Canonicalize identifiers before validation so the store sees the same values that
	// all lookup paths will use later.
	username, email, phone := s.normalizeAccountInput(params.Username, params.Email, params.Phone)
	if username == "" {
		return Account{}, "", false, ErrInvalidInput
	}
	if params.AccountKind == AccountKindUnspecified {
		return Account{}, "", false, ErrInvalidInput
	}
	if params.AccountKind == AccountKindUser && email == "" && phone == "" {
		return Account{}, "", false, ErrInvalidInput
	}

	now := params.RequestedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	accountID, err := newID("acc")
	if err != nil {
		return Account{}, "", false, fmt.Errorf("generate account ID: %w", err)
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
			return Account{}, "", false, fmt.Errorf("generate bot token for account %s: %w", accountID, err)
		}
		account.BotTokenHash = hashSecret(botToken)
	}

	// Save the account once the request is fully normalized. Any conflict after this point
	// is handled by the store or by the approval retry recovery path below.
	savedAccount, err := s.store.SaveAccount(ctx, account)
	if err != nil {
		if allowExisting && params.AccountKind == AccountKindUser && errors.Is(err, ErrConflict) {
			// Approval retries may hit an account that already exists because a previous
			// attempt created it and then failed during the join-request update.
			existingAccount, lookupErr := s.store.AccountByUsername(ctx, username)
			if lookupErr != nil {
				return Account{}, "", false, fmt.Errorf("load existing account %s after conflict: %w", username, lookupErr)
			}
			if !accountsMatchForRecovery(existingAccount, account) {
				return Account{}, "", false, ErrConflict
			}

			if params.IdempotencyKey != "" {
				// Cache the recovered account so subsequent retries do not re-run the conflict
				// recovery path or consume any additional identifiers.
				s.idempotency.storeCreateAccountResult(
					params.IdempotencyKey,
					fingerprint,
					createAccountCacheResult{
						account:  existingAccount,
						botToken: "",
					},
					s.currentTime(),
				)
			}

			return existingAccount, "", true, nil
		}

		return Account{}, "", false, fmt.Errorf("save account %s: %w", username, err)
	}

	// Persist the successful result after the account is durably written.
	if params.IdempotencyKey != "" {
		s.idempotency.storeCreateAccountResult(
			params.IdempotencyKey,
			fingerprint,
			createAccountCacheResult{
				account:  savedAccount,
				botToken: botToken,
			},
			s.currentTime(),
		)
	}

	return savedAccount, botToken, false, nil
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

// isNotFound centralizes the store-specific not-found check used by rollback paths.
func isNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
