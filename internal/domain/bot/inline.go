package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// TriggerInlineQuery enqueues one inline query update for a bot account.
func (s *Service) TriggerInlineQuery(ctx context.Context, params TriggerInlineQueryParams) (InlineQuery, error) {
	if err := s.validateContext(ctx, "trigger inline query"); err != nil {
		return InlineQuery{}, err
	}

	params.BotAccountID = strings.TrimSpace(params.BotAccountID)
	params.FromAccountID = strings.TrimSpace(params.FromAccountID)
	params.Query = strings.TrimSpace(params.Query)
	params.Offset = strings.TrimSpace(params.Offset)
	params.ChatType = strings.TrimSpace(params.ChatType)
	if params.BotAccountID == "" || params.FromAccountID == "" {
		return InlineQuery{}, ErrInvalidInput
	}

	botAccount, err := s.identity.AccountByID(ctx, params.BotAccountID)
	if err != nil {
		return InlineQuery{}, mapIdentityError(err)
	}
	if botAccount.Kind != identity.AccountKindBot || botAccount.Status != identity.AccountStatusActive {
		return InlineQuery{}, ErrForbidden
	}

	actor, err := s.identity.AccountByID(ctx, params.FromAccountID)
	if err != nil {
		return InlineQuery{}, mapIdentityError(err)
	}
	if actor.Status != identity.AccountStatusActive {
		return InlineQuery{}, ErrForbidden
	}

	queryID, err := newID("inq")
	if err != nil {
		return InlineQuery{}, err
	}
	now := s.currentTime()
	inlineQuery := InlineQuery{
		ID:       queryID,
		From:     userFromAccount(actor),
		Query:    params.Query,
		Offset:   params.Offset,
		ChatType: params.ChatType,
	}
	state := InlineQueryState{
		ID:            queryID,
		BotAccountID:  botAccount.ID,
		FromAccountID: actor.ID,
		Query:         params.Query,
		Offset:        params.Offset,
		ChatType:      params.ChatType,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	err = s.runTx(ctx, func(tx Store) error {
		if _, err := tx.SaveInlineQuery(ctx, state); err != nil {
			return fmt.Errorf("save inline query %s: %w", state.ID, err)
		}
		if _, err := tx.SaveUpdate(ctx, QueueEntry{
			BotAccountID:  botAccount.ID,
			EventID:       state.ID,
			UpdateType:    UpdateTypeInlineQuery,
			Payload:       Update{InlineQuery: &inlineQuery},
			NextAttemptAt: now,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return fmt.Errorf("save inline query update %s: %w", state.ID, err)
		}

		return nil
	})
	if err != nil {
		return InlineQuery{}, err
	}

	return inlineQuery, nil
}

// AnswerInlineQuery stores one answer for a pending inline query.
func (s *Service) AnswerInlineQuery(ctx context.Context, params AnswerInlineQueryParams) error {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return err
	}

	params.InlineQueryID = strings.TrimSpace(params.InlineQueryID)
	params.NextOffset = strings.TrimSpace(params.NextOffset)
	params.SwitchPMText = strings.TrimSpace(params.SwitchPMText)
	params.SwitchPMParam = strings.TrimSpace(params.SwitchPMParam)
	if params.InlineQueryID == "" || len(params.Results) == 0 {
		return ErrInvalidInput
	}

	query, err := s.store.InlineQueryByID(ctx, params.InlineQueryID)
	if err != nil {
		return fmt.Errorf("load inline query %s: %w", params.InlineQueryID, err)
	}
	if query.BotAccountID != account.ID {
		return ErrForbidden
	}

	now := s.currentTime()
	query.Answered = true
	query.Results = params.Results
	query.CacheTime = params.CacheTime
	query.IsPersonal = params.IsPersonal
	query.NextOffset = params.NextOffset
	query.SwitchPMText = params.SwitchPMText
	query.SwitchPMParam = params.SwitchPMParam
	query.AnsweredAt = now
	query.UpdatedAt = now

	if _, err := s.store.AnswerInlineQuery(ctx, query); err != nil {
		return fmt.Errorf("answer inline query %s: %w", query.ID, err)
	}

	return nil
}
