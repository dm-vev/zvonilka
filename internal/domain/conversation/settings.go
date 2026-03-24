package conversation

import "time"

// DefaultConversationSettings returns a conservative write-policy baseline.
func DefaultConversationSettings() ConversationSettings {
	return ConversationSettings{
		AllowReactions:           true,
		AllowForwards:            true,
		AllowThreads:             true,
		RequireEncryptedMessages: true,
		SlowModeInterval:         0,
	}
}

// normalizeConversationSettings fills in nil-like zero values from defaults.
//
// The current API surface still uses plain booleans, so the helper only applies
// defaults when the caller explicitly passes an empty settings structure.
func normalizeConversationSettings(settings ConversationSettings) ConversationSettings {
	defaults := DefaultConversationSettings()
	if settings == (ConversationSettings{}) {
		return defaults
	}

	if settings.SlowModeInterval < 0 {
		settings.SlowModeInterval = 0
	}
	settings.RequireEncryptedMessages = true

	return settings
}

// normalizeMessageDraft trims identifiers and clamps negative durations.
func normalizeMessageDraft(draft MessageDraft) MessageDraft {
	if draft.DeliverAt.Before(time.Time{}) {
		draft.DeliverAt = time.Time{}
	}

	return draft
}
