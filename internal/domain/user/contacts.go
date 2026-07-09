package user

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// ListContacts returns the owner's contacts after local query filtering.
func (s *Service) ListContacts(ctx context.Context, params ListContactsParams) ([]Contact, error) {
	if err := s.validateContext(ctx, "list contacts"); err != nil {
		return nil, err
	}

	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.AccountID == "" {
		return nil, ErrInvalidInput
	}

	contacts, err := s.store.ListContactsByAccountID(ctx, params.AccountID)
	if err != nil {
		return nil, fmt.Errorf("list contacts for %s: %w", params.AccountID, err)
	}

	query := strings.ToLower(strings.TrimSpace(params.Query))
	filtered := make([]Contact, 0, len(contacts))
	for _, contact := range contacts {
		if params.StarredOnly && !contact.Starred {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(contact.DisplayName + "\n" + contact.Username)
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		filtered = append(filtered, contact)
	}

	return filtered, nil
}

// AddContact creates or updates one manual contact relationship.
func (s *Service) AddContact(ctx context.Context, params AddContactParams) (Contact, error) {
	if err := s.validateContext(ctx, "add contact"); err != nil {
		return Contact{}, err
	}

	params.AccountID = strings.TrimSpace(params.AccountID)
	params.ContactUserID = strings.TrimSpace(params.ContactUserID)
	if params.AccountID == "" || params.ContactUserID == "" || params.AccountID == params.ContactUserID {
		return Contact{}, ErrInvalidInput
	}

	target, err := s.directory.AccountByID(ctx, params.ContactUserID)
	if err != nil {
		return Contact{}, fmt.Errorf("load contact account %s: %w", params.ContactUserID, err)
	}
	if target.Status != identity.AccountStatusActive {
		return Contact{}, ErrForbidden
	}

	now := s.currentTime()
	existing, err := s.store.ContactByAccountAndUserID(ctx, params.AccountID, params.ContactUserID)
	if err != nil && err != ErrNotFound {
		return Contact{}, fmt.Errorf("load existing contact %s:%s: %w", params.AccountID, params.ContactUserID, err)
	}

	contact := Contact{
		OwnerAccountID:   params.AccountID,
		ContactAccountID: params.ContactUserID,
		DisplayName:      strings.TrimSpace(params.Alias),
		Username:         target.Username,
		PhoneHash:        phoneHash(target.Phone),
		Source:           ContactSourceManual,
		Starred:          params.Starred,
		AddedAt:          now,
		UpdatedAt:        now,
	}
	if contact.DisplayName == "" {
		contact.DisplayName = target.DisplayName
	}
	if existing.OwnerAccountID != "" {
		contact.AddedAt = existing.AddedAt
		if contact.DisplayName == "" {
			contact.DisplayName = existing.DisplayName
		}
	}

	saved, err := s.store.SaveContact(ctx, contact)
	if err != nil {
		return Contact{}, fmt.Errorf("save contact %s:%s: %w", params.AccountID, params.ContactUserID, err)
	}

	return saved, nil
}

// RemoveContact removes one contact relationship.
func (s *Service) RemoveContact(ctx context.Context, params RemoveContactParams) (bool, error) {
	if err := s.validateContext(ctx, "remove contact"); err != nil {
		return false, err
	}

	params.AccountID = strings.TrimSpace(params.AccountID)
	params.ContactUserID = strings.TrimSpace(params.ContactUserID)
	if params.AccountID == "" || params.ContactUserID == "" {
		return false, ErrInvalidInput
	}

	if err := s.store.DeleteContact(ctx, params.AccountID, params.ContactUserID); err != nil {
		if err == ErrNotFound {
			return false, nil
		}
		return false, fmt.Errorf("delete contact %s:%s: %w", params.AccountID, params.ContactUserID, err)
	}

	return true, nil
}

// SyncContacts upserts synced contacts for one device snapshot and removes stale synced rows.
func (s *Service) SyncContacts(ctx context.Context, params SyncParams) (SyncResult, error) {
	if err := s.validateContext(ctx, "sync contacts"); err != nil {
		return SyncResult{}, err
	}

	params.AccountID = strings.TrimSpace(params.AccountID)
	params.SourceDeviceID = strings.TrimSpace(params.SourceDeviceID)
	if params.AccountID == "" || params.SourceDeviceID == "" {
		return SyncResult{}, ErrInvalidInput
	}

	var result SyncResult
	err := s.store.WithinTx(ctx, func(tx Store) error {
		existing, err := tx.ListContactsByAccountID(ctx, params.AccountID)
		if err != nil {
			return fmt.Errorf("list existing contacts for sync: %w", err)
		}

		existingByUser := make(map[string]Contact, len(existing))
		for _, contact := range existing {
			existingByUser[contact.ContactAccountID] = contact
		}

		matchedUsers := make(map[string]struct{}, len(params.Contacts))
		for _, synced := range params.Contacts {
			match, contact, matched, saveErr := s.syncOne(ctx, tx, params.AccountID, params.SourceDeviceID, synced, existingByUser)
			if saveErr != nil {
				return saveErr
			}
			if !matched {
				continue
			}
			if _, ok := matchedUsers[contact.ContactAccountID]; ok {
				continue
			}
			matchedUsers[contact.ContactAccountID] = struct{}{}
			result.Matches = append(result.Matches, match)

			existingContact, existed := existingByUser[contact.ContactAccountID]
			switch {
			case !existed:
				result.NewContacts++
			case contactChanged(existingContact, contact):
				result.UpdatedContacts++
			}

			existingByUser[contact.ContactAccountID] = contact
		}

		for _, contact := range existing {
			if contact.Source != ContactSourceSynced || contact.SourceDeviceID != params.SourceDeviceID {
				continue
			}
			if _, ok := matchedUsers[contact.ContactAccountID]; ok {
				continue
			}
			if err := tx.DeleteContact(ctx, params.AccountID, contact.ContactAccountID); err != nil {
				if err == ErrNotFound {
					continue
				}
				return fmt.Errorf("delete stale synced contact %s:%s: %w", params.AccountID, contact.ContactAccountID, err)
			}
			result.RemovedContacts++
		}

		return nil
	})
	if err != nil {
		return SyncResult{}, err
	}

	return result, nil
}

func (s *Service) syncOne(
	ctx context.Context,
	store Store,
	accountID string,
	sourceDeviceID string,
	synced SyncedContact,
	existingByUser map[string]Contact,
) (ContactMatch, Contact, bool, error) {
	synced.RawContactID = strings.TrimSpace(synced.RawContactID)
	synced.DisplayName = strings.TrimSpace(synced.DisplayName)
	synced.PhoneE164 = normalizePhone(synced.PhoneE164)
	synced.Email = normalizeEmail(synced.Email)
	synced.Checksum = strings.TrimSpace(synced.Checksum)
	if synced.RawContactID == "" {
		return ContactMatch{}, Contact{}, false, nil
	}

	target, matched, err := s.matchSyncedContact(ctx, synced)
	if err != nil {
		return ContactMatch{}, Contact{}, false, err
	}
	if !matched || target.ID == "" || target.ID == accountID || target.Status != identity.AccountStatusActive {
		return ContactMatch{}, Contact{}, false, nil
	}

	privacy, err := s.privacyByAccountID(ctx, store, target.ID)
	if err != nil {
		return ContactMatch{}, Contact{}, false, err
	}
	if !privacy.AllowContactSync {
		return ContactMatch{}, Contact{}, false, nil
	}

	now := s.currentTime()
	existing := existingByUser[target.ID]
	contact := Contact{
		OwnerAccountID:   accountID,
		ContactAccountID: target.ID,
		DisplayName:      synced.DisplayName,
		Username:         target.Username,
		PhoneHash:        phoneHash(synced.PhoneE164),
		Source:           ContactSourceSynced,
		Starred:          existing.Starred,
		AddedAt:          now,
		UpdatedAt:        now,
		RawContactID:     synced.RawContactID,
		SourceDeviceID:   sourceDeviceID,
		SyncChecksum:     synced.Checksum,
	}
	if contact.DisplayName == "" {
		contact.DisplayName = target.DisplayName
	}
	if existing.OwnerAccountID != "" {
		contact.AddedAt = existing.AddedAt
		if existing.Source != ContactSourceSynced && existing.Source != ContactSourceImported {
			contact.Source = existing.Source
			contact.DisplayName = existing.DisplayName
			contact.RawContactID = existing.RawContactID
			contact.SourceDeviceID = existing.SourceDeviceID
			contact.SyncChecksum = existing.SyncChecksum
		}
	}

	saved, err := store.SaveContact(ctx, contact)
	if err != nil {
		return ContactMatch{}, Contact{}, false, fmt.Errorf("save synced contact %s:%s: %w", accountID, target.ID, err)
	}

	return ContactMatch{
		RawContactID:  synced.RawContactID,
		ContactUserID: target.ID,
		DisplayName:   saved.DisplayName,
		Username:      saved.Username,
	}, saved, true, nil
}

func (s *Service) matchSyncedContact(ctx context.Context, synced SyncedContact) (identity.Account, bool, error) {
	if synced.PhoneE164 != "" {
		account, err := s.directory.AccountByPhone(ctx, synced.PhoneE164)
		if err == nil {
			return account, true, nil
		}
		if err != identity.ErrNotFound {
			return identity.Account{}, false, fmt.Errorf("match synced phone %s: %w", synced.PhoneE164, err)
		}
	}
	if synced.Email != "" {
		account, err := s.directory.AccountByEmail(ctx, synced.Email)
		if err == nil {
			return account, true, nil
		}
		if err != identity.ErrNotFound {
			return identity.Account{}, false, fmt.Errorf("match synced email %s: %w", synced.Email, err)
		}
	}

	return identity.Account{}, false, nil
}

func contactChanged(previous Contact, current Contact) bool {
	return previous.DisplayName != current.DisplayName ||
		previous.Username != current.Username ||
		previous.PhoneHash != current.PhoneHash ||
		previous.Source != current.Source ||
		previous.Starred != current.Starred ||
		previous.RawContactID != current.RawContactID ||
		previous.SourceDeviceID != current.SourceDeviceID ||
		previous.SyncChecksum != current.SyncChecksum
}
