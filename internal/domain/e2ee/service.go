package e2ee

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

type Service struct {
	store     Store
	directory Directory
	chats     Conversations
	now       func() time.Time
}

func NewService(store Store, directory Directory, chats Conversations) (*Service, error) {
	if store == nil || directory == nil || chats == nil {
		return nil, ErrInvalidInput
	}

	return &Service{
		store:     store,
		directory: directory,
		chats:     chats,
		now:       func() time.Time { return time.Now().UTC() },
	}, nil
}

func (s *Service) RotateDeviceKeys(ctx context.Context, params RotateDeviceKeysParams) (DeviceBundle, uint32, uint32, error) {
	if err := s.validateContext(ctx, "rotate device e2ee keys"); err != nil {
		return DeviceBundle{}, 0, 0, err
	}
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	if params.AccountID == "" || params.DeviceID == "" {
		return DeviceBundle{}, 0, 0, ErrInvalidInput
	}
	if err := validateSignedPreKey(params.SignedPreKey); err != nil {
		return DeviceBundle{}, 0, 0, err
	}
	for _, value := range params.OneTimePreKeys {
		if err := validateOneTimePreKey(value); err != nil {
			return DeviceBundle{}, 0, 0, err
		}
	}

	device, err := s.directory.DeviceByID(ctx, params.DeviceID)
	if err != nil {
		if err == identity.ErrNotFound {
			return DeviceBundle{}, 0, 0, ErrNotFound
		}
		return DeviceBundle{}, 0, 0, fmt.Errorf("load device %s: %w", params.DeviceID, err)
	}
	if device.AccountID != params.AccountID || device.Status != identity.DeviceStatusActive {
		return DeviceBundle{}, 0, 0, ErrForbidden
	}

	var (
		savedSigned            SignedPreKey
		expiredDirectSessions  uint32
		expiredGroupSenderKeys uint32
		expiresAt              = s.now()
	)
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
		if params.ExpirePendingDirectSessions {
			expiredDirectSessions, saveErr = tx.ExpirePendingDirectSessionsByDevice(ctx, params.AccountID, params.DeviceID, expiresAt)
			if saveErr != nil {
				return saveErr
			}
		}
		if params.ExpirePendingGroupSenderKeys {
			expiredGroupSenderKeys, saveErr = tx.ExpirePendingGroupSenderKeysBySenderDevice(ctx, params.AccountID, params.DeviceID, expiresAt)
			if saveErr != nil {
				return saveErr
			}
		}
		return nil
	}); err != nil {
		return DeviceBundle{}, 0, 0, err
	}

	bundles, err := s.fetchAccountBundles(ctx, s.store, FetchAccountBundlesParams{
		RequesterAccountID:   params.AccountID,
		RequesterDeviceID:    params.DeviceID,
		TargetAccountID:      params.AccountID,
		ConsumeOneTimePreKey: false,
	})
	if err != nil {
		return DeviceBundle{}, 0, 0, err
	}
	for _, bundle := range bundles {
		if bundle.DeviceID == params.DeviceID {
			bundle.SignedPreKey = savedSigned
			return bundle, expiredDirectSessions, expiredGroupSenderKeys, nil
		}
	}
	return DeviceBundle{}, 0, 0, ErrNotFound
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

func (s *Service) GetDeviceVerificationCode(ctx context.Context, params GetDeviceVerificationCodeParams) (DeviceVerificationCode, error) {
	if err := s.validateContext(ctx, "get device verification code"); err != nil {
		return DeviceVerificationCode{}, err
	}
	params.ObserverAccountID = strings.TrimSpace(params.ObserverAccountID)
	params.ObserverDeviceID = strings.TrimSpace(params.ObserverDeviceID)
	params.TargetAccountID = strings.TrimSpace(params.TargetAccountID)
	params.TargetDeviceID = strings.TrimSpace(params.TargetDeviceID)
	if params.ObserverAccountID == "" || params.ObserverDeviceID == "" || params.TargetAccountID == "" || params.TargetDeviceID == "" {
		return DeviceVerificationCode{}, ErrInvalidInput
	}

	observer, err := s.directory.DeviceByID(ctx, params.ObserverDeviceID)
	if err != nil {
		if err == identity.ErrNotFound {
			return DeviceVerificationCode{}, ErrNotFound
		}
		return DeviceVerificationCode{}, fmt.Errorf("load observer device %s: %w", params.ObserverDeviceID, err)
	}
	if observer.AccountID != params.ObserverAccountID || observer.Status != identity.DeviceStatusActive {
		return DeviceVerificationCode{}, ErrForbidden
	}

	target, err := s.directory.DeviceByID(ctx, params.TargetDeviceID)
	if err != nil {
		if err == identity.ErrNotFound {
			return DeviceVerificationCode{}, ErrNotFound
		}
		return DeviceVerificationCode{}, fmt.Errorf("load target device %s: %w", params.TargetDeviceID, err)
	}
	if target.AccountID != params.TargetAccountID || target.Status != identity.DeviceStatusActive {
		return DeviceVerificationCode{}, ErrForbidden
	}

	trusts, err := s.store.DeviceTrustsByObserverDevice(ctx, params.ObserverAccountID, params.ObserverDeviceID, params.TargetAccountID)
	if err != nil {
		return DeviceVerificationCode{}, err
	}
	currentState := DeviceTrustStateUnspecified
	for _, item := range trusts {
		if item.TargetDeviceID == params.TargetDeviceID {
			currentState = item.State
			break
		}
	}

	return DeviceVerificationCode{
		ObserverAccountID:    params.ObserverAccountID,
		ObserverDeviceID:     params.ObserverDeviceID,
		TargetAccountID:      params.TargetAccountID,
		TargetDeviceID:       params.TargetDeviceID,
		TargetKeyFingerprint: deviceKeyFingerprint(target),
		SafetyNumber:         safetyNumber(observer, target),
		CurrentTrustState:    currentState,
	}, nil
}

func (s *Service) VerifyDeviceSafetyNumber(ctx context.Context, params VerifyDeviceSafetyNumberParams) (DeviceTrust, error) {
	if err := s.validateContext(ctx, "verify device safety number"); err != nil {
		return DeviceTrust{}, err
	}
	params.SafetyNumber = normalizeSafetyNumber(params.SafetyNumber)
	code, err := s.GetDeviceVerificationCode(ctx, GetDeviceVerificationCodeParams{
		ObserverAccountID: params.ObserverAccountID,
		ObserverDeviceID:  params.ObserverDeviceID,
		TargetAccountID:   params.TargetAccountID,
		TargetDeviceID:    params.TargetDeviceID,
	})
	if err != nil {
		return DeviceTrust{}, err
	}
	if params.SafetyNumber == "" || code.SafetyNumber != params.SafetyNumber {
		return DeviceTrust{}, ErrConflict
	}
	return s.SetDeviceTrust(ctx, SetDeviceTrustParams{
		ObserverAccountID: params.ObserverAccountID,
		ObserverDeviceID:  params.ObserverDeviceID,
		TargetAccountID:   params.TargetAccountID,
		TargetDeviceID:    params.TargetDeviceID,
		State:             DeviceTrustStateTrusted,
		Note:              params.Note,
	})
}

func (s *Service) SetDeviceTrust(ctx context.Context, params SetDeviceTrustParams) (DeviceTrust, error) {
	if err := s.validateContext(ctx, "set device trust"); err != nil {
		return DeviceTrust{}, err
	}
	params.ObserverAccountID = strings.TrimSpace(params.ObserverAccountID)
	params.ObserverDeviceID = strings.TrimSpace(params.ObserverDeviceID)
	params.TargetAccountID = strings.TrimSpace(params.TargetAccountID)
	params.TargetDeviceID = strings.TrimSpace(params.TargetDeviceID)
	params.Note = strings.TrimSpace(params.Note)
	if params.ObserverAccountID == "" || params.ObserverDeviceID == "" || params.TargetAccountID == "" || params.TargetDeviceID == "" {
		return DeviceTrust{}, ErrInvalidInput
	}
	if !isValidDeviceTrustState(params.State) {
		return DeviceTrust{}, ErrInvalidInput
	}

	observer, err := s.directory.DeviceByID(ctx, params.ObserverDeviceID)
	if err != nil {
		if err == identity.ErrNotFound {
			return DeviceTrust{}, ErrNotFound
		}
		return DeviceTrust{}, fmt.Errorf("load observer device %s: %w", params.ObserverDeviceID, err)
	}
	if observer.AccountID != params.ObserverAccountID || observer.Status != identity.DeviceStatusActive {
		return DeviceTrust{}, ErrForbidden
	}

	target, err := s.directory.DeviceByID(ctx, params.TargetDeviceID)
	if err != nil {
		if err == identity.ErrNotFound {
			return DeviceTrust{}, ErrNotFound
		}
		return DeviceTrust{}, fmt.Errorf("load target device %s: %w", params.TargetDeviceID, err)
	}
	if target.AccountID != params.TargetAccountID {
		return DeviceTrust{}, ErrForbidden
	}

	now := s.now()
	row := DeviceTrust{
		ObserverAccountID: params.ObserverAccountID,
		ObserverDeviceID:  params.ObserverDeviceID,
		TargetAccountID:   params.TargetAccountID,
		TargetDeviceID:    params.TargetDeviceID,
		State:             params.State,
		KeyFingerprint:    deviceKeyFingerprint(target),
		Note:              params.Note,
		UpdatedAt:         now,
	}
	saved, err := s.store.SaveDeviceTrust(ctx, row)
	if err != nil {
		return DeviceTrust{}, err
	}
	return saved, nil
}

func (s *Service) ListDeviceTrusts(ctx context.Context, params ListDeviceTrustsParams) ([]DeviceTrust, error) {
	if err := s.validateContext(ctx, "list device trusts"); err != nil {
		return nil, err
	}
	params.ObserverAccountID = strings.TrimSpace(params.ObserverAccountID)
	params.ObserverDeviceID = strings.TrimSpace(params.ObserverDeviceID)
	params.TargetAccountID = strings.TrimSpace(params.TargetAccountID)
	if params.ObserverAccountID == "" || params.ObserverDeviceID == "" {
		return nil, ErrInvalidInput
	}

	observer, err := s.directory.DeviceByID(ctx, params.ObserverDeviceID)
	if err != nil {
		if err == identity.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load observer device %s: %w", params.ObserverDeviceID, err)
	}
	if observer.AccountID != params.ObserverAccountID || observer.Status != identity.DeviceStatusActive {
		return nil, ErrForbidden
	}

	rows, err := s.store.DeviceTrustsByObserverDevice(ctx, params.ObserverAccountID, params.ObserverDeviceID, params.TargetAccountID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) GetConversationKeyCoverage(ctx context.Context, params GetConversationKeyCoverageParams) ([]ConversationKeyCoverageEntry, error) {
	if err := s.validateContext(ctx, "get conversation key coverage"); err != nil {
		return nil, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.SenderAccountID = strings.TrimSpace(params.SenderAccountID)
	params.SenderDeviceID = strings.TrimSpace(params.SenderDeviceID)
	params.SenderKeyID = strings.TrimSpace(params.SenderKeyID)
	if params.ConversationID == "" || params.SenderAccountID == "" || params.SenderDeviceID == "" {
		return nil, ErrInvalidInput
	}

	device, err := s.directory.DeviceByID(ctx, params.SenderDeviceID)
	if err != nil {
		if err == identity.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load sender device %s: %w", params.SenderDeviceID, err)
	}
	if device.AccountID != params.SenderAccountID || device.Status != identity.DeviceStatusActive {
		return nil, ErrForbidden
	}

	conversationRow, err := s.chats.ConversationByID(ctx, params.ConversationID)
	if err != nil {
		return nil, mapConversationError("load conversation", params.ConversationID, err)
	}
	member, err := s.chats.ConversationMemberByConversationAndAccount(ctx, params.ConversationID, params.SenderAccountID)
	if err != nil {
		return nil, mapConversationError("authorize sender", params.ConversationID, err)
	}
	if !isActiveConversationMember(member) {
		return nil, ErrForbidden
	}

	trusts, err := s.store.DeviceTrustsByObserverDevice(ctx, params.SenderAccountID, params.SenderDeviceID, "")
	if err != nil {
		return nil, fmt.Errorf("load trust map for %s/%s: %w", params.SenderAccountID, params.SenderDeviceID, err)
	}
	trustByDevice := make(map[string]DeviceTrust, len(trusts))
	for _, item := range trusts {
		key := trustMapKey(item.TargetAccountID, item.TargetDeviceID)
		if _, exists := trustByDevice[key]; exists {
			continue
		}
		trustByDevice[key] = item
	}

	switch conversationRow.Kind {
	case conversation.ConversationKindDirect:
		return s.directConversationKeyCoverage(ctx, params, trustByDevice)
	case conversation.ConversationKindGroup:
		return s.groupConversationKeyCoverage(ctx, params, trustByDevice)
	default:
		return nil, ErrForbidden
	}
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

func (s *Service) PublishGroupSenderKeys(ctx context.Context, params PublishGroupSenderKeysParams) ([]GroupSenderKeyDistribution, error) {
	if err := s.validateContext(ctx, "publish group sender keys"); err != nil {
		return nil, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.SenderAccountID = strings.TrimSpace(params.SenderAccountID)
	params.SenderDeviceID = strings.TrimSpace(params.SenderDeviceID)
	params.SenderKeyID = strings.TrimSpace(params.SenderKeyID)
	if params.ConversationID == "" || params.SenderAccountID == "" || params.SenderDeviceID == "" || params.SenderKeyID == "" {
		return nil, ErrInvalidInput
	}
	if params.ExpiresAt.IsZero() {
		params.ExpiresAt = s.now().Add(30 * 24 * time.Hour)
	}
	if !params.ExpiresAt.After(s.now()) {
		return nil, ErrInvalidInput
	}

	device, err := s.directory.DeviceByID(ctx, params.SenderDeviceID)
	if err != nil {
		if err == identity.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load sender device %s: %w", params.SenderDeviceID, err)
	}
	if device.AccountID != params.SenderAccountID || device.Status != identity.DeviceStatusActive {
		return nil, ErrForbidden
	}

	conversationRow, err := s.chats.ConversationByID(ctx, params.ConversationID)
	if err != nil {
		return nil, mapConversationError("load conversation", params.ConversationID, err)
	}
	if conversationRow.Kind != conversation.ConversationKindGroup {
		return nil, ErrForbidden
	}
	member, err := s.chats.ConversationMemberByConversationAndAccount(ctx, params.ConversationID, params.SenderAccountID)
	if err != nil {
		return nil, mapConversationError("authorize sender", params.ConversationID, err)
	}
	if !isActiveConversationMember(member) {
		return nil, ErrForbidden
	}
	if len(params.Recipients) == 0 {
		return nil, ErrInvalidInput
	}

	members, err := s.chats.ConversationMembersByConversationID(ctx, params.ConversationID)
	if err != nil {
		return nil, mapConversationError("list conversation members", params.ConversationID, err)
	}
	activeMembers := make(map[string]struct{}, len(members))
	for _, item := range members {
		if isActiveConversationMember(item) {
			activeMembers[item.AccountID] = struct{}{}
		}
	}

	distributions := make([]GroupSenderKeyDistribution, 0, len(params.Recipients))
	seen := make(map[string]struct{}, len(params.Recipients))
	err = s.store.WithinTx(ctx, func(tx Store) error {
		for _, recipient := range params.Recipients {
			recipient.RecipientAccountID = strings.TrimSpace(recipient.RecipientAccountID)
			recipient.RecipientDeviceID = strings.TrimSpace(recipient.RecipientDeviceID)
			if recipient.RecipientAccountID == "" || recipient.RecipientDeviceID == "" {
				return ErrInvalidInput
			}
			if recipient.RecipientAccountID == params.SenderAccountID && recipient.RecipientDeviceID == params.SenderDeviceID {
				continue
			}
			if _, ok := activeMembers[recipient.RecipientAccountID]; !ok {
				return ErrForbidden
			}
			if err := validateSenderKeyPayload(recipient.Payload); err != nil {
				return err
			}
			targetDevice, loadErr := s.directory.DeviceByID(ctx, recipient.RecipientDeviceID)
			if loadErr != nil {
				if loadErr == identity.ErrNotFound {
					return ErrNotFound
				}
				return fmt.Errorf("load recipient device %s: %w", recipient.RecipientDeviceID, loadErr)
			}
			if targetDevice.AccountID != recipient.RecipientAccountID || targetDevice.Status != identity.DeviceStatusActive {
				return ErrForbidden
			}
			key := recipient.RecipientAccountID + "|" + recipient.RecipientDeviceID
			if _, ok := seen[key]; ok {
				return ErrConflict
			}
			seen[key] = struct{}{}

			distributionID, idErr := newID("gsk")
			if idErr != nil {
				return idErr
			}
			saved, saveErr := tx.SaveGroupSenderKeyDistribution(ctx, GroupSenderKeyDistribution{
				ID:                 distributionID,
				ConversationID:     params.ConversationID,
				SenderAccountID:    params.SenderAccountID,
				SenderDeviceID:     params.SenderDeviceID,
				RecipientAccountID: recipient.RecipientAccountID,
				RecipientDeviceID:  recipient.RecipientDeviceID,
				SenderKeyID:        params.SenderKeyID,
				Payload:            normalizeSenderKeyPayload(recipient.Payload),
				State:              GroupSenderKeyStatePending,
				CreatedAt:          s.now(),
				ExpiresAt:          params.ExpiresAt.UTC(),
			})
			if saveErr != nil {
				return saveErr
			}
			distributions = append(distributions, saved)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(distributions) == 0 {
		return nil, ErrConflict
	}

	return distributions, nil
}

func (s *Service) ListGroupSenderKeys(ctx context.Context, params ListGroupSenderKeysParams) ([]GroupSenderKeyDistribution, error) {
	if err := s.validateContext(ctx, "list group sender keys"); err != nil {
		return nil, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.RecipientAccountID = strings.TrimSpace(params.RecipientAccountID)
	params.RecipientDeviceID = strings.TrimSpace(params.RecipientDeviceID)
	if params.ConversationID == "" || params.RecipientAccountID == "" || params.RecipientDeviceID == "" {
		return nil, ErrInvalidInput
	}

	member, err := s.chats.ConversationMemberByConversationAndAccount(ctx, params.ConversationID, params.RecipientAccountID)
	if err != nil {
		return nil, mapConversationError("authorize recipient", params.ConversationID, err)
	}
	if !isActiveConversationMember(member) {
		return nil, ErrForbidden
	}
	device, err := s.directory.DeviceByID(ctx, params.RecipientDeviceID)
	if err != nil {
		if err == identity.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load recipient device %s: %w", params.RecipientDeviceID, err)
	}
	if device.AccountID != params.RecipientAccountID || device.Status != identity.DeviceStatusActive {
		return nil, ErrForbidden
	}

	distributions, err := s.store.GroupSenderKeyDistributionsByRecipientDevice(
		ctx,
		params.ConversationID,
		params.RecipientAccountID,
		params.RecipientDeviceID,
	)
	if err != nil {
		return nil, err
	}
	filtered := distributions[:0]
	now := s.now()
	for _, item := range distributions {
		if !item.ExpiresAt.IsZero() && !now.Before(item.ExpiresAt) {
			continue
		}
		if !params.IncludeAcknowledged && item.State == GroupSenderKeyStateAcknowledged {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func (s *Service) AcknowledgeGroupSenderKey(ctx context.Context, params AcknowledgeGroupSenderKeyParams) (GroupSenderKeyDistribution, error) {
	if err := s.validateContext(ctx, "acknowledge group sender key"); err != nil {
		return GroupSenderKeyDistribution{}, err
	}
	params.DistributionID = strings.TrimSpace(params.DistributionID)
	params.RecipientAccountID = strings.TrimSpace(params.RecipientAccountID)
	params.RecipientDeviceID = strings.TrimSpace(params.RecipientDeviceID)
	if params.DistributionID == "" || params.RecipientAccountID == "" || params.RecipientDeviceID == "" {
		return GroupSenderKeyDistribution{}, ErrInvalidInput
	}

	var updated GroupSenderKeyDistribution
	err := s.store.WithinTx(ctx, func(tx Store) error {
		row, loadErr := tx.GroupSenderKeyDistributionByID(ctx, params.DistributionID)
		if loadErr != nil {
			return loadErr
		}
		if row.RecipientAccountID != params.RecipientAccountID || row.RecipientDeviceID != params.RecipientDeviceID {
			return ErrForbidden
		}
		if row.State == GroupSenderKeyStateAcknowledged {
			updated = row
			return nil
		}
		if !row.ExpiresAt.IsZero() && !s.now().Before(row.ExpiresAt) {
			return ErrConflict
		}
		row.State = GroupSenderKeyStateAcknowledged
		row.AcknowledgedAt = s.now()
		var saveErr error
		updated, saveErr = tx.SaveGroupSenderKeyDistribution(ctx, row)
		return saveErr
	})
	if err != nil {
		return GroupSenderKeyDistribution{}, err
	}
	return updated, nil
}

func (s *Service) ValidateConversationPayload(ctx context.Context, params ValidateConversationPayloadParams) error {
	if err := s.validateContext(ctx, "validate conversation payload"); err != nil {
		return err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.SenderAccountID = strings.TrimSpace(params.SenderAccountID)
	params.SenderDeviceID = strings.TrimSpace(params.SenderDeviceID)
	params.PayloadKeyID = strings.TrimSpace(params.PayloadKeyID)
	if params.ConversationID == "" || params.SenderAccountID == "" || params.SenderDeviceID == "" || params.PayloadKeyID == "" {
		return ErrInvalidInput
	}

	conversationRow, err := s.chats.ConversationByID(ctx, params.ConversationID)
	if err != nil {
		return mapConversationError("load conversation", params.ConversationID, err)
	}

	switch conversationRow.Kind {
	case conversation.ConversationKindDirect:
		return s.validateDirectConversationPayload(ctx, params)
	case conversation.ConversationKindGroup:
		return s.validateGroupConversationPayload(ctx, params)
	default:
		return nil
	}
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

func validateSenderKeyPayload(value SenderKeyPayload) error {
	value.Algorithm = strings.TrimSpace(value.Algorithm)
	if value.Algorithm == "" || len(value.Ciphertext) == 0 {
		return ErrInvalidInput
	}
	return nil
}

func normalizeSenderKeyPayload(value SenderKeyPayload) SenderKeyPayload {
	value.Algorithm = strings.TrimSpace(value.Algorithm)
	value.Nonce = append([]byte(nil), value.Nonce...)
	value.Ciphertext = append([]byte(nil), value.Ciphertext...)
	value.Metadata = trimMetadata(value.Metadata)
	return value
}

func isValidDeviceTrustState(value DeviceTrustState) bool {
	switch value {
	case DeviceTrustStateTrusted, DeviceTrustStateUntrusted, DeviceTrustStateCompromised:
		return true
	default:
		return false
	}
}

func deviceKeyFingerprint(device identity.Device) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(device.PublicKey)))
	return hex.EncodeToString(sum[:])
}

func safetyNumber(observer identity.Device, target identity.Device) string {
	observerKey := strings.TrimSpace(observer.PublicKey)
	targetKey := strings.TrimSpace(target.PublicKey)
	if observerKey > targetKey {
		observerKey, targetKey = targetKey, observerKey
	}
	sum := sha256.Sum256([]byte(observerKey + ":" + targetKey))
	encoded := strings.ToUpper(hex.EncodeToString(sum[:16]))
	parts := make([]string, 0, len(encoded)/4)
	for i := 0; i < len(encoded); i += 4 {
		end := i + 4
		if end > len(encoded) {
			end = len(encoded)
		}
		parts = append(parts, encoded[i:end])
	}
	return strings.Join(parts, "-")
}

func normalizeSafetyNumber(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, " ", "")
	if value == "" {
		return ""
	}
	parts := make([]string, 0, len(value)/4)
	for i := 0; i < len(value); i += 4 {
		end := i + 4
		if end > len(value) {
			end = len(value)
		}
		parts = append(parts, value[i:end])
	}
	return strings.Join(parts, "-")
}

func isActiveConversationMember(member conversation.ConversationMember) bool {
	return member.LeftAt.IsZero() && !member.Banned
}

func mapConversationError(action string, conversationID string, err error) error {
	switch err {
	case nil:
		return nil
	case conversation.ErrNotFound:
		return ErrNotFound
	case conversation.ErrForbidden:
		return ErrForbidden
	default:
		return fmt.Errorf("%s %s: %w", action, conversationID, err)
	}
}

func (s *Service) validateDirectConversationPayload(ctx context.Context, params ValidateConversationPayloadParams) error {
	sessionID := strings.TrimSpace(params.PayloadMetadata["direct_session_id"])
	if sessionID == "" {
		return ErrInvalidInput
	}
	session, err := s.store.DirectSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.State != DirectSessionStateAcknowledged {
		return ErrConflict
	}
	if session.ExpiresAt.IsZero() || !s.now().Before(session.ExpiresAt) {
		return ErrConflict
	}
	if params.PayloadKeyID != session.SignedPreKey.Key.KeyID && params.PayloadKeyID != session.OneTimePreKey.Key.KeyID && params.PayloadKeyID != session.ID {
		return ErrConflict
	}
	if session.InitiatorAccountID == params.SenderAccountID && session.InitiatorDeviceID == params.SenderDeviceID {
		return nil
	}
	if session.RecipientAccountID == params.SenderAccountID && session.RecipientDeviceID == params.SenderDeviceID {
		return nil
	}
	return ErrForbidden
}

func (s *Service) validateGroupConversationPayload(ctx context.Context, params ValidateConversationPayloadParams) error {
	senderKeyID := strings.TrimSpace(params.PayloadMetadata["sender_key_id"])
	if senderKeyID == "" {
		return ErrInvalidInput
	}
	if params.PayloadKeyID != senderKeyID {
		return ErrConflict
	}
	distributions, err := s.store.GroupSenderKeyDistributionsBySenderKey(
		ctx,
		params.ConversationID,
		params.SenderAccountID,
		params.SenderDeviceID,
		senderKeyID,
	)
	if err != nil {
		return err
	}
	if len(distributions) == 0 {
		return ErrNotFound
	}
	now := s.now()
	for _, item := range distributions {
		if item.ExpiresAt.IsZero() || now.Before(item.ExpiresAt) {
			return nil
		}
	}
	return ErrConflict
}

func (s *Service) directConversationKeyCoverage(
	ctx context.Context,
	params GetConversationKeyCoverageParams,
	trustByDevice map[string]DeviceTrust,
) ([]ConversationKeyCoverageEntry, error) {
	members, err := s.chats.ConversationMembersByConversationID(ctx, params.ConversationID)
	if err != nil {
		return nil, mapConversationError("list conversation members", params.ConversationID, err)
	}

	sessions, err := s.store.DirectSessionsByParticipantDevice(ctx, params.SenderAccountID, params.SenderDeviceID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	result := make([]ConversationKeyCoverageEntry, 0)
	for _, member := range members {
		if !isActiveConversationMember(member) || member.AccountID == params.SenderAccountID {
			continue
		}
		devices, loadErr := s.directory.DevicesByAccountID(ctx, member.AccountID)
		if loadErr != nil {
			return nil, fmt.Errorf("list devices for %s: %w", member.AccountID, loadErr)
		}
		for _, device := range devices {
			if device.Status != identity.DeviceStatusActive || strings.TrimSpace(device.PublicKey) == "" {
				continue
			}
			entry := ConversationKeyCoverageEntry{
				AccountID:      member.AccountID,
				DeviceID:       device.ID,
				State:          ConversationKeyCoverageStateMissing,
				TrustState:     trustByDevice[trustMapKey(member.AccountID, device.ID)].State,
				KeyFingerprint: deviceKeyFingerprint(device),
			}
			for _, session := range sessions {
				if session.InitiatorAccountID == params.SenderAccountID && session.InitiatorDeviceID == params.SenderDeviceID {
					if session.RecipientAccountID != member.AccountID || session.RecipientDeviceID != device.ID {
						continue
					}
				} else if session.RecipientAccountID == params.SenderAccountID && session.RecipientDeviceID == params.SenderDeviceID {
					if session.InitiatorAccountID != member.AccountID || session.InitiatorDeviceID != device.ID {
						continue
					}
				} else {
					continue
				}
				entry = coverageEntryFromDirectSession(session, now, device, trustByDevice[trustMapKey(member.AccountID, device.ID)])
				entry.AccountID = member.AccountID
				entry.DeviceID = device.ID
				break
			}
			entry.VerificationRequired = trustRequiresVerification(entry.TrustState)
			result = append(result, entry)
		}
	}
	return result, nil
}

func (s *Service) groupConversationKeyCoverage(
	ctx context.Context,
	params GetConversationKeyCoverageParams,
	trustByDevice map[string]DeviceTrust,
) ([]ConversationKeyCoverageEntry, error) {
	members, err := s.chats.ConversationMembersByConversationID(ctx, params.ConversationID)
	if err != nil {
		return nil, mapConversationError("list conversation members", params.ConversationID, err)
	}
	distributions, err := s.store.GroupSenderKeyDistributionsBySenderDevice(ctx, params.ConversationID, params.SenderAccountID, params.SenderDeviceID)
	if err != nil {
		return nil, err
	}

	now := s.now()
	result := make([]ConversationKeyCoverageEntry, 0)
	for _, member := range members {
		if !isActiveConversationMember(member) || member.AccountID == params.SenderAccountID {
			continue
		}
		devices, loadErr := s.directory.DevicesByAccountID(ctx, member.AccountID)
		if loadErr != nil {
			return nil, fmt.Errorf("list devices for %s: %w", member.AccountID, loadErr)
		}
		for _, device := range devices {
			if device.Status != identity.DeviceStatusActive || strings.TrimSpace(device.PublicKey) == "" {
				continue
			}
			entry := ConversationKeyCoverageEntry{
				AccountID:      member.AccountID,
				DeviceID:       device.ID,
				State:          ConversationKeyCoverageStateMissing,
				TrustState:     trustByDevice[trustMapKey(member.AccountID, device.ID)].State,
				KeyFingerprint: deviceKeyFingerprint(device),
			}
			for _, item := range distributions {
				if item.RecipientAccountID != member.AccountID || item.RecipientDeviceID != device.ID {
					continue
				}
				if params.SenderKeyID != "" && item.SenderKeyID != params.SenderKeyID {
					continue
				}
				entry = coverageEntryFromSenderKeyDistribution(item, now, device, trustByDevice[trustMapKey(member.AccountID, device.ID)])
				entry.AccountID = member.AccountID
				entry.DeviceID = device.ID
				break
			}
			entry.VerificationRequired = trustRequiresVerification(entry.TrustState)
			result = append(result, entry)
		}
	}
	return result, nil
}

func coverageEntryFromDirectSession(
	value DirectSession,
	now time.Time,
	target identity.Device,
	trust DeviceTrust,
) ConversationKeyCoverageEntry {
	entry := ConversationKeyCoverageEntry{
		ReferenceID:          value.ID,
		ExpiresAt:            value.ExpiresAt,
		TrustState:           trust.State,
		KeyFingerprint:       deviceKeyFingerprint(target),
		VerificationRequired: trustRequiresVerification(trust.State),
	}
	if !value.ExpiresAt.IsZero() && !now.Before(value.ExpiresAt) {
		entry.State = ConversationKeyCoverageStateExpired
		return entry
	}
	switch value.State {
	case DirectSessionStateAcknowledged:
		entry.State = ConversationKeyCoverageStateReady
	case DirectSessionStatePending:
		entry.State = ConversationKeyCoverageStatePending
	default:
		entry.State = ConversationKeyCoverageStateMissing
	}
	return entry
}

func coverageEntryFromSenderKeyDistribution(
	value GroupSenderKeyDistribution,
	now time.Time,
	target identity.Device,
	trust DeviceTrust,
) ConversationKeyCoverageEntry {
	entry := ConversationKeyCoverageEntry{
		ReferenceID:          value.SenderKeyID,
		ExpiresAt:            value.ExpiresAt,
		TrustState:           trust.State,
		KeyFingerprint:       deviceKeyFingerprint(target),
		VerificationRequired: trustRequiresVerification(trust.State),
	}
	if !value.ExpiresAt.IsZero() && !now.Before(value.ExpiresAt) {
		entry.State = ConversationKeyCoverageStateExpired
		return entry
	}
	switch value.State {
	case GroupSenderKeyStateAcknowledged:
		entry.State = ConversationKeyCoverageStateReady
	case GroupSenderKeyStatePending:
		entry.State = ConversationKeyCoverageStatePending
	default:
		entry.State = ConversationKeyCoverageStateMissing
	}
	return entry
}

func trustMapKey(accountID string, deviceID string) string {
	return accountID + "|" + deviceID
}

func trustRequiresVerification(state DeviceTrustState) bool {
	return state != DeviceTrustStateTrusted
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
