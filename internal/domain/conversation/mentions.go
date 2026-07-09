package conversation

func validateMentionTargets(members []ConversationMember, mentionAccountIDs []string) error {
	if len(mentionAccountIDs) == 0 {
		return nil
	}

	allowed := make(map[string]struct{}, len(members))
	for _, member := range members {
		if !isActiveMember(member) {
			continue
		}
		allowed[member.AccountID] = struct{}{}
	}

	for _, mentionAccountID := range mentionAccountIDs {
		if _, ok := allowed[mentionAccountID]; !ok {
			return ErrForbidden
		}
	}

	return nil
}
