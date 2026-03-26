package bot

import "time"

// NormalizeWebhook validates and normalizes one webhook record.
func NormalizeWebhook(webhook Webhook, now time.Time) (Webhook, error) {
	return webhook.normalize(now)
}

// NormalizeQueueEntry validates and normalizes one queue entry.
func NormalizeQueueEntry(entry QueueEntry, now time.Time) (QueueEntry, error) {
	return entry.normalize(now)
}

// NormalizeCursor validates and normalizes one worker cursor.
func NormalizeCursor(cursor Cursor, now time.Time) (Cursor, error) {
	return cursor.normalize(now)
}

// NormalizeCallback validates and normalizes one callback query record.
func NormalizeCallback(callback Callback, now time.Time) (Callback, error) {
	return callback.normalize(now)
}

// NormalizeInlineQuery validates and normalizes one inline query state record.
func NormalizeInlineQuery(query InlineQueryState, now time.Time) (InlineQueryState, error) {
	return query.normalize(now)
}
