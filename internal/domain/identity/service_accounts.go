package identity

import (
	"context"
	"errors"
	"fmt"
)

// SubmitJoinRequest stores a pending request for moderator review.
func (s *Service) SubmitJoinRequest(ctx context.Context, params SubmitJoinRequestParams) (JoinRequest, error) {
	if err := s.validateContext(ctx, "submit join request"); err != nil {
		return JoinRequest{}, err
	}

	username, email, phone := s.normalizeAccountInput(params.Username, params.Email, params.Phone)
	if username == "" {
		return JoinRequest{}, ErrInvalidInput
	}
	if email == "" && phone == "" {
		return JoinRequest{}, ErrInvalidInput
	}

	if _, err := s.store.AccountByUsername(ctx, username); err == nil {
		return JoinRequest{}, ErrConflict
	} else if err != nil && !isNotFound(err) {
		return JoinRequest{}, fmt.Errorf("check username availability for %s: %w", username, err)
	}

	if email != "" {
		if _, err := s.store.AccountByEmail(ctx, email); err == nil {
			return JoinRequest{}, ErrConflict
		} else if err != nil && !isNotFound(err) {
			return JoinRequest{}, fmt.Errorf("check email availability for %s: %w", email, err)
		}
	}

	if phone != "" {
		if _, err := s.store.AccountByPhone(ctx, phone); err == nil {
			return JoinRequest{}, ErrConflict
		} else if err != nil && !isNotFound(err) {
			return JoinRequest{}, fmt.Errorf("check phone availability for %s: %w", phone, err)
		}
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

	return saved, nil
}

// ApproveJoinRequest converts a join request into an active user account.
func (s *Service) ApproveJoinRequest(ctx context.Context, params ApproveJoinRequestParams) (JoinRequest, Account, error) {
	if err := s.validateContext(ctx, "approve join request"); err != nil {
		return JoinRequest{}, Account{}, err
	}
	if params.JoinRequestID == "" {
		return JoinRequest{}, Account{}, ErrInvalidInput
	}

	joinRequest, err := s.store.JoinRequestByID(ctx, params.JoinRequestID)
	if err != nil {
		return JoinRequest{}, Account{}, fmt.Errorf("load join request %s: %w", params.JoinRequestID, err)
	}
	if joinRequest.Status != JoinRequestStatusPending {
		return JoinRequest{}, Account{}, ErrConflict
	}

	account, botToken, err := s.createAccount(
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
			RequestedAt:    s.currentTime(),
		},
	)
	if err != nil {
		return JoinRequest{}, Account{}, err
	}
	if botToken != "" {
		return JoinRequest{}, Account{}, fmt.Errorf("join request created unexpected bot token")
	}

	now := s.currentTime()
	joinRequest.Status = JoinRequestStatusApproved
	joinRequest.ReviewedAt = now
	joinRequest.ReviewedBy = params.ReviewedBy
	joinRequest.DecisionReason = trimmed(params.DecisionReason)

	savedJoinRequest, err := s.store.SaveJoinRequest(ctx, joinRequest)
	if err != nil {
		return JoinRequest{}, Account{}, fmt.Errorf("update join request %s: %w", joinRequest.ID, err)
	}

	return savedJoinRequest, account, nil
}

// RejectJoinRequest marks a join request as rejected.
func (s *Service) RejectJoinRequest(ctx context.Context, params RejectJoinRequestParams) (JoinRequest, error) {
	if err := s.validateContext(ctx, "reject join request"); err != nil {
		return JoinRequest{}, err
	}
	if params.JoinRequestID == "" {
		return JoinRequest{}, ErrInvalidInput
	}

	joinRequest, err := s.store.JoinRequestByID(ctx, params.JoinRequestID)
	if err != nil {
		return JoinRequest{}, fmt.Errorf("load join request %s: %w", params.JoinRequestID, err)
	}
	if joinRequest.Status != JoinRequestStatusPending {
		return JoinRequest{}, ErrConflict
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

	return savedJoinRequest, nil
}

// CreateAccount creates a human or bot account directly.
func (s *Service) CreateAccount(ctx context.Context, params CreateAccountParams) (Account, string, error) {
	if err := s.validateContext(ctx, "create account"); err != nil {
		return Account{}, "", err
	}

	return s.createAccount(ctx, params)
}

func (s *Service) createAccount(ctx context.Context, params CreateAccountParams) (Account, string, error) {
	username, email, phone := s.normalizeAccountInput(params.Username, params.Email, params.Phone)
	if username == "" {
		return Account{}, "", ErrInvalidInput
	}
	if params.AccountKind == AccountKindUnspecified {
		return Account{}, "", ErrInvalidInput
	}
	if params.AccountKind == AccountKindUser && email == "" && phone == "" {
		return Account{}, "", ErrInvalidInput
	}

	if _, err := s.store.AccountByUsername(ctx, username); err == nil {
		return Account{}, "", ErrConflict
	} else if err != nil && !isNotFound(err) {
		return Account{}, "", fmt.Errorf("check username availability for %s: %w", username, err)
	}

	if email != "" {
		if _, err := s.store.AccountByEmail(ctx, email); err == nil {
			return Account{}, "", ErrConflict
		} else if err != nil && !isNotFound(err) {
			return Account{}, "", fmt.Errorf("check email availability for %s: %w", email, err)
		}
	}

	if phone != "" {
		if _, err := s.store.AccountByPhone(ctx, phone); err == nil {
			return Account{}, "", ErrConflict
		} else if err != nil && !isNotFound(err) {
			return Account{}, "", fmt.Errorf("check phone availability for %s: %w", phone, err)
		}
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

	savedAccount, err := s.store.SaveAccount(ctx, account)
	if err != nil {
		return Account{}, "", fmt.Errorf("save account %s: %w", username, err)
	}

	return savedAccount, botToken, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
