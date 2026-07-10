package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/reaction"
)

type sqlConn interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Store persists the global emoji reaction catalog in PostgreSQL.
type Store struct {
	db     *sql.DB
	tx     *sql.Tx
	schema string
}

// New constructs a PostgreSQL reaction catalog store.
func New(db *sql.DB, schema string) (*Store, error) {
	if db == nil {
		return nil, reaction.ErrInvalidInput
	}

	return &Store{db: db, schema: strings.TrimSpace(schema)}, nil
}

// WithinTx executes a catalog update in one database transaction.
func (s *Store) WithinTx(ctx context.Context, fn func(reaction.Store) error) error {
	if s == nil || s.db == nil || fn == nil || ctx == nil {
		return reaction.ErrInvalidInput
	}
	if s.tx != nil {
		return fn(s)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reaction catalog transaction: %w", err)
	}
	txStore := &Store{db: s.db, tx: tx, schema: s.schema}
	if err := fn(txStore); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("rollback reaction catalog transaction: %w", rollbackErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reaction catalog transaction: %w", err)
	}
	return nil
}

// SaveDefinition inserts or updates one reaction definition.
func (s *Store) SaveDefinition(ctx context.Context, definition reaction.Definition) (reaction.Definition, error) {
	if s == nil || s.db == nil || ctx == nil || definition.Emoji == "" {
		return reaction.Definition{}, reaction.ErrInvalidInput
	}
	query := fmt.Sprintf(`
INSERT INTO %s (
	emoji, title, active, sort_order, static_icon_media_id, appear_animation_media_id,
	select_animation_media_id, activate_animation_media_id, effect_animation_media_id,
	around_animation_media_id, center_animation_media_id, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''), NULLIF($11, ''), NOW(), NOW())
ON CONFLICT (emoji) DO UPDATE SET
	title = EXCLUDED.title,
	active = EXCLUDED.active,
	sort_order = EXCLUDED.sort_order,
	static_icon_media_id = EXCLUDED.static_icon_media_id,
	appear_animation_media_id = EXCLUDED.appear_animation_media_id,
	select_animation_media_id = EXCLUDED.select_animation_media_id,
	activate_animation_media_id = EXCLUDED.activate_animation_media_id,
	effect_animation_media_id = EXCLUDED.effect_animation_media_id,
	around_animation_media_id = EXCLUDED.around_animation_media_id,
	center_animation_media_id = EXCLUDED.center_animation_media_id,
	updated_at = NOW()
RETURNING emoji, title, active, sort_order, static_icon_media_id, appear_animation_media_id,
	select_animation_media_id, activate_animation_media_id, effect_animation_media_id,
	around_animation_media_id, center_animation_media_id
`, s.tableName("emoji_reaction_catalog"))

	row := s.conn().QueryRowContext(ctx, query,
		definition.Emoji,
		definition.Title,
		definition.Active,
		definition.SortOrder,
		definition.StaticIcon,
		definition.AppearAnimation,
		definition.SelectAnimation,
		definition.ActivateAnimation,
		definition.EffectAnimation,
		definition.AroundAnimation,
		definition.CenterAnimation,
	)
	return scanDefinition(row)
}

// DefinitionByEmoji returns one catalog definition.
func (s *Store) DefinitionByEmoji(ctx context.Context, emoji string) (reaction.Definition, error) {
	if s == nil || s.db == nil || ctx == nil {
		return reaction.Definition{}, reaction.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return reaction.Definition{}, err
	}
	emoji = strings.TrimSpace(emoji)
	if emoji == "" {
		return reaction.Definition{}, reaction.ErrInvalidInput
	}
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE emoji = $1`, definitionColumns, s.tableName("emoji_reaction_catalog"))
	definition, err := scanDefinition(s.conn().QueryRowContext(ctx, query, emoji))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return reaction.Definition{}, reaction.ErrNotFound
		}
		return reaction.Definition{}, fmt.Errorf("load reaction %q: %w", emoji, err)
	}
	return definition, nil
}

// Definitions returns the catalog in stable picker order.
func (s *Store) Definitions(ctx context.Context) ([]reaction.Definition, error) {
	if s == nil || s.db == nil || ctx == nil {
		return nil, reaction.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT %s FROM %s ORDER BY sort_order ASC, emoji ASC`, definitionColumns, s.tableName("emoji_reaction_catalog"))
	rows, err := s.conn().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list reaction catalog: %w", err)
	}
	defer rows.Close()

	definitions := make([]reaction.Definition, 0)
	for rows.Next() {
		definition, scanErr := scanDefinition(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan reaction catalog: %w", scanErr)
		}
		definitions = append(definitions, definition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reaction catalog: %w", err)
	}
	return definitions, nil
}

const definitionColumns = `emoji, title, active, sort_order, static_icon_media_id, appear_animation_media_id,
select_animation_media_id, activate_animation_media_id, effect_animation_media_id,
around_animation_media_id, center_animation_media_id`

func (s *Store) conn() sqlConn {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

func (s *Store) tableName(name string) string {
	schema := s.schema
	if schema == "" {
		schema = "public"
	}
	return fmt.Sprintf(`"%s"."%s"`, strings.ReplaceAll(schema, `"`, `""`), name)
}

func scanDefinition(row interface{ Scan(...any) error }) (reaction.Definition, error) {
	var definition reaction.Definition
	var around, center sql.NullString
	if err := row.Scan(
		&definition.Emoji,
		&definition.Title,
		&definition.Active,
		&definition.SortOrder,
		&definition.StaticIcon,
		&definition.AppearAnimation,
		&definition.SelectAnimation,
		&definition.ActivateAnimation,
		&definition.EffectAnimation,
		&around,
		&center,
	); err != nil {
		return reaction.Definition{}, err
	}
	if around.Valid {
		definition.AroundAnimation = around.String
	}
	if center.Valid {
		definition.CenterAnimation = center.String
	}
	return definition, nil
}
