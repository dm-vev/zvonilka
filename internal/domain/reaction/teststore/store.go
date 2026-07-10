package teststore

import (
	"context"
	"sync"

	"github.com/dm-vev/zvonilka/internal/domain/reaction"
)

// Store is a concurrency-safe in-memory reaction catalog for tests.
type Store struct {
	mu          sync.RWMutex
	definitions map[string]reaction.Definition
}

// New constructs an empty in-memory reaction catalog.
func New() *Store {
	return &Store{definitions: make(map[string]reaction.Definition)}
}

// WithinTx executes fn while holding the catalog write lock.
func (s *Store) WithinTx(ctx context.Context, fn func(reaction.Store) error) error {
	if ctx == nil || fn == nil {
		return reaction.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn(&transaction{store: s})
}

// SaveDefinition stores one reaction definition.
func (s *Store) SaveDefinition(ctx context.Context, definition reaction.Definition) (reaction.Definition, error) {
	if err := validateContext(ctx); err != nil {
		return reaction.Definition{}, err
	}
	if definition.Emoji == "" {
		return reaction.Definition{}, reaction.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.definitions[definition.Emoji] = definition
	return definition, nil
}

// DefinitionByEmoji loads one reaction definition.
func (s *Store) DefinitionByEmoji(ctx context.Context, emoji string) (reaction.Definition, error) {
	if err := validateContext(ctx); err != nil {
		return reaction.Definition{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	definition, ok := s.definitions[emoji]
	if !ok {
		return reaction.Definition{}, reaction.ErrNotFound
	}
	return definition, nil
}

// Definitions returns all definitions in insertion-independent order.
func (s *Store) Definitions(ctx context.Context) ([]reaction.Definition, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	definitions := make([]reaction.Definition, 0, len(s.definitions))
	for _, definition := range s.definitions {
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

type transaction struct {
	store *Store
}

func (t *transaction) WithinTx(ctx context.Context, fn func(reaction.Store) error) error {
	if ctx == nil || fn == nil {
		return reaction.ErrInvalidInput
	}
	return fn(t)
}

func (t *transaction) SaveDefinition(ctx context.Context, definition reaction.Definition) (reaction.Definition, error) {
	if err := validateContext(ctx); err != nil {
		return reaction.Definition{}, err
	}
	if definition.Emoji == "" {
		return reaction.Definition{}, reaction.ErrInvalidInput
	}
	t.store.definitions[definition.Emoji] = definition
	return definition, nil
}

func (t *transaction) DefinitionByEmoji(ctx context.Context, emoji string) (reaction.Definition, error) {
	if err := validateContext(ctx); err != nil {
		return reaction.Definition{}, err
	}
	definition, ok := t.store.definitions[emoji]
	if !ok {
		return reaction.Definition{}, reaction.ErrNotFound
	}
	return definition, nil
}

func (t *transaction) Definitions(ctx context.Context) ([]reaction.Definition, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}
	definitions := make([]reaction.Definition, 0, len(t.store.definitions))
	for _, definition := range t.store.definitions {
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func validateContext(ctx context.Context) error {
	if ctx == nil {
		return reaction.ErrInvalidInput
	}
	return ctx.Err()
}

var _ reaction.Store = (*Store)(nil)
var _ reaction.Store = (*transaction)(nil)
