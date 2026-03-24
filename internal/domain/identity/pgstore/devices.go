package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SaveDevice stores a device and indexes it by account.
func (s *Store) SaveDevice(ctx context.Context, device identity.Device) (identity.Device, error) {
	if err := s.requireContext(ctx); err != nil {
		return identity.Device{}, err
	}
	if err := s.requireStore(); err != nil {
		return identity.Device{}, err
	}
	if device.ID == "" {
		return identity.Device{}, identity.ErrInvalidInput
	}

	if strings.TrimSpace(device.SessionID) != "" {
		session, err := s.SessionByID(ctx, device.SessionID)
		if err != nil && !errors.Is(err, identity.ErrNotFound) {
			return identity.Device{}, fmt.Errorf("load session %s for device %s: %w", device.SessionID, device.ID, err)
		}
		if err == nil && session.AccountID != device.AccountID {
			return identity.Device{}, identity.ErrConflict
		}
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, account_id, session_id, name, platform, status, public_key, push_token, created_at, last_seen_at, revoked_at, last_rotated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (id) DO UPDATE SET
	account_id = EXCLUDED.account_id,
	session_id = EXCLUDED.session_id,
	name = EXCLUDED.name,
	platform = EXCLUDED.platform,
	status = EXCLUDED.status,
	public_key = EXCLUDED.public_key,
	push_token = EXCLUDED.push_token,
	last_seen_at = EXCLUDED.last_seen_at,
	revoked_at = EXCLUDED.revoked_at,
	last_rotated_at = EXCLUDED.last_rotated_at
RETURNING %s
`, s.table("identity_devices"), deviceColumnList)

	saved, err := scanDevice(s.conn().QueryRowContext(ctx, query,
		device.ID,
		device.AccountID,
		device.SessionID,
		device.Name,
		device.Platform,
		device.Status,
		device.PublicKey,
		device.PushToken,
		device.CreatedAt.UTC(),
		device.LastSeenAt.UTC(),
		nullTime(device.RevokedAt),
		nullTime(device.LastRotatedAt),
	))
	if err != nil {
		if mappedErr := mapConstraintError(err, s.mapDeviceForeignKeyViolation(ctx, device)); mappedErr != nil {
			return identity.Device{}, mappedErr
		}
		return identity.Device{}, fmt.Errorf("save device %s: %w", device.ID, err)
	}

	return saved, nil
}

// DeleteDevice removes a device by primary key.
func (s *Store) DeleteDevice(ctx context.Context, deviceID string) error {
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if err := s.requireStore(); err != nil {
		return err
	}
	if deviceID == "" {
		return identity.ErrInvalidInput
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, s.table("identity_devices"))
	result, err := s.conn().ExecContext(ctx, query, deviceID)
	if err != nil {
		if isForeignKeyViolation(err) {
			return identity.ErrConflict
		}
		return fmt.Errorf("delete device %s: %w", deviceID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete device %s: %w", deviceID, err)
	}
	if rowsAffected == 0 {
		return identity.ErrNotFound
	}

	return nil
}

// DeviceByID resolves a device by primary key.
func (s *Store) DeviceByID(ctx context.Context, deviceID string) (identity.Device, error) {
	if err := s.requireContext(ctx); err != nil {
		return identity.Device{}, err
	}
	if err := s.requireStore(); err != nil {
		return identity.Device{}, err
	}
	if strings.TrimSpace(deviceID) == "" {
		return identity.Device{}, identity.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, deviceColumnList, s.table("identity_devices"))
	device, err := scanDevice(s.conn().QueryRowContext(ctx, query, deviceID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.Device{}, identity.ErrNotFound
		}
		return identity.Device{}, fmt.Errorf("load device %s: %w", deviceID, err)
	}

	return device, nil
}

// DevicesByAccountID lists devices for an account.
func (s *Store) DevicesByAccountID(ctx context.Context, accountID string) ([]identity.Device, error) {
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE account_id = $1 ORDER BY created_at ASC, id ASC`,
		deviceColumnList,
		s.table("identity_devices"),
	)
	rows, err := s.conn().QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("list devices for account %s: %w", accountID, err)
	}
	defer rows.Close()

	devices := make([]identity.Device, 0)
	for rows.Next() {
		device, scanErr := scanDevice(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan device for account %s: %w", accountID, scanErr)
		}
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices for account %s: %w", accountID, err)
	}

	return devices, nil
}
