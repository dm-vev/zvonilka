package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const defaultAPIBaseURL = "https://zvonilka.geartech.club"

var supportedMethods = []string{
	"getMe", "getUpdates", "setWebhook", "deleteWebhook", "getWebhookInfo",
	"sendMessage", "sendPhoto", "sendDocument", "sendVideo", "sendAnimation",
	"sendAudio", "sendVideoNote", "sendLocation", "sendVenue", "sendContact",
	"sendPoll", "sendGame", "sendDice", "sendVoice", "sendSticker",
	"forwardMessage", "forwardMessages", "copyMessage", "copyMessages",
	"editMessageText", "editMessageCaption", "editMessageMedia",
	"editMessageLiveLocation", "editMessageReplyMarkup", "stopPoll", "deleteMessage",
	"getChat", "getChatMember", "setGameScore", "getGameHighScores", "getFile",
	"setMyCommands", "getMyCommands", "deleteMyCommands", "setMyName", "getMyName",
	"setMyDescription", "getMyDescription", "setMyShortDescription", "getMyShortDescription",
	"setChatMenuButton", "getChatMenuButton", "setMyDefaultAdministratorRights",
	"getMyDefaultAdministratorRights", "answerCallbackQuery", "answerInlineQuery",
}

var upstreamBotAPI95Gaps = []string{
	"sendMessageDraft",
	"setChatMemberTag",
}

type demoBot struct {
	client      *botAPIClient
	pollTimeout int
	webhookURL  string
	botUsername string
	logger      *log.Logger
}

type updateEnvelope struct {
	ID            int64          `json:"update_id"`
	Message       map[string]any `json:"message"`
	EditedMessage map[string]any `json:"edited_message"`
	ChannelPost   map[string]any `json:"channel_post"`
	CallbackQuery map[string]any `json:"callback_query"`
	InlineQuery   map[string]any `json:"inline_query"`
}

func main() {
	loadDotEnv(".env")

	token := strings.TrimSpace(os.Getenv("ZVONILKA_BOT_TOKEN"))
	if token == "" {
		log.Fatal("ZVONILKA_BOT_TOKEN is required")
	}

	baseURL := strings.TrimSpace(os.Getenv("ZVONILKA_BOT_API_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultAPIBaseURL
	}

	pollTimeout := envInt("ZVONILKA_BOT_POLL_TIMEOUT", 5)
	if pollTimeout < 1 || pollTimeout > 50 {
		pollTimeout = 5
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	bot := &demoBot{
		client:      newBotAPIClient(baseURL, token),
		pollTimeout: pollTimeout,
		webhookURL:  strings.TrimSpace(os.Getenv("ZVONILKA_BOT_WEBHOOK_URL")),
		logger:      log.New(os.Stderr, "ZvonilkaDemoBot: ", log.LstdFlags|log.Lmicroseconds),
	}
	if err := bot.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		bot.logger.Fatal(err)
	}
}

func (b *demoBot) run(ctx context.Context) error {
	var me map[string]any
	if err := b.client.call(ctx, "getMe", nil, &me); err != nil {
		return fmt.Errorf("authenticate bot: %w", err)
	}
	b.botUsername = stringValue(me["username"])
	b.logger.Printf("connected as @%s", b.botUsername)

	if err := b.configure(ctx); err != nil {
		b.logger.Printf("optional profile setup failed: %v", err)
	}

	var offset int64
	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var updates []updateEnvelope
		err := b.client.call(ctx, "getUpdates", map[string]any{
			"offset":          offset,
			"limit":           100,
			"timeout":         b.pollTimeout,
			"allowed_updates": []string{"message", "edited_message", "channel_post", "callback_query", "inline_query"},
		}, &updates)
		if err != nil {
			b.logger.Printf("poll failed: %v; retrying in %s", err, backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff < 16*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second

		for _, update := range updates {
			if update.ID >= offset {
				offset = update.ID + 1
			}
			if err := b.handleUpdate(ctx, update); err != nil {
				b.logger.Printf("update %d failed: %v", update.ID, err)
			}
		}
	}
}

func (b *demoBot) configure(ctx context.Context) error {
	commands := []map[string]any{
		{"command": "start", "description": "Open the complete Zvonilka demo"},
		{"command": "help", "description": "Show help"},
		{"command": "methods", "description": "List supported Bot API methods"},
		{"command": "demo_all", "description": "Run safe API demonstrations"},
		{"command": "demo_core", "description": "Messages, callbacks and editing"},
		{"command": "demo_media", "description": "Locations, polls and media guide"},
		{"command": "demo_rich", "description": "Location, venue, contact, poll, dice and game"},
		{"command": "demo_profile", "description": "Profile, commands and menu"},
		{"command": "demo_webhook", "description": "Webhook lifecycle"},
		{"command": "media", "description": "Send a media file by file_id"},
	}
	if err := b.client.call(ctx, "setMyCommands", map[string]any{"commands": commands}, nil); err != nil {
		return err
	}
	_ = b.client.call(ctx, "setMyName", map[string]any{"name": "Zvonilka Demo Bot"}, nil)
	_ = b.client.call(ctx, "setMyDescription", map[string]any{
		"description": "A live demonstration of the Zvonilka Bot API compatibility surface.",
	}, nil)
	_ = b.client.call(ctx, "setMyShortDescription", map[string]any{
		"short_description": "Zvonilka Bot API demo",
	}, nil)
	_ = b.client.call(ctx, "setChatMenuButton", map[string]any{
		"menu_button": map[string]any{"type": "commands"},
	}, nil)
	return nil
}

func (b *demoBot) handleUpdate(ctx context.Context, update updateEnvelope) error {
	switch {
	case update.CallbackQuery != nil:
		return b.handleCallback(ctx, update.CallbackQuery)
	case update.InlineQuery != nil:
		return b.handleInlineQuery(ctx, update.InlineQuery)
	case update.Message != nil:
		return b.handleMessage(ctx, update.Message)
	case update.EditedMessage != nil:
		chatID := chatID(update.EditedMessage)
		if chatID == "" {
			return nil
		}
		return b.sendText(ctx, chatID, "Получено edited_message: сообщение изменено пользователем.", nil)
	case update.ChannelPost != nil:
		chatID := chatID(update.ChannelPost)
		if chatID == "" {
			return nil
		}
		return b.sendText(ctx, chatID, "Получено channel_post: канал опубликовал сообщение.", nil)
	default:
		return nil
	}
}

func (b *demoBot) handleMessage(ctx context.Context, message map[string]any) error {
	chat := nestedMap(message, "chat")
	chatID := stringValue(chat["id"])
	if chatID == "" {
		return nil
	}

	text := strings.TrimSpace(stringValue(message["text"]))
	if text == "" {
		return b.sendText(ctx, chatID, "Вижу сообщение без текста. Используй /start, чтобы открыть демо.", nil)
	}
	command, args := parseCommand(text)
	if command == "" {
		return b.sendText(ctx, chatID, "Я получил: "+text+"\n\nИспользуй /start для меню демонстрации.", mainMenu())
	}

	userID := stringValue(nestedMap(message, "from")["id"])
	switch command {
	case "start", "help", "demo":
		return b.sendText(ctx, chatID, welcomeText(), mainMenu())
	case "methods":
		return b.sendText(ctx, chatID, methodReport(), nil)
	case "demo_all":
		return b.demoAll(ctx, chatID, userID)
	case "demo_core":
		return b.demoCore(ctx, chatID, userID)
	case "demo_media":
		return b.demoMediaGuide(ctx, chatID)
	case "demo_rich":
		return b.demoRich(ctx, chatID)
	case "demo_profile":
		return b.demoProfile(ctx, chatID)
	case "demo_webhook":
		return b.demoWebhook(ctx, chatID)
	case "media":
		return b.demoMedia(ctx, chatID, args)
	case "callback":
		return b.sendText(ctx, chatID, "Нажми кнопку в меню: callback_query будет обработан автоматически.", mainMenu())
	default:
		return b.sendText(ctx, chatID, "Неизвестная команда /"+command+". Открой /help.", mainMenu())
	}
}

func (b *demoBot) handleCallback(ctx context.Context, callback map[string]any) error {
	callbackID := stringValue(callback["id"])
	if callbackID != "" {
		if err := b.client.call(ctx, "answerCallbackQuery", map[string]any{
			"callback_query_id": callbackID,
			"text":              "Запрос обработан ZvonilkaDemoBot",
		}, nil); err != nil {
			b.logger.Printf("answer callback failed: %v", err)
		}
	}

	message := nestedMap(callback, "message")
	chatID := chatID(message)
	if chatID == "" {
		return nil
	}
	switch stringValue(callback["data"]) {
	case "demo:core":
		return b.demoCore(ctx, chatID, stringValue(nestedMap(message, "from")["id"]))
	case "demo:media":
		return b.demoMediaGuide(ctx, chatID)
	case "demo:profile":
		return b.demoProfile(ctx, chatID)
	case "demo:webhook":
		return b.demoWebhook(ctx, chatID)
	case "demo:all":
		return b.demoAll(ctx, chatID, stringValue(nestedMap(message, "from")["id"]))
	default:
		return b.sendText(ctx, chatID, "Неизвестная callback-кнопка.", mainMenu())
	}
}

func (b *demoBot) handleInlineQuery(ctx context.Context, query map[string]any) error {
	queryID := stringValue(query["id"])
	if queryID == "" {
		return nil
	}
	return b.client.call(ctx, "answerInlineQuery", map[string]any{
		"inline_query_id": queryID,
		"cache_time":      1,
		"is_personal":     true,
		"results": []map[string]any{{
			"type":        "article",
			"id":          "zvonilka-demo",
			"title":       "Zvonilka Demo Bot",
			"description": "Inline result from the Bot API demo",
			"input_message_content": map[string]any{
				"message_text": "Inline query работает: ZvonilkaDemoBot ответил через answerInlineQuery.",
			},
		}},
	}, nil)
}

func (b *demoBot) demoAll(ctx context.Context, chatID, userID string) error {
	var me map[string]any
	var chat map[string]any
	var webhook map[string]any
	var commands []map[string]any

	results := []string{}
	results = appendCallResult(results, "getMe", b.client.call(ctx, "getMe", nil, &me))
	results = appendCallResult(results, "getChat", b.client.call(ctx, "getChat", map[string]any{"chat_id": chatID}, &chat))
	results = appendCallResult(results, "getWebhookInfo", b.client.call(ctx, "getWebhookInfo", nil, &webhook))
	results = appendCallResult(results, "getMyCommands", b.client.call(ctx, "getMyCommands", nil, &commands))
	results = appendCallResult(results, "getChatMenuButton", b.client.call(ctx, "getChatMenuButton", nil, nil))
	results = appendCallResult(results, "getMyName", b.client.call(ctx, "getMyName", nil, nil))
	results = appendCallResult(results, "getMyDescription", b.client.call(ctx, "getMyDescription", nil, nil))
	results = appendCallResult(results, "getMyShortDescription", b.client.call(ctx, "getMyShortDescription", nil, nil))
	results = appendCallResult(results, "getMyDefaultAdministratorRights", b.client.call(ctx, "getMyDefaultAdministratorRights", nil, nil))
	if userID != "" {
		results = appendCallResult(results, "getChatMember", b.client.call(ctx, "getChatMember", map[string]any{
			"chat_id": chatID,
			"user_id": userID,
		}, nil))
	}

	text := "<b>Zvonilka Bot API demo</b>\n\n" + strings.Join(results, "\n")
	text += "\n\nДля действий с сообщениями открой /demo_core. Для медиа: /media photo &lt;file_id&gt;."
	text += "\nИнлайн: вызови бота в режиме inline. Webhook: задай ZVONILKA_BOT_WEBHOOK_URL."
	return b.sendText(ctx, chatID, text, mainMenu())
}

func (b *demoBot) demoCore(ctx context.Context, chatID, userID string) error {
	var sent map[string]any
	if err := b.client.call(ctx, "sendMessage", map[string]any{
		"chat_id": chatID,
		"text":    "sendMessage: это живое сообщение. Через секунду оно будет отредактировано.",
		"reply_markup": map[string]any{
			"inline_keyboard": [][]map[string]any{{
				{"text": "Нажми callback", "callback_data": "demo:core"},
			}},
		},
	}, &sent); err != nil {
		return err
	}

	messageID := stringValue(sent["message_id"])
	if messageID == "" {
		return b.sendText(ctx, chatID, "sendMessage сработал, но message_id не пришёл.", nil)
	}
	results := []string{"sendMessage: ok"}
	results = appendCallResult(results, "editMessageText", b.client.call(ctx, "editMessageText", map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       "editMessageText: сообщение изменено сервером.",
	}, nil))
	results = appendCallResult(results, "editMessageReplyMarkup", b.client.call(ctx, "editMessageReplyMarkup", map[string]any{
		"chat_id": chatID, "message_id": messageID,
		"reply_markup": map[string]any{"inline_keyboard": [][]map[string]any{{
			{"text": "Новая callback-кнопка", "callback_data": "demo:core"},
		}}},
	}, nil))
	results = appendCallResult(results, "copyMessage", b.client.call(ctx, "copyMessage", map[string]any{
		"chat_id": chatID, "from_chat_id": chatID, "message_id": messageID,
	}, nil))
	results = appendCallResult(results, "forwardMessage", b.client.call(ctx, "forwardMessage", map[string]any{
		"chat_id": chatID, "from_chat_id": chatID, "message_id": messageID,
	}, nil))
	results = appendCallResult(results, "forwardMessages", b.client.call(ctx, "forwardMessages", map[string]any{
		"chat_id": chatID, "from_chat_id": chatID, "message_ids": []string{messageID},
	}, nil))
	results = appendCallResult(results, "copyMessages", b.client.call(ctx, "copyMessages", map[string]any{
		"chat_id": chatID, "from_chat_id": chatID, "message_ids": []string{messageID},
	}, nil))
	if userID != "" {
		results = appendCallResult(results, "getChatMember", b.client.call(ctx, "getChatMember", map[string]any{
			"chat_id": chatID, "user_id": userID,
		}, nil))
	}
	results = appendCallResult(results, "deleteMessage", b.client.call(ctx, "deleteMessage", map[string]any{
		"chat_id": chatID, "message_id": messageID,
	}, nil))
	return b.sendText(ctx, chatID, "<b>Core methods</b>\n\n"+strings.Join(results, "\n")+"\n\nCallback обработается при нажатии кнопки.", mainMenu())
}

func (b *demoBot) demoMediaGuide(ctx context.Context, chatID string) error {
	return b.sendText(ctx, chatID, "<b>Media and rich content</b>\n\n"+
		"/media photo &lt;file_id&gt;\n"+
		"/media document &lt;file_id&gt;\n"+
		"/media video &lt;file_id&gt;\n"+"/media animation &lt;file_id&gt;\n"+"/media audio &lt;file_id&gt;\n"+"/media voice &lt;file_id&gt;\n"+"/media sticker &lt;file_id&gt;\n"+"/media video_note &lt;file_id&gt;\n\n"+"Также /demo_rich отправляет location, venue, contact, poll и dice.\n"+"После poll можно вызвать stopPoll через кнопку в исходном сообщении.", mainMenu())
}

func (b *demoBot) demoMedia(ctx context.Context, chatID string, args []string) error {
	if len(args) == 0 {
		return b.demoMediaGuide(ctx, chatID)
	}
	if args[0] == "rich" {
		return b.demoRich(ctx, chatID)
	}
	if len(args) < 2 {
		return b.sendText(ctx, chatID, "Формат: /media photo &lt;file_id&gt;", nil)
	}

	methodByKind := map[string]string{
		"photo": "sendPhoto", "document": "sendDocument", "video": "sendVideo",
		"animation": "sendAnimation", "audio": "sendAudio", "voice": "sendVoice",
		"sticker": "sendSticker", "video_note": "sendVideoNote",
	}
	method := methodByKind[strings.ToLower(args[0])]
	if method == "" {
		return b.sendText(ctx, chatID, "Неизвестный тип медиа. Открой /demo_media.", nil)
	}
	field := map[string]string{
		"sendPhoto": "photo", "sendDocument": "document", "sendVideo": "video",
		"sendAnimation": "animation", "sendAudio": "audio", "sendVoice": "voice",
		"sendSticker": "sticker", "sendVideoNote": "video_note",
	}[method]
	params := map[string]any{"chat_id": chatID, field: args[1]}
	if len(args) > 2 {
		params["caption"] = strings.Join(args[2:], " ")
	}
	if method == "sendSticker" || method == "sendVideoNote" {
		delete(params, "caption")
	}
	var result map[string]any
	err := b.client.call(ctx, method, params, &result)
	if err != nil {
		return b.sendText(ctx, chatID, method+" failed: "+err.Error(), nil)
	}

	messageID := stringValue(result["message_id"])
	results := []string{method + ": ok (message_id=" + messageID + ")"}
	results = appendCallResult(results, "getFile", b.client.call(ctx, "getFile", map[string]any{"file_id": args[1]}, nil))
	if method != "sendSticker" && method != "sendVideoNote" && messageID != "" {
		results = appendCallResult(results, "editMessageCaption", b.client.call(ctx, "editMessageCaption", map[string]any{
			"chat_id": chatID, "message_id": messageID, "caption": "Caption edited by ZvonilkaDemoBot",
		}, nil))
		results = appendCallResult(results, "editMessageMedia", b.client.call(ctx, "editMessageMedia", map[string]any{
			"chat_id": chatID, "message_id": messageID,
			"media": map[string]any{"type": args[0], "media": args[1], "caption": "Media edited by ZvonilkaDemoBot"},
		}, nil))
	}
	return b.sendText(ctx, chatID, strings.Join(results, "\n"), nil)
}

func (b *demoBot) demoRich(ctx context.Context, chatID string) error {
	var poll map[string]any
	var game map[string]any
	var location map[string]any
	results := []string{}
	results = appendCallResult(results, "sendLocation", b.client.call(ctx, "sendLocation", map[string]any{
		"chat_id": chatID, "latitude": 55.7558, "longitude": 37.6173,
	}, &location))
	if locationID := stringValue(location["message_id"]); locationID != "" {
		results = appendCallResult(results, "editMessageLiveLocation", b.client.call(ctx, "editMessageLiveLocation", map[string]any{
			"chat_id": chatID, "message_id": locationID, "latitude": 55.7560, "longitude": 37.6180,
		}, nil))
	}
	results = appendCallResult(results, "sendVenue", b.client.call(ctx, "sendVenue", map[string]any{
		"chat_id": chatID, "latitude": 55.7558, "longitude": 37.6173,
		"title": "Zvonilka HQ", "address": "Moscow",
	}, nil))
	results = appendCallResult(results, "sendContact", b.client.call(ctx, "sendContact", map[string]any{
		"chat_id": chatID, "phone_number": "+70000000000", "first_name": "Zvonilka",
	}, nil))
	results = appendCallResult(results, "sendDice", b.client.call(ctx, "sendDice", map[string]any{
		"chat_id": chatID, "emoji": "🎲",
	}, nil))
	results = appendCallResult(results, "sendPoll", b.client.call(ctx, "sendPoll", map[string]any{
		"chat_id": chatID, "question": "Which API surface should we test next?",
		"options": []string{"Messages", "Media", "Federation"}, "is_anonymous": false,
	}, &poll))
	if pollMessageID := stringValue(poll["message_id"]); pollMessageID != "" {
		results = appendCallResult(results, "stopPoll", b.client.call(ctx, "stopPoll", map[string]any{
			"chat_id": chatID, "message_id": pollMessageID,
		}, nil))
	}
	results = appendCallResult(results, "sendGame", b.client.call(ctx, "sendGame", map[string]any{
		"chat_id": chatID, "game_short_name": "zvonilka_demo",
	}, &game))
	if gameMessageID := stringValue(game["message_id"]); gameMessageID != "" {
		var me map[string]any
		if err := b.client.call(ctx, "getMe", nil, &me); err != nil {
			results = appendCallResult(results, "getMe for game", err)
		} else {
			gameUserID := stringValue(me["id"])
			results = appendCallResult(results, "setGameScore", b.client.call(ctx, "setGameScore", map[string]any{
				"user_id": gameUserID, "score": 10, "force": true,
				"chat_id": chatID, "message_id": gameMessageID,
			}, nil))
			results = appendCallResult(results, "getGameHighScores", b.client.call(ctx, "getGameHighScores", map[string]any{
				"user_id": gameUserID, "chat_id": chatID, "message_id": gameMessageID,
			}, nil))
		}
	}
	return b.sendText(ctx, chatID, "<b>Rich content</b>\n\n"+strings.Join(results, "\n")+"\n\nДля файлов используй /media &lt;kind&gt; &lt;file_id&gt;.", mainMenu())
}

func (b *demoBot) demoProfile(ctx context.Context, chatID string) error {
	commands := []map[string]any{{"command": "start", "description": "Open demo"}}
	results := []string{}
	results = appendCallResult(results, "setMyCommands", b.client.call(ctx, "setMyCommands", map[string]any{"commands": commands}, nil))
	results = appendCallResult(results, "getMyCommands", b.client.call(ctx, "getMyCommands", nil, nil))
	results = appendCallResult(results, "deleteMyCommands", b.client.call(ctx, "deleteMyCommands", nil, nil))
	results = appendCallResult(results, "setMyCommands", b.client.call(ctx, "setMyCommands", map[string]any{"commands": commands}, nil))
	results = appendCallResult(results, "setMyName", b.client.call(ctx, "setMyName", map[string]any{"name": "Zvonilka Demo Bot"}, nil))
	results = appendCallResult(results, "getMyName", b.client.call(ctx, "getMyName", nil, nil))
	results = appendCallResult(results, "setMyDescription", b.client.call(ctx, "setMyDescription", map[string]any{"description": "Live Zvonilka Bot API demo"}, nil))
	results = appendCallResult(results, "getMyDescription", b.client.call(ctx, "getMyDescription", nil, nil))
	results = appendCallResult(results, "setMyShortDescription", b.client.call(ctx, "setMyShortDescription", map[string]any{"short_description": "Zvonilka demo"}, nil))
	results = appendCallResult(results, "getMyShortDescription", b.client.call(ctx, "getMyShortDescription", nil, nil))
	results = appendCallResult(results, "setChatMenuButton", b.client.call(ctx, "setChatMenuButton", map[string]any{
		"chat_id": chatID, "menu_button": map[string]any{"type": "commands"},
	}, nil))
	results = appendCallResult(results, "getChatMenuButton", b.client.call(ctx, "getChatMenuButton", map[string]any{"chat_id": chatID}, nil))
	results = appendCallResult(results, "getMyDefaultAdministratorRights", b.client.call(ctx, "getMyDefaultAdministratorRights", nil, nil))
	results = appendCallResult(results, "setMyDefaultAdministratorRights", b.client.call(ctx, "setMyDefaultAdministratorRights", map[string]any{
		"rights": map[string]any{},
	}, nil))
	return b.sendText(ctx, chatID, "<b>Profile and bot settings</b>\n\n"+strings.Join(results, "\n"), mainMenu())
}

func (b *demoBot) demoWebhook(ctx context.Context, chatID string) error {
	if b.webhookURL == "" {
		return b.sendText(ctx, chatID, "Webhook URL не задан. Укажи ZVONILKA_BOT_WEBHOOK_URL и перезапусти демо.\n\nLong polling уже работает через getUpdates.", mainMenu())
	}
	results := []string{}
	results = appendCallResult(results, "setWebhook", b.client.call(ctx, "setWebhook", map[string]any{
		"url": b.webhookURL, "secret_token": "zvonilka-demo",
	}, nil))
	results = appendCallResult(results, "getWebhookInfo", b.client.call(ctx, "getWebhookInfo", nil, nil))
	results = appendCallResult(results, "deleteWebhook", b.client.call(ctx, "deleteWebhook", map[string]any{"drop_pending_updates": false}, nil))
	return b.sendText(ctx, chatID, "<b>Webhook lifecycle</b>\n\n"+strings.Join(results, "\n")+"\n\nПосле deleteWebhook бот снова использует long polling.", mainMenu())
}

func (b *demoBot) sendText(ctx context.Context, chatID, text string, markup map[string]any) error {
	params := map[string]any{"chat_id": chatID, "text": text}
	if markup != nil {
		params["reply_markup"] = markup
	}
	return b.client.call(ctx, "sendMessage", params, nil)
}

func mainMenu() map[string]any {
	return map[string]any{"inline_keyboard": [][]map[string]any{
		{{"text": "Все безопасные вызовы", "callback_data": "demo:all"}},
		{{"text": "Messages / callbacks", "callback_data": "demo:core"}},
		{{"text": "Media / rich content", "callback_data": "demo:media"}},
		{{"text": "Profile / menu / commands", "callback_data": "demo:profile"}},
		{{"text": "Webhook lifecycle", "callback_data": "demo:webhook"}},
	}}
}

func welcomeText() string {
	return "<b>ZvonilkaDemoBot</b>\n\n" +
		"Это живая витрина Bot API сервера Zvonilka. Кнопки ниже вызывают реальные RPC/HTTP операции, а ошибки сервера показываются прямо в чате.\n\n" +
		"Команды: /demo_all, /demo_core, /demo_media, /demo_profile, /demo_webhook, /methods."
}

func methodReport() string {
	var builder strings.Builder
	builder.WriteString("<b>Methods exposed by the current Zvonilka server</b>\n\n")
	for index, method := range supportedMethods {
		fmt.Fprintf(&builder, "%02d. %s\n", index+1, method)
	}
	builder.WriteString("\nBot API 9.5 capability gaps in the current server:\n")
	for _, method := range upstreamBotAPI95Gaps {
		builder.WriteString("- " + method + ": server capability missing\n")
	}
	return builder.String()
}

func appendCallResult(results []string, method string, err error) []string {
	if err != nil {
		return append(results, method+": error ("+err.Error()+")")
	}
	return append(results, method+": ok")
}

func parseCommand(text string) (string, []string) {
	parts := strings.Fields(text)
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "/") {
		return "", nil
	}
	command := strings.TrimPrefix(parts[0], "/")
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	return strings.ToLower(command), parts[1:]
}

func chatID(value map[string]any) string {
	return stringValue(nestedMap(value, "chat")["id"])
}

func nestedMap(value map[string]any, key string) map[string]any {
	child, ok := value[key].(map[string]any)
	if !ok {
		return nil
	}
	return child
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case float32:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return fmt.Sprint(typed)
	}
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return value
}

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		if _, exists := os.LookupEnv(strings.TrimSpace(key)); exists {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		_ = os.Setenv(strings.TrimSpace(key), value)
	}
}
