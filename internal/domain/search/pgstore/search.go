package pgstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/search"
)

// SearchDocuments resolves search hits from stored documents.
func (s *Store) SearchDocuments(ctx context.Context, params search.SearchParams) (search.SearchResult, error) {
	if err := s.requireContext(ctx); err != nil {
		return search.SearchResult{}, err
	}
	if err := s.requireStore(); err != nil {
		return search.SearchResult{}, err
	}

	params.Query = strings.TrimSpace(strings.ToLower(params.Query))
	params.Scopes = search.NormalizeScopes(params.Scopes)
	params.ConversationIDs = search.NormalizeIDs(params.ConversationIDs)
	params.UserIDs = search.NormalizeIDs(params.UserIDs)
	params.AccountKinds = search.NormalizeAccountKinds(params.AccountKinds)
	if params.Limit <= 0 {
		params.Limit = 25
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	clauses := make([]string, 0, 8)
	args := make([]any, 0, 8)
	addInClause := func(column string, values []string) {
		if len(values) == 0 {
			return
		}
		placeholders := make([]string, 0, len(values))
		for _, value := range values {
			args = append(args, value)
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		}
		clauses = append(clauses, fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ",")))
	}

	addInClause("scope", scopesToStrings(params.Scopes))
	addInClause("conversation_id", params.ConversationIDs)
	addInClause("user_id", params.UserIDs)
	addInClause("account_kind", params.AccountKinds)
	if !params.UpdatedAfter.IsZero() {
		args = append(args, params.UpdatedAfter.UTC())
		clauses = append(clauses, fmt.Sprintf("updated_at > $%d", len(args)))
	}
	if !params.UpdatedBefore.IsZero() {
		args = append(args, params.UpdatedBefore.UTC())
		clauses = append(clauses, fmt.Sprintf("updated_at < $%d", len(args)))
	}
	if params.Query != "" {
		args = append(args, params.Query)
		queryArg := len(args)
		searchClause := fmt.Sprintf(
			"(to_tsvector('simple', search_text) @@ websearch_to_tsquery('simple', $%d) OR search_text ILIKE '%%' || $%d || '%%')",
			queryArg,
			queryArg,
		)
		clauses = append(clauses, searchClause)
	}

	whereClause := ""
	if len(clauses) > 0 {
		whereClause = " WHERE " + strings.Join(clauses, " AND ")
	}

	orderClause := " ORDER BY updated_at DESC, target_id ASC"
	if params.Query != "" {
		queryArg := len(args)
		orderClause = fmt.Sprintf(
			" ORDER BY ts_rank_cd(to_tsvector('simple', search_text), websearch_to_tsquery('simple', $%d)) DESC, updated_at DESC, target_id ASC",
			queryArg,
		)
	}

	args = append(args, params.Limit, params.Offset)
	query := fmt.Sprintf(`
SELECT %s, COUNT(*) OVER() AS total_size
FROM %s%s%s
LIMIT $%d OFFSET $%d
`, documentColumnList, s.table("search_documents"), whereClause, orderClause, len(args)-1, len(args))

	rows, err := s.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return search.SearchResult{}, fmt.Errorf("search documents: %w", err)
	}
	defer rows.Close()

	hits := make([]search.Hit, 0)
	var totalSize uint64
	for rows.Next() {
		var (
			document  search.Document
			metadata  []byte
			createdAt sql.NullTime
			updatedAt sql.NullTime
			rowTotal  uint64
		)
		if err := rows.Scan(
			&document.Scope,
			&document.TargetID,
			&document.EntityType,
			&document.Title,
			&document.Subtitle,
			&document.Snippet,
			&document.SearchText,
			&document.ConversationID,
			&document.MessageID,
			&document.MediaID,
			&document.UserID,
			&document.AccountKind,
			&metadata,
			&createdAt,
			&updatedAt,
			&rowTotal,
		); err != nil {
			return search.SearchResult{}, fmt.Errorf("scan search document: %w", err)
		}
		if totalSize == 0 {
			totalSize = rowTotal
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &document.Metadata); err != nil {
				return search.SearchResult{}, fmt.Errorf("decode search metadata: %w", err)
			}
		}
		document.CreatedAt = decodeTime(createdAt)
		document.UpdatedAt = decodeTime(updatedAt)
		hits = append(hits, hitFromDocument(document))
	}
	if err := rows.Err(); err != nil {
		return search.SearchResult{}, fmt.Errorf("iterate search documents: %w", err)
	}

	result := search.SearchResult{
		Hits:      hits,
		TotalSize: totalSize,
	}
	if params.Offset+params.Limit < int(totalSize) {
		result.NextPageToken = fmt.Sprintf("%d", params.Offset+params.Limit)
	}

	return result, nil
}

func scopesToStrings(scopes []search.SearchScope) []string {
	if len(scopes) == 0 {
		return nil
	}

	values := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		values = append(values, string(scope))
	}

	return values
}
