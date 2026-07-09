package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestDebugLoginRequiresFlag(t *testing.T) {
	t.Parallel()

	service, err := identity.NewService(identitytest.NewMemoryStore(), identity.NoopCodeSender{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, _, err = service.BeginLogin(context.Background(), identity.BeginLoginParams{
		Phone: "+88807777777",
	})
	if err == nil {
		t.Fatalf("expected debug phone login to fail when debug login is disabled")
	}
}

func TestDebugLoginUsesLastFiveDigits(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.June, 26, 12, 0, 0, 0, time.UTC)
	service, err := identity.NewService(
		identitytest.NewMemoryStore(),
		identity.NoopCodeSender{},
		identity.WithDebugLogin(true),
		identity.WithNow(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	challenge, targets, err := service.BeginLogin(context.Background(), identity.BeginLoginParams{
		Phone:       "+88807777777",
		DeviceName:  "debug-phone",
		Platform:    identity.DevicePlatformIOS,
		RequestedAt: now,
	})
	if err != nil {
		t.Fatalf("begin debug login: %v", err)
	}
	if len(targets) != 1 || targets[0].Channel != identity.LoginDeliveryChannelManual {
		t.Fatalf("unexpected debug targets: %+v", targets)
	}

	result, err := service.VerifyLoginCode(context.Background(), identity.VerifyLoginCodeParams{
		ChallengeID: challenge.ID,
		Code:        "77777",
		DeviceName:  "debug-phone",
		Platform:    identity.DevicePlatformIOS,
		PublicKey:   "debug-key",
	})
	if err != nil {
		t.Fatalf("verify debug login: %v", err)
	}
	if result.Session.AccountID == "" || result.Tokens.AccessToken == "" {
		t.Fatalf("expected issued session and tokens, got %+v", result)
	}
}
