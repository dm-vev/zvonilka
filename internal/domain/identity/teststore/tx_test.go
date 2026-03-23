package teststore

import (
	"context"
	"errors"
	"testing"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/dm-vev/zvonilka/internal/domain/storage"
)

func TestWithinTxRollsBackOnError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore().(*memoryStore)

	if _, err := store.SaveAccount(ctx, identity.Account{ID: "acc-1", Username: "alice"}); err != nil {
		t.Fatalf("save baseline account: %v", err)
	}

	wantErr := errors.New("boom")
	err := store.WithinTx(ctx, func(tx identity.Store) error {
		if _, err := tx.SaveAccount(ctx, identity.Account{ID: "acc-2", Username: "bob"}); err != nil {
			return err
		}

		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("within tx error: got %v, want %v", err, wantErr)
	}

	if _, err := store.AccountByID(ctx, "acc-1"); err != nil {
		t.Fatalf("baseline account disappeared after rollback: %v", err)
	}
	if _, err := store.AccountByID(ctx, "acc-2"); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("rolled back account visible after rollback: %v", err)
	}
}

func TestWithinTxCommitMarkerCommitsState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore().(*memoryStore)

	wantErr := errors.New("keep")
	err := store.WithinTx(ctx, func(tx identity.Store) error {
		if _, err := tx.SaveAccount(ctx, identity.Account{ID: "acc-1", Username: "alice"}); err != nil {
			return err
		}

		return storage.Commit(wantErr)
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("within tx error: got %v, want %v", err, wantErr)
	}

	if _, err := store.AccountByID(ctx, "acc-1"); err != nil {
		t.Fatalf("committed account missing after commit marker: %v", err)
	}
}

func TestSaveAccountCopiesRolesAcrossReads(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore().(*memoryStore)

	roles := []identity.Role{identity.RoleAdmin, identity.RoleModerator}
	if _, err := store.SaveAccount(ctx, identity.Account{
		ID:       "acc-1",
		Username: "alice",
		Roles:    roles,
	}); err != nil {
		t.Fatalf("save account: %v", err)
	}

	roles[0] = identity.RoleOwner

	accountByID, err := store.AccountByID(ctx, "acc-1")
	if err != nil {
		t.Fatalf("load account by id: %v", err)
	}
	if accountByID.Roles[0] != identity.RoleAdmin {
		t.Fatalf("account roles mutated through input slice: got %v", accountByID.Roles)
	}

	accountByID.Roles[0] = identity.RoleSupport
	accountByUsername, err := store.AccountByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("load account by username: %v", err)
	}
	if accountByUsername.Roles[0] != identity.RoleAdmin {
		t.Fatalf("account roles mutated through read alias: got %v", accountByUsername.Roles)
	}
}

func TestSaveLoginChallengeCopiesTargetsAcrossReads(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore().(*memoryStore)

	targets := []identity.LoginTarget{
		{
			Channel:         identity.LoginDeliveryChannelSMS,
			DestinationMask: "***1234",
			Primary:         true,
			Verified:        true,
		},
	}
	if _, err := store.SaveLoginChallenge(ctx, identity.LoginChallenge{
		ID:        "chal-1",
		AccountID: "acc-1",
		Targets:   targets,
	}); err != nil {
		t.Fatalf("save challenge: %v", err)
	}

	targets[0].DestinationMask = "***0000"

	challengeByID, err := store.LoginChallengeByID(ctx, "chal-1")
	if err != nil {
		t.Fatalf("load challenge by id: %v", err)
	}
	if challengeByID.Targets[0].DestinationMask != "***1234" {
		t.Fatalf("challenge targets mutated through input slice: got %v", challengeByID.Targets)
	}

	challengeByID.Targets[0].DestinationMask = "***9999"
	again, err := store.LoginChallengeByID(ctx, "chal-1")
	if err != nil {
		t.Fatalf("reload challenge by id: %v", err)
	}
	if again.Targets[0].DestinationMask != "***1234" {
		t.Fatalf("challenge targets mutated through read alias: got %v", again.Targets)
	}
}
