package call

import (
	"context"
	"fmt"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// ConversationStore exposes the conversation reads needed by the call domain.
type ConversationStore interface {
	ConversationByID(ctx context.Context, conversationID string) (conversation.Conversation, error)
	ConversationMemberByConversationAndAccount(
		ctx context.Context,
		conversationID string,
		accountID string,
	) (conversation.ConversationMember, error)
	ConversationMembersByConversationID(ctx context.Context, conversationID string) ([]conversation.ConversationMember, error)
}

// Service owns call state and RTC signaling orchestration.
type Service struct {
	store         Store
	conversations ConversationStore
	runtime       Runtime
	settings      Settings
	rtc           RTCConfig
	now           func() time.Time
}

// Option configures a service instance.
type Option func(*Service)

// NewService constructs a call service.
func NewService(store Store, conversations ConversationStore, runtime Runtime, opts ...Option) (*Service, error) {
	if store == nil || conversations == nil || runtime == nil {
		return nil, ErrInvalidInput
	}

	service := &Service{
		store:         store,
		conversations: conversations,
		runtime:       runtime,
		settings:      DefaultSettings(),
		rtc:           RTCConfig{CredentialTTL: 15 * time.Minute},
		now:           func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	service.settings = service.settings.normalize()
	service.rtc = service.rtc.normalize()

	return service, nil
}

// WithNow overrides the service clock.
func WithNow(now func() time.Time) Option {
	return func(service *Service) {
		if service != nil && now != nil {
			service.now = now
		}
	}
}

// WithSettings overrides the call settings.
func WithSettings(settings Settings) Option {
	return func(service *Service) {
		if service != nil {
			service.settings = settings
		}
	}
}

// WithRTC overrides the RTC settings used for ICE config generation.
func WithRTC(cfg RTCConfig) Option {
	return func(service *Service) {
		if service != nil {
			service.rtc = cfg
		}
	}
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

func activeMember(member conversation.ConversationMember) bool {
	return member.LeftAt.IsZero() && !member.Banned
}

func (s *Service) runTx(ctx context.Context, fn func(Store) error) error {
	if err := s.validateContext(ctx, "call tx"); err != nil {
		return err
	}
	if fn == nil {
		return ErrInvalidInput
	}

	return s.store.WithinTx(ctx, fn)
}
