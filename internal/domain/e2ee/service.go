package e2ee

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

type Service struct {
	store     Store
	directory Directory
	now       func() time.Time
}

func NewService(store Store, directory Directory) (*Service, error) {
	if store == nil || directory == nil {
		return nil, ErrInvalidInput
	}

	return &Service{
		store:     store,
		directory: directory,
		now:       func() time.Time { return time.Now().UTC() },
	}, nil
}

func (s *Service) UploadDevicePreKeys(ctx context.Context, params UploadDevicePreKeysParams) (DeviceBundle, error) {
	if err := s.validateContext(ctx, "upload device prekeys"); err != nil {
		return DeviceBundle{}, err
	}
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.AccountID == "" || params.DeviceID == "" {
		return DeviceBundle{}, ErrInvalidInput
	}
	if err := validateSignedPreKey(params.SignedPreKey); err != nil {
		return DeviceBundle{}, err
	}
	for _, value := range params.OneTimePreKeys {
		if err := validateOneTimePreKey(value); err != nil {
			return DeviceBundle{}, err
		}
	}

	device, err := s.directory.DeviceByID(ctx, params.DeviceID)
	if err != nil {
		if err == identity.ErrNotFound {
			return DeviceBundle{}, ErrNotFound
		}
		return DeviceBundle{}, fmt.Errorf("load device %s: %w", params.DeviceID, err)
	}
	if device.AccountID != params.AccountID {
		return DeviceBundle{}, ErrForbidden
	}
	if device.Status != identity.DeviceStatusActive {
		return DeviceBundle{}, ErrForbidden
	}

	var savedSigned SignedPreKey
	if err := s.store.WithinTx(ctx, func(tx Store) error {
		var saveErr error
		savedSigned, saveErr = tx.SaveSignedPreKey(ctx, params.AccountID, params.DeviceID, normalizeSignedPreKey(params.SignedPreKey))
		if saveErr != nil {
			return saveErr
		}
		if params.ReplaceOneTimePreKey {
			if deleteErr := tx.DeleteOneTimePreKeysByDevice(ctx, params.AccountID, params.DeviceID); deleteErr != nil {
				return deleteErr
			}
		}
		if len(params.OneTimePreKeys) > 0 {
			if saveErr := tx.SaveOneTimePreKeys(ctx, params.AccountID, params.DeviceID, normalizeOneTimePreKeys(params.OneTimePreKeys)); saveErr != nil {
				return saveErr
			}
		}
		return nil
	}); err != nil {
		return DeviceBundle{}, err
	}

	bundles, err := s.fetchAccountBundles(ctx, s.store, FetchAccountBundlesParams{
		RequesterAccountID:   params.AccountID,
		RequesterDeviceID:    params.DeviceID,
		TargetAccountID:      params.AccountID,
		ConsumeOneTimePreKey: false,
	})
	if err != nil {
		return DeviceBundle{}, err
	}
	for _, bundle := range bundles {
		if bundle.DeviceID == params.DeviceID {
			bundle.SignedPreKey = savedSigned
			return bundle, nil
		}
	}

	return DeviceBundle{}, ErrNotFound
}

func (s *Service) GetAccountBundles(ctx context.Context, params FetchAccountBundlesParams) ([]DeviceBundle, error) {
	if err := s.validateContext(ctx, "get account bundles"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.TargetAccountID) == "" || strings.TrimSpace(params.RequesterAccountID) == "" {
		return nil, ErrInvalidInput
	}
	if params.ConsumeOneTimePreKey {
		var bundles []DeviceBundle
		err := s.store.WithinTx(ctx, func(tx Store) error {
			var loadErr error
			bundles, loadErr = s.fetchAccountBundles(ctx, tx, params)
			return loadErr
		})
		return bundles, err
	}
	return s.fetchAccountBundles(ctx, s.store, params)
}

func (s *Service) fetchAccountBundles(ctx context.Context, store Store, params FetchAccountBundlesParams) ([]DeviceBundle, error) {
	account, err := s.directory.AccountByID(ctx, strings.TrimSpace(params.TargetAccountID))
	if err != nil {
		if err == identity.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load target account %s: %w", params.TargetAccountID, err)
	}
	if account.Status != identity.AccountStatusActive {
		return nil, ErrForbidden
	}

	devices, err := s.directory.DevicesByAccountID(ctx, account.ID)
	if err != nil {
		return nil, fmt.Errorf("list target devices for %s: %w", account.ID, err)
	}

	result := make([]DeviceBundle, 0, len(devices))
	for _, device := range devices {
		if device.Status != identity.DeviceStatusActive || strings.TrimSpace(device.PublicKey) == "" {
			continue
		}
		signed, err := store.SignedPreKeyByDevice(ctx, account.ID, device.ID)
		if err != nil {
			if err == ErrNotFound {
				continue
			}
			return nil, err
		}
		available, err := store.CountAvailableOneTimePreKeys(ctx, account.ID, device.ID)
		if err != nil {
			return nil, err
		}

		bundle := DeviceBundle{
			AccountID:           account.ID,
			DeviceID:            device.ID,
			IdentityKey:         identityKeyFromDevice(device),
			SignedPreKey:        signed,
			OneTimePreKeysAvail: available,
			DeviceLastSeenAt:    device.LastSeenAt,
		}
		if params.ConsumeOneTimePreKey {
			claimed, claimErr := store.ClaimOneTimePreKey(
				ctx,
				account.ID,
				device.ID,
				params.RequesterAccountID,
				params.RequesterDeviceID,
			)
			if claimErr != nil && claimErr != ErrNotFound {
				return nil, claimErr
			}
			if claimErr == nil {
				bundle.OneTimePreKey = claimed
				if bundle.OneTimePreKeysAvail > 0 {
					bundle.OneTimePreKeysAvail--
				}
			}
		}
		result = append(result, bundle)
	}

	return result, nil
}

func (s *Service) validateContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return fmt.Errorf("%s: %w", operation, ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func validateSignedPreKey(value SignedPreKey) error {
	if err := validatePublicKey(value.Key); err != nil {
		return err
	}
	if len(value.Signature) == 0 {
		return ErrInvalidInput
	}
	return nil
}

func validateOneTimePreKey(value OneTimePreKey) error {
	return validatePublicKey(value.Key)
}

func validatePublicKey(value PublicKey) error {
	value.KeyID = strings.TrimSpace(value.KeyID)
	value.Algorithm = strings.TrimSpace(value.Algorithm)
	if value.KeyID == "" || value.Algorithm == "" || len(value.PublicKey) == 0 {
		return ErrInvalidInput
	}
	return nil
}

func normalizeSignedPreKey(value SignedPreKey) SignedPreKey {
	value.Key = normalizePublicKey(value.Key)
	value.Signature = append([]byte(nil), value.Signature...)
	return value
}

func normalizeOneTimePreKeys(values []OneTimePreKey) []OneTimePreKey {
	if len(values) == 0 {
		return nil
	}
	result := make([]OneTimePreKey, len(values))
	for i := range values {
		result[i] = values[i]
		result[i].Key = normalizePublicKey(result[i].Key)
	}
	return result
}

func normalizePublicKey(value PublicKey) PublicKey {
	value.KeyID = strings.TrimSpace(value.KeyID)
	value.Algorithm = strings.TrimSpace(value.Algorithm)
	value.PublicKey = append([]byte(nil), value.PublicKey...)
	return value
}

func identityKeyFromDevice(device identity.Device) PublicKey {
	publicKey := strings.TrimSpace(device.PublicKey)
	if publicKey == "" {
		return PublicKey{}
	}
	decoded, err := base64.RawStdEncoding.DecodeString(publicKey)
	if err != nil {
		decoded = []byte(publicKey)
	}
	return PublicKey{
		KeyID:     device.ID,
		Algorithm: "device_public_key",
		PublicKey: decoded,
		CreatedAt: device.CreatedAt,
		RotatedAt: device.LastRotatedAt,
	}
}
