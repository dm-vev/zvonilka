package teststore

import (
	"context"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (s *memoryStore) SaveRights(_ context.Context, state bot.AdminRightsState) (bot.AdminRightsState, error) {
	state.BotAccountID = strings.TrimSpace(state.BotAccountID)
	if state.BotAccountID == "" {
		return bot.AdminRightsState{}, bot.ErrInvalidInput
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = time.Now().UTC()
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = state.CreatedAt
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.rightsByKey[rightsKey(state.BotAccountID, state.ForChannels)] = cloneRightsState(state)
	s.version++

	return cloneRightsState(state), nil
}

func (s *memoryStore) RightsByScope(
	_ context.Context,
	botAccountID string,
	forChannels bool,
) (bot.AdminRightsState, error) {
	botAccountID = strings.TrimSpace(botAccountID)
	if botAccountID == "" {
		return bot.AdminRightsState{}, bot.ErrInvalidInput
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.rightsByKey[rightsKey(botAccountID, forChannels)]
	if !ok {
		return bot.AdminRightsState{}, bot.ErrNotFound
	}

	return cloneRightsState(value), nil
}

func (s *memoryStore) DeleteRights(_ context.Context, botAccountID string, forChannels bool) error {
	botAccountID = strings.TrimSpace(botAccountID)
	if botAccountID == "" {
		return bot.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := rightsKey(botAccountID, forChannels)
	if _, ok := s.rightsByKey[key]; !ok {
		return bot.ErrNotFound
	}
	delete(s.rightsByKey, key)
	s.version++

	return nil
}
