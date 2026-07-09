package conversation

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// CreateInvite creates one reusable invite for a conversation.
func (s *Service) CreateInvite(ctx context.Context, params CreateInviteParams) (ConversationInvite, error) {
	if err := s.validateContext(ctx, "create invite"); err != nil {
		return ConversationInvite{}, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	params.AllowedRoles = normalizeInviteRoles(params.AllowedRoles)
	if params.ConversationID == "" || params.ActorAccountID == "" {
		return ConversationInvite{}, ErrInvalidInput
	}
	if len(params.AllowedRoles) == 0 {
		params.AllowedRoles = []MemberRole{MemberRoleMember}
	}

	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = s.currentTime()
	}

	var saved ConversationInvite
	err := s.runTx(ctx, func(tx Store) error {
		conversationRow, actorMember, err := s.loadConversationActor(ctx, tx, params.ConversationID, params.ActorAccountID)
		if err != nil {
			return err
		}
		if conversationRow.Kind == ConversationKindDirect || conversationRow.Kind == ConversationKindSavedMessages {
			return ErrForbidden
		}

		policy, err := s.policyForConversation(ctx, tx, conversationRow, "")
		if err != nil {
			return err
		}
		if !canAddMembers(actorMember, params.ActorAccountID, policy, inviteHighestRole(params.AllowedRoles)) {
			return ErrForbidden
		}

		inviteID, err := newID("inv")
		if err != nil {
			return fmt.Errorf("generate invite id: %w", err)
		}
		code, err := randomToken(18)
		if err != nil {
			return fmt.Errorf("generate invite code: %w", err)
		}

		saved, err = tx.SaveConversationInvite(ctx, ConversationInvite{
			ID:                 inviteID,
			ConversationID:     conversationRow.ID,
			Code:               code,
			CreatedByAccountID: params.ActorAccountID,
			AllowedRoles:       params.AllowedRoles,
			ExpiresAt:          params.ExpiresAt,
			MaxUses:            params.MaxUses,
			CreatedAt:          createdAt,
			UpdatedAt:          createdAt,
		})
		if err != nil {
			return fmt.Errorf("save invite for conversation %s: %w", conversationRow.ID, err)
		}
		if _, _, err := s.appendConversationEvent(ctx, tx, conversationRow, params.ActorAccountID, "", EventTypeConversationUpdated, "invite", map[string]string{
			"change":    "created",
			"invite_id": saved.ID,
		}, createdAt); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return ConversationInvite{}, err
	}

	return saved, nil
}

// ListInvites returns the active and revoked invites visible to the caller.
func (s *Service) ListInvites(ctx context.Context, params ListInvitesParams) ([]ConversationInvite, error) {
	if err := s.validateContext(ctx, "list invites"); err != nil {
		return nil, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.ConversationID == "" || params.AccountID == "" {
		return nil, ErrInvalidInput
	}

	conversationRow, actorMember, err := s.loadConversationActor(ctx, s.store, params.ConversationID, params.AccountID)
	if err != nil {
		return nil, err
	}
	if conversationRow.Kind == ConversationKindDirect || conversationRow.Kind == ConversationKindSavedMessages {
		return nil, ErrForbidden
	}

	policy, err := s.policyForConversation(ctx, s.store, conversationRow, "")
	if err != nil {
		return nil, err
	}
	if !canAddMembers(actorMember, params.AccountID, policy, MemberRoleMember) {
		return nil, ErrForbidden
	}

	invites, err := s.store.ConversationInvitesByConversationID(ctx, params.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("list invites for conversation %s: %w", params.ConversationID, err)
	}

	return invites, nil
}

// RevokeInvite marks one invite as no longer usable.
func (s *Service) RevokeInvite(ctx context.Context, params RevokeInviteParams) (ConversationInvite, error) {
	if err := s.validateContext(ctx, "revoke invite"); err != nil {
		return ConversationInvite{}, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.InviteID = strings.TrimSpace(params.InviteID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	if params.ConversationID == "" || params.InviteID == "" || params.ActorAccountID == "" {
		return ConversationInvite{}, ErrInvalidInput
	}

	revokedAt := params.RevokedAt
	if revokedAt.IsZero() {
		revokedAt = s.currentTime()
	}

	var saved ConversationInvite
	err := s.runTx(ctx, func(tx Store) error {
		conversationRow, actorMember, err := s.loadConversationActor(ctx, tx, params.ConversationID, params.ActorAccountID)
		if err != nil {
			return err
		}
		if conversationRow.Kind == ConversationKindDirect || conversationRow.Kind == ConversationKindSavedMessages {
			return ErrForbidden
		}

		policy, err := s.policyForConversation(ctx, tx, conversationRow, "")
		if err != nil {
			return err
		}
		if !canAddMembers(actorMember, params.ActorAccountID, policy, MemberRoleMember) {
			return ErrForbidden
		}

		saved, err = tx.ConversationInviteByConversationAndID(ctx, params.ConversationID, params.InviteID)
		if err != nil {
			return fmt.Errorf("load invite %s in %s: %w", params.InviteID, params.ConversationID, err)
		}
		if saved.Revoked {
			return nil
		}

		saved.Revoked = true
		saved.RevokedAt = revokedAt
		saved.UpdatedAt = revokedAt
		saved, err = tx.SaveConversationInvite(ctx, saved)
		if err != nil {
			return fmt.Errorf("save revoked invite %s in %s: %w", params.InviteID, params.ConversationID, err)
		}
		if _, _, err := s.appendConversationEvent(ctx, tx, conversationRow, params.ActorAccountID, "", EventTypeConversationUpdated, "invite", map[string]string{
			"change":    "revoked",
			"invite_id": saved.ID,
		}, revokedAt); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return ConversationInvite{}, err
	}

	return saved, nil
}

func normalizeInviteRoles(roles []MemberRole) []MemberRole {
	if len(roles) == 0 {
		return nil
	}

	seen := make(map[MemberRole]struct{}, len(roles))
	normalized := make([]MemberRole, 0, len(roles))
	for _, role := range roles {
		role = normalizeMemberRole(role)
		if role == MemberRoleUnspecified || role == MemberRoleOwner {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		normalized = append(normalized, role)
	}
	slices.Sort(normalized)
	return normalized
}

func inviteHighestRole(roles []MemberRole) MemberRole {
	if slices.Contains(roles, MemberRoleAdmin) {
		return MemberRoleAdmin
	}
	if slices.Contains(roles, MemberRoleMember) {
		return MemberRoleMember
	}
	if slices.Contains(roles, MemberRoleGuest) {
		return MemberRoleGuest
	}
	return MemberRoleUnspecified
}
