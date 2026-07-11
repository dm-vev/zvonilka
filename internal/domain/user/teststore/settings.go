package teststore

import (
	"context"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

func (s *memoryStore) SaveAccountSettings(_ context.Context, settings domainuser.AccountSettings) (domainuser.AccountSettings, error) {
	if settings.AccountID == "" {
		return domainuser.AccountSettings{}, domainuser.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	settings.Browser.Exceptions = append([]domainuser.BrowserDomainException(nil), settings.Browser.Exceptions...)
	s.settings[settings.AccountID] = settings
	return settings, nil
}

func (s *memoryStore) AccountSettingsByAccountID(_ context.Context, accountID string) (domainuser.AccountSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	settings, ok := s.settings[accountID]
	if !ok {
		return domainuser.AccountSettings{}, domainuser.ErrNotFound
	}
	settings.Browser.Exceptions = append([]domainuser.BrowserDomainException(nil), settings.Browser.Exceptions...)
	return settings, nil
}
