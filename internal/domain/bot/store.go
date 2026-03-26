package bot

import (
	"context"
	"time"
)

// Store persists bot webhook and update state.
type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error

	EnsurePublicID(ctx context.Context, kind PublicIDKind, internalID string) (int64, error)
	InternalIDByPublic(ctx context.Context, kind PublicIDKind, publicID int64) (string, error)

	SaveWebhook(ctx context.Context, webhook Webhook) (Webhook, error)
	WebhookByBotAccountID(ctx context.Context, botAccountID string) (Webhook, error)
	ListWebhooks(ctx context.Context) ([]Webhook, error)
	DeleteWebhook(ctx context.Context, botAccountID string) error

	SaveUpdate(ctx context.Context, entry QueueEntry) (QueueEntry, error)
	PendingUpdates(
		ctx context.Context,
		botAccountID string,
		offset int64,
		allowed []UpdateType,
		before time.Time,
		limit int,
	) ([]QueueEntry, error)
	DeleteUpdatesBefore(ctx context.Context, botAccountID string, offset int64) error
	DeleteUpdate(ctx context.Context, botAccountID string, updateID int64) error
	DeleteAllUpdates(ctx context.Context, botAccountID string) error
	PendingUpdateCount(ctx context.Context, botAccountID string) (int, error)
	RetryUpdate(ctx context.Context, params RetryParams) (QueueEntry, error)

	SaveCallback(ctx context.Context, callback Callback) (Callback, error)
	CallbackByID(ctx context.Context, callbackID string) (Callback, error)
	AnswerCallback(ctx context.Context, callback Callback) (Callback, error)

	SaveInlineQuery(ctx context.Context, query InlineQueryState) (InlineQueryState, error)
	InlineQueryByID(ctx context.Context, queryID string) (InlineQueryState, error)
	AnswerInlineQuery(ctx context.Context, query InlineQueryState) (InlineQueryState, error)

	SaveCommands(ctx context.Context, set CommandSet) (CommandSet, error)
	CommandsByScope(ctx context.Context, botAccountID string, scope CommandScope, languageCode string) (CommandSet, error)
	DeleteCommands(ctx context.Context, botAccountID string, scope CommandScope, languageCode string) error

	SaveMenu(ctx context.Context, state MenuState) (MenuState, error)
	MenuByChat(ctx context.Context, botAccountID string, chatID string) (MenuState, error)

	SaveProfile(ctx context.Context, value ProfileValue) (ProfileValue, error)
	ProfileByLanguage(
		ctx context.Context,
		botAccountID string,
		kind ProfileKind,
		languageCode string,
	) (ProfileValue, error)
	DeleteProfile(ctx context.Context, botAccountID string, kind ProfileKind, languageCode string) error

	SaveRights(ctx context.Context, state AdminRightsState) (AdminRightsState, error)
	RightsByScope(ctx context.Context, botAccountID string, forChannels bool) (AdminRightsState, error)
	DeleteRights(ctx context.Context, botAccountID string, forChannels bool) error

	SaveScore(ctx context.Context, state GameScore) (GameScore, error)
	ScoreByMessageAndAccount(ctx context.Context, botAccountID string, messageID string, accountID string) (GameScore, error)
	ListScoresByMessage(ctx context.Context, botAccountID string, messageID string) ([]GameScore, error)

	SaveCursor(ctx context.Context, cursor Cursor) (Cursor, error)
	CursorByName(ctx context.Context, name string) (Cursor, error)
}
