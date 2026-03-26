package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	teststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestRefreshSessionRotatesTokensAndInvalidatesOldRefreshToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 26, 12, 0, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "refresh-user",
		DisplayName: "Refresh User",
		Email:       "refresh@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username: account.Username,
		Delivery: identity.LoginDeliveryChannelEmail,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}

	result, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID: challenge.ID,
		Code:        sender.codeFor(targets[0].DestinationMask),
		DeviceName:  "phone",
		Platform:    identity.DevicePlatformIOS,
		PublicKey:   "pub-1",
	})
	if err != nil {
		t.Fatalf("verify login: %v", err)
	}

	refreshed, err := svc.RefreshSession(ctx, identity.RefreshSessionParams{
		RefreshToken: result.Tokens.RefreshToken,
		DeviceID:     result.Device.ID,
	})
	if err != nil {
		t.Fatalf("refresh session: %v", err)
	}
	if refreshed.Session.ID != result.Session.ID {
		t.Fatalf("expected same session on refresh, got %s vs %s", refreshed.Session.ID, result.Session.ID)
	}
	if refreshed.Device.ID != result.Device.ID {
		t.Fatalf("expected same device on refresh, got %s vs %s", refreshed.Device.ID, result.Device.ID)
	}
	if refreshed.Tokens.AccessToken == result.Tokens.AccessToken {
		t.Fatalf("expected rotated access token")
	}
	if refreshed.Tokens.RefreshToken == result.Tokens.RefreshToken {
		t.Fatalf("expected rotated refresh token")
	}

	_, err = svc.RefreshSession(ctx, identity.RefreshSessionParams{
		RefreshToken: result.Tokens.RefreshToken,
		DeviceID:     result.Device.ID,
	})
	if !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("expected old refresh token to be unauthorized, got %v", err)
	}
}

func TestAuthenticateAccessTokenRejectsRevokedSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 26, 12, 30, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "auth-user",
		DisplayName: "Auth User",
		Email:       "auth@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username: account.Username,
		Delivery: identity.LoginDeliveryChannelEmail,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}

	result, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID: challenge.ID,
		Code:        sender.codeFor(targets[0].DestinationMask),
		DeviceName:  "desktop",
		Platform:    identity.DevicePlatformDesktop,
		PublicKey:   "pub-2",
	})
	if err != nil {
		t.Fatalf("verify login: %v", err)
	}

	authContext, err := svc.AuthenticateAccessToken(ctx, result.Tokens.AccessToken)
	if err != nil {
		t.Fatalf("authenticate access token: %v", err)
	}
	if authContext.Account.ID != account.ID {
		t.Fatalf("expected account %s, got %s", account.ID, authContext.Account.ID)
	}

	if _, err := svc.RevokeSession(ctx, identity.RevokeSessionParams{SessionID: result.Session.ID}); err != nil {
		t.Fatalf("revoke session: %v", err)
	}

	_, err = svc.AuthenticateAccessToken(ctx, result.Tokens.AccessToken)
	if !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("expected unauthorized after session revoke, got %v", err)
	}
}

func TestAuthenticateAccessTokenRejectsExpiredToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 26, 13, 0, 0, 0, time.UTC)
	clock := now

	svc, err := identity.NewService(
		store,
		sender,
		identity.WithNow(func() time.Time { return clock }),
		identity.WithSettings(identity.Settings{
			AccessTokenTTL:  time.Minute,
			RefreshTokenTTL: 24 * time.Hour,
			ChallengeTTL:    10 * time.Minute,
			JoinRequestTTL:  72 * time.Hour,
			LoginCodeLength: 6,
		}),
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "expired-user",
		DisplayName: "Expired User",
		Email:       "expired@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username: account.Username,
		Delivery: identity.LoginDeliveryChannelEmail,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}

	result, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID: challenge.ID,
		Code:        sender.codeFor(targets[0].DestinationMask),
		DeviceName:  "tablet",
		Platform:    identity.DevicePlatformIOS,
		PublicKey:   "pub-3",
	})
	if err != nil {
		t.Fatalf("verify login: %v", err)
	}

	clock = clock.Add(2 * time.Minute)

	_, err = svc.AuthenticateAccessToken(ctx, result.Tokens.AccessToken)
	if !errors.Is(err, identity.ErrExpiredToken) {
		t.Fatalf("expected expired token, got %v", err)
	}
}
