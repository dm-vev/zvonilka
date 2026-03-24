package presence

import "context"

// Store persists explicit presence state.
type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error

	SavePresence(ctx context.Context, presence Presence) (Presence, error)
	PresenceByAccountID(ctx context.Context, accountID string) (Presence, error)
	PresencesByAccountIDs(ctx context.Context, accountIDs []string) (map[string]Presence, error)
}
