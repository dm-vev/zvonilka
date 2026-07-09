package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/translation"
)

// Store persists translation cache rows in PostgreSQL.
type Store struct {
	db     *sql.DB
	schema string
}

// New constructs a PostgreSQL-backed translation store.
func New(db *sql.DB, schema string) (*Store, error) {
	if db == nil {
		return nil, translation.ErrInvalidInput
	}

	return &Store{
		db:     db,
		schema: strings.TrimSpace(schema),
	}, nil
}

func (s *Store) table(name string) string {
	if s.schema == "" {
		return name
	}

	return s.schema + "." + name
}

func (s *Store) SaveTranslation(
	ctx context.Context,
	row translation.Translation,
) (translation.Translation, error) {
	if ctx == nil {
		return translation.Translation{}, translation.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return translation.Translation{}, err
	}

	row.MessageID = strings.TrimSpace(row.MessageID)
	row.TargetLanguage = strings.ToLower(strings.TrimSpace(row.TargetLanguage))
	row.SourceLanguage = strings.TrimSpace(row.SourceLanguage)
	row.Provider = strings.TrimSpace(row.Provider)
	row.TranslatedText = strings.TrimSpace(row.TranslatedText)
	if row.MessageID == "" || row.TargetLanguage == "" || row.TranslatedText == "" {
		return translation.Translation{}, translation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	message_id, target_language, source_language, provider, translated_text, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (message_id, target_language) DO UPDATE SET
	source_language = EXCLUDED.source_language,
	provider = EXCLUDED.provider,
	translated_text = EXCLUDED.translated_text,
	updated_at = EXCLUDED.updated_at
RETURNING message_id, target_language, source_language, provider, translated_text, created_at, updated_at
`, s.table("conversation_message_translations"))

	saved := translation.Translation{}
	err := s.db.QueryRowContext(
		ctx,
		query,
		row.MessageID,
		row.TargetLanguage,
		row.SourceLanguage,
		row.Provider,
		row.TranslatedText,
		row.CreatedAt.UTC(),
		row.UpdatedAt.UTC(),
	).Scan(
		&saved.MessageID,
		&saved.TargetLanguage,
		&saved.SourceLanguage,
		&saved.Provider,
		&saved.TranslatedText,
		&saved.CreatedAt,
		&saved.UpdatedAt,
	)
	if err != nil {
		return translation.Translation{}, fmt.Errorf("save translation %s/%s: %w", row.MessageID, row.TargetLanguage, err)
	}
	saved.CreatedAt = saved.CreatedAt.UTC()
	saved.UpdatedAt = saved.UpdatedAt.UTC()

	return saved, nil
}

func (s *Store) TranslationByMessageAndLanguage(
	ctx context.Context,
	messageID string,
	targetLanguage string,
) (translation.Translation, error) {
	if ctx == nil {
		return translation.Translation{}, translation.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return translation.Translation{}, err
	}

	messageID = strings.TrimSpace(messageID)
	targetLanguage = strings.ToLower(strings.TrimSpace(targetLanguage))
	if messageID == "" || targetLanguage == "" {
		return translation.Translation{}, translation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT message_id, target_language, source_language, provider, translated_text, created_at, updated_at
FROM %s
WHERE message_id = $1 AND target_language = $2
`, s.table("conversation_message_translations"))

	row := translation.Translation{}
	err := s.db.QueryRowContext(ctx, query, messageID, targetLanguage).Scan(
		&row.MessageID,
		&row.TargetLanguage,
		&row.SourceLanguage,
		&row.Provider,
		&row.TranslatedText,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return translation.Translation{}, translation.ErrNotFound
		}
		return translation.Translation{}, fmt.Errorf("load translation %s/%s: %w", messageID, targetLanguage, err)
	}
	row.CreatedAt = row.CreatedAt.UTC()
	row.UpdatedAt = row.UpdatedAt.UTC()

	return row, nil
}
