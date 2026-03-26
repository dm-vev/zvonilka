package teststore

import (
	"context"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/search"
)

func TestSearchStoreUpsertSearchAndDelete(t *testing.T) {
	t.Parallel()

	store := New()
	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)

	document, err := store.UpsertDocument(ctx, search.Document{
		Scope:      search.SearchScopeUsers,
		EntityType: "user",
		TargetID:   "acc-1",
		Title:      "Alice",
		Subtitle:   "alice",
		SearchText: "alice alice example",
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("upsert document: %v", err)
	}
	if document.ID == "" {
		t.Fatal("expected document ID")
	}

	result, err := store.SearchDocuments(ctx, search.SearchParams{
		Query:  "alice",
		Limit:  10,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("search documents: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(result.Hits))
	}

	if err := store.DeleteDocument(ctx, search.SearchScopeUsers, "acc-1"); err != nil {
		t.Fatalf("delete document: %v", err)
	}
	if _, err := store.DocumentByScopeAndTarget(ctx, search.SearchScopeUsers, "acc-1"); err != search.ErrNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}
