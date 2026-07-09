package user

import (
	"context"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// Directory exposes the account lookups the user domain needs from identity.
type Directory interface {
	AccountByID(ctx context.Context, accountID string) (identity.Account, error)
	AccountByEmail(ctx context.Context, email string) (identity.Account, error)
	AccountByPhone(ctx context.Context, phone string) (identity.Account, error)
}

// Store persists privacy, contact, and block state.
type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error

	SavePrivacy(ctx context.Context, privacy Privacy) (Privacy, error)
	PrivacyByAccountID(ctx context.Context, accountID string) (Privacy, error)

	SaveContact(ctx context.Context, contact Contact) (Contact, error)
	ContactByAccountAndUserID(ctx context.Context, accountID string, userID string) (Contact, error)
	ListContactsByAccountID(ctx context.Context, accountID string) ([]Contact, error)
	DeleteContact(ctx context.Context, accountID string, userID string) error

	SaveBlock(ctx context.Context, block BlockEntry) (BlockEntry, error)
	BlockByAccountAndUserID(ctx context.Context, accountID string, userID string) (BlockEntry, error)
	ListBlocksByAccountID(ctx context.Context, accountID string) ([]BlockEntry, error)
	DeleteBlock(ctx context.Context, accountID string, userID string) error
}
