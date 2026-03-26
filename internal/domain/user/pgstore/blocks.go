package pgstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

func (s *Store) SaveBlock(ctx context.Context, block domainuser.BlockEntry) (domainuser.BlockEntry, error) {
	if err := s.requireStore(); err != nil {
		return domainuser.BlockEntry{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return domainuser.BlockEntry{}, err
	}
	if strings.TrimSpace(block.OwnerAccountID) == "" || strings.TrimSpace(block.BlockedAccountID) == "" {
		return domainuser.BlockEntry{}, domainuser.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (owner_account_id, blocked_account_id, reason, blocked_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (owner_account_id, blocked_account_id) DO UPDATE SET
	reason = EXCLUDED.reason,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("user_blocks"), blockColumnList)

	saved, err := scanBlock(s.conn().QueryRowContext(ctx, query,
		block.OwnerAccountID,
		block.BlockedAccountID,
		block.Reason,
		block.BlockedAt.UTC(),
		block.UpdatedAt.UTC(),
	))
	if err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return domainuser.BlockEntry{}, mapped
		}
		return domainuser.BlockEntry{}, fmt.Errorf("save block %s:%s: %w", block.OwnerAccountID, block.BlockedAccountID, err)
	}
	return saved, nil
}

func (s *Store) BlockByAccountAndUserID(
	ctx context.Context,
	accountID string,
	userID string,
) (domainuser.BlockEntry, error) {
	if err := s.requireStore(); err != nil {
		return domainuser.BlockEntry{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return domainuser.BlockEntry{}, err
	}
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(userID) == "" {
		return domainuser.BlockEntry{}, domainuser.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE owner_account_id = $1 AND blocked_account_id = $2`, blockColumnList, s.table("user_blocks"))
	block, err := scanBlock(s.conn().QueryRowContext(ctx, query, accountID, userID))
	if err != nil {
		if err == sql.ErrNoRows {
			return domainuser.BlockEntry{}, domainuser.ErrNotFound
		}
		return domainuser.BlockEntry{}, fmt.Errorf("load block %s:%s: %w", accountID, userID, err)
	}
	return block, nil
}

func (s *Store) ListBlocksByAccountID(ctx context.Context, accountID string) ([]domainuser.BlockEntry, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE owner_account_id = $1 ORDER BY blocked_at ASC, blocked_account_id ASC`, blockColumnList, s.table("user_blocks"))
	rows, err := s.conn().QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("list blocks for %s: %w", accountID, err)
	}
	defer rows.Close()

	blocks := make([]domainuser.BlockEntry, 0)
	for rows.Next() {
		block, scanErr := scanBlock(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan block for %s: %w", accountID, scanErr)
		}
		blocks = append(blocks, block)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate blocks for %s: %w", accountID, err)
	}
	return blocks, nil
}

func (s *Store) DeleteBlock(ctx context.Context, accountID string, userID string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(userID) == "" {
		return domainuser.ErrInvalidInput
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE owner_account_id = $1 AND blocked_account_id = $2`, s.table("user_blocks"))
	result, err := s.conn().ExecContext(ctx, query, accountID, userID)
	if err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return mapped
		}
		return fmt.Errorf("delete block %s:%s: %w", accountID, userID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete block %s:%s: %w", accountID, userID, err)
	}
	if rowsAffected == 0 {
		return domainuser.ErrNotFound
	}
	return nil
}
