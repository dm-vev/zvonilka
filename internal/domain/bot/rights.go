package bot

import (
	"context"
	"fmt"
	"time"
)

// AdminRights describes one Telegram-shaped default admin-rights set.
type AdminRights struct {
	IsAnonymous             bool `json:"is_anonymous"`
	CanManageChat           bool `json:"can_manage_chat"`
	CanDeleteMessages       bool `json:"can_delete_messages"`
	CanManageVideoChats     bool `json:"can_manage_video_chats"`
	CanRestrictMembers      bool `json:"can_restrict_members"`
	CanPromoteMembers       bool `json:"can_promote_members"`
	CanChangeInfo           bool `json:"can_change_info"`
	CanInviteUsers          bool `json:"can_invite_users"`
	CanPostMessages         bool `json:"can_post_messages"`
	CanEditMessages         bool `json:"can_edit_messages"`
	CanPinMessages          bool `json:"can_pin_messages"`
	CanPostStories          bool `json:"can_post_stories"`
	CanEditStories          bool `json:"can_edit_stories"`
	CanDeleteStories        bool `json:"can_delete_stories"`
	CanManageTopics         bool `json:"can_manage_topics"`
	CanManageDirectMessages bool `json:"can_manage_direct_messages"`
	CanManageTags           bool `json:"can_manage_tags"`
}

// AdminRightsState stores one default administrator-rights row.
type AdminRightsState struct {
	BotAccountID string
	ForChannels  bool
	Rights       AdminRights
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SetRightsParams describes one setMyDefaultAdministratorRights request.
type SetRightsParams struct {
	BotToken    string
	ForChannels bool
	Rights      *AdminRights
}

// GetRightsParams describes one getMyDefaultAdministratorRights request.
type GetRightsParams struct {
	BotToken    string
	ForChannels bool
}

// SetMyDefaultAdministratorRights stores default administrator rights.
func (s *Service) SetMyDefaultAdministratorRights(ctx context.Context, params SetRightsParams) error {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return err
	}
	if params.Rights == nil {
		err = s.store.DeleteRights(ctx, account.ID, params.ForChannels)
		if err == ErrNotFound {
			return nil
		}
		if err != nil {
			return fmt.Errorf("delete bot default administrator rights: %w", err)
		}

		return nil
	}

	now := s.currentTime()
	_, err = s.store.SaveRights(ctx, AdminRightsState{
		BotAccountID: account.ID,
		ForChannels:  params.ForChannels,
		Rights:       *params.Rights,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return fmt.Errorf("save bot default administrator rights: %w", err)
	}

	return nil
}

// GetMyDefaultAdministratorRights resolves default administrator rights.
func (s *Service) GetMyDefaultAdministratorRights(ctx context.Context, params GetRightsParams) (AdminRights, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return AdminRights{}, err
	}

	state, err := s.store.RightsByScope(ctx, account.ID, params.ForChannels)
	if err == nil {
		return state.Rights, nil
	}
	if err == ErrNotFound {
		return AdminRights{}, nil
	}

	return AdminRights{}, fmt.Errorf("load bot default administrator rights: %w", err)
}
