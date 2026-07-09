package notification

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// Service owns notification preferences, overrides, tokens, and delivery hints.
type Service struct {
	store    Store
	identity identity.Store
	now      func() time.Time
	settings Settings
}

// NewService constructs a notification service.
func NewService(store Store, identityStore identity.Store, opts ...Option) (*Service, error) {
	if store == nil || identityStore == nil {
		return nil, ErrInvalidInput
	}

	service := &Service{
		store:    store,
		identity: identityStore,
		now:      func() time.Time { return time.Now().UTC() },
		settings: DefaultSettings(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	service.settings = service.settings.normalize()

	return service, nil
}

// SetPreference persists account-level notification preferences.
func (s *Service) SetPreference(ctx context.Context, params SetPreferenceParams) (Preference, error) {
	if err := s.validateContext(ctx, "set notification preference"); err != nil {
		return Preference{}, err
	}

	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.AccountID == "" {
		return Preference{}, ErrInvalidInput
	}

	if _, err := s.activeAccount(ctx, params.AccountID); err != nil {
		return Preference{}, err
	}

	preference := Preference{
		AccountID:      params.AccountID,
		Enabled:        params.Enabled,
		DirectEnabled:  params.DirectEnabled,
		GroupEnabled:   params.GroupEnabled,
		ChannelEnabled: params.ChannelEnabled,
		MentionEnabled: params.MentionEnabled,
		ReplyEnabled:   params.ReplyEnabled,
		QuietHours:     params.QuietHours,
		MutedUntil:     params.MutedUntil,
		UpdatedAt:      params.UpdatedAt,
	}
	if preference.UpdatedAt.IsZero() {
		preference.UpdatedAt = s.currentTime()
	}

	preference, err := preference.normalize(s.currentTime())
	if err != nil {
		return Preference{}, err
	}

	saved, err := s.store.SavePreference(ctx, preference)
	if err != nil {
		return Preference{}, fmt.Errorf("save notification preference for account %s: %w", params.AccountID, err)
	}

	return saved, nil
}

// PreferenceByAccountID resolves the effective preference for an account.
func (s *Service) PreferenceByAccountID(ctx context.Context, accountID string) (Preference, error) {
	if err := s.validateContext(ctx, "load notification preference"); err != nil {
		return Preference{}, err
	}

	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return Preference{}, ErrInvalidInput
	}

	if _, err := s.activeAccount(ctx, accountID); err != nil {
		return Preference{}, err
	}

	preference, err := s.store.PreferenceByAccountID(ctx, accountID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return defaultPreference(accountID, s.currentTime()), nil
		}

		return Preference{}, fmt.Errorf("load notification preference for account %s: %w", accountID, err)
	}

	return preference, nil
}

// SetConversationOverride persists a per-chat notification override.
func (s *Service) SetConversationOverride(ctx context.Context, params SetOverrideParams) (ConversationOverride, error) {
	if err := s.validateContext(ctx, "set notification override"); err != nil {
		return ConversationOverride{}, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.ConversationID == "" || params.AccountID == "" {
		return ConversationOverride{}, ErrInvalidInput
	}
	if _, err := s.activeAccount(ctx, params.AccountID); err != nil {
		return ConversationOverride{}, err
	}

	override := ConversationOverride{
		ConversationID: params.ConversationID,
		AccountID:      params.AccountID,
		Muted:          params.Muted,
		MentionsOnly:   params.MentionsOnly,
		MutedUntil:     params.MutedUntil,
		UpdatedAt:      params.UpdatedAt,
	}
	if override.UpdatedAt.IsZero() {
		override.UpdatedAt = s.currentTime()
	}

	override, err := override.normalize(s.currentTime())
	if err != nil {
		return ConversationOverride{}, err
	}

	saved, err := s.store.SaveOverride(ctx, override)
	if err != nil {
		return ConversationOverride{}, fmt.Errorf(
			"save notification override for conversation %s and account %s: %w",
			params.ConversationID,
			params.AccountID,
			err,
		)
	}

	return saved, nil
}

// ConversationOverrideByConversationAndAccount resolves the effective per-chat override.
func (s *Service) ConversationOverrideByConversationAndAccount(
	ctx context.Context,
	conversationID string,
	accountID string,
) (ConversationOverride, error) {
	if err := s.validateContext(ctx, "load notification override"); err != nil {
		return ConversationOverride{}, err
	}

	conversationID = strings.TrimSpace(conversationID)
	accountID = strings.TrimSpace(accountID)
	if conversationID == "" || accountID == "" {
		return ConversationOverride{}, ErrInvalidInput
	}

	override, err := s.store.OverrideByConversationAndAccount(ctx, conversationID, accountID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ConversationOverride{
				ConversationID: conversationID,
				AccountID:      accountID,
			}, nil
		}

		return ConversationOverride{}, fmt.Errorf(
			"load notification override for conversation %s and account %s: %w",
			conversationID,
			accountID,
			err,
		)
	}

	return override, nil
}

// RegisterPushToken persists a device push token registration.
func (s *Service) RegisterPushToken(ctx context.Context, params RegisterPushTokenParams) (PushToken, error) {
	if err := s.validateContext(ctx, "register push token"); err != nil {
		return PushToken{}, err
	}

	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	params.Provider = strings.TrimSpace(params.Provider)
	params.Token = strings.TrimSpace(params.Token)
	if params.AccountID == "" || params.DeviceID == "" || params.Provider == "" || params.Token == "" {
		return PushToken{}, ErrInvalidInput
	}
	if _, err := s.activeAccount(ctx, params.AccountID); err != nil {
		return PushToken{}, err
	}

	tokenID, err := newID("push")
	if err != nil {
		return PushToken{}, fmt.Errorf("generate push token id: %w", err)
	}

	token := PushToken{
		ID:        tokenID,
		AccountID: params.AccountID,
		DeviceID:  params.DeviceID,
		Provider:  params.Provider,
		Token:     params.Token,
		Platform:  params.Platform,
		Enabled:   true,
		CreatedAt: params.CreatedAt,
		UpdatedAt: params.CreatedAt,
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = s.currentTime()
		token.UpdatedAt = token.CreatedAt
	}

	token, err = token.normalize(s.currentTime())
	if err != nil {
		return PushToken{}, err
	}

	saved, err := s.store.SavePushToken(ctx, token)
	if err != nil {
		return PushToken{}, fmt.Errorf("save push token for account %s and device %s: %w", params.AccountID, params.DeviceID, err)
	}

	return saved, nil
}

// PushTokensByAccountID lists active push tokens for an account.
func (s *Service) PushTokensByAccountID(ctx context.Context, accountID string) ([]PushToken, error) {
	if err := s.validateContext(ctx, "list push tokens"); err != nil {
		return nil, err
	}

	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, ErrInvalidInput
	}

	tokens, err := s.store.PushTokensByAccountID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list push tokens for account %s: %w", accountID, err)
	}

	return tokens, nil
}

// PushTokenByID resolves one active push token by ID.
func (s *Service) PushTokenByID(ctx context.Context, tokenID string) (PushToken, error) {
	if err := s.validateContext(ctx, "load push token"); err != nil {
		return PushToken{}, err
	}

	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return PushToken{}, ErrInvalidInput
	}

	token, err := s.store.PushTokenByID(ctx, tokenID)
	if err != nil {
		return PushToken{}, fmt.Errorf("load push token %s: %w", tokenID, err)
	}
	if !token.Enabled || !token.RevokedAt.IsZero() {
		return PushToken{}, ErrNotFound
	}

	return token, nil
}

// RevokePushToken revokes a push token by ID.
func (s *Service) RevokePushToken(ctx context.Context, params RevokePushTokenParams) (PushToken, error) {
	if err := s.validateContext(ctx, "revoke push token"); err != nil {
		return PushToken{}, err
	}

	params.TokenID = strings.TrimSpace(params.TokenID)
	if params.TokenID == "" {
		return PushToken{}, ErrInvalidInput
	}

	token, err := s.store.PushTokenByID(ctx, params.TokenID)
	if err != nil {
		return PushToken{}, fmt.Errorf("load push token %s: %w", params.TokenID, err)
	}

	token.Enabled = false
	token.RevokedAt = params.RevokedAt
	if token.RevokedAt.IsZero() {
		token.RevokedAt = s.currentTime()
	}
	token.UpdatedAt = token.RevokedAt

	saved, err := s.store.SavePushToken(ctx, token)
	if err != nil {
		return PushToken{}, fmt.Errorf("revoke push token %s: %w", params.TokenID, err)
	}

	return saved, nil
}

// QueueDelivery persists a delivery hint.
func (s *Service) QueueDelivery(ctx context.Context, params QueueDeliveryParams) (Delivery, error) {
	if err := s.validateContext(ctx, "queue delivery"); err != nil {
		return Delivery{}, err
	}

	now := s.currentTime()
	delivery := Delivery{
		ID:             "",
		DedupKey:       params.DedupKey,
		EventID:        params.EventID,
		ConversationID: params.ConversationID,
		MessageID:      params.MessageID,
		AccountID:      params.AccountID,
		DeviceID:       params.DeviceID,
		PushTokenID:    params.PushTokenID,
		Kind:           params.Kind,
		Reason:         params.Reason,
		Mode:           params.Mode,
		State:          params.State,
		Priority:       params.Priority,
		Attempts:       params.Attempts,
		NextAttemptAt:  params.NextAttemptAt,
		CreatedAt:      params.CreatedAt,
		UpdatedAt:      params.CreatedAt,
	}
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = now
		delivery.UpdatedAt = delivery.CreatedAt
	}
	if delivery.NextAttemptAt.IsZero() {
		delivery.NextAttemptAt = delivery.CreatedAt
	}
	if delivery.ID == "" {
		delivery.ID = delivery.DedupKey
	}

	delivery, err := delivery.normalize(now)
	if err != nil {
		return Delivery{}, err
	}

	saved, err := s.store.SaveDelivery(ctx, delivery)
	if err != nil {
		return Delivery{}, fmt.Errorf("save delivery %s: %w", delivery.DedupKey, err)
	}

	return saved, nil
}

// ClaimDeliveries acquires a lease on queued deliveries that are ready for execution.
func (s *Service) ClaimDeliveries(ctx context.Context, params ClaimDeliveriesParams) ([]Delivery, error) {
	if err := s.validateContext(ctx, "claim deliveries"); err != nil {
		return nil, err
	}
	if params.Limit <= 0 {
		return nil, nil
	}

	now := s.currentTime()
	if params.Before.IsZero() {
		params.Before = now
	}
	if params.LeaseDuration <= 0 {
		params.LeaseDuration = s.settings.DeliveryLeaseTTL
	}
	if params.LeaseDuration <= 0 {
		return nil, ErrInvalidInput
	}
	if strings.TrimSpace(params.LeaseToken) == "" {
		leaseToken, err := newID("lease")
		if err != nil {
			return nil, fmt.Errorf("generate delivery lease token: %w", err)
		}
		params.LeaseToken = leaseToken
	}

	deliveries, err := s.store.ClaimDeliveries(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("claim deliveries: %w", err)
	}

	return deliveries, nil
}

// MarkDeliveryDelivered records a successful delivery attempt.
func (s *Service) MarkDeliveryDelivered(ctx context.Context, params MarkDeliveryDeliveredParams) (Delivery, error) {
	if err := s.validateContext(ctx, "mark delivery delivered"); err != nil {
		return Delivery{}, err
	}

	params.DeliveryID = strings.TrimSpace(params.DeliveryID)
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	if params.DeliveryID == "" || params.LeaseToken == "" {
		return Delivery{}, ErrInvalidInput
	}
	if params.DeliveredAt.IsZero() {
		params.DeliveredAt = s.currentTime()
	}

	delivery, err := s.store.MarkDeliveryDelivered(ctx, params)
	if err != nil {
		return Delivery{}, fmt.Errorf("mark delivery %s delivered: %w", params.DeliveryID, err)
	}

	return delivery, nil
}

// FailDelivery records a terminal delivery failure.
func (s *Service) FailDelivery(ctx context.Context, params FailDeliveryParams) (Delivery, error) {
	if err := s.validateContext(ctx, "fail delivery"); err != nil {
		return Delivery{}, err
	}

	params.DeliveryID = strings.TrimSpace(params.DeliveryID)
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	params.LastError = strings.TrimSpace(params.LastError)
	if params.DeliveryID == "" || params.LeaseToken == "" || params.LastError == "" {
		return Delivery{}, ErrInvalidInput
	}
	if params.FailedAt.IsZero() {
		params.FailedAt = s.currentTime()
	}

	delivery, err := s.store.FailDelivery(ctx, params)
	if err != nil {
		return Delivery{}, fmt.Errorf("fail delivery %s: %w", params.DeliveryID, err)
	}

	return delivery, nil
}

// RetryDelivery schedules a retry for a delivery hint.
func (s *Service) RetryDelivery(ctx context.Context, params RetryDeliveryParams) (Delivery, error) {
	if err := s.validateContext(ctx, "retry delivery"); err != nil {
		return Delivery{}, err
	}

	params.DeliveryID = strings.TrimSpace(params.DeliveryID)
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	params.LastError = strings.TrimSpace(params.LastError)
	if params.DeliveryID == "" || params.LastError == "" {
		return Delivery{}, ErrInvalidInput
	}

	delivery, err := s.store.DeliveryByID(ctx, params.DeliveryID)
	if err != nil {
		return Delivery{}, fmt.Errorf("load delivery %s: %w", params.DeliveryID, err)
	}

	switch delivery.State {
	case DeliveryStateDelivered, DeliveryStateSuppressed:
		return Delivery{}, ErrConflict
	}

	retryAt := params.RetryAt
	now := s.currentTime()
	if params.AttemptedAt.IsZero() {
		params.AttemptedAt = now
	}
	if retryAt.IsZero() {
		retryAt = now.Add(s.retryBackoff(delivery.Attempts + 1))
	}
	if retryAt.Before(params.AttemptedAt) {
		retryAt = params.AttemptedAt
	}
	params.RetryAt = retryAt
	params.MaxAttempts = s.settings.MaxAttempts

	saved, err := s.store.RetryDelivery(ctx, RetryDeliveryParams{
		DeliveryID:  params.DeliveryID,
		LeaseToken:  params.LeaseToken,
		LastError:   params.LastError,
		RetryAt:     params.RetryAt,
		MaxAttempts: params.MaxAttempts,
		AttemptedAt: params.AttemptedAt,
	})
	if err != nil {
		return Delivery{}, fmt.Errorf("retry delivery %s: %w", params.DeliveryID, err)
	}

	return saved, nil
}

// DeliveryByID resolves a delivery hint by primary key.
func (s *Service) DeliveryByID(ctx context.Context, deliveryID string) (Delivery, error) {
	if err := s.validateContext(ctx, "load delivery"); err != nil {
		return Delivery{}, err
	}

	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return Delivery{}, ErrInvalidInput
	}

	delivery, err := s.store.DeliveryByID(ctx, deliveryID)
	if err != nil {
		return Delivery{}, fmt.Errorf("load delivery %s: %w", deliveryID, err)
	}

	return delivery, nil
}

// DeliveriesDue returns queued deliveries that are ready for another attempt.
func (s *Service) DeliveriesDue(ctx context.Context, before time.Time, limit int) ([]Delivery, error) {
	if err := s.validateContext(ctx, "list deliveries due"); err != nil {
		return nil, err
	}

	deliveries, err := s.store.DeliveriesDue(ctx, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list deliveries due: %w", err)
	}

	return deliveries, nil
}

// SaveWorkerCursor advances the worker cursor.
func (s *Service) SaveWorkerCursor(ctx context.Context, params SaveWorkerCursorParams) (WorkerCursor, error) {
	if err := s.validateContext(ctx, "save worker cursor"); err != nil {
		return WorkerCursor{}, err
	}

	cursor := WorkerCursor{
		Name:         params.Name,
		LastSequence: params.LastSequence,
		UpdatedAt:    params.UpdatedAt,
	}
	if cursor.UpdatedAt.IsZero() {
		cursor.UpdatedAt = s.currentTime()
	}

	cursor, err := cursor.normalize(s.currentTime())
	if err != nil {
		return WorkerCursor{}, err
	}

	saved, err := s.store.SaveWorkerCursor(ctx, cursor)
	if err != nil {
		return WorkerCursor{}, fmt.Errorf("save worker cursor %s: %w", cursor.Name, err)
	}

	return saved, nil
}

// WorkerCursorByName resolves the worker cursor by name.
func (s *Service) WorkerCursorByName(ctx context.Context, name string) (WorkerCursor, error) {
	if err := s.validateContext(ctx, "load worker cursor"); err != nil {
		return WorkerCursor{}, err
	}

	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return WorkerCursor{}, ErrInvalidInput
	}

	cursor, err := s.store.WorkerCursorByName(ctx, name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return WorkerCursor{Name: name, UpdatedAt: s.currentTime()}, nil
		}

		return WorkerCursor{}, fmt.Errorf("load worker cursor %s: %w", name, err)
	}

	return cursor, nil
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

func (s *Service) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}

	return s.now().UTC()
}

func (s *Service) activeAccount(ctx context.Context, accountID string) (identity.Account, error) {
	account, err := s.identity.AccountByID(ctx, accountID)
	if err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			return identity.Account{}, ErrNotFound
		}

		return identity.Account{}, fmt.Errorf("load account %s: %w", accountID, err)
	}
	if account.Status != identity.AccountStatusActive {
		return identity.Account{}, ErrNotFound
	}

	return account, nil
}

func (s *Service) retryBackoff(attempts int) time.Duration {
	settings := s.settings.normalize()
	if attempts <= 1 {
		return settings.RetryInitialBackoff
	}

	backoff := settings.RetryInitialBackoff
	for attempt := 1; attempt < attempts; attempt++ {
		if backoff >= settings.RetryMaxBackoff {
			return settings.RetryMaxBackoff
		}
		if backoff > settings.RetryMaxBackoff/2 {
			return settings.RetryMaxBackoff
		}
		backoff *= 2
	}
	if backoff > settings.RetryMaxBackoff {
		return settings.RetryMaxBackoff
	}

	return backoff
}
