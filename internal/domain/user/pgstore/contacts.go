package pgstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

func (s *Store) SaveContact(ctx context.Context, contact domainuser.Contact) (domainuser.Contact, error) {
	if err := s.requireStore(); err != nil {
		return domainuser.Contact{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return domainuser.Contact{}, err
	}
	if strings.TrimSpace(contact.OwnerAccountID) == "" || strings.TrimSpace(contact.ContactAccountID) == "" {
		return domainuser.Contact{}, domainuser.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	owner_account_id, contact_account_id, display_name, username, phone_hash, source, starred,
	raw_contact_id, source_device_id, sync_checksum, added_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (owner_account_id, contact_account_id) DO UPDATE SET
	display_name = EXCLUDED.display_name,
	username = EXCLUDED.username,
	phone_hash = EXCLUDED.phone_hash,
	source = EXCLUDED.source,
	starred = EXCLUDED.starred,
	raw_contact_id = EXCLUDED.raw_contact_id,
	source_device_id = EXCLUDED.source_device_id,
	sync_checksum = EXCLUDED.sync_checksum,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("user_contacts"), contactColumnList)

	saved, err := scanContact(s.conn().QueryRowContext(ctx, query,
		contact.OwnerAccountID,
		contact.ContactAccountID,
		contact.DisplayName,
		contact.Username,
		contact.PhoneHash,
		contact.Source,
		contact.Starred,
		contact.RawContactID,
		contact.SourceDeviceID,
		contact.SyncChecksum,
		contact.AddedAt.UTC(),
		contact.UpdatedAt.UTC(),
	))
	if err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return domainuser.Contact{}, mapped
		}
		return domainuser.Contact{}, fmt.Errorf("save contact %s:%s: %w", contact.OwnerAccountID, contact.ContactAccountID, err)
	}
	return saved, nil
}

func (s *Store) ContactByAccountAndUserID(
	ctx context.Context,
	accountID string,
	userID string,
) (domainuser.Contact, error) {
	if err := s.requireStore(); err != nil {
		return domainuser.Contact{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return domainuser.Contact{}, err
	}
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(userID) == "" {
		return domainuser.Contact{}, domainuser.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE owner_account_id = $1 AND contact_account_id = $2`, contactColumnList, s.table("user_contacts"))
	contact, err := scanContact(s.conn().QueryRowContext(ctx, query, accountID, userID))
	if err != nil {
		if err == sql.ErrNoRows {
			return domainuser.Contact{}, domainuser.ErrNotFound
		}
		return domainuser.Contact{}, fmt.Errorf("load contact %s:%s: %w", accountID, userID, err)
	}
	return contact, nil
}

func (s *Store) ListContactsByAccountID(ctx context.Context, accountID string) ([]domainuser.Contact, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE owner_account_id = $1 ORDER BY starred DESC, display_name ASC, contact_account_id ASC`,
		contactColumnList,
		s.table("user_contacts"),
	)
	rows, err := s.conn().QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("list contacts for %s: %w", accountID, err)
	}
	defer rows.Close()

	contacts := make([]domainuser.Contact, 0)
	for rows.Next() {
		contact, scanErr := scanContact(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan contact for %s: %w", accountID, scanErr)
		}
		contacts = append(contacts, contact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contacts for %s: %w", accountID, err)
	}
	return contacts, nil
}

func (s *Store) DeleteContact(ctx context.Context, accountID string, userID string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(userID) == "" {
		return domainuser.ErrInvalidInput
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE owner_account_id = $1 AND contact_account_id = $2`, s.table("user_contacts"))
	result, err := s.conn().ExecContext(ctx, query, accountID, userID)
	if err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return mapped
		}
		return fmt.Errorf("delete contact %s:%s: %w", accountID, userID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete contact %s:%s: %w", accountID, userID, err)
	}
	if rowsAffected == 0 {
		return domainuser.ErrNotFound
	}
	return nil
}
