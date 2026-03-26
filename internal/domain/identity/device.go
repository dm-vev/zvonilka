package identity

import "context"

// RotateDeviceKey replaces the public key for one existing device.
func (s *Service) RotateDeviceKey(ctx context.Context, params RotateDeviceKeyParams) (Device, error) {
	if err := s.validateContext(ctx, "rotate device key"); err != nil {
		return Device{}, err
	}
	if params.AccountID == "" || params.DeviceID == "" || params.PublicKey == "" {
		return Device{}, ErrInvalidInput
	}

	var saved Device
	err := s.store.WithinTx(ctx, func(tx Store) error {
		account, err := s.lockAccount(ctx, tx, params.AccountID)
		if err != nil {
			return err
		}
		if account.Status != AccountStatusActive {
			return ErrForbidden
		}

		device, err := tx.DeviceByID(ctx, params.DeviceID)
		if err != nil {
			return err
		}
		if device.AccountID != params.AccountID || device.Status == DeviceStatusRevoked {
			return ErrForbidden
		}

		now := params.RequestedAt
		if now.IsZero() {
			now = s.currentTime()
		}

		device.PublicKey = params.PublicKey
		device.LastRotatedAt = now
		device.LastSeenAt = now

		saved, err = tx.SaveDevice(ctx, device)
		return err
	})
	if err != nil {
		return Device{}, err
	}

	return saved, nil
}
