package teststore

import (
	"context"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SaveDevice stores a device and indexes it by account for later fanout and cleanup.
func (s *memoryStore) SaveDevice(_ context.Context, device identity.Device) (identity.Device, error) {
	if device.ID == "" {
		return identity.Device{}, identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.devicesByID[device.ID] = device
	s.deviceIDsForAccountLocked(device.AccountID)[device.ID] = struct{}{}

	return device, nil
}

// DeleteDevice removes a device and detaches it from the owning account index.
func (s *memoryStore) DeleteDevice(_ context.Context, deviceID string) error {
	if deviceID == "" {
		return identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	device, ok := s.devicesByID[deviceID]
	if !ok {
		return identity.ErrNotFound
	}

	delete(s.devicesByID, deviceID)
	if deviceIDs, ok := s.deviceIDsByAccount[device.AccountID]; ok {
		delete(deviceIDs, deviceID)
	}

	return nil
}

// DeviceByID resolves a device by its primary key.
func (s *memoryStore) DeviceByID(_ context.Context, deviceID string) (identity.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	device, ok := s.devicesByID[deviceID]
	if !ok {
		return identity.Device{}, identity.ErrNotFound
	}

	return device, nil
}

// DevicesByAccountID lists all known devices for an account.
func (s *memoryStore) DevicesByAccountID(_ context.Context, accountID string) ([]identity.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	deviceIDs, ok := s.deviceIDsByAccount[accountID]
	if !ok {
		return []identity.Device{}, nil
	}

	devices := make([]identity.Device, 0, len(deviceIDs))
	for deviceID := range deviceIDs {
		device, ok := s.devicesByID[deviceID]
		if !ok {
			continue
		}
		devices = append(devices, device)
	}

	return devices, nil
}

// SaveSession stores a session and indexes it by account for later lookup.
func (s *memoryStore) SaveSession(_ context.Context, session identity.Session) (identity.Session, error) {
	if session.ID == "" {
		return identity.Session{}, identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessionsByID[session.ID] = session
	s.sessionIDsForAccountLocked(session.AccountID)[session.ID] = struct{}{}

	return session, nil
}

// DeleteSession removes a session and detaches it from the owning account index.
func (s *memoryStore) DeleteSession(_ context.Context, sessionID string) error {
	if sessionID == "" {
		return identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessionsByID[sessionID]
	if !ok {
		return identity.ErrNotFound
	}

	delete(s.sessionsByID, sessionID)
	if sessionIDs, ok := s.sessionIDsByAccount[session.AccountID]; ok {
		delete(sessionIDs, sessionID)
	}

	return nil
}

// SessionByID resolves a session by its primary key.
func (s *memoryStore) SessionByID(_ context.Context, sessionID string) (identity.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessionsByID[sessionID]
	if !ok {
		return identity.Session{}, identity.ErrNotFound
	}

	return session, nil
}

// SessionsByAccountID lists all known sessions for an account.
func (s *memoryStore) SessionsByAccountID(_ context.Context, accountID string) ([]identity.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionIDs, ok := s.sessionIDsByAccount[accountID]
	if !ok {
		return []identity.Session{}, nil
	}

	sessions := make([]identity.Session, 0, len(sessionIDs))
	for sessionID := range sessionIDs {
		session, ok := s.sessionsByID[sessionID]
		if !ok {
			continue
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// UpdateSession replaces an existing session row while preserving the account index.
func (s *memoryStore) UpdateSession(_ context.Context, session identity.Session) (identity.Session, error) {
	if session.ID == "" {
		return identity.Session{}, identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessionsByID[session.ID]; !ok {
		return identity.Session{}, identity.ErrNotFound
	}

	s.sessionsByID[session.ID] = session
	if _, ok := s.sessionIDsByAccount[session.AccountID]; !ok {
		return identity.Session{}, identity.ErrNotFound
	}
	s.sessionIDsByAccount[session.AccountID][session.ID] = struct{}{}

	return session, nil
}
