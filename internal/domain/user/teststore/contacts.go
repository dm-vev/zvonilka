package teststore

import (
	"context"
	"sort"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

func (s *memoryStore) SaveContact(_ context.Context, contact domainuser.Contact) (domainuser.Contact, error) {
	if contact.OwnerAccountID == "" || contact.ContactAccountID == "" || contact.OwnerAccountID == contact.ContactAccountID {
		return domainuser.Contact{}, domainuser.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.contacts[contactKey(contact.OwnerAccountID, contact.ContactAccountID)] = contact
	return contact, nil
}

func (s *memoryStore) ContactByAccountAndUserID(
	_ context.Context,
	accountID string,
	userID string,
) (domainuser.Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	contact, ok := s.contacts[contactKey(accountID, userID)]
	if !ok {
		return domainuser.Contact{}, domainuser.ErrNotFound
	}

	return contact, nil
}

func (s *memoryStore) ListContactsByAccountID(_ context.Context, accountID string) ([]domainuser.Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	contacts := make([]domainuser.Contact, 0)
	for _, contact := range s.contacts {
		if contact.OwnerAccountID != accountID {
			continue
		}
		contacts = append(contacts, contact)
	}
	sort.Slice(contacts, func(i, j int) bool {
		if contacts[i].Starred != contacts[j].Starred {
			return contacts[i].Starred
		}
		if contacts[i].DisplayName != contacts[j].DisplayName {
			return contacts[i].DisplayName < contacts[j].DisplayName
		}
		return contacts[i].ContactAccountID < contacts[j].ContactAccountID
	})
	return contacts, nil
}

func (s *memoryStore) DeleteContact(_ context.Context, accountID string, userID string) error {
	key := contactKey(accountID, userID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.contacts[key]; !ok {
		return domainuser.ErrNotFound
	}
	delete(s.contacts, key)
	return nil
}
