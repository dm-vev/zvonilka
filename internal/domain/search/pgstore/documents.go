package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/search"
)

// UpsertDocument inserts or updates a search document.
func (s *Store) UpsertDocument(ctx context.Context, document search.Document) (search.Document, error) {
	if err := s.requireContext(ctx); err != nil {
		return search.Document{}, err
	}
	if err := s.requireStore(); err != nil {
		return search.Document{}, err
	}
	if document.TargetID == "" {
		return search.Document{}, search.ErrInvalidInput
	}
	if s.tx != nil {
		return s.upsertDocument(ctx, document)
	}

	var saved search.Document
	err := s.withTransaction(ctx, func(tx search.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).upsertDocument(ctx, document)
		return saveErr
	})
	if err != nil {
		return search.Document{}, err
	}

	return saved, nil
}

func (s *Store) upsertDocument(ctx context.Context, document search.Document) (search.Document, error) {
	normalized, err := search.NormalizeDocument(document, time.Now().UTC())
	if err != nil {
		return search.Document{}, err
	}

	metadata, err := encodeMetadata(normalized.Metadata)
	if err != nil {
		return search.Document{}, fmt.Errorf("encode search metadata for %s: %w", normalized.ID, err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	scope, target_id, entity_type, title, subtitle, snippet, search_text,
	conversation_id, message_id, media_id, user_id, account_kind, metadata,
	created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13,
	$14, $15
)
ON CONFLICT (scope, target_id) DO UPDATE SET
	entity_type = EXCLUDED.entity_type,
	title = EXCLUDED.title,
	subtitle = EXCLUDED.subtitle,
	snippet = EXCLUDED.snippet,
	search_text = EXCLUDED.search_text,
	conversation_id = EXCLUDED.conversation_id,
	message_id = EXCLUDED.message_id,
	media_id = EXCLUDED.media_id,
	user_id = EXCLUDED.user_id,
	account_kind = EXCLUDED.account_kind,
	metadata = EXCLUDED.metadata,
	created_at = %s.created_at,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("search_documents"), s.table("search_documents"), documentColumnList)

	var saved search.Document
	saved, err = scanDocument(s.conn().QueryRowContext(ctx, query,
		normalized.Scope,
		normalized.TargetID,
		normalized.EntityType,
		normalized.Title,
		normalized.Subtitle,
		normalized.Snippet,
		normalized.SearchText,
		normalized.ConversationID,
		normalized.MessageID,
		normalized.MediaID,
		normalized.UserID,
		normalized.AccountKind,
		metadata,
		normalized.CreatedAt.UTC(),
		normalized.UpdatedAt.UTC(),
	))
	if err != nil {
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return search.Document{}, mappedErr
		}
		if errors.Is(err, sql.ErrNoRows) {
			return search.Document{}, search.ErrNotFound
		}
		return search.Document{}, fmt.Errorf("upsert search document %s: %w", normalized.ID, err)
	}

	return saved, nil
}

// DeleteDocument removes a document by scope and target ID.
func (s *Store) DeleteDocument(ctx context.Context, scope search.SearchScope, targetID string) error {
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if err := s.requireStore(); err != nil {
		return err
	}
	scope = search.NormalizeScope(scope)
	targetID = strings.TrimSpace(targetID)
	if scope == search.SearchScopeUnspecified || targetID == "" {
		return search.ErrInvalidInput
	}
	if s.tx != nil {
		return s.deleteDocument(ctx, scope, targetID)
	}

	return s.withTransaction(ctx, func(tx search.Store) error {
		return tx.(*Store).deleteDocument(ctx, scope, targetID)
	})
}

func (s *Store) deleteDocument(ctx context.Context, scope search.SearchScope, targetID string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE scope = $1 AND target_id = $2`, s.table("search_documents"))
	result, err := s.conn().ExecContext(ctx, query, scope, targetID)
	if err != nil {
		return fmt.Errorf("delete search document %s:%s: %w", scope, targetID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete search document %s:%s: %w", scope, targetID, err)
	}
	if rowsAffected == 0 {
		return search.ErrNotFound
	}

	return nil
}

// DocumentByScopeAndTarget resolves one document by key.
func (s *Store) DocumentByScopeAndTarget(ctx context.Context, scope search.SearchScope, targetID string) (search.Document, error) {
	if err := s.requireContext(ctx); err != nil {
		return search.Document{}, err
	}
	if err := s.requireStore(); err != nil {
		return search.Document{}, err
	}
	scope = search.NormalizeScope(scope)
	targetID = strings.TrimSpace(targetID)
	if scope == search.SearchScopeUnspecified || targetID == "" {
		return search.Document{}, search.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE scope = $1 AND target_id = $2`, documentColumnList, s.table("search_documents"))
	document, err := scanDocument(s.conn().QueryRowContext(ctx, query, scope, targetID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return search.Document{}, search.ErrNotFound
		}
		return search.Document{}, fmt.Errorf("load search document %s:%s: %w", scope, targetID, err)
	}

	return document, nil
}
