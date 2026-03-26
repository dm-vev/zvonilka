package bot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/dm-vev/zvonilka/internal/domain/media"
)

type mediaReader interface {
	MediaAssetByID(ctx context.Context, mediaID string) (media.MediaAsset, error)
}

// Service coordinates bot-facing HTTP semantics over messenger domains.
type Service struct {
	store          Store
	identity       *identity.Service
	conversations  *conversation.Service
	conversationDB conversation.Store
	media          mediaReader
	settings       Settings
	now            func() time.Time
}

// NewService constructs a bot service backed by the provided stores.
func NewService(
	store Store,
	identityService *identity.Service,
	conversationService *conversation.Service,
	conversationStore conversation.Store,
	mediaStore mediaReader,
	opts ...Option,
) (*Service, error) {
	if store == nil || identityService == nil || conversationService == nil || conversationStore == nil || mediaStore == nil {
		return nil, ErrInvalidInput
	}

	service := &Service{
		store:          store,
		identity:       identityService,
		conversations:  conversationService,
		conversationDB: conversationStore,
		media:          mediaStore,
		settings:       DefaultSettings(),
		now:            func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}

	return service, nil
}

func (s *Service) currentTime() time.Time {
	if s == nil || s.now == nil {
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

func (s *Service) runTx(ctx context.Context, fn func(Store) error) error {
	if err := s.validateContext(ctx, "bot tx"); err != nil {
		return err
	}
	if fn == nil {
		return ErrInvalidInput
	}

	return s.store.WithinTx(ctx, fn)
}

func clampLimit(limit int, fallback int, maxValue int) int {
	if limit <= 0 {
		limit = fallback
	}
	if maxValue > 0 && limit > maxValue {
		return maxValue
	}

	return limit
}

func uniqueUpdateTypes(values []UpdateType) []UpdateType {
	seen := make(map[UpdateType]struct{}, len(values))
	result := make([]UpdateType, 0, len(values))
	for _, value := range values {
		value = normalizeUpdateType(value)
		if value == UpdateTypeUnspecified {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}

func normalizeUpdateType(value UpdateType) UpdateType {
	switch value {
	case UpdateTypeMessage, UpdateTypeEditedMessage, UpdateTypeChannelPost, UpdateTypeEditedChannelPost, UpdateTypeCallbackQuery, UpdateTypeInlineQuery, UpdateTypeChosenInlineResult, UpdateTypeChatMember, UpdateTypeMyChatMember:
		return value
	default:
		return UpdateTypeUnspecified
	}
}

func webhookInfo(webhook Webhook, pendingCount int) WebhookInfo {
	info := WebhookInfo{
		URL:                  webhook.URL,
		HasCustomCertificate: false,
		PendingUpdateCount:   pendingCount,
		MaxConnections:       webhook.MaxConnections,
	}
	if len(webhook.AllowedUpdates) > 0 {
		info.AllowedUpdates = make([]string, 0, len(webhook.AllowedUpdates))
		for _, updateType := range webhook.AllowedUpdates {
			info.AllowedUpdates = append(info.AllowedUpdates, string(updateType))
		}
	}
	if !webhook.LastErrorAt.IsZero() {
		info.LastErrorDate = webhook.LastErrorAt.UTC().Unix()
	}
	info.LastErrorMessage = webhook.LastErrorMessage

	return info
}

func mapConversationError(err error) error {
	switch {
	case errors.Is(err, conversation.ErrInvalidInput):
		return ErrInvalidInput
	case errors.Is(err, conversation.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, conversation.ErrConflict):
		return ErrConflict
	case errors.Is(err, conversation.ErrForbidden):
		return ErrForbidden
	default:
		return err
	}
}

func mapIdentityError(err error) error {
	switch {
	case errors.Is(err, identity.ErrInvalidInput):
		return ErrInvalidInput
	case errors.Is(err, identity.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, identity.ErrConflict):
		return ErrConflict
	case errors.Is(err, identity.ErrForbidden):
		return ErrForbidden
	default:
		return err
	}
}
