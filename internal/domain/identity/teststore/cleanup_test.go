package teststore

import (
	"context"
	"errors"
	"testing"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

func TestDeleteAccountCascadesRecords(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore().(*memoryStore)

	if _, err := store.SaveAccount(ctx, identity.Account{ID: "acc-1", Username: "alice"}); err != nil {
		t.Fatalf("save account: %v", err)
	}
	if _, err := store.SaveDevice(ctx, identity.Device{ID: "dev-1", AccountID: "acc-1"}); err != nil {
		t.Fatalf("save device: %v", err)
	}
	if _, err := store.SaveSession(ctx, identity.Session{ID: "sess-1", AccountID: "acc-1"}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if _, err := store.SaveLoginChallenge(ctx, identity.LoginChallenge{ID: "chal-1", AccountID: "acc-1"}); err != nil {
		t.Fatalf("save challenge: %v", err)
	}

	if err := store.DeleteAccount(ctx, "acc-1"); err != nil {
		t.Fatalf("delete account: %v", err)
	}

	if _, err := store.AccountByID(ctx, "acc-1"); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("deleted account still visible: %v", err)
	}
	if _, err := store.AccountByUsername(ctx, "alice"); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("deleted account still indexed by username: %v", err)
	}

	devices, err := store.DevicesByAccountID(ctx, "acc-1")
	if err != nil {
		t.Fatalf("list devices after delete: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected no devices after account delete, got %d", len(devices))
	}

	sessions, err := store.SessionsByAccountID(ctx, "acc-1")
	if err != nil {
		t.Fatalf("list sessions after delete: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no sessions after account delete, got %d", len(sessions))
	}

	if _, err := store.LoginChallengeByID(ctx, "chal-1"); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("deleted account challenge still visible: %v", err)
	}
}

func TestSaveDeviceReindexesAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore().(*memoryStore)

	if _, err := store.SaveDevice(ctx, identity.Device{ID: "dev-1", AccountID: "acc-1"}); err != nil {
		t.Fatalf("save device: %v", err)
	}
	if _, err := store.SaveDevice(ctx, identity.Device{ID: "dev-1", AccountID: "acc-2"}); err != nil {
		t.Fatalf("resave device: %v", err)
	}

	devices, err := store.DevicesByAccountID(ctx, "acc-1")
	if err != nil {
		t.Fatalf("list old account devices: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected no devices for old account, got %d", len(devices))
	}

	devices, err = store.DevicesByAccountID(ctx, "acc-2")
	if err != nil {
		t.Fatalf("list new account devices: %v", err)
	}
	if len(devices) != 1 || devices[0].AccountID != "acc-2" {
		t.Fatalf("unexpected reindexed devices: %+v", devices)
	}
}

func TestSaveSessionReindexesAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore().(*memoryStore)

	if _, err := store.SaveSession(ctx, identity.Session{ID: "sess-1", AccountID: "acc-1"}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if _, err := store.SaveSession(ctx, identity.Session{ID: "sess-1", AccountID: "acc-2"}); err != nil {
		t.Fatalf("resave session: %v", err)
	}

	sessions, err := store.SessionsByAccountID(ctx, "acc-1")
	if err != nil {
		t.Fatalf("list old account sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no sessions for old account, got %d", len(sessions))
	}

	sessions, err = store.SessionsByAccountID(ctx, "acc-2")
	if err != nil {
		t.Fatalf("list new account sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].AccountID != "acc-2" {
		t.Fatalf("unexpected reindexed sessions: %+v", sessions)
	}
}
