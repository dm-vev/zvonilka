package conversation

import "time"

// DefaultConversationSettings returns a conservative write-policy baseline with opt-in encryption.
func DefaultConversationSettings() ConversationSettings {
	return ConversationSettings{
		AllowReactions:           true,
		AllowForwards:            true,
		AllowThreads:             true,
		RequireEncryptedMessages: false,
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

	return settings
}

// normalizeMessageDraft trims identifiers, deduplicates mention targets, and clamps negative durations.
func normalizeMessageDraft(draft MessageDraft) MessageDraft {
	draft.MentionAccountIDs = uniqueIDs(draft.MentionAccountIDs)
	if draft.DeliverAt.Before(time.Time{}) {
		draft.DeliverAt = time.Time{}
	}

	return draft
}
