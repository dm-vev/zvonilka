package identity

import (
	"context"
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
	now             func() time.Time
	joinRequestTTL  time.Duration
	challengeTTL    time.Duration
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

// NewService constructs a service backed by the provided store and sender.
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

func (s *Service) currentTime() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}

	return s.now().UTC()
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

func (s *Service) normalizeAccountInput(username, email, phone string) (string, string, string) {
	return normalizeUsername(username), normalizeEmail(email), normalizePhone(phone)
}

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

func (s *Service) issueSession(
	ctx context.Context,
	account Account,
	deviceName string,
	platform DevicePlatform,
	publicKey string,
	pushToken string,
) (LoginResult, error) {
	if publicKey == "" {
		return LoginResult{}, ErrInvalidInput
	}
	if account.Status != AccountStatusActive {
		return LoginResult{}, ErrForbidden
	}

	now := s.currentTime()
	if deviceName == "" {
		deviceName = account.DisplayName
		if deviceName == "" {
			deviceName = account.Username
		}
	}

	deviceID, err := newID("dev")
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate device ID: %w", err)
	}

	sessionID, err := newID("sess")
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate session ID: %w", err)
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

	savedDevice, err := s.store.SaveDevice(ctx, device)
	if err != nil {
		return LoginResult{}, fmt.Errorf("save device for account %s: %w", account.ID, err)
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

	savedSession, err := s.store.SaveSession(ctx, session)
	if err != nil {
		return LoginResult{}, fmt.Errorf("save session for account %s: %w", account.ID, err)
	}

	account.LastAuthAt = now
	account.UpdatedAt = now
	if _, err := s.store.SaveAccount(ctx, account); err != nil {
		return LoginResult{}, fmt.Errorf("update account %s after authentication: %w", account.ID, err)
	}

	accessToken, err := randomToken(32)
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate access token for account %s: %w", account.ID, err)
	}

	refreshToken, err := randomToken(32)
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate refresh token for account %s: %w", account.ID, err)
	}

	return LoginResult{
		Tokens: Tokens{
			AccessToken:      accessToken,
			RefreshToken:     refreshToken,
			TokenType:        "Bearer",
			ExpiresAt:        now.Add(s.accessTokenTTL),
			RefreshExpiresAt: now.Add(s.refreshTokenTTL),
		},
		Session: savedSession,
		Device:  savedDevice,
	}, nil
}

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

func hasContactInformation(account Account) bool {
	return account.Email != "" || account.Phone != ""
}

func trimmed(value string) string {
	return strings.TrimSpace(value)
}
