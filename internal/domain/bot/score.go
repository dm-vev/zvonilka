package bot

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// GameScore stores one persisted score entry for a game message.
type GameScore struct {
	BotAccountID string
	MessageID    string
	AccountID    string
	Score        int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// HighScore describes one Telegram-shaped game high score row.
type HighScore struct {
	Position int  `json:"position"`
	User     User `json:"user"`
	Score    int  `json:"score"`
}

// SetGameScoreParams describes one setGameScore request.
type SetGameScoreParams struct {
	BotToken           string
	ChatID             string
	MessageID          string
	UserID             string
	Score              int
	Force              bool
	DisableEditMessage bool
}

// GetGameHighScoresParams describes one getGameHighScores request.
type GetGameHighScoresParams struct {
	BotToken  string
	ChatID    string
	MessageID string
	UserID    string
}

// SetGameScore stores or updates one score for a game message and returns the current game message.
func (s *Service) SetGameScore(ctx context.Context, params SetGameScoreParams) (Message, error) {
	accountID, conv, _, raw, err := s.loadRawMessage(ctx, params.BotToken, params.ChatID, params.MessageID)
	if err != nil {
		return Message{}, err
	}
	if messageShape(raw) != "game" {
		return Message{}, ErrInvalidInput
	}

	params.UserID = strings.TrimSpace(params.UserID)
	if params.UserID == "" || params.Score < 0 {
		return Message{}, ErrInvalidInput
	}

	if _, err := s.identity.AccountByID(ctx, params.UserID); err != nil {
		return Message{}, mapIdentityError(err)
	}

	existing, err := s.store.ScoreByMessageAndAccount(ctx, accountID, raw.ID, params.UserID)
	if err != nil && err != ErrNotFound {
		return Message{}, fmt.Errorf("load bot game score: %w", err)
	}
	if err == nil && existing.Score > params.Score && !params.Force {
		return Message{}, ErrInvalidInput
	}

	now := s.currentTime()
	createdAt := now
	if err == nil && !existing.CreatedAt.IsZero() {
		createdAt = existing.CreatedAt
	}

	_, err = s.store.SaveScore(ctx, GameScore{
		BotAccountID: accountID,
		MessageID:    raw.ID,
		AccountID:    params.UserID,
		Score:        params.Score,
		CreatedAt:    createdAt,
		UpdatedAt:    now,
	})
	if err != nil {
		return Message{}, fmt.Errorf("save bot game score: %w", err)
	}

	return s.GetMessage(ctx, GetMessageParams{
		BotToken:  params.BotToken,
		ChatID:    conv.ID,
		MessageID: raw.ID,
	})
}

// GetGameHighScores returns the current scoreboard for one game message.
func (s *Service) GetGameHighScores(ctx context.Context, params GetGameHighScoresParams) ([]HighScore, error) {
	accountID, _, _, raw, err := s.loadRawMessage(ctx, params.BotToken, params.ChatID, params.MessageID)
	if err != nil {
		return nil, err
	}
	if messageShape(raw) != "game" {
		return nil, ErrInvalidInput
	}

	params.UserID = strings.TrimSpace(params.UserID)
	if params.UserID == "" {
		return nil, ErrInvalidInput
	}
	if _, err := s.identity.AccountByID(ctx, params.UserID); err != nil {
		return nil, mapIdentityError(err)
	}

	scores, err := s.store.ListScoresByMessage(ctx, accountID, raw.ID)
	if err != nil {
		return nil, fmt.Errorf("list bot game scores: %w", err)
	}

	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		if !scores[i].UpdatedAt.Equal(scores[j].UpdatedAt) {
			return scores[i].UpdatedAt.Before(scores[j].UpdatedAt)
		}
		return scores[i].AccountID < scores[j].AccountID
	})

	result := make([]HighScore, 0, len(scores))
	for index, score := range scores {
		userAccount, err := s.identity.AccountByID(ctx, score.AccountID)
		if err != nil {
			return nil, mapIdentityError(err)
		}
		result = append(result, HighScore{
			Position: index + 1,
			User:     userFromAccount(userAccount),
			Score:    score.Score,
		})
	}

	return result, nil
}

func (s GameScore) normalize(now time.Time) (GameScore, error) {
	s.BotAccountID = strings.TrimSpace(s.BotAccountID)
	s.MessageID = strings.TrimSpace(s.MessageID)
	s.AccountID = strings.TrimSpace(s.AccountID)
	if s.BotAccountID == "" || s.MessageID == "" || s.AccountID == "" || s.Score < 0 {
		return GameScore{}, ErrInvalidInput
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now.UTC()
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = s.CreatedAt
	}

	return s, nil
}

// NormalizeGameScore validates and normalizes one game-score record.
func NormalizeGameScore(score GameScore, now time.Time) (GameScore, error) {
	return score.normalize(now)
}
