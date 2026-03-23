package identity_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	teststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

// barrierSaveAccountStore blocks both concurrent saves until they have all arrived.
type barrierSaveAccountStore struct {
	identity.Store

	mu      sync.Mutex
	entered int
	release chan struct{}
}

// newBarrierSaveAccountStore constructs the synchronization wrapper used by the atomicity test.
func newBarrierSaveAccountStore(store identity.Store) *barrierSaveAccountStore {
	return &barrierSaveAccountStore{
		Store:   store,
		release: make(chan struct{}),
	}
}

// SaveAccount waits for both concurrent callers before delegating to the wrapped store.
func (s *barrierSaveAccountStore) SaveAccount(
	ctx context.Context,
	account identity.Account,
) (identity.Account, error) {
	s.mu.Lock()
	s.entered++
	if s.entered == 2 {
		close(s.release)
	}
	release := s.release
	s.mu.Unlock()

	<-release
	return s.Store.SaveAccount(ctx, account)
}

func TestCreateAccountRejectsConcurrentDuplicateIdentifiersAtomically(t *testing.T) {
	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := newBarrierSaveAccountStore(baseStore)
	sender := &recordingCodeSender{}
	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time {
		return time.Date(2026, time.March, 24, 1, 30, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	const (
		username    = "atomic-create-user"
		email       = "atomic-create@example.com"
		displayName = "Atomic Create User"
	)

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			_, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
				Username:    username,
				DisplayName: displayName,
				Email:       email,
				AccountKind: identity.AccountKindUser,
				CreatedBy:   "admin-1",
			})
			results <- err
		}()
	}

	wg.Wait()
	close(results)

	successCount := 0
	conflictCount := 0
	for err := range results {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, identity.ErrConflict):
			conflictCount++
		default:
			t.Fatalf("unexpected create account error: %v", err)
		}
	}

	if successCount != 1 {
		t.Fatalf("expected one successful create, got %d", successCount)
	}
	if conflictCount != 1 {
		t.Fatalf("expected one conflicting create, got %d", conflictCount)
	}

	storedAccount, err := baseStore.AccountByUsername(ctx, username)
	if err != nil {
		t.Fatalf("load stored account: %v", err)
	}
	if storedAccount.Email != email {
		t.Fatalf("expected stored account email %s, got %s", email, storedAccount.Email)
	}
}
