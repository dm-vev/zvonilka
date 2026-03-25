package presence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

type identityReader interface {
	AccountByID(ctx context.Context, accountID string) (identity.Account, error)
	SessionsByAccountID(ctx context.Context, accountID string) ([]identity.Session, error)
	DevicesByAccountID(ctx context.Context, accountID string) ([]identity.Device, error)
}

// Service resolves explicit presence and derived last-seen values.
type Service struct {
	store    Store
	identity identityReader
	now      func() time.Time
	settings Settings
}

// Option configures a service instance.
type Option func(*Service)

// NewService constructs a presence service.
func NewService(store Store, identityStore identityReader, opts ...Option) (*Service, error) {
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

// WithNow overrides the service clock.
func WithNow(now func() time.Time) Option {
	return func(service *Service) {
		if service != nil && now != nil {
			service.now = now
		}
	}
}

// WithSettings overrides the presence derivation settings.
func WithSettings(settings Settings) Option {
	return func(service *Service) {
		if service != nil {
			service.settings = settings.normalize()
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

// SetPresence persists explicit presence state for an account.
func (s *Service) SetPresence(ctx context.Context, params SetParams) (Presence, error) {
	if err := s.validateContext(ctx, "set presence"); err != nil {
		return Presence{}, err
	}

	params.AccountID = strings.TrimSpace(params.AccountID)
	params.CustomStatus = strings.TrimSpace(params.CustomStatus)
	if params.AccountID == "" {
		return Presence{}, ErrInvalidInput
	}
	if params.HideLastSeenFor < 0 {
		return Presence{}, ErrInvalidInput
	}

	_, err := s.activeAccount(ctx, params.AccountID)
	if err != nil {
		return Presence{}, err
	}

	existing, err := s.store.PresenceByAccountID(ctx, params.AccountID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Presence{}, fmt.Errorf("load presence for account %s: %w", params.AccountID, err)
	}

	recordedAt := params.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = s.currentTime()
	}

	next := existing
	next.AccountID = params.AccountID
	next.CustomStatus = params.CustomStatus
	next.UpdatedAt = recordedAt
	if params.HideLastSeenFor > 0 {
		next.HiddenUntil = recordedAt.Add(params.HideLastSeenFor)
	} else {
		next.HiddenUntil = time.Time{}
	}

	normalizedState, err := normalizePresenceState(params.State)
	if err != nil {
		return Presence{}, err
	}
	if normalizedState == PresenceStateUnspecified && existing.State != PresenceStateUnspecified {
		normalizedState = existing.State
	}
	if normalizedState == PresenceStateUnspecified {
		normalizedState = PresenceStateOffline
	}
	next.State = normalizedState

	saved, err := s.store.SavePresence(ctx, next)
	if err != nil {
		return Presence{}, fmt.Errorf("save presence for account %s: %w", params.AccountID, err)
	}

	return saved, nil
}

// GetPresence resolves the visible presence snapshot for a viewer and subject.
func (s *Service) GetPresence(ctx context.Context, params GetParams) (Snapshot, error) {
	if err := s.validateContext(ctx, "get presence"); err != nil {
		return Snapshot{}, err
	}

	params.AccountID = strings.TrimSpace(params.AccountID)
	params.ViewerAccountID = strings.TrimSpace(params.ViewerAccountID)
	if params.AccountID == "" {
		return Snapshot{}, ErrInvalidInput
	}

	account, err := s.activeAccount(ctx, params.AccountID)
	if err != nil {
		return Snapshot{}, err
	}

	record, err := s.store.PresenceByAccountID(ctx, params.AccountID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Snapshot{}, fmt.Errorf("load presence for account %s: %w", params.AccountID, err)
	}

	lastSeenAt, err := s.lastSeenAt(ctx, account)
	if err != nil {
		return Snapshot{}, err
	}

	snapshot := s.snapshot(params.ViewerAccountID, record, lastSeenAt)
	snapshot.AccountID = params.AccountID
	return snapshot, nil
}

// ListPresence resolves presence snapshots for a set of accounts.
func (s *Service) ListPresence(ctx context.Context, accountIDs []string, viewerAccountID string) (map[string]Snapshot, error) {
	if err := s.validateContext(ctx, "list presence"); err != nil {
		return nil, err
	}

	ids := NormalizeIDs(accountIDs)
	if len(ids) == 0 {
		return make(map[string]Snapshot), nil
	}

	snapshots := make(map[string]Snapshot, len(ids))
	for _, accountID := range ids {
		snapshot, err := s.GetPresence(ctx, GetParams{
			AccountID:       accountID,
			ViewerAccountID: viewerAccountID,
		})
		if err != nil {
			return nil, fmt.Errorf("resolve presence for account %s: %w", accountID, err)
		}
		snapshots[accountID] = snapshot
	}

	return snapshots, nil
}
