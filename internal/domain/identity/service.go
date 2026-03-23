package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	defaultJoinRequestTTL = 72 * time.Hour
	defaultChallengeTTL   = 10 * time.Minute
	defaultAccessTTL      = 30 * time.Minute
	defaultRefreshTTL     = 30 * 24 * time.Hour
)

// Service coordinates account lifecycle, login, and session management.
type Service struct {
	store           Store
	sender          CodeSender
	idempotency     *idempotencyCache
	now             func() time.Time
	joinRequestTTL  time.Duration
	challengeTTL    time.Duration
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

// NewService constructs a service backed by the provided store and sender.
//
// The service defaults are safe for tests and local bootstraps, but callers may override
// the clock and lifecycle limits with options when they need deterministic behavior.
func NewService(store Store, sender CodeSender, opts ...Option) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if sender == nil {
		sender = NoopCodeSender{}
	}

	service := &Service{
		store:           store,
		sender:          sender,
		idempotency:     newIdempotencyCache(),
		now:             func() time.Time { return time.Now().UTC() },
		joinRequestTTL:  defaultJoinRequestTTL,
		challengeTTL:    defaultChallengeTTL,
		accessTokenTTL:  defaultAccessTTL,
		refreshTokenTTL: defaultRefreshTTL,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}

	return service, nil
}

// currentTime returns the service clock in UTC.
//
// The helper centralizes the nil-clock fallback so the rest of the package can assume
// a stable, UTC-normalized time source.
func (s *Service) currentTime() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}

	return s.now().UTC()
}

// validateContext rejects nil or cancelled contexts before a domain operation starts.
//
// The returned error is wrapped with the operation name so callers can trace which
// boundary rejected the request.
func (s *Service) validateContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return fmt.Errorf("%s: %w", operation, ErrInvalidInput)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return nil
}

// normalizeAccountInput canonicalizes the account identifiers that can be used for lookup.
//
// Username and email are trimmed and lower-cased; phone numbers are reduced to digits only.
// That keeps uniqueness checks and login lookups aligned with the persisted representation.
func (s *Service) normalizeAccountInput(username, email, phone string) (string, string, string) {
	return normalizeUsername(username), normalizeEmail(email), normalizePhone(phone)
}

// lookupAccountByIdentifier resolves exactly one normalized identifier.
//
// The login flow accepts one of username, email, or phone. Supporting more than one at once
// would make retry and idempotency semantics ambiguous, so the helper fails fast.
func (s *Service) lookupAccountByIdentifier(ctx context.Context, username, email, phone string) (Account, error) {
	count := 0
	if username != "" {
		count++
	}
	if email != "" {
		count++
	}
	if phone != "" {
		count++
	}
	if count != 1 {
		return Account{}, ErrInvalidInput
	}

	switch {
	case username != "":
		account, err := s.store.AccountByUsername(ctx, username)
		if err != nil {
			return Account{}, fmt.Errorf("lookup account by username %q: %w", username, err)
		}
		return account, nil
	case email != "":
		account, err := s.store.AccountByEmail(ctx, email)
		if err != nil {
			return Account{}, fmt.Errorf("lookup account by email %q: %w", email, err)
		}
		return account, nil
	default:
		account, err := s.store.AccountByPhone(ctx, phone)
		if err != nil {
			return Account{}, fmt.Errorf("lookup account by phone %q: %w", phone, err)
		}
		return account, nil
	}
}

/*
	issueSession creates a device/session pair and returns bearer tokens for the account.

The store interface is intentionally not transactional yet, so the function records the
device first, then the session, then the auth metadata. If any later step fails, the
defer block rolls back the partial writes so callers do not observe half-created login
state.
*/
func (s *Service) issueSession(
	ctx context.Context,
	account Account,
	deviceName string,
	platform DevicePlatform,
	publicKey string,
	pushToken string,
) (result LoginResult, err error) {
	if publicKey == "" {
		err = ErrInvalidInput
		return
	}
	if account.Status != AccountStatusActive {
		err = ErrForbidden
		return
	}

	var (
		savedDevice  Device
		savedSession Session
	)

	// Roll back partial writes if any later step fails after the device/session pair starts
	// materializing. The saved IDs are captured separately so cleanup can stay idempotent.
	defer func() {
		if err == nil {
			return
		}
		if rollbackErr := s.rollbackDeviceAndSession(ctx, savedDevice.ID, savedSession.ID); rollbackErr != nil {
			err = errors.Join(err, rollbackErr)
		}
	}()

	now := s.currentTime()
	if deviceName == "" {
		deviceName = account.DisplayName
		if deviceName == "" {
			deviceName = account.Username
		}
	}

	deviceID, err := newID("dev")
	if err != nil {
		err = fmt.Errorf("generate device ID: %w", err)
		return
	}

	sessionID, err := newID("sess")
	if err != nil {
		err = fmt.Errorf("generate session ID: %w", err)
		return
	}

	device := Device{
		ID:         deviceID,
		AccountID:  account.ID,
		SessionID:  sessionID,
		Name:       deviceName,
		Platform:   platform,
		Status:     DeviceStatusActive,
		PublicKey:  publicKey,
		PushToken:  pushToken,
		CreatedAt:  now,
		LastSeenAt: now,
	}

	savedDevice, saveErr := s.store.SaveDevice(ctx, device)
	if saveErr != nil {
		err = fmt.Errorf("save device for account %s: %w", account.ID, saveErr)
		return
	}

	session := Session{
		ID:             sessionID,
		AccountID:      account.ID,
		DeviceID:       savedDevice.ID,
		DeviceName:     deviceName,
		DevicePlatform: platform,
		Status:         SessionStatusActive,
		Current:        true,
		CreatedAt:      now,
		LastSeenAt:     now,
	}

	savedSession, saveErr = s.store.SaveSession(ctx, session)
	if saveErr != nil {
		err = fmt.Errorf("save session for account %s: %w", account.ID, saveErr)
		return
	}

	accessToken, err := randomToken(32)
	if err != nil {
		err = fmt.Errorf("generate access token for account %s: %w", account.ID, err)
		return
	}

	refreshToken, err := randomToken(32)
	if err != nil {
		err = fmt.Errorf("generate refresh token for account %s: %w", account.ID, err)
		return
	}

	account.LastAuthAt = now
	account.UpdatedAt = now
	if _, saveErr := s.store.SaveAccount(ctx, account); saveErr != nil {
		err = fmt.Errorf("update account %s after authentication: %w", account.ID, saveErr)
		return
	}

	result = LoginResult{
		Tokens: Tokens{
			AccessToken:      accessToken,
			RefreshToken:     refreshToken,
			TokenType:        "Bearer",
			ExpiresAt:        now.Add(s.accessTokenTTL),
			RefreshExpiresAt: now.Add(s.refreshTokenTTL),
		},
		Session: savedSession,
		Device:  savedDevice,
	}
	return
}

// rollbackDeviceAndSession deletes the session and device created by a failed login path.
//
// Missing rows are tolerated because the caller may already have removed one side of the
// pair during its own cleanup.
func (s *Service) rollbackDeviceAndSession(ctx context.Context, deviceID, sessionID string) error {
	var rollbackErr error

	if sessionID != "" {
		if err := s.store.DeleteSession(ctx, sessionID); err != nil && !errors.Is(err, ErrNotFound) {
			rollbackErr = errors.Join(
				rollbackErr,
				fmt.Errorf("delete session %s: %w", sessionID, err),
			)
		}
	}

	if deviceID != "" {
		if err := s.store.DeleteDevice(ctx, deviceID); err != nil && !errors.Is(err, ErrNotFound) {
			rollbackErr = errors.Join(
				rollbackErr,
				fmt.Errorf("delete device %s: %w", deviceID, err),
			)
		}
	}

	return rollbackErr
}

// selectLoginTargets chooses the available code delivery channels for an account.
//
// Email is preferred over SMS when both are present, but the caller can still request a
// specific channel or a manual bootstrap path.
func (s *Service) selectLoginTargets(account Account, delivery LoginDeliveryChannel) ([]LoginTarget, LoginDeliveryChannel, error) {
	available := make([]LoginTarget, 0, 2)

	if account.Email != "" {
		available = append(available, LoginTarget{
			Channel:         LoginDeliveryChannelEmail,
			DestinationMask: maskEmail(account.Email),
			Primary:         true,
			Verified:        true,
		})
	}

	if account.Phone != "" {
		available = append(available, LoginTarget{
			Channel:         LoginDeliveryChannelSMS,
			DestinationMask: maskPhone(account.Phone),
			Primary:         len(available) == 0,
			Verified:        true,
		})
	}

	if len(available) == 0 {
		return nil, LoginDeliveryChannelUnspecified, ErrInvalidInput
	}

	switch delivery {
	case LoginDeliveryChannelUnspecified:
		return available, available[0].Channel, nil
	case LoginDeliveryChannelEmail:
		if account.Email == "" {
			return nil, LoginDeliveryChannelUnspecified, ErrInvalidInput
		}
		return []LoginTarget{available[0]}, LoginDeliveryChannelEmail, nil
	case LoginDeliveryChannelSMS:
		if account.Phone == "" {
			return nil, LoginDeliveryChannelUnspecified, ErrInvalidInput
		}
		if account.Email != "" {
			return available[1:], LoginDeliveryChannelSMS, nil
		}
		return available[:1], LoginDeliveryChannelSMS, nil
	case LoginDeliveryChannelManual:
		return available, LoginDeliveryChannelManual, nil
	default:
		return nil, LoginDeliveryChannelUnspecified, ErrInvalidInput
	}
}

// hasContactInformation reports whether the account can receive a login challenge.
func hasContactInformation(account Account) bool {
	return account.Email != "" || account.Phone != ""
}

// trimmed removes leading and trailing whitespace from user-provided text.
func trimmed(value string) string {
	return strings.TrimSpace(value)
}
