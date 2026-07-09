package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AddMembers adds or re-activates conversation members.
func (s *Service) AddMembers(ctx context.Context, params AddMembersParams) ([]ConversationMember, error) {
	if err := s.validateContext(ctx, "add members"); err != nil {
		return nil, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	params.InvitedByAccountID = strings.TrimSpace(params.InvitedByAccountID)
	params.AccountIDs = uniqueIDs(params.AccountIDs)
	role := normalizeMemberRole(params.Role)
	if role == MemberRoleUnspecified {
		role = MemberRoleMember
	}
	if params.ConversationID == "" || params.ActorAccountID == "" || len(params.AccountIDs) == 0 {
		return nil, ErrInvalidInput
	}
	if role == MemberRoleOwner {
		return nil, ErrInvalidInput
	}
	if params.InvitedByAccountID == "" {
		params.InvitedByAccountID = params.ActorAccountID
	}

	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = s.currentTime()
	}

	var savedMembers []ConversationMember
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
		if !canAddMembers(actorMember, params.ActorAccountID, policy, role) {
			return ErrForbidden
		}

		savedMembers = make([]ConversationMember, 0, len(params.AccountIDs))
		changed := false
		for _, accountID := range params.AccountIDs {
			if accountID == conversationRow.OwnerAccountID {
				return ErrConflict
			}

			member, exists, err := upsertConversationMember(ctx, tx, conversationRow.ID, accountID)
			if err != nil {
				return err
			}
			if exists && isActiveMember(member) && member.Role == role && member.InvitedByAccountID == params.InvitedByAccountID {
				savedMembers = append(savedMembers, member)
				continue
			}

			member.Role = role
			member.Banned = false
			member.LeftAt = time.Time{}
			member.InvitedByAccountID = params.InvitedByAccountID
			if !exists || !isActiveMember(member) {
				member.JoinedAt = createdAt
			}

			member, err = tx.SaveConversationMember(ctx, member)
			if err != nil {
				return fmt.Errorf("save member %s in %s: %w", accountID, conversationRow.ID, err)
			}
			savedMembers = append(savedMembers, member)
			changed = true
		}

		if !changed {
			return nil
		}
		eventMetadata := map[string]string{
			"change":       "added",
			"member_count": fmt.Sprintf("%d", len(savedMembers)),
		}
		if len(savedMembers) == 1 {
			eventMetadata["target_account_id"] = savedMembers[0].AccountID
		}
		eventMetadata["target_account_ids"] = joinMemberAccountIDs(savedMembers)
		if _, _, err := s.appendConversationEvent(ctx, tx, conversationRow, params.ActorAccountID, "", EventTypeConversationMembers, "members", map[string]string{
			"change":             eventMetadata["change"],
			"member_count":       eventMetadata["member_count"],
			"target_account_id":  eventMetadata["target_account_id"],
			"target_account_ids": eventMetadata["target_account_ids"],
		}, createdAt); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.indexConversationByID(ctx, params.ConversationID)
	return savedMembers, nil
}

// RemoveMembers marks one or more members as having left the conversation.
func (s *Service) RemoveMembers(ctx context.Context, params RemoveMembersParams) (uint32, error) {
	if err := s.validateContext(ctx, "remove members"); err != nil {
		return 0, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	params.AccountIDs = uniqueIDs(params.AccountIDs)
	if params.ConversationID == "" || params.ActorAccountID == "" || len(params.AccountIDs) == 0 {
		return 0, ErrInvalidInput
	}

	removedAt := params.RemovedAt
	if removedAt.IsZero() {
		removedAt = s.currentTime()
	}

	var removed uint32
	err := s.runTx(ctx, func(tx Store) error {
		conversationRow, actorMember, err := s.loadConversationActor(ctx, tx, params.ConversationID, params.ActorAccountID)
		if err != nil {
			return err
		}
		if conversationRow.Kind == ConversationKindSavedMessages {
			return ErrForbidden
		}

		for _, accountID := range params.AccountIDs {
			member, err := tx.ConversationMemberByConversationAndAccount(ctx, params.ConversationID, accountID)
			if err != nil {
				if err == ErrNotFound {
					continue
				}
				return fmt.Errorf("load member %s in %s: %w", accountID, params.ConversationID, err)
			}
			if !canRemoveMember(actorMember, member, params.ActorAccountID, conversationRow.OwnerAccountID) {
				return ErrForbidden
			}
			if !member.LeftAt.IsZero() && !member.Banned {
				continue
			}

			member.LeftAt = removedAt
			member.Banned = false
			member.Muted = false
			if _, err := tx.SaveConversationMember(ctx, member); err != nil {
				return fmt.Errorf("remove member %s in %s: %w", accountID, params.ConversationID, err)
			}
			removed++
		}

		if removed == 0 {
			return nil
		}
		targetIDs := make([]string, 0, len(params.AccountIDs))
		for _, accountID := range params.AccountIDs {
			targetIDs = append(targetIDs, accountID)
		}
		eventMetadata := map[string]string{
			"change":       "removed",
			"member_count": fmt.Sprintf("%d", removed),
		}
		if len(targetIDs) == 1 {
			eventMetadata["target_account_id"] = targetIDs[0]
		}
		eventMetadata["target_account_ids"] = strings.Join(targetIDs, ",")
		if _, _, err := s.appendConversationEvent(ctx, tx, conversationRow, params.ActorAccountID, "", EventTypeConversationMembers, "members", map[string]string{
			"change":             eventMetadata["change"],
			"member_count":       eventMetadata["member_count"],
			"target_account_id":  eventMetadata["target_account_id"],
			"target_account_ids": eventMetadata["target_account_ids"],
		}, removedAt); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	s.indexConversationByID(ctx, params.ConversationID)
	return removed, nil
}

// UpdateMemberRole changes the role of an active conversation member.
func (s *Service) UpdateMemberRole(ctx context.Context, params UpdateMemberRoleParams) (ConversationMember, error) {
	if err := s.validateContext(ctx, "update member role"); err != nil {
		return ConversationMember{}, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	params.TargetAccountID = strings.TrimSpace(params.TargetAccountID)
	params.Role = normalizeMemberRole(params.Role)
	if params.ConversationID == "" || params.ActorAccountID == "" || params.TargetAccountID == "" || params.Role == MemberRoleUnspecified {
		return ConversationMember{}, ErrInvalidInput
	}
	if params.Role == MemberRoleOwner || params.TargetAccountID == params.ActorAccountID {
		return ConversationMember{}, ErrForbidden
	}

	updatedAt := params.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = s.currentTime()
	}

	var saved ConversationMember
	err := s.runTx(ctx, func(tx Store) error {
		conversationRow, actorMember, err := s.loadConversationActor(ctx, tx, params.ConversationID, params.ActorAccountID)
		if err != nil {
			return err
		}
		if conversationRow.Kind == ConversationKindDirect || conversationRow.Kind == ConversationKindSavedMessages {
			return ErrForbidden
		}

		targetMember, err := tx.ConversationMemberByConversationAndAccount(ctx, params.ConversationID, params.TargetAccountID)
		if err != nil {
			return fmt.Errorf("load target member %s in %s: %w", params.TargetAccountID, params.ConversationID, err)
		}
		if !isActiveMember(targetMember) || targetMember.AccountID == conversationRow.OwnerAccountID {
			return ErrForbidden
		}
		if !canUpdateMemberRole(actorMember, targetMember, params.ActorAccountID, params.Role) {
			return ErrForbidden
		}
		if targetMember.Role == params.Role {
			saved = targetMember
			return nil
		}

		previousRole := targetMember.Role
		targetMember.Role = params.Role
		saved, err = tx.SaveConversationMember(ctx, targetMember)
		if err != nil {
			return fmt.Errorf("save target member %s in %s: %w", params.TargetAccountID, params.ConversationID, err)
		}
		if _, _, err := s.appendConversationEvent(ctx, tx, conversationRow, params.ActorAccountID, "", EventTypeConversationMembers, "members", map[string]string{
			"change":            "role_updated",
			"target_account_id": params.TargetAccountID,
			"role":              string(params.Role),
			"previous_role":     string(previousRole),
		}, updatedAt); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return ConversationMember{}, err
	}

	return saved, nil
}

func joinMemberAccountIDs(members []ConversationMember) string {
	values := make([]string, 0, len(members))
	for _, member := range members {
		accountID := strings.TrimSpace(member.AccountID)
		if accountID == "" {
			continue
		}
		values = append(values, accountID)
	}

	return strings.Join(values, ",")
}

func normalizeMemberRole(role MemberRole) MemberRole {
	switch strings.ToLower(strings.TrimSpace(string(role))) {
	case string(MemberRoleOwner):
		return MemberRoleOwner
	case string(MemberRoleAdmin):
		return MemberRoleAdmin
	case string(MemberRoleMember):
		return MemberRoleMember
	case string(MemberRoleGuest):
		return MemberRoleGuest
	default:
		return MemberRoleUnspecified
	}
}

func canAddMembers(actor ConversationMember, actorAccountID string, policy ModerationPolicy, targetRole MemberRole) bool {
	if actor.AccountID != actorAccountID || !isActiveMember(actor) {
		return false
	}
	if policy.OnlyAdminsCanAddMembers && !canModerateRole(actor.Role) {
		return false
	}
	if targetRole == MemberRoleAdmin {
		return canModerateRole(actor.Role)
	}

	return true
}

func canRemoveMember(actor ConversationMember, target ConversationMember, actorAccountID string, ownerAccountID string) bool {
	if actor.AccountID != actorAccountID || !isActiveMember(actor) || !isActiveMember(target) {
		return false
	}
	if target.AccountID == ownerAccountID {
		return false
	}
	if target.AccountID == actorAccountID {
		return actor.Role != MemberRoleOwner
	}
	if actor.Role == MemberRoleOwner {
		return true
	}
	if actor.Role != MemberRoleAdmin {
		return false
	}

	return target.Role != MemberRoleOwner && target.Role != MemberRoleAdmin
}

func canUpdateMemberRole(actor ConversationMember, target ConversationMember, actorAccountID string, role MemberRole) bool {
	if actor.AccountID != actorAccountID || !isActiveMember(actor) || !isActiveMember(target) {
		return false
	}
	switch actor.Role {
	case MemberRoleOwner:
		return target.Role != MemberRoleOwner && role != MemberRoleOwner
	case MemberRoleAdmin:
		if target.Role == MemberRoleOwner || target.Role == MemberRoleAdmin || role == MemberRoleAdmin || role == MemberRoleOwner {
			return false
		}
		return true
	default:
		return false
	}
}

func canModerateRole(role MemberRole) bool {
	return role == MemberRoleOwner || role == MemberRoleAdmin
}

func upsertConversationMember(
	ctx context.Context,
	store Store,
	conversationID string,
	accountID string,
) (ConversationMember, bool, error) {
	member, err := store.ConversationMemberByConversationAndAccount(ctx, conversationID, accountID)
	if err == nil {
		return member, true, nil
	}
	if err != ErrNotFound {
		return ConversationMember{}, false, fmt.Errorf("load member %s in %s: %w", accountID, conversationID, err)
	}

	return ConversationMember{
		ConversationID: conversationID,
		AccountID:      accountID,
	}, false, nil
}
