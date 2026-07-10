package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestUpdateProfileAvatarPresenceAndIdempotency(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	store := identitytest.NewMemoryStore()
	service, err := identity.NewService(store, nil, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	account, _, err := service.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "avatar-user",
		DisplayName: "Avatar User",
		Email:       "avatar@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "test",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := service.UpdateProfile(ctx, identity.UpdateProfileParams{
		AccountID:        account.ID,
		Bio:              "profile bio",
		BioSet:           true,
		CustomBadgeEmoji: "🔥",
		CustomBadgeSet:   true,
		FieldMask:        []string{"bio", "custom_badge_emoji"},
		RequestedAt:      now,
	}); err != nil {
		t.Fatalf("seed profile fields: %v", err)
	}

	avatarID := "media-avatar-1"
	params := identity.UpdateProfileParams{
		AccountID:      account.ID,
		AvatarMediaID:  &avatarID,
		FieldMask:      []string{"avatar"},
		IdempotencyKey: "profile-avatar-1",
		RequestedAt:    now,
	}
	first, err := service.UpdateProfileWithResult(ctx, params)
	if err != nil {
		t.Fatalf("set avatar: %v", err)
	}
	if first.Replayed || first.Account.AvatarMediaID != avatarID {
		t.Fatalf("unexpected first update: %+v", first)
	}
	if first.Account.Bio != "profile bio" || first.Account.CustomBadgeEmoji != "🔥" {
		t.Fatalf("avatar patch overwrote profile fields: %+v", first.Account)
	}

	replay, err := service.UpdateProfileWithResult(ctx, params)
	if err != nil {
		t.Fatalf("replay avatar update: %v", err)
	}
	if !replay.Replayed || replay.Account.AvatarMediaID != avatarID {
		t.Fatalf("unexpected replay: %+v", replay)
	}

	clearAvatar := ""
	cleared, err := service.UpdateProfile(ctx, identity.UpdateProfileParams{
		AccountID:     account.ID,
		AvatarMediaID: &clearAvatar,
		FieldMask:     []string{"avatar"},
		RequestedAt:   now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("clear avatar: %v", err)
	}
	if cleared.AvatarMediaID != "" {
		t.Fatalf("avatar was not cleared: %+v", cleared)
	}
}

func TestUpdateProfileCanClearUsernameWithPresence(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	store := identitytest.NewMemoryStore()
	service, err := identity.NewService(store, nil, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	account, _, err := service.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "clear-user",
		Email:       "clear@example.com",
		AccountKind: identity.AccountKindUser,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	updated, err := service.UpdateProfile(ctx, identity.UpdateProfileParams{
		AccountID:   account.ID,
		UsernameSet: true,
		FieldMask:   []string{"username"},
	})
	if err != nil {
		t.Fatalf("clear username: %v", err)
	}
	if updated.Username != "" {
		t.Fatalf("username was not cleared: %+v", updated)
	}
	if _, err := store.AccountByUsername(ctx, "clear-user"); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("old username lookup error = %v", err)
	}
	_, err = service.UpdateProfile(ctx, identity.UpdateProfileParams{
		AccountID:   account.ID,
		Username:    "next-user",
		UsernameSet: true,
		FieldMask:   []string{"username"},
		RequestedAt: now.Add(time.Minute),
	})
	if !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("username cooldown error = %v", err)
	}
}

func TestUpdateProfileRejectsInvalidUsername(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := identitytest.NewMemoryStore()
	service, err := identity.NewService(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	account, _, err := service.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "valid-user",
		Email:       "valid@example.com",
		AccountKind: identity.AccountKindUser,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	_, err = service.UpdateProfile(ctx, identity.UpdateProfileParams{
		AccountID:   account.ID,
		Username:    "bad name",
		UsernameSet: true,
		FieldMask:   []string{"username"},
	})
	if !errors.Is(err, identity.ErrInvalidInput) {
		t.Fatalf("invalid username error = %v", err)
	}
}
