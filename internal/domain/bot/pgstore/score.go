package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (s *Store) SaveScore(ctx context.Context, state bot.GameScore) (bot.GameScore, error) {
	if err := s.requireStore(); err != nil {
		return bot.GameScore{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.GameScore{}, err
	}
	if s.tx != nil {
		return s.saveScore(ctx, state)
	}

	var saved bot.GameScore
	err := s.WithinTx(ctx, func(tx bot.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveScore(ctx, state)
		return saveErr
	})
	if err != nil {
		return bot.GameScore{}, err
	}

	return saved, nil
}

func (s *Store) saveScore(ctx context.Context, state bot.GameScore) (bot.GameScore, error) {
	now := time.Now().UTC()
	value, err := bot.NormalizeGameScore(state, now)
	if err != nil {
		return bot.GameScore{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	bot_account_id, message_id, account_id, score, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (bot_account_id, message_id, account_id) DO UPDATE SET
	score = EXCLUDED.score,
	updated_at = EXCLUDED.updated_at
RETURNING bot_account_id, message_id, account_id, score, created_at, updated_at
`, s.table("bot_game_scores"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		value.BotAccountID,
		value.MessageID,
		value.AccountID,
		value.Score,
		value.CreatedAt.UTC(),
		value.UpdatedAt.UTC(),
	)

	saved, err := scanScore(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.GameScore{}, bot.ErrNotFound
		}
		if mapped := mapConstraintError(err); mapped != nil {
			return bot.GameScore{}, mapped
		}

		return bot.GameScore{}, fmt.Errorf(
			"save bot game score for %s/%s/%s: %w",
			value.BotAccountID,
			value.MessageID,
			value.AccountID,
			err,
		)
	}

	return saved, nil
}

func (s *Store) ScoreByMessageAndAccount(
	ctx context.Context,
	botAccountID string,
	messageID string,
	accountID string,
) (bot.GameScore, error) {
	if err := s.requireStore(); err != nil {
		return bot.GameScore{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.GameScore{}, err
	}

	botAccountID = strings.TrimSpace(botAccountID)
	messageID = strings.TrimSpace(messageID)
	accountID = strings.TrimSpace(accountID)
	if botAccountID == "" || messageID == "" || accountID == "" {
		return bot.GameScore{}, bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT bot_account_id, message_id, account_id, score, created_at, updated_at
FROM %s
WHERE bot_account_id = $1 AND message_id = $2 AND account_id = $3
`, s.table("bot_game_scores"))

	row := s.conn().QueryRowContext(ctx, query, botAccountID, messageID, accountID)
	score, err := scanScore(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.GameScore{}, bot.ErrNotFound
		}

		return bot.GameScore{}, fmt.Errorf(
			"load bot game score for %s/%s/%s: %w",
			botAccountID,
			messageID,
			accountID,
			err,
		)
	}

	return score, nil
}

func (s *Store) ListScoresByMessage(
	ctx context.Context,
	botAccountID string,
	messageID string,
) ([]bot.GameScore, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}

	botAccountID = strings.TrimSpace(botAccountID)
	messageID = strings.TrimSpace(messageID)
	if botAccountID == "" || messageID == "" {
		return nil, bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT bot_account_id, message_id, account_id, score, created_at, updated_at
FROM %s
WHERE bot_account_id = $1 AND message_id = $2
ORDER BY score DESC, updated_at ASC, account_id ASC
`, s.table("bot_game_scores"))

	rows, err := s.conn().QueryContext(ctx, query, botAccountID, messageID)
	if err != nil {
		return nil, fmt.Errorf("list bot game scores for %s/%s: %w", botAccountID, messageID, err)
	}
	defer rows.Close()

	scores := make([]bot.GameScore, 0)
	for rows.Next() {
		score, scanErr := scanScore(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan bot game scores for %s/%s: %w", botAccountID, messageID, scanErr)
		}
		scores = append(scores, score)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bot game scores for %s/%s: %w", botAccountID, messageID, err)
	}

	return scores, nil
}
