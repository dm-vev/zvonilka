package bot

import (
	"strings"
	"time"
)

// UpdateType identifies one Telegram-shaped update variant.
type UpdateType string

// Supported bot update types.
const (
	UpdateTypeUnspecified        UpdateType = ""
	UpdateTypeMessage            UpdateType = "message"
	UpdateTypeEditedMessage      UpdateType = "edited_message"
	UpdateTypeChannelPost        UpdateType = "channel_post"
	UpdateTypeEditedChannelPost  UpdateType = "edited_channel_post"
	UpdateTypeCallbackQuery      UpdateType = "callback_query"
	UpdateTypeInlineQuery        UpdateType = "inline_query"
	UpdateTypeChosenInlineResult UpdateType = "chosen_inline_result"
	UpdateTypeChatMember         UpdateType = "chat_member"
	UpdateTypeMyChatMember       UpdateType = "my_chat_member"
)

// ChatType identifies a Bot API chat kind.
type ChatType string

// Supported bot chat types.
const (
	ChatTypeUnspecified ChatType = ""
	ChatTypePrivate     ChatType = "private"
	ChatTypeGroup       ChatType = "group"
	ChatTypeSupergroup  ChatType = "supergroup"
	ChatTypeChannel     ChatType = "channel"
)

// MemberStatus identifies one bot-visible chat membership state.
type MemberStatus string

// Supported chat-member statuses.
const (
	MemberStatusUnspecified   MemberStatus = ""
	MemberStatusCreator       MemberStatus = "creator"
	MemberStatusAdministrator MemberStatus = "administrator"
	MemberStatusMember        MemberStatus = "member"
	MemberStatusLeft          MemberStatus = "left"
	MemberStatusKicked        MemberStatus = "kicked"
	MemberStatusRestricted    MemberStatus = "restricted"
)

// User describes a Telegram-shaped bot user projection.
type User struct {
	ID                      string `json:"id"`
	IsBot                   bool   `json:"is_bot"`
	FirstName               string `json:"first_name"`
	Username                string `json:"username,omitempty"`
	CanJoinGroups           bool   `json:"can_join_groups,omitempty"`
	CanReadAllGroupMessages bool   `json:"can_read_all_group_messages,omitempty"`
	SupportsInlineQueries   bool   `json:"supports_inline_queries,omitempty"`
}

// Chat describes a Telegram-shaped chat projection.
type Chat struct {
	ID       string   `json:"id"`
	Type     ChatType `json:"type"`
	Title    string   `json:"title,omitempty"`
	Username string   `json:"username,omitempty"`
	IsForum  bool     `json:"is_forum,omitempty"`
}

// ChatMember describes a Telegram-shaped chat-member projection.
type ChatMember struct {
	User   User         `json:"user"`
	Status MemberStatus `json:"status"`
}

// ChatMemberUpdated describes one Telegram-shaped chat member update.
type ChatMemberUpdated struct {
	Chat          Chat       `json:"chat"`
	From          User       `json:"from"`
	Date          int64      `json:"date"`
	OldChatMember ChatMember `json:"old_chat_member"`
	NewChatMember ChatMember `json:"new_chat_member"`
}

// InlineKeyboardButton describes one Telegram-shaped inline keyboard button.
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// InlineKeyboardMarkup describes a Telegram-shaped inline keyboard layout.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// File describes one bot-visible file projection.
type File struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id,omitempty"`
	FileSize     uint64 `json:"file_size,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
}

// PhotoSize describes one Telegram-shaped photo size projection.
type PhotoSize struct {
	File
	Width  uint32 `json:"width"`
	Height uint32 `json:"height"`
}

// Document describes one Telegram-shaped document projection.
type Document struct {
	File
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// Video describes one Telegram-shaped video projection.
type Video struct {
	File
	Width    uint32 `json:"width,omitempty"`
	Height   uint32 `json:"height,omitempty"`
	Duration int    `json:"duration,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// Animation describes one Telegram-shaped animation projection.
type Animation struct {
	File
	Width    uint32 `json:"width,omitempty"`
	Height   uint32 `json:"height,omitempty"`
	Duration int    `json:"duration,omitempty"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// Audio describes one Telegram-shaped audio projection.
type Audio struct {
	File
	Duration int    `json:"duration,omitempty"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// VideoNote describes one Telegram-shaped video note projection.
type VideoNote struct {
	File
	Length   uint32 `json:"length,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

// Voice describes one Telegram-shaped voice projection.
type Voice struct {
	File
	Duration int    `json:"duration,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// Sticker describes one Telegram-shaped sticker projection.
type Sticker struct {
	File
	Width    uint32 `json:"width,omitempty"`
	Height   uint32 `json:"height,omitempty"`
	Emoji    string `json:"emoji,omitempty"`
	SetName  string `json:"set_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// Location describes one Telegram-shaped location projection.
type Location struct {
	Longitude            float64 `json:"longitude"`
	Latitude             float64 `json:"latitude"`
	HorizontalAccuracy   float64 `json:"horizontal_accuracy,omitempty"`
	LivePeriod           int     `json:"live_period,omitempty"`
	Heading              int     `json:"heading,omitempty"`
	ProximityAlertRadius int     `json:"proximity_alert_radius,omitempty"`
}

// Contact describes one Telegram-shaped contact projection.
type Contact struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name,omitempty"`
	UserID      string `json:"user_id,omitempty"`
	VCard       string `json:"vcard,omitempty"`
}

// PollOption describes one Telegram-shaped poll option projection.
type PollOption struct {
	Text       string `json:"text"`
	VoterCount int    `json:"voter_count,omitempty"`
}

// Poll describes one Telegram-shaped poll projection.
type Poll struct {
	ID                    string       `json:"id"`
	Question              string       `json:"question"`
	Options               []PollOption `json:"options"`
	TotalVoterCount       int          `json:"total_voter_count"`
	IsClosed              bool         `json:"is_closed"`
	IsAnonymous           bool         `json:"is_anonymous"`
	Type                  string       `json:"type,omitempty"`
	AllowsMultipleAnswers bool         `json:"allows_multiple_answers,omitempty"`
}

// InlineQuery describes one Telegram-shaped inline query update.
type InlineQuery struct {
	ID       string `json:"id"`
	From     User   `json:"from"`
	Query    string `json:"query"`
	Offset   string `json:"offset,omitempty"`
	ChatType string `json:"chat_type,omitempty"`
}

// InputTextMessageContent describes text content for one inline result.
type InputTextMessageContent struct {
	MessageText string `json:"message_text"`
}

// InlineQueryResult describes one supported inline result shape.
type InlineQueryResult struct {
	Type                string                   `json:"type"`
	ID                  string                   `json:"id"`
	Title               string                   `json:"title,omitempty"`
	Description         string                   `json:"description,omitempty"`
	Caption             string                   `json:"caption,omitempty"`
	InputMessageContent *InputTextMessageContent `json:"input_message_content,omitempty"`
	ReplyMarkup         *InlineKeyboardMarkup    `json:"reply_markup,omitempty"`
	PhotoURL            string                   `json:"photo_url,omitempty"`
	AudioURL            string                   `json:"audio_url,omitempty"`
	DocumentURL         string                   `json:"document_url,omitempty"`
	GIFURL              string                   `json:"gif_url,omitempty"`
	Mpeg4URL            string                   `json:"mpeg4_url,omitempty"`
	VideoURL            string                   `json:"video_url,omitempty"`
	MimeType            string                   `json:"mime_type,omitempty"`
	ThumbURL            string                   `json:"thumb_url,omitempty"`
}

// ChosenInlineResult describes one Telegram-shaped chosen inline result update.
type ChosenInlineResult struct {
	ResultID        string `json:"result_id"`
	From            User   `json:"from"`
	Query           string `json:"query"`
	InlineMessageID string `json:"inline_message_id,omitempty"`
}

// Message describes a Telegram-shaped message projection.
type Message struct {
	MessageID       string                `json:"message_id"`
	MessageThreadID string                `json:"message_thread_id,omitempty"`
	Date            int64                 `json:"date"`
	EditDate        int64                 `json:"edit_date,omitempty"`
	Chat            Chat                  `json:"chat"`
	From            *User                 `json:"from,omitempty"`
	Text            string                `json:"text,omitempty"`
	Caption         string                `json:"caption,omitempty"`
	Photo           []PhotoSize           `json:"photo,omitempty"`
	Document        *Document             `json:"document,omitempty"`
	Video           *Video                `json:"video,omitempty"`
	Animation       *Animation            `json:"animation,omitempty"`
	Audio           *Audio                `json:"audio,omitempty"`
	VideoNote       *VideoNote            `json:"video_note,omitempty"`
	Voice           *Voice                `json:"voice,omitempty"`
	Sticker         *Sticker              `json:"sticker,omitempty"`
	Location        *Location             `json:"location,omitempty"`
	Contact         *Contact              `json:"contact,omitempty"`
	Poll            *Poll                 `json:"poll,omitempty"`
	Venue           *Venue                `json:"venue,omitempty"`
	Dice            *Dice                 `json:"dice,omitempty"`
	ReplyMarkup     *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	ReplyToMessage  *Message              `json:"reply_to_message,omitempty"`
}

// CallbackQuery describes one Telegram-shaped callback query.
type CallbackQuery struct {
	ID              string   `json:"id"`
	From            User     `json:"from"`
	Message         *Message `json:"message,omitempty"`
	ChatInstance    string   `json:"chat_instance"`
	Data            string   `json:"data,omitempty"`
	InlineMessageID string   `json:"inline_message_id,omitempty"`
}

// Update describes a Telegram-shaped update payload.
type Update struct {
	UpdateID           int64               `json:"update_id"`
	Message            *Message            `json:"message,omitempty"`
	EditedMessage      *Message            `json:"edited_message,omitempty"`
	ChannelPost        *Message            `json:"channel_post,omitempty"`
	EditedChannelPost  *Message            `json:"edited_channel_post,omitempty"`
	CallbackQuery      *CallbackQuery      `json:"callback_query,omitempty"`
	InlineQuery        *InlineQuery        `json:"inline_query,omitempty"`
	ChosenInlineResult *ChosenInlineResult `json:"chosen_inline_result,omitempty"`
	ChatMember         *ChatMemberUpdated  `json:"chat_member,omitempty"`
	MyChatMember       *ChatMemberUpdated  `json:"my_chat_member,omitempty"`
}

// Webhook stores one bot webhook configuration row.
type Webhook struct {
	BotAccountID     string
	URL              string
	SecretToken      string
	AllowedUpdates   []UpdateType
	MaxConnections   int
	LastErrorMessage string
	LastErrorAt      time.Time
	LastSuccessAt    time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// QueueEntry stores one queued bot update.
type QueueEntry struct {
	UpdateID      int64
	BotAccountID  string
	EventID       string
	UpdateType    UpdateType
	Payload       Update
	Attempts      int
	NextAttemptAt time.Time
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Cursor stores one worker sequence watermark.
type Cursor struct {
	Name         string
	LastSequence uint64
	UpdatedAt    time.Time
}

// Callback stores one pending or answered callback query state.
type Callback struct {
	ID              string
	BotAccountID    string
	FromAccountID   string
	ConversationID  string
	MessageID       string
	MessageThreadID string
	ChatInstance    string
	Data            string
	AnsweredText    string
	AnsweredURL     string
	ShowAlert       bool
	CacheTime       int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	AnsweredAt      time.Time
}

// InlineQueryState stores one pending or answered inline query state.
type InlineQueryState struct {
	ID            string
	BotAccountID  string
	FromAccountID string
	Query         string
	Offset        string
	ChatType      string
	Answered      bool
	Results       []InlineQueryResult
	CacheTime     int
	IsPersonal    bool
	NextOffset    string
	SwitchPMText  string
	SwitchPMParam string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	AnsweredAt    time.Time
}

// WebhookInfo describes the bot-visible webhook state.
type WebhookInfo struct {
	URL                  string   `json:"url"`
	HasCustomCertificate bool     `json:"has_custom_certificate"`
	PendingUpdateCount   int      `json:"pending_update_count"`
	LastErrorDate        int64    `json:"last_error_date,omitempty"`
	LastErrorMessage     string   `json:"last_error_message,omitempty"`
	MaxConnections       int      `json:"max_connections,omitempty"`
	AllowedUpdates       []string `json:"allowed_updates,omitempty"`
}

func (w Webhook) normalize(now time.Time) (Webhook, error) {
	w.BotAccountID = strings.TrimSpace(w.BotAccountID)
	w.URL = strings.TrimSpace(w.URL)
	w.SecretToken = strings.TrimSpace(w.SecretToken)
	w.AllowedUpdates = uniqueUpdateTypes(w.AllowedUpdates)
	if w.BotAccountID == "" {
		return Webhook{}, ErrInvalidInput
	}
	if w.MaxConnections <= 0 {
		w.MaxConnections = 40
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now.UTC()
	}
	if w.UpdatedAt.IsZero() {
		w.UpdatedAt = w.CreatedAt
	}

	return w, nil
}

func (q QueueEntry) normalize(now time.Time) (QueueEntry, error) {
	q.BotAccountID = strings.TrimSpace(q.BotAccountID)
	q.EventID = strings.TrimSpace(q.EventID)
	if q.BotAccountID == "" || q.EventID == "" || q.UpdateType == UpdateTypeUnspecified {
		return QueueEntry{}, ErrInvalidInput
	}
	if q.Attempts < 0 {
		q.Attempts = 0
	}
	if q.NextAttemptAt.IsZero() {
		q.NextAttemptAt = now.UTC()
	}
	if q.CreatedAt.IsZero() {
		q.CreatedAt = now.UTC()
	}
	if q.UpdatedAt.IsZero() {
		q.UpdatedAt = q.CreatedAt
	}

	return q, nil
}

func (c Cursor) normalize(now time.Time) (Cursor, error) {
	c.Name = strings.TrimSpace(strings.ToLower(c.Name))
	if c.Name == "" {
		return Cursor{}, ErrInvalidInput
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now.UTC()
	}

	return c, nil
}

func (m InlineKeyboardMarkup) normalize() (InlineKeyboardMarkup, error) {
	if len(m.InlineKeyboard) == 0 {
		return InlineKeyboardMarkup{}, ErrInvalidInput
	}

	result := InlineKeyboardMarkup{
		InlineKeyboard: make([][]InlineKeyboardButton, 0, len(m.InlineKeyboard)),
	}
	for _, row := range m.InlineKeyboard {
		if len(row) == 0 {
			continue
		}
		nextRow := make([]InlineKeyboardButton, 0, len(row))
		for _, button := range row {
			button.Text = strings.TrimSpace(button.Text)
			button.CallbackData = strings.TrimSpace(button.CallbackData)
			button.URL = strings.TrimSpace(button.URL)
			if button.Text == "" {
				return InlineKeyboardMarkup{}, ErrInvalidInput
			}
			if (button.CallbackData == "") == (button.URL == "") {
				return InlineKeyboardMarkup{}, ErrInvalidInput
			}
			nextRow = append(nextRow, button)
		}
		if len(nextRow) > 0 {
			result.InlineKeyboard = append(result.InlineKeyboard, nextRow)
		}
	}
	if len(result.InlineKeyboard) == 0 {
		return InlineKeyboardMarkup{}, ErrInvalidInput
	}

	return result, nil
}

func (c Callback) normalize(now time.Time) (Callback, error) {
	c.ID = strings.TrimSpace(c.ID)
	c.BotAccountID = strings.TrimSpace(c.BotAccountID)
	c.FromAccountID = strings.TrimSpace(c.FromAccountID)
	c.ConversationID = strings.TrimSpace(c.ConversationID)
	c.MessageID = strings.TrimSpace(c.MessageID)
	c.MessageThreadID = strings.TrimSpace(c.MessageThreadID)
	c.ChatInstance = strings.TrimSpace(c.ChatInstance)
	c.Data = strings.TrimSpace(c.Data)
	c.AnsweredText = strings.TrimSpace(c.AnsweredText)
	c.AnsweredURL = strings.TrimSpace(c.AnsweredURL)
	if c.ID == "" || c.BotAccountID == "" || c.FromAccountID == "" || c.ConversationID == "" || c.MessageID == "" || c.ChatInstance == "" {
		return Callback{}, ErrInvalidInput
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now.UTC()
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = c.CreatedAt
	}
	if c.CacheTime < 0 {
		return Callback{}, ErrInvalidInput
	}
	if c.AnsweredText == "" && c.AnsweredURL == "" && c.ShowAlert {
		return Callback{}, ErrInvalidInput
	}

	return c, nil
}

func (q InlineQueryState) normalize(now time.Time) (InlineQueryState, error) {
	q.ID = strings.TrimSpace(q.ID)
	q.BotAccountID = strings.TrimSpace(q.BotAccountID)
	q.FromAccountID = strings.TrimSpace(q.FromAccountID)
	q.Query = strings.TrimSpace(q.Query)
	q.Offset = strings.TrimSpace(q.Offset)
	q.ChatType = strings.TrimSpace(q.ChatType)
	q.NextOffset = strings.TrimSpace(q.NextOffset)
	q.SwitchPMText = strings.TrimSpace(q.SwitchPMText)
	q.SwitchPMParam = strings.TrimSpace(q.SwitchPMParam)
	if q.ID == "" || q.BotAccountID == "" || q.FromAccountID == "" {
		return InlineQueryState{}, ErrInvalidInput
	}
	if q.CacheTime < 0 {
		return InlineQueryState{}, ErrInvalidInput
	}
	for index, result := range q.Results {
		result.Type = strings.TrimSpace(result.Type)
		result.ID = strings.TrimSpace(result.ID)
		result.Title = strings.TrimSpace(result.Title)
		result.Description = strings.TrimSpace(result.Description)
		result.Caption = strings.TrimSpace(result.Caption)
		result.PhotoURL = strings.TrimSpace(result.PhotoURL)
		result.AudioURL = strings.TrimSpace(result.AudioURL)
		result.DocumentURL = strings.TrimSpace(result.DocumentURL)
		result.GIFURL = strings.TrimSpace(result.GIFURL)
		result.Mpeg4URL = strings.TrimSpace(result.Mpeg4URL)
		result.VideoURL = strings.TrimSpace(result.VideoURL)
		result.MimeType = strings.TrimSpace(result.MimeType)
		result.ThumbURL = strings.TrimSpace(result.ThumbURL)
		if result.InputMessageContent != nil {
			result.InputMessageContent.MessageText = strings.TrimSpace(result.InputMessageContent.MessageText)
		}
		if result.Type == "" {
			result.Type = "article"
		}
		if !validInlineResult(result) {
			return InlineQueryState{}, ErrInvalidInput
		}
		if result.ReplyMarkup != nil {
			markup, err := result.ReplyMarkup.normalize()
			if err != nil {
				return InlineQueryState{}, ErrInvalidInput
			}
			result.ReplyMarkup = &markup
		}
		q.Results[index] = result
	}
	if q.Answered && len(q.Results) == 0 {
		return InlineQueryState{}, ErrInvalidInput
	}
	if q.CreatedAt.IsZero() {
		q.CreatedAt = now.UTC()
	}
	if q.UpdatedAt.IsZero() {
		q.UpdatedAt = q.CreatedAt
	}
	if q.Answered && q.AnsweredAt.IsZero() {
		q.AnsweredAt = q.UpdatedAt
	}

	return q, nil
}

func validInlineResult(result InlineQueryResult) bool {
	if result.ID == "" {
		return false
	}

	switch result.Type {
	case "article":
		return result.Title != "" && result.InputMessageContent != nil && result.InputMessageContent.MessageText != ""
	case "photo":
		return result.PhotoURL != ""
	case "audio":
		return result.Title != "" && result.AudioURL != ""
	case "document":
		return result.Title != "" && result.DocumentURL != ""
	case "gif":
		return result.GIFURL != ""
	case "mpeg4_gif":
		return result.Mpeg4URL != ""
	case "video":
		return result.Title != "" && result.VideoURL != ""
	default:
		return false
	}
}
