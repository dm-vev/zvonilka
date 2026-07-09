package identity

import "context"

// ApproveDeviceLink activates one pending device after an existing device authorizes it.
func (s *Service) ApproveDeviceLink(ctx context.Context, params ApproveDeviceLinkParams) (Device, error) {
	if err := s.validateContext(ctx, "approve device link"); err != nil {
		return Device{}, err
	}
	if params.AccountID == "" || params.ApproverDeviceID == "" || params.TargetDeviceID == "" {
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

		approver, err := tx.DeviceByID(ctx, params.ApproverDeviceID)
		if err != nil {
			return err
		}
		if approver.AccountID != account.ID || approver.Status != DeviceStatusActive {
			return ErrForbidden
		}

		target, err := tx.DeviceByID(ctx, params.TargetDeviceID)
		if err != nil {
			return err
		}
		if target.AccountID != account.ID {
			return ErrForbidden
		}
		if target.Status == DeviceStatusActive {
			saved = target
			return nil
		}
		if target.Status != DeviceStatusUnverified {
			return ErrConflict
		}

		now := params.RequestedAt
		if now.IsZero() {
			now = s.currentTime()
		}

		target.Status = DeviceStatusActive
		target.LastSeenAt = now

		saved, err = tx.SaveDevice(ctx, target)
		return err
	})
	if err != nil {
		return Device{}, err
	}

	return saved, nil
}
