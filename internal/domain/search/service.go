package search

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Service coordinates search index writes and query resolution.
type Service struct {
	store    Store
	now      func() time.Time
	settings Settings
}

// NewService constructs a search service.
func NewService(store Store, opts ...Option) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}

	service := &Service{
		store:    store,
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

// Search resolves search hits using the stored search documents.
func (s *Service) Search(ctx context.Context, params SearchParams) (SearchResult, error) {
	if err := s.validateContext(ctx, "search"); err != nil {
		return SearchResult{}, err
	}

	params.Query = normalizeQuery(params.Query)
	params.Scopes = normalizeScopes(params.Scopes)
	params.ConversationIDs = normalizeIDs(params.ConversationIDs)
	params.UserIDs = normalizeIDs(params.UserIDs)
	params.AccountKinds = normalizeAccountKinds(params.AccountKinds)
	params.Limit = s.limit(params.Limit)
	if params.Limit <= 0 {
		params.Limit = s.settings.DefaultLimit
	}

	if params.Query != "" && len(params.Query) < s.settings.MinQueryLength {
		return SearchResult{}, ErrInvalidInput
	}

	result, err := s.store.SearchDocuments(ctx, params)
	if err != nil {
		return SearchResult{}, fmt.Errorf("search documents for query %q: %w", params.Query, err)
	}

	if params.IncludeSnippets && s.settings.SnippetLength > 0 {
		for i := range result.Hits {
			result.Hits[i].Snippet = trimSnippet(result.Hits[i].Snippet, s.settings.SnippetLength)
		}
	}
	if !params.IncludeSnippets {
		for i := range result.Hits {
			result.Hits[i].Snippet = ""
		}
	}

	return result, nil
}

// UpsertDocument indexes one search document.
func (s *Service) UpsertDocument(ctx context.Context, document Document) (Document, error) {
	if err := s.validateContext(ctx, "upsert search document"); err != nil {
		return Document{}, err
	}

	now := s.currentTime()
	normalized, err := normalizeDocument(document, now)
	if err != nil {
		return Document{}, err
	}

	saved, err := s.store.UpsertDocument(ctx, normalized)
	if err != nil {
		return Document{}, fmt.Errorf("upsert search document %s: %w", normalized.ID, err)
	}

	return saved, nil
}

// DeleteDocument removes one indexed document.
func (s *Service) DeleteDocument(ctx context.Context, scope SearchScope, targetID string) error {
	if err := s.validateContext(ctx, "delete search document"); err != nil {
		return err
	}

	scope = normalizeScope(scope)
	targetID = strings.TrimSpace(targetID)
	if scope == SearchScopeUnspecified || targetID == "" {
		return ErrInvalidInput
	}

	if err := s.store.DeleteDocument(ctx, scope, targetID); err != nil {
		return fmt.Errorf("delete search document %s:%s: %w", scope, targetID, err)
	}

	return nil
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

func (s *Service) limit(limit int) int {
	if limit <= 0 {
		return s.settings.DefaultLimit
	}
	if s.settings.MaxLimit > 0 && limit > s.settings.MaxLimit {
		return s.settings.MaxLimit
	}

	return limit
}

func trimSnippet(snippet string, length int) string {
	snippet = strings.TrimSpace(snippet)
	if length <= 0 || snippet == "" {
		return snippet
	}

	runes := []rune(snippet)
	if len(runes) <= length {
		return snippet
	}

	return strings.TrimSpace(string(runes[:length]))
}
