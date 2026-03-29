package e2ee

import (
	"context"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error
	SaveSignedPreKey(ctx context.Context, accountID string, deviceID string, value SignedPreKey) (SignedPreKey, error)
	SignedPreKeyByDevice(ctx context.Context, accountID string, deviceID string) (SignedPreKey, error)
	DeleteOneTimePreKeysByDevice(ctx context.Context, accountID string, deviceID string) error
	SaveOneTimePreKeys(ctx context.Context, accountID string, deviceID string, values []OneTimePreKey) error
	ClaimOneTimePreKey(
		ctx context.Context,
		accountID string,
		deviceID string,
		claimedByAccountID string,
		claimedByDeviceID string,
	) (OneTimePreKey, error)
	CountAvailableOneTimePreKeys(ctx context.Context, accountID string, deviceID string) (uint32, error)
	SaveDirectSession(ctx context.Context, value DirectSession) (DirectSession, error)
	DirectSessionByID(ctx context.Context, sessionID string) (DirectSession, error)
	DirectSessionsByRecipientDevice(ctx context.Context, accountID string, deviceID string) ([]DirectSession, error)
}

type Directory interface {
	AccountByID(ctx context.Context, accountID string) (identity.Account, error)
	DeviceByID(ctx context.Context, deviceID string) (identity.Device, error)
	DevicesByAccountID(ctx context.Context, accountID string) ([]identity.Device, error)
}
