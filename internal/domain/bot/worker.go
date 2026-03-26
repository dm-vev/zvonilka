package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// Worker fans out conversation events into bot updates and webhook deliveries.
type Worker struct {
	service *Service
	client  *http.Client
	now     func() time.Time
}

// NewWorker constructs a bot worker.
func NewWorker(service *Service, client *http.Client) (*Worker, error) {
	if service == nil {
		return nil, ErrInvalidInput
	}
	if client == nil {
		client = &http.Client{Timeout: service.settings.WebhookTimeout}
	}

	return &Worker{
		service: service,
		client:  client,
		now:     func() time.Time { return time.Now().UTC() },
	}, nil
}

// Run polls conversation events and webhook queues.
func (w *Worker) Run(ctx context.Context, logger *slog.Logger) error {
	if ctx == nil || logger == nil {
		return ErrInvalidInput
	}

	ticker := time.NewTicker(w.service.settings.FanoutPollInterval)
	defer ticker.Stop()

	for {
		if err := w.processFanout(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.ErrorContext(ctx, "process bot event fanout", "err", err)
		}
		if err := w.processWebhooks(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.ErrorContext(ctx, "process bot webhooks", "err", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) processFanout(ctx context.Context) error {
	cursor, err := w.service.store.CursorByName(ctx, fanoutWorkerName())
	if err != nil && err != ErrNotFound {
		return fmt.Errorf("load bot worker cursor: %w", err)
	}
	if err == ErrNotFound {
		cursor.Name = fanoutWorkerName()
	}

	events, err := w.service.conversationDB.EventsAfterSequence(
		ctx,
		cursor.LastSequence,
		w.service.settings.FanoutBatchSize,
		nil,
	)
	if err != nil {
		return fmt.Errorf("load conversation events after %d: %w", cursor.LastSequence, err)
	}
	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		now := w.currentTime()
		if err := w.service.runTx(ctx, func(tx Store) error {
			updates, buildErr := w.buildUpdatesForEvent(ctx, event)
			if buildErr != nil {
				return buildErr
			}
			for _, entry := range updates {
				if _, err := tx.SaveUpdate(ctx, entry); err != nil {
					return fmt.Errorf("save bot update for event %s: %w", event.EventID, err)
				}
			}

			cursor.LastSequence = event.Sequence
			cursor.UpdatedAt = now
			if _, err := tx.SaveCursor(ctx, cursor); err != nil {
				return fmt.Errorf("save bot worker cursor: %w", err)
			}

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func (w *Worker) buildUpdatesForEvent(ctx context.Context, event conversation.EventEnvelope) ([]QueueEntry, error) {
	if event.ConversationID == "" {
		return nil, nil
	}

	conv, err := w.service.conversationDB.ConversationByID(ctx, event.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("load conversation %s: %w", event.ConversationID, err)
	}
	updateType := updateTypeForEvent(event.EventType, conv)
	if updateType == UpdateTypeUnspecified {
		return nil, nil
	}

	actor, err := w.service.identity.AccountByID(ctx, event.ActorAccountID)
	if err != nil {
		return nil, nil
	}
	if actor.Kind == identity.AccountKindBot {
		return nil, nil
	}
	members, err := w.service.conversationDB.ConversationMembersByConversationID(ctx, conv.ID)
	if err != nil {
		return nil, fmt.Errorf("load conversation members %s: %w", conv.ID, err)
	}

	var message conversation.Message
	if event.MessageID != "" {
		message, err = w.service.conversations.GetMessage(ctx, conversation.GetMessageParams{
			ConversationID: conv.ID,
			MessageID:      event.MessageID,
			AccountID:      event.ActorAccountID,
		})
		if err != nil && updateType != UpdateTypeChatMember && updateType != UpdateTypeMyChatMember {
			return nil, nil
		}
	}

	result := make([]QueueEntry, 0)
	for _, member := range members {
		if member.AccountID == event.ActorAccountID || member.LeftAt.IsZero() == false || member.Banned {
			continue
		}
		botAccount, err := w.service.identity.AccountByID(ctx, member.AccountID)
		if err != nil {
			continue
		}
		if botAccount.Kind != identity.AccountKindBot || botAccount.Status != identity.AccountStatusActive {
			continue
		}
		if updateType == UpdateTypeChatMember {
			entry, ok, err := w.memberUpdateEntry(ctx, event, conv, members, actor, member, botAccount)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			result = append(result, entry)
			continue
		}
		if !shouldRoute(conv, message, botAccount.ID) {
			continue
		}

		payload, err := w.updatePayload(ctx, botAccount.ID, conv, members, message, event, updateType)
		if err != nil {
			return nil, err
		}
		entry, err := NormalizeQueueEntry(QueueEntry{
			BotAccountID:  botAccount.ID,
			EventID:       event.EventID,
			UpdateType:    updateType,
			Payload:       payload,
			NextAttemptAt: w.currentTime(),
			CreatedAt:     event.CreatedAt,
			UpdatedAt:     event.CreatedAt,
		}, w.currentTime())
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}

	return result, nil
}

func (w *Worker) memberUpdateEntry(
	ctx context.Context,
	event conversation.EventEnvelope,
	conv conversation.Conversation,
	members []conversation.ConversationMember,
	actor identity.Account,
	botMember conversation.ConversationMember,
	botAccount identity.Account,
) (QueueEntry, bool, error) {
	targetAccountID := strings.TrimSpace(event.Metadata["target_account_id"])
	if targetAccountID == "" {
		return QueueEntry{}, false, nil
	}

	targetMember, found := memberByAccountID(members, targetAccountID)
	if !found {
		return QueueEntry{}, false, nil
	}
	targetAccount, err := w.service.identity.AccountByID(ctx, targetAccountID)
	if err != nil {
		return QueueEntry{}, false, nil
	}

	updateType, oldMember, newMember, ok := memberUpdateForEvent(event, botMember, botAccount.ID, targetMember, targetAccount)
	if !ok {
		return QueueEntry{}, false, nil
	}

	chat, err := w.service.chatForConversation(ctx, botAccount.ID, conv, members)
	if err != nil {
		return QueueEntry{}, false, err
	}
	payload := Update{
		ChatMember: &ChatMemberUpdated{
			Chat:          chat,
			From:          userFromAccount(actor),
			Date:          event.CreatedAt.UTC().Unix(),
			OldChatMember: oldMember,
			NewChatMember: newMember,
		},
	}
	if updateType == UpdateTypeMyChatMember {
		payload.MyChatMember = payload.ChatMember
		payload.ChatMember = nil
	}

	entry, err := NormalizeQueueEntry(QueueEntry{
		BotAccountID:  botAccount.ID,
		EventID:       event.EventID + ":" + targetAccountID + ":" + string(updateType),
		UpdateType:    updateType,
		Payload:       payload,
		NextAttemptAt: w.currentTime(),
		CreatedAt:     event.CreatedAt,
		UpdatedAt:     event.CreatedAt,
	}, w.currentTime())
	if err != nil {
		return QueueEntry{}, false, err
	}

	return entry, true, nil
}

func (w *Worker) updatePayload(
	ctx context.Context,
	botAccountID string,
	conv conversation.Conversation,
	members []conversation.ConversationMember,
	message conversation.Message,
	event conversation.EventEnvelope,
	updateType UpdateType,
) (Update, error) {
	message = messageAtEvent(message, event)

	projected, err := w.service.messageForConversation(ctx, botAccountID, conv, members, message, true)
	if err != nil {
		return Update{}, err
	}

	update := Update{}
	switch updateType {
	case UpdateTypeMessage:
		update.Message = &projected
	case UpdateTypeEditedMessage:
		update.EditedMessage = &projected
	case UpdateTypeChannelPost:
		update.ChannelPost = &projected
	case UpdateTypeEditedChannelPost:
		update.EditedChannelPost = &projected
	}

	return update, nil
}

func messageAtEvent(message conversation.Message, event conversation.EventEnvelope) conversation.Message {
	if event.MessageID == "" {
		return message
	}

	switch event.EventType {
	case conversation.EventTypeMessageCreated:
		message.Payload = event.Payload
		message.Metadata = cloneSnapshotMetadata(event.Metadata)
		message.DisableLinkPreviews = metadataBool(event.Metadata["disable_link_previews"], message.DisableLinkPreviews)
		message.EditedAt = time.Time{}
	case conversation.EventTypeMessageEdited:
		message.Payload = event.Payload
		message.Metadata = cloneSnapshotMetadata(event.Metadata)
		message.DisableLinkPreviews = metadataBool(event.Metadata["disable_link_previews"], message.DisableLinkPreviews)
		message.EditedAt = event.CreatedAt
	}

	return message
}

func cloneSnapshotMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		cloned[key] = value
	}

	if len(cloned) == 0 {
		return nil
	}

	return cloned
}

func metadataBool(raw string, fallback bool) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}

	return value
}

func memberUpdateForEvent(
	event conversation.EventEnvelope,
	botMember conversation.ConversationMember,
	botAccountID string,
	targetMember conversation.ConversationMember,
	targetAccount identity.Account,
) (UpdateType, ChatMember, ChatMember, bool) {
	change := strings.TrimSpace(event.Metadata["change"])
	if targetMember.AccountID != botAccountID && !botCanObserveMemberUpdate(botMember.Role) {
		return UpdateTypeUnspecified, ChatMember{}, ChatMember{}, false
	}

	newRestricted := targetMember.Muted
	newProjection := ChatMember{
		User:   userFromAccount(targetAccount),
		Status: memberStatus(targetMember, newRestricted),
	}
	oldMember := targetMember

	switch change {
	case "added":
		oldMember.LeftAt = event.CreatedAt
		oldMember.Banned = false
		oldProjection := ChatMember{
			User:   userFromAccount(targetAccount),
			Status: memberStatus(oldMember, false),
		}
		if targetMember.AccountID == botAccountID {
			return UpdateTypeMyChatMember, oldProjection, newProjection, true
		}
		return UpdateTypeChatMember, oldProjection, newProjection, true
	case "removed":
		oldMember.LeftAt = time.Time{}
		oldProjection := ChatMember{
			User:   userFromAccount(targetAccount),
			Status: memberStatus(oldMember, oldMember.Muted),
		}
		if targetMember.AccountID == botAccountID {
			return UpdateTypeMyChatMember, oldProjection, newProjection, true
		}
		return UpdateTypeChatMember, oldProjection, newProjection, true
	case "role_updated":
		previousRole := conversation.MemberRole(strings.TrimSpace(event.Metadata["previous_role"]))
		if previousRole == conversation.MemberRoleUnspecified {
			return UpdateTypeUnspecified, ChatMember{}, ChatMember{}, false
		}
		oldMember.Role = previousRole
		oldProjection := ChatMember{
			User:   userFromAccount(targetAccount),
			Status: memberStatus(oldMember, oldMember.Muted),
		}
		if targetMember.AccountID == botAccountID {
			return UpdateTypeMyChatMember, oldProjection, newProjection, true
		}
		return UpdateTypeChatMember, oldProjection, newProjection, true
	default:
		return UpdateTypeUnspecified, ChatMember{}, ChatMember{}, false
	}
}

func memberByAccountID(members []conversation.ConversationMember, accountID string) (conversation.ConversationMember, bool) {
	for _, member := range members {
		if member.AccountID == accountID {
			return member, true
		}
	}

	return conversation.ConversationMember{}, false
}

func botCanObserveMemberUpdate(role conversation.MemberRole) bool {
	return role == conversation.MemberRoleOwner || role == conversation.MemberRoleAdmin
}

func (w *Worker) processWebhooks(ctx context.Context) error {
	webhooks, err := w.service.store.ListWebhooks(ctx)
	if err != nil {
		return fmt.Errorf("list bot webhooks: %w", err)
	}

	for _, webhook := range webhooks {
		updates, err := w.service.store.PendingUpdates(
			ctx,
			webhook.BotAccountID,
			0,
			webhook.AllowedUpdates,
			w.currentTime(),
			w.service.settings.WebhookBatchSize,
		)
		if err != nil {
			return fmt.Errorf("load webhook updates for bot %s: %w", webhook.BotAccountID, err)
		}

		for _, entry := range updates {
			if err := w.deliverWebhook(ctx, webhook, entry); err != nil {
				return err
			}
		}
	}

	return nil
}

func (w *Worker) deliverWebhook(ctx context.Context, webhook Webhook, entry QueueEntry) error {
	body, err := json.Marshal(entry.Payload)
	if err != nil {
		return fmt.Errorf("encode webhook update %d: %w", entry.UpdateID, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request for bot %s: %w", webhook.BotAccountID, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if webhook.SecretToken != "" {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", webhook.SecretToken)
	}

	resp, err := w.client.Do(req)
	if err == nil && resp != nil {
		defer resp.Body.Close()
	}

	if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return w.service.runTx(ctx, func(tx Store) error {
			if err := tx.DeleteUpdate(ctx, webhook.BotAccountID, entry.UpdateID); err != nil {
				return fmt.Errorf("acknowledge delivered bot update %d: %w", entry.UpdateID, err)
			}
			_, saveErr := tx.SaveWebhook(ctx, Webhook{
				BotAccountID:   webhook.BotAccountID,
				URL:            webhook.URL,
				SecretToken:    webhook.SecretToken,
				AllowedUpdates: webhook.AllowedUpdates,
				MaxConnections: webhook.MaxConnections,
				LastSuccessAt:  w.currentTime(),
				CreatedAt:      webhook.CreatedAt,
				UpdatedAt:      w.currentTime(),
			})
			if saveErr != nil {
				return fmt.Errorf("save successful webhook state for bot %s: %w", webhook.BotAccountID, saveErr)
			}

			return nil
		})
	}

	lastError := "webhook delivery failed"
	if err != nil {
		lastError = err.Error()
	} else if resp != nil {
		lastError = resp.Status
	}

	return w.service.runTx(ctx, func(tx Store) error {
		_, retryErr := tx.RetryUpdate(ctx, RetryParams{
			BotAccountID:  webhook.BotAccountID,
			UpdateID:      entry.UpdateID,
			AttemptedAt:   w.currentTime(),
			NextAttemptAt: w.nextAttemptAt(entry.Attempts + 1),
			LastError:     lastError,
		})
		if retryErr != nil {
			return fmt.Errorf("retry webhook update %d: %w", entry.UpdateID, retryErr)
		}
		_, saveErr := tx.SaveWebhook(ctx, Webhook{
			BotAccountID:     webhook.BotAccountID,
			URL:              webhook.URL,
			SecretToken:      webhook.SecretToken,
			AllowedUpdates:   webhook.AllowedUpdates,
			MaxConnections:   webhook.MaxConnections,
			LastErrorMessage: lastError,
			LastErrorAt:      w.currentTime(),
			LastSuccessAt:    webhook.LastSuccessAt,
			CreatedAt:        webhook.CreatedAt,
			UpdatedAt:        w.currentTime(),
		})
		if saveErr != nil {
			return fmt.Errorf("save failed webhook state for bot %s: %w", webhook.BotAccountID, saveErr)
		}

		return nil
	})
}

func (w *Worker) nextAttemptAt(attempts int) time.Time {
	backoff := w.service.settings.RetryInitialBackoff
	if backoff <= 0 {
		backoff = DefaultSettings().RetryInitialBackoff
	}
	for i := 1; i < attempts; i++ {
		backoff *= 2
		if backoff >= w.service.settings.RetryMaxBackoff {
			backoff = w.service.settings.RetryMaxBackoff
			break
		}
	}

	return w.currentTime().Add(backoff)
}

func (w *Worker) currentTime() time.Time {
	if w == nil || w.now == nil {
		return time.Now().UTC()
	}

	return w.now().UTC()
}
