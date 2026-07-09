package identity

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var debugLoginPhones = map[string]string{
	"88807777777": "debug-alpha",
	"88810000001": "debug-beta",
	"88810000002": "debug-gamma",
}

func debugLoginCode(phone string) (string, bool) {
	phone = normalizePhone(phone)
	if _, ok := debugLoginPhones[phone]; !ok || len(phone) < 5 {
		return "", false
	}

	return phone[len(phone)-5:], true
}

func (s *Service) beginDebugLogin(
	ctx context.Context,
	phone string,
	params BeginLoginParams,
) (LoginChallenge, []LoginTarget, error) {
	code, ok := debugLoginCode(phone)
	if !ok {
		return LoginChallenge{}, nil, ErrInvalidInput
	}

	account, err := s.accountOrCreateDebugAccount(ctx, phone, params.RequestedAt)
	if err != nil {
		return LoginChallenge{}, nil, err
	}
	if account.Kind != AccountKindUser || account.Status != AccountStatusActive {
		return LoginChallenge{}, nil, ErrForbidden
	}

	challengeID, err := newID("chal")
	if err != nil {
		return LoginChallenge{}, nil, fmt.Errorf("generate debug challenge ID for account %s: %w", account.ID, err)
	}

	now := params.RequestedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	targets := []LoginTarget{{
		Channel:         LoginDeliveryChannelManual,
		DestinationMask: maskPhone(phone),
		Primary:         true,
		Verified:        true,
	}}
	challenge := LoginChallenge{
		ID:              challengeID,
		AccountID:       account.ID,
		AccountKind:     account.Kind,
		Purpose:         LoginChallengePurposeLogin,
		CodeHash:        hashSecret(code),
		DeliveryChannel: LoginDeliveryChannelManual,
		Targets:         targets,
		ExpiresAt:       now.Add(s.challengeTTL),
		CreatedAt:       now,
	}

	savedChallenge, err := s.store.SaveLoginChallenge(ctx, challenge)
	if err != nil {
		return LoginChallenge{}, nil, fmt.Errorf("save debug login challenge for account %s: %w", account.ID, err)
	}

	return savedChallenge, targets, nil
}

func (s *Service) accountOrCreateDebugAccount(ctx context.Context, phone string, requestedAt time.Time) (Account, error) {
	account, err := s.store.AccountByPhone(ctx, phone)
	if err == nil {
		return account, nil
	}
	if !isNotFound(err) {
		return Account{}, fmt.Errorf("lookup debug account by phone %s: %w", phone, err)
	}

	suffix := debugLoginPhones[phone]
	account, _, err = s.CreateAccount(ctx, CreateAccountParams{
		Username:    suffix,
		DisplayName: "Debug " + suffix,
		Phone:       phone,
		AccountKind: AccountKindUser,
		CreatedBy:   "debug-login",
		RequestedAt: requestedAt,
	})
	if err == nil {
		return account, nil
	}
	if !isNotFound(err) && !errors.Is(err, ErrConflict) {
		return Account{}, fmt.Errorf("create debug account %s: %w", phone, err)
	}

	account, err = s.store.AccountByPhone(ctx, phone)
	if err != nil {
		return Account{}, fmt.Errorf("load debug account after create conflict %s: %w", phone, err)
	}

	return account, nil
}
