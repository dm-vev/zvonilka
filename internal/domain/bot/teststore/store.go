package teststore

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

// NewMemoryStore builds an in-memory bot store for tests.
func NewMemoryStore() bot.Store {
	return &memoryStore{
		webhooksByBot:  make(map[string]bot.Webhook),
		updatesByID:    make(map[int64]bot.QueueEntry),
		updateIDsByBot: make(map[string][]int64),
		updateIDByKey:  make(map[string]int64),
		cursorsByName:  make(map[string]bot.Cursor),
		nextUpdateID:   1,
	}
}

type memoryStore struct {
	mu sync.RWMutex

	webhooksByBot  map[string]bot.Webhook
	updatesByID    map[int64]bot.QueueEntry
	updateIDsByBot map[string][]int64
	updateIDByKey  map[string]int64
	cursorsByName  map[string]bot.Cursor
	nextUpdateID   int64
}

type txStore struct {
	*memoryStore
}

func (s *memoryStore) WithinTx(_ context.Context, fn func(bot.Store) error) error {
	if s == nil || fn == nil {
		return bot.ErrInvalidInput
	}

	s.mu.RLock()
	snapshot := s.cloneLocked()
	s.mu.RUnlock()

	tx := &txStore{memoryStore: snapshot}
	if err := fn(tx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.replaceLocked(snapshot)

	return nil
}

func (s *memoryStore) SaveWebhook(_ context.Context, webhook bot.Webhook) (bot.Webhook, error) {
	value, err := bot.NormalizeWebhook(webhook, time.Now().UTC())
	if err != nil {
		return bot.Webhook{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.webhooksByBot[value.BotAccountID] = cloneWebhook(value)

	return cloneWebhook(value), nil
}

func (s *memoryStore) WebhookByBotAccountID(_ context.Context, botAccountID string) (bot.Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.webhooksByBot[botAccountID]
	if !ok {
		return bot.Webhook{}, bot.ErrNotFound
	}

	return cloneWebhook(value), nil
}

func (s *memoryStore) ListWebhooks(_ context.Context) ([]bot.Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]bot.Webhook, 0, len(s.webhooksByBot))
	for _, value := range s.webhooksByBot {
		result = append(result, cloneWebhook(value))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].BotAccountID < result[j].BotAccountID
	})

	return result, nil
}

func (s *memoryStore) DeleteWebhook(_ context.Context, botAccountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.webhooksByBot[botAccountID]; !ok {
		return bot.ErrNotFound
	}
	delete(s.webhooksByBot, botAccountID)

	return nil
}

func (s *memoryStore) SaveUpdate(_ context.Context, entry bot.QueueEntry) (bot.QueueEntry, error) {
	value, err := bot.NormalizeQueueEntry(entry, time.Now().UTC())
	if err != nil {
		return bot.QueueEntry{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := updateKey(value.BotAccountID, value.EventID, value.UpdateType)
	if existingID, ok := s.updateIDByKey[key]; ok {
		return cloneEntry(s.updatesByID[existingID]), nil
	}

	value.UpdateID = s.nextUpdateID
	s.nextUpdateID++
	s.updatesByID[value.UpdateID] = cloneEntry(value)
	s.updateIDByKey[key] = value.UpdateID
	s.updateIDsByBot[value.BotAccountID] = append(s.updateIDsByBot[value.BotAccountID], value.UpdateID)

	return cloneEntry(value), nil
}

func (s *memoryStore) PendingUpdates(
	_ context.Context,
	botAccountID string,
	offset int64,
	allowed []bot.UpdateType,
	before time.Time,
	limit int,
) ([]bot.QueueEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allowedSet := make(map[bot.UpdateType]struct{}, len(allowed))
	for _, value := range allowed {
		if value == bot.UpdateTypeUnspecified {
			continue
		}
		allowedSet[value] = struct{}{}
	}

	ids := append([]int64(nil), s.updateIDsByBot[botAccountID]...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	result := make([]bot.QueueEntry, 0, len(ids))
	for _, updateID := range ids {
		if updateID < offset {
			continue
		}
		entry, ok := s.updatesByID[updateID]
		if !ok {
			continue
		}
		if len(allowedSet) > 0 {
			if _, ok := allowedSet[entry.UpdateType]; !ok {
				continue
			}
		}
		if !before.IsZero() && entry.NextAttemptAt.After(before) {
			continue
		}
		result = append(result, cloneEntry(entry))
		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result, nil
}

func (s *memoryStore) DeleteUpdatesBefore(_ context.Context, botAccountID string, offset int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := s.updateIDsByBot[botAccountID]
	filtered := ids[:0]
	for _, updateID := range ids {
		if updateID >= offset {
			filtered = append(filtered, updateID)
			continue
		}
		entry, ok := s.updatesByID[updateID]
		if !ok {
			continue
		}
		delete(s.updatesByID, updateID)
		delete(s.updateIDByKey, updateKey(entry.BotAccountID, entry.EventID, entry.UpdateType))
	}
	s.updateIDsByBot[botAccountID] = append([]int64(nil), filtered...)

	return nil
}

func (s *memoryStore) DeleteUpdate(_ context.Context, botAccountID string, updateID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.updatesByID[updateID]
	if !ok || entry.BotAccountID != botAccountID {
		return bot.ErrNotFound
	}
	delete(s.updatesByID, updateID)
	delete(s.updateIDByKey, updateKey(entry.BotAccountID, entry.EventID, entry.UpdateType))
	s.deleteBotUpdateIDLocked(botAccountID, updateID)

	return nil
}

func (s *memoryStore) DeleteAllUpdates(_ context.Context, botAccountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, updateID := range s.updateIDsByBot[botAccountID] {
		entry, ok := s.updatesByID[updateID]
		if !ok {
			continue
		}
		delete(s.updatesByID, updateID)
		delete(s.updateIDByKey, updateKey(entry.BotAccountID, entry.EventID, entry.UpdateType))
	}
	delete(s.updateIDsByBot, botAccountID)

	return nil
}

func (s *memoryStore) PendingUpdateCount(_ context.Context, botAccountID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.updateIDsByBot[botAccountID]), nil
}

func (s *memoryStore) RetryUpdate(_ context.Context, params bot.RetryParams) (bot.QueueEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.updatesByID[params.UpdateID]
	if !ok || entry.BotAccountID != params.BotAccountID {
		return bot.QueueEntry{}, bot.ErrNotFound
	}

	entry.Attempts++
	entry.NextAttemptAt = params.NextAttemptAt.UTC()
	entry.LastError = params.LastError
	entry.UpdatedAt = params.AttemptedAt.UTC()
	s.updatesByID[entry.UpdateID] = cloneEntry(entry)

	return cloneEntry(entry), nil
}

func (s *memoryStore) SaveCursor(_ context.Context, cursor bot.Cursor) (bot.Cursor, error) {
	value, err := bot.NormalizeCursor(cursor, time.Now().UTC())
	if err != nil {
		return bot.Cursor{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.cursorsByName[value.Name]; ok && value.LastSequence <= existing.LastSequence {
		return cloneCursor(existing), nil
	}
	s.cursorsByName[value.Name] = cloneCursor(value)

	return cloneCursor(value), nil
}

func (s *memoryStore) CursorByName(_ context.Context, name string) (bot.Cursor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.cursorsByName[name]
	if !ok {
		return bot.Cursor{}, bot.ErrNotFound
	}

	return cloneCursor(value), nil
}

func (s *memoryStore) cloneLocked() *memoryStore {
	clone := NewMemoryStore().(*memoryStore)
	clone.nextUpdateID = s.nextUpdateID
	for key, value := range s.webhooksByBot {
		clone.webhooksByBot[key] = cloneWebhook(value)
	}
	for key, value := range s.updatesByID {
		clone.updatesByID[key] = cloneEntry(value)
	}
	for key, values := range s.updateIDsByBot {
		clone.updateIDsByBot[key] = append([]int64(nil), values...)
	}
	for key, value := range s.updateIDByKey {
		clone.updateIDByKey[key] = value
	}
	for key, value := range s.cursorsByName {
		clone.cursorsByName[key] = cloneCursor(value)
	}

	return clone
}

func (s *memoryStore) replaceLocked(snapshot *memoryStore) {
	s.webhooksByBot = snapshot.webhooksByBot
	s.updatesByID = snapshot.updatesByID
	s.updateIDsByBot = snapshot.updateIDsByBot
	s.updateIDByKey = snapshot.updateIDByKey
	s.cursorsByName = snapshot.cursorsByName
	s.nextUpdateID = snapshot.nextUpdateID
}

func (s *memoryStore) deleteBotUpdateIDLocked(botAccountID string, updateID int64) {
	ids := s.updateIDsByBot[botAccountID]
	filtered := ids[:0]
	for _, id := range ids {
		if id != updateID {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 {
		delete(s.updateIDsByBot, botAccountID)
		return
	}
	s.updateIDsByBot[botAccountID] = append([]int64(nil), filtered...)
}

func updateKey(botAccountID string, eventID string, updateType bot.UpdateType) string {
	return botAccountID + ":" + eventID + ":" + string(updateType)
}

func cloneWebhook(value bot.Webhook) bot.Webhook {
	value.AllowedUpdates = append([]bot.UpdateType(nil), value.AllowedUpdates...)
	return value
}

func cloneEntry(value bot.QueueEntry) bot.QueueEntry {
	value.Payload = cloneUpdate(value.Payload)
	return value
}

func cloneUpdate(value bot.Update) bot.Update {
	if value.Message != nil {
		message := cloneMessage(*value.Message)
		value.Message = &message
	}
	if value.EditedMessage != nil {
		message := cloneMessage(*value.EditedMessage)
		value.EditedMessage = &message
	}
	if value.ChannelPost != nil {
		message := cloneMessage(*value.ChannelPost)
		value.ChannelPost = &message
	}
	if value.EditedChannelPost != nil {
		message := cloneMessage(*value.EditedChannelPost)
		value.EditedChannelPost = &message
	}
	return value
}

func cloneMessage(value bot.Message) bot.Message {
	value.Photo = append([]bot.PhotoSize(nil), value.Photo...)
	if value.Document != nil {
		document := *value.Document
		value.Document = &document
	}
	if value.Video != nil {
		video := *value.Video
		value.Video = &video
	}
	if value.Voice != nil {
		voice := *value.Voice
		value.Voice = &voice
	}
	if value.Sticker != nil {
		sticker := *value.Sticker
		value.Sticker = &sticker
	}
	if value.ReplyToMessage != nil {
		reply := cloneMessage(*value.ReplyToMessage)
		value.ReplyToMessage = &reply
	}
	return value
}

func cloneCursor(value bot.Cursor) bot.Cursor {
	return value
}
