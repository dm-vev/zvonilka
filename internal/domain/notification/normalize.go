package notification

import "time"

// NormalizePreference validates and normalizes a notification preference.
func NormalizePreference(preference Preference, now time.Time) (Preference, error) {
	return preference.normalize(now)
}

// NormalizeConversationOverride validates and normalizes a chat override.
func NormalizeConversationOverride(override ConversationOverride, now time.Time) (ConversationOverride, error) {
	return override.normalize(now)
}

// NormalizePushToken validates and normalizes a push token.
func NormalizePushToken(token PushToken, now time.Time) (PushToken, error) {
	return token.normalize(now)
}

// NormalizeDelivery validates and normalizes a delivery hint.
func NormalizeDelivery(delivery Delivery, now time.Time) (Delivery, error) {
	return delivery.normalize(now)
}

// NormalizeWorkerCursor validates and normalizes a worker cursor.
func NormalizeWorkerCursor(cursor WorkerCursor, now time.Time) (WorkerCursor, error) {
	return cursor.normalize(now)
}
