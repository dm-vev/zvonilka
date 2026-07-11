package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
	usertest "github.com/dm-vev/zvonilka/internal/domain/user/teststore"
)

func TestAccountSettingsPartialUpdatePreservesOtherFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	directory, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	if err != nil {
		t.Fatalf("new identity service: %v", err)
	}
	service, err := domainuser.NewService(usertest.NewMemoryStore(), directory)
	if err != nil {
		t.Fatalf("new user service: %v", err)
	}
	account, _, err := directory.CreateAccount(ctx, identity.CreateAccountParams{
		Username: "settings-user", DisplayName: "Settings", Email: "settings@example.com",
		AccountKind: identity.AccountKindUser, CreatedBy: "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	defaults, err := service.GetAccountSettings(ctx, account.ID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	updated, err := service.UpdateAccountSettings(ctx, domainuser.UpdateAccountSettingsParams{
		AccountID: account.ID,
		Settings:  domainuser.AccountSettings{DefaultReaction: "🔥"},
		FieldMask: []string{"default_reaction"},
	})
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if updated.DefaultReaction != "🔥" || updated.AccountTTLDays != defaults.AccountTTLDays ||
		updated.AutoDownload.WiFi.MaxVideoBytes != defaults.AutoDownload.WiFi.MaxVideoBytes {
		t.Fatalf("partial update lost settings: defaults=%+v updated=%+v", defaults, updated)
	}

	_, err = service.UpdateAccountSettings(ctx, domainuser.UpdateAccountSettingsParams{
		AccountID: account.ID,
		Settings:  domainuser.AccountSettings{AccountTTLDays: 1},
		FieldMask: []string{"account_ttl_days"},
	})
	if !errors.Is(err, domainuser.ErrInvalidInput) {
		t.Fatalf("invalid TTL error = %v", err)
	}
}
