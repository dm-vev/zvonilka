package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// mapDeviceForeignKeyViolation classifies a device write failure caused by foreign-key checks.
func (s *Store) mapDeviceForeignKeyViolation(ctx context.Context, device identity.Device) error {
	if _, err := s.AccountByID(ctx, device.AccountID); err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			return identity.ErrNotFound
		}

		return fmt.Errorf("load account %s after device foreign key violation: %w", device.AccountID, err)
	}

	session, err := s.SessionByID(ctx, device.SessionID)
	if err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			return identity.ErrNotFound
		}

		return fmt.Errorf("load session %s after device foreign key violation: %w", device.SessionID, err)
	}
	if session.AccountID != device.AccountID {
		return identity.ErrConflict
	}

	return identity.ErrConflict
}

// mapSessionForeignKeyViolation classifies a session write failure caused by foreign-key checks.
func (s *Store) mapSessionForeignKeyViolation(ctx context.Context, session identity.Session) error {
	if _, err := s.AccountByID(ctx, session.AccountID); err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			return identity.ErrNotFound
		}

		return fmt.Errorf("load account %s after session foreign key violation: %w", session.AccountID, err)
	}

	device, err := s.DeviceByID(ctx, session.DeviceID)
	if err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			return identity.ErrNotFound
		}

		return fmt.Errorf("load device %s after session foreign key violation: %w", session.DeviceID, err)
	}
	if device.AccountID != session.AccountID {
		return identity.ErrConflict
	}

	return identity.ErrConflict
}
