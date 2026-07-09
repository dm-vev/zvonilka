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

	if previous, ok := s.devicesByID[device.ID]; ok {
		s.removeDeviceIndexLocked(previous.AccountID, previous.ID)
	}

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
	s.removeDeviceIndexLocked(device.AccountID, deviceID)

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

// removeDeviceIndexLocked detaches a device from the per-account index.
func (s *memoryStore) removeDeviceIndexLocked(accountID string, deviceID string) {
	if deviceIDs, ok := s.deviceIDsByAccount[accountID]; ok {
		delete(deviceIDs, deviceID)
		if len(deviceIDs) == 0 {
			delete(s.deviceIDsByAccount, accountID)
		}
	}
}
