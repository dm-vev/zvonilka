package user_test

import (
	"context"
	"testing"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
	usertest "github.com/dm-vev/zvonilka/internal/domain/user/teststore"
)

func TestSyncContactsUsesTransactionalPrivacyLookup(t *testing.T) {
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

	owner, _, err := directory.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "owner-user",
		DisplayName: "Owner",
		Email:       "owner@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create owner account: %v", err)
	}
	peer, _, err := directory.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "peer-user",
		DisplayName: "Peer",
		Phone:       "+1 (555) 100-2000",
		Email:       "peer@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create peer account: %v", err)
	}

	result, err := service.SyncContacts(ctx, domainuser.SyncParams{
		AccountID:      owner.ID,
		SourceDeviceID: "device-1",
		Contacts: []domainuser.SyncedContact{{
			RawContactID: "raw-peer",
			DisplayName:  "Peer Contact",
			PhoneE164:    peer.Phone,
			Checksum:     "v1",
		}},
	})
	if err != nil {
		t.Fatalf("sync contacts: %v", err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("expected one sync match, got %+v", result)
	}
	if result.Matches[0].ContactUserID != peer.ID {
		t.Fatalf("expected match for %s, got %+v", peer.ID, result.Matches[0])
	}
}
