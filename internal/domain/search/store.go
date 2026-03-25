package search

import "context"

// Indexer mutates search documents.
type Indexer interface {
	UpsertDocument(ctx context.Context, document Document) (Document, error)
	DeleteDocument(ctx context.Context, scope SearchScope, targetID string) error
}

// Store persists search documents.
type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error

	UpsertDocument(ctx context.Context, document Document) (Document, error)
	DeleteDocument(ctx context.Context, scope SearchScope, targetID string) error
	DocumentByScopeAndTarget(ctx context.Context, scope SearchScope, targetID string) (Document, error)
	SearchDocuments(ctx context.Context, params SearchParams) (SearchResult, error)
}
