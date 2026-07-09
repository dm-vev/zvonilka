package user

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// BlockUser stores one owner->blocked relationship.
func (s *Service) BlockUser(ctx context.Context, params BlockParams) (BlockEntry, error) {
	if err := s.validateContext(ctx, "block user"); err != nil {
		return BlockEntry{}, err
	}

	params.AccountID = strings.TrimSpace(params.AccountID)
	params.BlockedUserID = strings.TrimSpace(params.BlockedUserID)
	if params.AccountID == "" || params.BlockedUserID == "" || params.AccountID == params.BlockedUserID {
		return BlockEntry{}, ErrInvalidInput
	}

	target, err := s.directory.AccountByID(ctx, params.BlockedUserID)
	if err != nil {
		return BlockEntry{}, fmt.Errorf("load blocked account %s: %w", params.BlockedUserID, err)
	}
	if target.Status != identity.AccountStatusActive {
		return BlockEntry{}, ErrForbidden
	}

	now := s.currentTime()
	entry := BlockEntry{
		OwnerAccountID:   params.AccountID,
		BlockedAccountID: params.BlockedUserID,
		Reason:           strings.TrimSpace(params.Reason),
		BlockedAt:        now,
		UpdatedAt:        now,
	}

	existing, err := s.store.BlockByAccountAndUserID(ctx, params.AccountID, params.BlockedUserID)
	if err != nil && err != ErrNotFound {
		return BlockEntry{}, fmt.Errorf("load existing block %s:%s: %w", params.AccountID, params.BlockedUserID, err)
	}
	if existing.OwnerAccountID != "" {
		entry.BlockedAt = existing.BlockedAt
	}

	saved, err := s.store.SaveBlock(ctx, entry)
	if err != nil {
		return BlockEntry{}, fmt.Errorf("save block %s:%s: %w", params.AccountID, params.BlockedUserID, err)
	}

	return saved, nil
}

// UnblockUser removes one owner->blocked relationship.
func (s *Service) UnblockUser(ctx context.Context, params UnblockParams) (bool, error) {
	if err := s.validateContext(ctx, "unblock user"); err != nil {
		return false, err
	}

	params.AccountID = strings.TrimSpace(params.AccountID)
	params.BlockedUserID = strings.TrimSpace(params.BlockedUserID)
	if params.AccountID == "" || params.BlockedUserID == "" {
		return false, ErrInvalidInput
	}

	if err := s.store.DeleteBlock(ctx, params.AccountID, params.BlockedUserID); err != nil {
		if err == ErrNotFound {
			return false, nil
		}
		return false, fmt.Errorf("delete block %s:%s: %w", params.AccountID, params.BlockedUserID, err)
	}

	return true, nil
}

// ListBlockedUsers returns the owner's blocklist.
func (s *Service) ListBlockedUsers(ctx context.Context, accountID string) ([]BlockEntry, error) {
	if err := s.validateContext(ctx, "list blocked users"); err != nil {
		return nil, err
	}

	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, ErrInvalidInput
	}

	entries, err := s.store.ListBlocksByAccountID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list blocked users for %s: %w", accountID, err)
	}

	return entries, nil
}

// Relations returns contact/block flags for the owner across the supplied user IDs.
func (s *Service) Relations(ctx context.Context, accountID string, userIDs []string) (map[string]Relation, error) {
	if err := s.validateContext(ctx, "resolve user relations"); err != nil {
		return nil, err
	}

	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, ErrInvalidInput
	}

	contacts, err := s.store.ListContactsByAccountID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list contacts for relation resolution: %w", err)
	}
	blocks, err := s.store.ListBlocksByAccountID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list blocks for relation resolution: %w", err)
	}

	result := make(map[string]Relation, len(userIDs))
	for _, userID := range userIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		result[userID] = Relation{}
	}

	for _, contact := range contacts {
		relation, ok := result[contact.ContactAccountID]
		if !ok {
			continue
		}
		relation.IsContact = true
		result[contact.ContactAccountID] = relation
	}
	for _, block := range blocks {
		relation, ok := result[block.BlockedAccountID]
		if !ok {
			continue
		}
		relation.IsBlocked = true
		result[block.BlockedAccountID] = relation
	}

	return result, nil
}
