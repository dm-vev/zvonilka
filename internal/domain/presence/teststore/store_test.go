package teststore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/presence"
)

func TestWithinTxRollsBackChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Date(2026, time.March, 24, 17, 30, 0, 0, time.UTC)

	err := store.WithinTx(ctx, func(tx presence.Store) error {
		_, saveErr := tx.SavePresence(ctx, presence.Presence{
			AccountID: "acc-1",
			State:     presence.PresenceStateOnline,
			UpdatedAt: now,
		})
		if saveErr != nil {
			return saveErr
		}

		return errors.New("rollback")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}

	_, err = store.PresenceByAccountID(ctx, "acc-1")
	if !errors.Is(err, presence.ErrNotFound) {
		t.Fatalf("expected rollback to discard changes, got %v", err)
	}
}

func TestWithinTxCommitsChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Date(2026, time.March, 24, 17, 45, 0, 0, time.UTC)

	err := store.WithinTx(ctx, func(tx presence.Store) error {
		_, saveErr := tx.SavePresence(ctx, presence.Presence{
			AccountID:    "acc-1",
			State:        presence.PresenceStateBusy,
			CustomStatus: "In a meeting",
			UpdatedAt:    now,
		})
		return saveErr
	})
	if err != nil {
		t.Fatalf("commit tx: %v", err)
	}

	saved, err := store.PresenceByAccountID(ctx, "acc-1")
	if err != nil {
		t.Fatalf("load presence after commit: %v", err)
	}
	if saved.State != presence.PresenceStateBusy {
		t.Fatalf("expected busy state, got %s", saved.State)
	}
	if saved.CustomStatus != "In a meeting" {
		t.Fatalf("expected committed status, got %q", saved.CustomStatus)
	}
}

func TestPresenceByAccountIDReturnsIsolatedCopy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Date(2026, time.March, 24, 18, 0, 0, 0, time.UTC)

	if _, err := store.SavePresence(ctx, presence.Presence{
		AccountID: "acc-1",
		State:     presence.PresenceStateAway,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save presence: %v", err)
	}

	first, err := store.PresenceByAccountID(ctx, "acc-1")
	if err != nil {
		t.Fatalf("load presence: %v", err)
	}
	first.State = presence.PresenceStateBusy
	first.CustomStatus = "mutated"

	second, err := store.PresenceByAccountID(ctx, "acc-1")
	if err != nil {
		t.Fatalf("reload presence: %v", err)
	}
	if second.State != presence.PresenceStateAway {
		t.Fatalf("expected isolated stored state, got %s", second.State)
	}
	if second.CustomStatus != "" {
		t.Fatalf("expected isolated stored status, got %q", second.CustomStatus)
	}
}

func TestSavePresenceRejectsZeroUpdatedAt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore()

	_, err := store.SavePresence(ctx, presence.Presence{
		AccountID: "acc-1",
		State:     presence.PresenceStateOnline,
	})
	if !errors.Is(err, presence.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}
