package pgstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/search"
)

type rowScanner interface {
	Scan(dest ...any) error
}

const documentColumnList = `
	scope,
	target_id,
	entity_type,
	title,
	subtitle,
	snippet,
	search_text,
	conversation_id,
	message_id,
	media_id,
	user_id,
	account_kind,
	metadata,
	created_at,
	updated_at
`

func scanDocument(row rowScanner) (search.Document, error) {
	var (
		document  search.Document
		metadata  []byte
		createdAt sql.NullTime
		updatedAt sql.NullTime
	)

	if err := row.Scan(
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
	); err != nil {
		return search.Document{}, err
	}

	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &document.Metadata); err != nil {
			return search.Document{}, fmt.Errorf("decode search metadata: %w", err)
		}
	}

	document.CreatedAt = decodeTime(createdAt)
	document.UpdatedAt = decodeTime(updatedAt)
	document.ID = search.DocumentID(document.Scope, document.TargetID)

	return document, nil
}

func encodeMetadata(metadata map[string]string) (string, error) {
	if len(metadata) == 0 {
		return "{}", nil
	}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}

func decodeTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}

	return value.Time.UTC()
}

func hitFromDocument(document search.Document) search.Hit {
	return search.Hit{
		HitID:          document.ID,
		Scope:          document.Scope,
		Title:          document.Title,
		Subtitle:       document.Subtitle,
		Snippet:        document.Snippet,
		TargetID:       document.TargetID,
		ConversationID: document.ConversationID,
		MessageID:      document.MessageID,
		MediaID:        document.MediaID,
		UserID:         document.UserID,
		AccountKind:    document.AccountKind,
		Metadata:       document.Metadata,
		UpdatedAt:      document.UpdatedAt,
	}
}
