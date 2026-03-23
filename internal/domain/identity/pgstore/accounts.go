package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SaveAccount inserts or replaces an account while preserving uniqueness semantics.
func (s *Store) SaveAccount(ctx context.Context, account identity.Account) (identity.Account, error) {
	if err := s.requireContext(ctx); err != nil {
		return identity.Account{}, err
	}
	if err := s.requireStore(); err != nil {
		return identity.Account{}, err
	}
	if account.ID == "" {
		return identity.Account{}, identity.ErrInvalidInput
	}
	if s.tx != nil {
		return s.saveAccount(ctx, account)
	}

	var saved identity.Account
	err := s.withTransaction(ctx, func(tx identity.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveAccount(ctx, account)
		return saveErr
	})
	if err != nil {
		return identity.Account{}, err
	}

	return saved, nil
}

func (s *Store) saveAccount(ctx context.Context, account identity.Account) (identity.Account, error) {
	account.ID = strings.TrimSpace(account.ID)
	account.Username = strings.TrimSpace(account.Username)
	account.DisplayName = strings.TrimSpace(account.DisplayName)
	account.Bio = strings.TrimSpace(account.Bio)
	account.Email = strings.TrimSpace(account.Email)
	account.Phone = strings.TrimSpace(account.Phone)
	account.CreatedBy = strings.TrimSpace(account.CreatedBy)
	account.CustomBadgeEmoji = strings.TrimSpace(account.CustomBadgeEmoji)

	if err := s.lockIdentifiers(ctx, s.accountLockKeys(account)...); err != nil {
		return identity.Account{}, err
	}

	if conflict, err := s.accountConflict(ctx, account); err != nil {
		return identity.Account{}, err
	} else if conflict {
		return identity.Account{}, identity.ErrConflict
	}

	rolesRaw, err := encodeRoles(account.Roles)
	if err != nil {
		return identity.Account{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, kind, username, display_name, bio, email, phone, roles, status, bot_token_hash,
	created_by, created_at, updated_at, disabled_at, last_auth_at, custom_badge_emoji
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
	$11, $12, $13, $14, $15, $16
)
ON CONFLICT (id) DO UPDATE SET
	kind = EXCLUDED.kind,
	username = EXCLUDED.username,
	display_name = EXCLUDED.display_name,
	bio = EXCLUDED.bio,
	email = EXCLUDED.email,
	phone = EXCLUDED.phone,
	roles = EXCLUDED.roles,
	status = EXCLUDED.status,
	bot_token_hash = EXCLUDED.bot_token_hash,
	created_by = EXCLUDED.created_by,
	updated_at = EXCLUDED.updated_at,
	disabled_at = EXCLUDED.disabled_at,
	last_auth_at = EXCLUDED.last_auth_at,
	custom_badge_emoji = EXCLUDED.custom_badge_emoji
RETURNING %s
`, s.table("identity_accounts"), accountColumnList)

	var saved identity.Account
	saved, err = scanAccount(s.conn().QueryRowContext(ctx, query,
		account.ID,
		account.Kind,
		account.Username,
		account.DisplayName,
		account.Bio,
		account.Email,
		account.Phone,
		rolesRaw,
		account.Status,
		account.BotTokenHash,
		account.CreatedBy,
		account.CreatedAt.UTC(),
		account.UpdatedAt.UTC(),
		nullTime(account.DisabledAt),
		nullTime(account.LastAuthAt),
		account.CustomBadgeEmoji,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return identity.Account{}, identity.ErrConflict
		}
		return identity.Account{}, fmt.Errorf("save account %s: %w", account.Username, err)
	}

	return saved, nil
}

// DeleteAccount removes an account and all cascading rows.
func (s *Store) DeleteAccount(ctx context.Context, accountID string) error {
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if err := s.requireStore(); err != nil {
		return err
	}
	if accountID == "" {
		return identity.ErrInvalidInput
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, s.table("identity_accounts"))
	result, err := s.conn().ExecContext(ctx, query, accountID)
	if err != nil {
		return fmt.Errorf("delete account %s: %w", accountID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete account %s: %w", accountID, err)
	}
	if rowsAffected == 0 {
		return identity.ErrNotFound
	}

	return nil
}

// AccountByID resolves an account by primary key.
func (s *Store) AccountByID(ctx context.Context, accountID string) (identity.Account, error) {
	if err := s.requireContext(ctx); err != nil {
		return identity.Account{}, err
	}
	if err := s.requireStore(); err != nil {
		return identity.Account{}, err
	}
	return s.accountByColumn(ctx, "id", accountID)
}

// AccountByUsername resolves an account by username.
func (s *Store) AccountByUsername(ctx context.Context, username string) (identity.Account, error) {
	if err := s.requireContext(ctx); err != nil {
		return identity.Account{}, err
	}
	if err := s.requireStore(); err != nil {
		return identity.Account{}, err
	}
	return s.accountByColumn(ctx, "username", username)
}

// AccountByEmail resolves an account by email.
func (s *Store) AccountByEmail(ctx context.Context, email string) (identity.Account, error) {
	if err := s.requireContext(ctx); err != nil {
		return identity.Account{}, err
	}
	if err := s.requireStore(); err != nil {
		return identity.Account{}, err
	}
	return s.accountByColumn(ctx, "email", email)
}

// AccountByPhone resolves an account by phone.
func (s *Store) AccountByPhone(ctx context.Context, phone string) (identity.Account, error) {
	if err := s.requireContext(ctx); err != nil {
		return identity.Account{}, err
	}
	if err := s.requireStore(); err != nil {
		return identity.Account{}, err
	}
	return s.accountByColumn(ctx, "phone", phone)
}

// AccountByBotTokenHash resolves a bot account by token hash.
func (s *Store) AccountByBotTokenHash(ctx context.Context, tokenHash string) (identity.Account, error) {
	if err := s.requireContext(ctx); err != nil {
		return identity.Account{}, err
	}
	if err := s.requireStore(); err != nil {
		return identity.Account{}, err
	}
	return s.accountByColumn(ctx, "bot_token_hash", tokenHash)
}

func (s *Store) accountByColumn(ctx context.Context, column string, value string) (identity.Account, error) {
	if strings.TrimSpace(value) == "" {
		return identity.Account{}, identity.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %s = $1`, accountColumnList, s.table("identity_accounts"), column)
	account, err := scanAccount(s.conn().QueryRowContext(ctx, query, value))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.Account{}, identity.ErrNotFound
		}
		return identity.Account{}, fmt.Errorf("load account by %s: %w", column, err)
	}

	return account, nil
}

func (s *Store) accountConflict(ctx context.Context, account identity.Account) (bool, error) {
	if conflict, err := s.accountConflictByColumn(ctx, "id", account.ID, account.ID); err != nil || conflict {
		return conflict, err
	}
	if conflict, err := s.accountConflictByColumn(ctx, "username", account.Username, account.ID); err != nil || conflict {
		return conflict, err
	}
	if conflict, err := s.accountConflictByColumn(ctx, "email", account.Email, account.ID); err != nil || conflict {
		return conflict, err
	}
	if conflict, err := s.accountConflictByColumn(ctx, "phone", account.Phone, account.ID); err != nil || conflict {
		return conflict, err
	}
	if conflict, err := s.accountConflictByColumn(ctx, "bot_token_hash", account.BotTokenHash, account.ID); err != nil || conflict {
		return conflict, err
	}

	return false, nil
}

func (s *Store) accountConflictByColumn(ctx context.Context, column string, value string, selfID string) (bool, error) {
	if strings.TrimSpace(value) == "" {
		return false, nil
	}

	query := fmt.Sprintf(`SELECT id FROM %s WHERE %s = $1 LIMIT 1`, s.table("identity_accounts"), column)
	var foundID string
	if err := s.conn().QueryRowContext(ctx, query, value).Scan(&foundID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("check account conflict by %s: %w", column, err)
	}

	return foundID != selfID, nil
}
