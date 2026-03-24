package identity

import (
	"context"
	"fmt"
	"strings"
	"time"
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
	loginCodeLength int
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
		store:       store,
		sender:      sender,
		now:         func() time.Time { return time.Now().UTC() },
		idempotency: newIdempotencyCache(),
	}

	defaults := DefaultSettings()
	service.joinRequestTTL = defaults.JoinRequestTTL
	service.challengeTTL = defaults.ChallengeTTL
	service.accessTokenTTL = defaults.AccessTokenTTL
	service.refreshTokenTTL = defaults.RefreshTokenTTL
	service.loginCodeLength = defaults.LoginCodeLength

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

// lockAccount acquires the account boundary and reloads the latest row snapshot.
//
// The account row is locked first so account-wide revoke, login, and device flows
// serialize on the same boundary without writing any unrelated account fields.
func (s *Service) lockAccount(ctx context.Context, store Store, accountID string) (Account, error) {
	if accountID == "" {
		return Account{}, ErrInvalidInput
	}

	if err := store.LockAccount(ctx, accountID); err != nil {
		return Account{}, fmt.Errorf("lock account %s: %w", accountID, err)
	}

	account, err := store.AccountByID(ctx, accountID)
	if err != nil {
		return Account{}, fmt.Errorf("reload account %s after lock: %w", accountID, err)
	}

	return account, nil
}

// touchAccount persists the current account snapshot after the account boundary is held.
//
// The helper is used after lockAccount has reloaded the latest row so benign account
// metadata updates can be written without clobbering concurrent edits.
func (s *Service) touchAccount(ctx context.Context, store Store, account Account) error {
	if account.ID == "" {
		return ErrInvalidInput
	}

	if _, err := store.SaveAccount(ctx, account); err != nil {
		return fmt.Errorf("touch account %s: %w", account.ID, err)
	}

	return nil
}

// issueSession creates a device/session pair and returns bearer tokens for the account.
//
// The caller is expected to run the helper inside a store transaction when the
// provider supports it so device and session writes commit atomically after the
// account boundary has been locked and reloaded.
func (s *Service) issueSession(
	ctx context.Context,
	store Store,
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
	if account.ID == "" {
		err = ErrInvalidInput
		return
	}

	accountID := account.ID
	account, err = s.lockAccount(ctx, store, accountID)
	if err != nil {
		err = fmt.Errorf("load account %s before session issuance: %w", accountID, err)
		return
	}
	if account.Status != AccountStatusActive {
		err = ErrForbidden
		return
	}

	now := s.currentTime()
	account.LastAuthAt = now
	account.UpdatedAt = now
	if err = s.touchAccount(ctx, store, account); err != nil {
		err = fmt.Errorf("update account %s before session issuance: %w", account.ID, err)
		return
	}
	if deviceName == "" {
		deviceName = account.DisplayName
		if deviceName == "" {
			deviceName = account.Username
		}
	}

	if err = s.touchAccount(ctx, store, account); err != nil {
		err = fmt.Errorf("update account %s before session issuance: %w", account.ID, err)
		return
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

	savedDevice, saveErr := store.SaveDevice(ctx, device)
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

	savedSession, saveErr := store.SaveSession(ctx, session)
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
