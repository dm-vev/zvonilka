package pgstore

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func encodeTime(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: value.UTC(), Valid: true}
}

func decodeTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}

	return value.Time.UTC()
}

func scanWebhook(row rowScanner) (bot.Webhook, error) {
	var (
		webhook     bot.Webhook
		rawAllowed  []byte
		lastErrorAt sql.NullTime
		lastSuccess sql.NullTime
	)

	if err := row.Scan(
		&webhook.BotAccountID,
		&webhook.URL,
		&webhook.SecretToken,
		&rawAllowed,
		&webhook.MaxConnections,
		&webhook.LastErrorMessage,
		&lastErrorAt,
		&lastSuccess,
		&webhook.CreatedAt,
		&webhook.UpdatedAt,
	); err != nil {
		return bot.Webhook{}, err
	}

	allowed, err := decodeAllowedUpdates(rawAllowed)
	if err != nil {
		return bot.Webhook{}, err
	}
	webhook.AllowedUpdates = allowed
	webhook.LastErrorAt = decodeTime(lastErrorAt)
	webhook.LastSuccessAt = decodeTime(lastSuccess)
	webhook.CreatedAt = webhook.CreatedAt.UTC()
	webhook.UpdatedAt = webhook.UpdatedAt.UTC()

	return webhook, nil
}

func scanUpdate(row rowScanner) (bot.QueueEntry, error) {
	var (
		entry   bot.QueueEntry
		payload []byte
	)

	if err := row.Scan(
		&entry.UpdateID,
		&entry.BotAccountID,
		&entry.EventID,
		&entry.UpdateType,
		&payload,
		&entry.Attempts,
		&entry.NextAttemptAt,
		&entry.LastError,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	); err != nil {
		return bot.QueueEntry{}, err
	}

	update, err := decodeUpdate(payload)
	if err != nil {
		return bot.QueueEntry{}, err
	}
	entry.Payload = update
	entry.NextAttemptAt = entry.NextAttemptAt.UTC()
	entry.CreatedAt = entry.CreatedAt.UTC()
	entry.UpdatedAt = entry.UpdatedAt.UTC()

	return entry, nil
}

func scanCursor(row rowScanner) (bot.Cursor, error) {
	var cursor bot.Cursor

	if err := row.Scan(&cursor.Name, &cursor.LastSequence, &cursor.UpdatedAt); err != nil {
		return bot.Cursor{}, err
	}

	cursor.UpdatedAt = cursor.UpdatedAt.UTC()

	return cursor, nil
}

func scanCallback(row rowScanner) (bot.Callback, error) {
	var (
		callback   bot.Callback
		answeredAt sql.NullTime
	)

	if err := row.Scan(
		&callback.ID,
		&callback.BotAccountID,
		&callback.FromAccountID,
		&callback.ConversationID,
		&callback.MessageID,
		&callback.MessageThreadID,
		&callback.ChatInstance,
		&callback.Data,
		&callback.AnsweredText,
		&callback.AnsweredURL,
		&callback.ShowAlert,
		&callback.CacheTime,
		&callback.CreatedAt,
		&callback.UpdatedAt,
		&answeredAt,
	); err != nil {
		return bot.Callback{}, err
	}

	callback.AnsweredAt = decodeTime(answeredAt)
	callback.CreatedAt = callback.CreatedAt.UTC()
	callback.UpdatedAt = callback.UpdatedAt.UTC()

	return callback, nil
}

func encodeInlineResults(values []bot.InlineQueryResult) ([]byte, error) {
	if len(values) == 0 {
		return []byte("[]"), nil
	}

	return json.Marshal(values)
}

func decodeInlineResults(raw []byte) ([]bot.InlineQueryResult, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var values []bot.InlineQueryResult
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}

	return values, nil
}

func scanInlineQuery(row rowScanner) (bot.InlineQueryState, error) {
	var (
		query      bot.InlineQueryState
		rawResults []byte
		answeredAt sql.NullTime
	)

	if err := row.Scan(
		&query.ID,
		&query.BotAccountID,
		&query.FromAccountID,
		&query.Query,
		&query.Offset,
		&query.ChatType,
		&query.Answered,
		&rawResults,
		&query.CacheTime,
		&query.IsPersonal,
		&query.NextOffset,
		&query.SwitchPMText,
		&query.SwitchPMParam,
		&query.CreatedAt,
		&query.UpdatedAt,
		&answeredAt,
	); err != nil {
		return bot.InlineQueryState{}, err
	}

	results, err := decodeInlineResults(rawResults)
	if err != nil {
		return bot.InlineQueryState{}, err
	}
	query.Results = results
	query.AnsweredAt = decodeTime(answeredAt)
	query.CreatedAt = query.CreatedAt.UTC()
	query.UpdatedAt = query.UpdatedAt.UTC()

	return query, nil
}

func scanScore(row rowScanner) (bot.GameScore, error) {
	var score bot.GameScore

	if err := row.Scan(
		&score.BotAccountID,
		&score.MessageID,
		&score.AccountID,
		&score.Score,
		&score.CreatedAt,
		&score.UpdatedAt,
	); err != nil {
		return bot.GameScore{}, err
	}

	score.CreatedAt = score.CreatedAt.UTC()
	score.UpdatedAt = score.UpdatedAt.UTC()

	return score, nil
}
