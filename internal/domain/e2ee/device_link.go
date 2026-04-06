package e2ee

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// QueueDeviceLinkTransfer stores one encrypted device-link bundle for a sibling device.
func (s *Service) QueueDeviceLinkTransfer(
	ctx context.Context,
	params QueueDeviceLinkTransferParams,
) (DeviceLinkTransfer, error) {
	if err := s.validateContext(ctx, "queue device link transfer"); err != nil {
		return DeviceLinkTransfer{}, err
	}
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.SourceDeviceID = strings.TrimSpace(params.SourceDeviceID)
	params.TargetDeviceID = strings.TrimSpace(params.TargetDeviceID)
	if params.AccountID == "" || params.SourceDeviceID == "" || params.TargetDeviceID == "" {
		return DeviceLinkTransfer{}, ErrInvalidInput
	}
	if err := validateSenderKeyPayload(params.Payload); err != nil {
		return DeviceLinkTransfer{}, err
	}

	source, err := s.directory.DeviceByID(ctx, params.SourceDeviceID)
	if err != nil {
		return DeviceLinkTransfer{}, fmt.Errorf("load source device %s: %w", params.SourceDeviceID, err)
	}
	if source.AccountID != params.AccountID || source.Status != identity.DeviceStatusActive {
		return DeviceLinkTransfer{}, ErrForbidden
	}

	target, err := s.directory.DeviceByID(ctx, params.TargetDeviceID)
	if err != nil {
		return DeviceLinkTransfer{}, fmt.Errorf("load target device %s: %w", params.TargetDeviceID, err)
	}
	if target.AccountID != params.AccountID || target.Status == identity.DeviceStatusRevoked {
		return DeviceLinkTransfer{}, ErrForbidden
	}

	transferID, err := newID("dlt")
	if err != nil {
		return DeviceLinkTransfer{}, fmt.Errorf("generate device link transfer ID: %w", err)
	}
	createdAt := params.RequestedAt
	if createdAt.IsZero() {
		createdAt = s.now()
	}

	return s.store.SaveDeviceLinkTransfer(ctx, DeviceLinkTransfer{
		ID:             transferID,
		AccountID:      params.AccountID,
		SourceDeviceID: params.SourceDeviceID,
		TargetDeviceID: params.TargetDeviceID,
		Payload:        normalizeSenderKeyPayload(params.Payload),
		CreatedAt:      createdAt.UTC(),
	})
}

// ConsumeDeviceLinkTransfer fetches and clears the pending link bundle for the device.
func (s *Service) ConsumeDeviceLinkTransfer(
	ctx context.Context,
	params ConsumeDeviceLinkTransferParams,
) (DeviceLinkTransfer, error) {
	if err := s.validateContext(ctx, "consume device link transfer"); err != nil {
		return DeviceLinkTransfer{}, err
	}
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.AccountID == "" || params.DeviceID == "" {
		return DeviceLinkTransfer{}, ErrInvalidInput
	}

	device, err := s.directory.DeviceByID(ctx, params.DeviceID)
	if err != nil {
		return DeviceLinkTransfer{}, fmt.Errorf("load target device %s: %w", params.DeviceID, err)
	}
	if device.AccountID != params.AccountID || device.Status == identity.DeviceStatusRevoked {
		return DeviceLinkTransfer{}, ErrForbidden
	}

	transfer, err := s.store.DeviceLinkTransferByTargetDevice(ctx, params.AccountID, params.DeviceID)
	if err != nil {
		return DeviceLinkTransfer{}, err
	}
	if err := s.store.DeleteDeviceLinkTransfer(ctx, transfer.ID); err != nil {
		return DeviceLinkTransfer{}, err
	}

	return transfer, nil
}
