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

func (s *Service) CreateDirectSessions(ctx context.Context, params CreateDirectSessionsParams) ([]DirectSession, error) {
	if err := s.validateContext(ctx, "create direct sessions"); err != nil {
		return nil, err
	}
	params.InitiatorAccountID = strings.TrimSpace(params.InitiatorAccountID)
	params.InitiatorDeviceID = strings.TrimSpace(params.InitiatorDeviceID)
	params.TargetAccountID = strings.TrimSpace(params.TargetAccountID)
	params.TargetDeviceID = strings.TrimSpace(params.TargetDeviceID)
	if params.InitiatorAccountID == "" || params.InitiatorDeviceID == "" || params.TargetAccountID == "" {
		return nil, ErrInvalidInput
	}
	if err := validatePublicKey(params.InitiatorEphemeral); err != nil {
		return nil, err
	}
	if err := validateBootstrapPayload(params.Bootstrap); err != nil {
		return nil, err
	}
	if params.ExpiresAt.IsZero() {
		params.ExpiresAt = s.now().Add(7 * 24 * time.Hour)
	}
	if !params.ExpiresAt.After(s.now()) {
		return nil, ErrInvalidInput
	}

	initiator, err := s.directory.DeviceByID(ctx, params.InitiatorDeviceID)
	if err != nil {
		if err == identity.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load initiator device %s: %w", params.InitiatorDeviceID, err)
	}
	if initiator.AccountID != params.InitiatorAccountID || initiator.Status != identity.DeviceStatusActive {
		return nil, ErrForbidden
	}

	targetAccount, err := s.directory.AccountByID(ctx, params.TargetAccountID)
	if err != nil {
		if err == identity.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load target account %s: %w", params.TargetAccountID, err)
	}
	if targetAccount.Status != identity.AccountStatusActive {
		return nil, ErrForbidden
	}

	targetDevices, err := s.directory.DevicesByAccountID(ctx, params.TargetAccountID)
	if err != nil {
		return nil, fmt.Errorf("list target devices for %s: %w", params.TargetAccountID, err)
	}

	sessions := make([]DirectSession, 0, len(targetDevices))
	err = s.store.WithinTx(ctx, func(tx Store) error {
		for _, device := range targetDevices {
			if device.Status != identity.DeviceStatusActive || strings.TrimSpace(device.PublicKey) == "" {
				continue
			}
			if params.TargetDeviceID != "" && device.ID != params.TargetDeviceID {
				continue
			}
			signed, loadErr := tx.SignedPreKeyByDevice(ctx, params.TargetAccountID, device.ID)
			if loadErr != nil {
				if loadErr == ErrNotFound {
					continue
				}
				return loadErr
			}

			claimed, claimErr := tx.ClaimOneTimePreKey(
				ctx,
				params.TargetAccountID,
				device.ID,
				params.InitiatorAccountID,
				params.InitiatorDeviceID,
			)
			if claimErr != nil && claimErr != ErrNotFound {
				return claimErr
			}

			sessionID, idErr := newID("dse")
			if idErr != nil {
				return idErr
			}
			createdAt := s.now()
			saved, saveErr := tx.SaveDirectSession(ctx, DirectSession{
				ID:                 sessionID,
				InitiatorAccountID: params.InitiatorAccountID,
				InitiatorDeviceID:  params.InitiatorDeviceID,
				RecipientAccountID: params.TargetAccountID,
				RecipientDeviceID:  device.ID,
				InitiatorEphemeral: normalizePublicKey(params.InitiatorEphemeral),
				IdentityKey:        identityKeyFromDevice(device),
				SignedPreKey:       signed,
				OneTimePreKey:      claimed,
				Bootstrap:          normalizeBootstrapPayload(params.Bootstrap),
				State:              DirectSessionStatePending,
				CreatedAt:          createdAt,
				ExpiresAt:          params.ExpiresAt.UTC(),
			})
			if saveErr != nil {
				return saveErr
			}
			sessions = append(sessions, saved)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if params.TargetDeviceID != "" && len(sessions) == 0 {
		return nil, ErrNotFound
	}
	if len(sessions) == 0 {
		return nil, ErrConflict
	}

	return sessions, nil
}

func (s *Service) ListDeviceSessions(ctx context.Context, params ListDeviceSessionsParams) ([]DirectSession, error) {
	if err := s.validateContext(ctx, "list device sessions"); err != nil {
		return nil, err
	}
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	params.PeerAccountID = strings.TrimSpace(params.PeerAccountID)
	if params.AccountID == "" || params.DeviceID == "" {
		return nil, ErrInvalidInput
	}

	device, err := s.directory.DeviceByID(ctx, params.DeviceID)
	if err != nil {
		if err == identity.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load recipient device %s: %w", params.DeviceID, err)
	}
	if device.AccountID != params.AccountID || device.Status != identity.DeviceStatusActive {
		return nil, ErrForbidden
	}

	sessions, err := s.store.DirectSessionsByRecipientDevice(ctx, params.AccountID, params.DeviceID)
	if err != nil {
		return nil, err
	}
	filtered := sessions[:0]
	now := s.now()
	for _, session := range sessions {
		if !session.ExpiresAt.IsZero() && !now.Before(session.ExpiresAt) {
			continue
		}
		if !params.IncludeAcknowledged && session.State == DirectSessionStateAcknowledged {
			continue
		}
		if params.PeerAccountID != "" && session.InitiatorAccountID != params.PeerAccountID {
			continue
		}
		filtered = append(filtered, session)
	}

	return filtered, nil
}

func (s *Service) AcknowledgeDirectSession(ctx context.Context, params AcknowledgeDirectSessionParams) (DirectSession, error) {
	if err := s.validateContext(ctx, "acknowledge direct session"); err != nil {
		return DirectSession{}, err
	}
	params.SessionID = strings.TrimSpace(params.SessionID)
	params.RecipientAccountID = strings.TrimSpace(params.RecipientAccountID)
	params.RecipientDeviceID = strings.TrimSpace(params.RecipientDeviceID)
	if params.SessionID == "" || params.RecipientAccountID == "" || params.RecipientDeviceID == "" {
		return DirectSession{}, ErrInvalidInput
	}

	var updated DirectSession
	err := s.store.WithinTx(ctx, func(tx Store) error {
		session, loadErr := tx.DirectSessionByID(ctx, params.SessionID)
		if loadErr != nil {
			return loadErr
		}
		if session.RecipientAccountID != params.RecipientAccountID || session.RecipientDeviceID != params.RecipientDeviceID {
			return ErrForbidden
		}
		if session.State == DirectSessionStateAcknowledged {
			updated = session
			return nil
		}
		if !session.ExpiresAt.IsZero() && !s.now().Before(session.ExpiresAt) {
			return ErrConflict
		}
		session.State = DirectSessionStateAcknowledged
		session.AcknowledgedAt = s.now()
		var saveErr error
		updated, saveErr = tx.SaveDirectSession(ctx, session)
		return saveErr
	})
	if err != nil {
		return DirectSession{}, err
	}

	return updated, nil
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

func validateBootstrapPayload(value BootstrapPayload) error {
	value.Algorithm = strings.TrimSpace(value.Algorithm)
	if value.Algorithm == "" || len(value.Ciphertext) == 0 {
		return ErrInvalidInput
	}
	return nil
}

func normalizePublicKey(value PublicKey) PublicKey {
	value.KeyID = strings.TrimSpace(value.KeyID)
	value.Algorithm = strings.TrimSpace(value.Algorithm)
	value.PublicKey = append([]byte(nil), value.PublicKey...)
	value.CreatedAt = value.CreatedAt.UTC()
	value.RotatedAt = value.RotatedAt.UTC()
	value.ExpiresAt = value.ExpiresAt.UTC()
	return value
}

func normalizeBootstrapPayload(value BootstrapPayload) BootstrapPayload {
	value.Algorithm = strings.TrimSpace(value.Algorithm)
	value.Nonce = append([]byte(nil), value.Nonce...)
	value.Ciphertext = append([]byte(nil), value.Ciphertext...)
	value.Metadata = trimMetadata(value.Metadata)
	return value
}

func trimMetadata(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	result := make(map[string]string, len(value))
	for key, item := range value {
		key = strings.TrimSpace(key)
		item = strings.TrimSpace(item)
		if key == "" || item == "" {
			continue
		}
		result[key] = item
	}
	if len(result) == 0 {
		return nil
	}
	return result
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
