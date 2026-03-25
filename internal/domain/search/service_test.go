package search_test

import (
	"context"
	"testing"
	"time"

	searchdomain "github.com/dm-vev/zvonilka/internal/domain/search"
	"github.com/dm-vev/zvonilka/internal/domain/search/teststore"
)

func TestServiceSearchNormalizesParameters(t *testing.T) {
	t.Parallel()

	store := teststore.New()
	service, err := searchdomain.NewService(store, searchdomain.WithSettings(searchdomain.Settings{
		DefaultLimit:   5,
		MaxLimit:       10,
		MinQueryLength: 2,
		SnippetLength:  80,
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	if _, err := service.UpsertDocument(ctx, searchdomain.Document{
		Scope:      searchdomain.SearchScopeUsers,
		EntityType: "user",
		TargetID:   "acc-1",
		Title:      "Alice Example",
		Subtitle:   "alice",
		Snippet:    "owner",
		UserID:     "acc-1",
		Metadata:   map[string]string{"bio": "owner"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("upsert document: %v", err)
	}

	result, err := service.Search(ctx, searchdomain.SearchParams{
		Query:           "  ALICE  ",
		Scopes:          []searchdomain.SearchScope{searchdomain.SearchScopeUsers, searchdomain.SearchScopeUsers},
		UserIDs:         []string{" acc-1 ", "acc-1"},
		IncludeSnippets: false,
		Limit:           100,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(result.Hits))
	}
	if result.Hits[0].Snippet != "" {
		t.Fatalf("expected snippet to be stripped, got %q", result.Hits[0].Snippet)
	}
	if result.TotalSize != 1 {
		t.Fatalf("expected total size 1, got %d", result.TotalSize)
	}
}

func TestServiceRejectsShortQuery(t *testing.T) {
	t.Parallel()

	store := teststore.New()
	service, err := searchdomain.NewService(store, searchdomain.WithSettings(searchdomain.DefaultSettings()))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.Search(context.Background(), searchdomain.SearchParams{Query: "a"})
	if err == nil {
		t.Fatal("expected search to fail")
	}
	if err != searchdomain.ErrInvalidInput {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestServiceTrimsSnippets(t *testing.T) {
	t.Parallel()

	store := teststore.New()
	service, err := searchdomain.NewService(store, searchdomain.WithSettings(searchdomain.Settings{
		DefaultLimit:   5,
		MaxLimit:       10,
		MinQueryLength: 2,
		SnippetLength:  5,
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	if _, err := service.UpsertDocument(ctx, searchdomain.Document{
		Scope:      searchdomain.SearchScopeMedia,
		EntityType: "asset",
		TargetID:   "media-1",
		Title:      "Photo",
		Snippet:    "long snippet",
		UserID:     "acc-1",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("upsert document: %v", err)
	}

	result, err := service.Search(ctx, searchdomain.SearchParams{
		Query:           "photo",
		IncludeSnippets: true,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(result.Hits))
	}
	if result.Hits[0].Snippet != "long" {
		t.Fatalf("expected trimmed snippet, got %q", result.Hits[0].Snippet)
	}
}
