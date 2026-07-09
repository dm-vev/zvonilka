package teststore

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/search"
)

// New returns an in-memory search store for tests.
func New() search.Store {
	return &memoryStore{
		documentsByKey: make(map[string]search.Document),
	}
}

type memoryStore struct {
	mu             sync.RWMutex
	documentsByKey map[string]search.Document
}

// WithinTx executes the callback against a snapshot of the store.
func (s *memoryStore) WithinTx(_ context.Context, fn func(search.Store) error) error {
	if fn == nil {
		return search.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := s.cloneLocked()
	tx := &memoryStore{
		documentsByKey: snapshot.documentsByKey,
	}

	if err := fn(tx); err != nil {
		return err
	}

	s.documentsByKey = tx.documentsByKey
	return nil
}

func (s *memoryStore) UpsertDocument(_ context.Context, document search.Document) (search.Document, error) {
	if document.TargetID == "" {
		return search.Document{}, search.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stored, err := search.NormalizeDocument(document, time.Now().UTC())
	if err != nil {
		return search.Document{}, err
	}

	key := documentKey(stored.Scope, stored.TargetID)
	if previous, ok := s.documentsByKey[key]; ok && previous.CreatedAt.Before(stored.CreatedAt) {
		stored.CreatedAt = previous.CreatedAt
	}
	s.documentsByKey[key] = stored

	return cloneDocument(stored), nil
}

func (s *memoryStore) DeleteDocument(_ context.Context, scope search.SearchScope, targetID string) error {
	scope = search.NormalizeScope(scope)
	if scope == search.SearchScopeUnspecified || targetID == "" {
		return search.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := documentKey(scope, targetID)
	if _, ok := s.documentsByKey[key]; !ok {
		return search.ErrNotFound
	}
	delete(s.documentsByKey, key)

	return nil
}

func (s *memoryStore) DocumentByScopeAndTarget(_ context.Context, scope search.SearchScope, targetID string) (search.Document, error) {
	scope = search.NormalizeScope(scope)
	if scope == search.SearchScopeUnspecified || targetID == "" {
		return search.Document{}, search.ErrInvalidInput
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	key := documentKey(scope, targetID)
	document, ok := s.documentsByKey[key]
	if !ok {
		return search.Document{}, search.ErrNotFound
	}

	return cloneDocument(document), nil
}

func (s *memoryStore) SearchDocuments(_ context.Context, params search.SearchParams) (search.SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hits := make([]search.Hit, 0, len(s.documentsByKey))
	for _, document := range s.documentsByKey {
		if !matchesDocument(document, params) {
			continue
		}
		hits = append(hits, hitFromDocument(document))
	}
	totalMatches := len(hits)

	if params.Limit <= 0 {
		params.Limit = 25
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	end := params.Offset + params.Limit
	if params.Offset > len(hits) {
		hits = nil
	} else if end < len(hits) {
		hits = hits[params.Offset:end]
	} else {
		hits = hits[params.Offset:]
	}

	result := search.SearchResult{
		Hits:      hits,
		TotalSize: uint64(totalMatches),
	}
	if params.Offset+params.Limit < totalMatches {
		result.NextPageToken = strconv.Itoa(params.Offset + params.Limit)
	}

	return result, nil
}

func (s *memoryStore) cloneLocked() *memoryStore {
	clone := &memoryStore{
		documentsByKey: make(map[string]search.Document, len(s.documentsByKey)),
	}
	for key, document := range s.documentsByKey {
		clone.documentsByKey[key] = cloneDocument(document)
	}

	return clone
}

func documentKey(scope search.SearchScope, targetID string) string {
	return strings.TrimSpace(string(scope)) + ":" + strings.TrimSpace(targetID)
}
