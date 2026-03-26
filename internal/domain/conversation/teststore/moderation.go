package teststore

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func moderationPolicyKey(targetKind conversation.ModerationTargetKind, targetID string) string {
	return strings.TrimSpace(string(targetKind)) + "|" + strings.TrimSpace(targetID)
}

func moderationReportKey(reportID string) string {
	return strings.TrimSpace(reportID)
}

func moderationActionKey(actionID string) string {
	return strings.TrimSpace(actionID)
}

func moderationRestrictionKey(targetKind conversation.ModerationTargetKind, targetID string, accountID string) string {
	return moderationPolicyKey(targetKind, targetID) + "|" + strings.TrimSpace(accountID)
}

func moderationRateStateKey(targetKind conversation.ModerationTargetKind, targetID string, accountID string) string {
	return moderationRestrictionKey(targetKind, targetID, accountID)
}

func topicModerationTargetID(conversationID string, topicID string) string {
	return strings.TrimSpace(conversationID) + "\x1f" + strings.TrimSpace(topicID)
}

func (s *memoryStore) SaveModerationPolicy(ctx context.Context, policy conversation.ModerationPolicy) (conversation.ModerationPolicy, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.ModerationPolicy{}, err
	}
	policy.TargetKind = conversation.ModerationTargetKind(strings.TrimSpace(string(policy.TargetKind)))
	policy.TargetID = strings.TrimSpace(policy.TargetID)
	if policy.TargetKind == conversation.ModerationTargetKindUnspecified || policy.TargetID == "" {
		return conversation.ModerationPolicy{}, conversation.ErrInvalidInput
	}
	if policy.CreatedAt.IsZero() || policy.UpdatedAt.IsZero() || policy.UpdatedAt.Before(policy.CreatedAt) {
		return conversation.ModerationPolicy{}, conversation.ErrInvalidInput
	}
	if (policy.AntiSpamWindow == 0) != (policy.AntiSpamBurstLimit == 0) {
		return conversation.ModerationPolicy{}, conversation.ErrInvalidInput
	}

	s.moderationPoliciesByKey[moderationPolicyKey(policy.TargetKind, policy.TargetID)] = policy
	return policy, nil
}

func (s *memoryStore) ModerationPolicyByTarget(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string) (conversation.ModerationPolicy, error) {
	if err := s.validateRead(ctx); err != nil {
		return conversation.ModerationPolicy{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetKind = conversation.ModerationTargetKind(strings.TrimSpace(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" {
		return conversation.ModerationPolicy{}, conversation.ErrNotFound
	}

	policy, ok := s.moderationPoliciesByKey[moderationPolicyKey(targetKind, targetID)]
	if !ok {
		return conversation.ModerationPolicy{}, conversation.ErrNotFound
	}

	return policy, nil
}

func (s *memoryStore) SaveModerationReport(ctx context.Context, report conversation.ModerationReport) (conversation.ModerationReport, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.ModerationReport{}, err
	}
	report.ID = strings.TrimSpace(report.ID)
	report.TargetKind = conversation.ModerationTargetKind(strings.TrimSpace(string(report.TargetKind)))
	report.TargetID = strings.TrimSpace(report.TargetID)
	report.ReporterAccountID = strings.TrimSpace(report.ReporterAccountID)
	report.TargetAccountID = strings.TrimSpace(report.TargetAccountID)
	report.Reason = strings.TrimSpace(report.Reason)
	report.Details = strings.TrimSpace(report.Details)
	report.Resolution = strings.TrimSpace(report.Resolution)
	report.ReviewedByAccountID = strings.TrimSpace(report.ReviewedByAccountID)
	if report.ID == "" || report.TargetKind == conversation.ModerationTargetKindUnspecified || report.TargetID == "" || report.ReporterAccountID == "" || report.Reason == "" {
		return conversation.ModerationReport{}, conversation.ErrInvalidInput
	}
	if report.Status == conversation.ModerationReportStatusUnspecified {
		return conversation.ModerationReport{}, conversation.ErrInvalidInput
	}
	if report.CreatedAt.IsZero() || report.UpdatedAt.IsZero() || report.UpdatedAt.Before(report.CreatedAt) {
		return conversation.ModerationReport{}, conversation.ErrInvalidInput
	}
	switch report.Status {
	case conversation.ModerationReportStatusPending:
		if report.ReviewedByAccountID != "" || !report.ReviewedAt.IsZero() {
			return conversation.ModerationReport{}, conversation.ErrInvalidInput
		}
	case conversation.ModerationReportStatusResolved, conversation.ModerationReportStatusRejected:
		if report.ReviewedByAccountID == "" || report.ReviewedAt.IsZero() {
			return conversation.ModerationReport{}, conversation.ErrInvalidInput
		}
	default:
		return conversation.ModerationReport{}, conversation.ErrInvalidInput
	}

	s.moderationReportsByID[moderationReportKey(report.ID)] = report
	return report, nil
}

func (s *memoryStore) ModerationReportByID(ctx context.Context, reportID string) (conversation.ModerationReport, error) {
	if err := s.validateRead(ctx); err != nil {
		return conversation.ModerationReport{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	reportID = moderationReportKey(reportID)
	if reportID == "" {
		return conversation.ModerationReport{}, conversation.ErrNotFound
	}

	report, ok := s.moderationReportsByID[reportID]
	if !ok {
		return conversation.ModerationReport{}, conversation.ErrNotFound
	}

	return report, nil
}

func (s *memoryStore) ModerationReportsByTarget(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string) ([]conversation.ModerationReport, error) {
	if err := s.validateRead(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetKind = conversation.ModerationTargetKind(strings.TrimSpace(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" {
		return nil, conversation.ErrInvalidInput
	}

	reports := make([]conversation.ModerationReport, 0)
	for _, report := range s.moderationReportsByID {
		if report.TargetKind != targetKind || report.TargetID != targetID {
			continue
		}
		reports = append(reports, report)
	}

	sort.Slice(reports, func(i, j int) bool {
		if reports[i].CreatedAt.Equal(reports[j].CreatedAt) {
			return reports[i].ID < reports[j].ID
		}
		return reports[i].CreatedAt.After(reports[j].CreatedAt)
	})

	return reports, nil
}

func (s *memoryStore) SaveModerationAction(ctx context.Context, action conversation.ModerationAction) (conversation.ModerationAction, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.ModerationAction{}, err
	}
	action.ID = strings.TrimSpace(action.ID)
	action.TargetKind = conversation.ModerationTargetKind(strings.TrimSpace(string(action.TargetKind)))
	action.TargetID = strings.TrimSpace(action.TargetID)
	action.ActorAccountID = strings.TrimSpace(action.ActorAccountID)
	action.TargetAccountID = strings.TrimSpace(action.TargetAccountID)
	action.Reason = strings.TrimSpace(action.Reason)
	if action.ID == "" || action.TargetKind == conversation.ModerationTargetKindUnspecified || action.TargetID == "" || action.ActorAccountID == "" || action.Type == conversation.ModerationActionTypeUnspecified {
		return conversation.ModerationAction{}, conversation.ErrInvalidInput
	}
	if action.CreatedAt.IsZero() {
		return conversation.ModerationAction{}, conversation.ErrInvalidInput
	}
	if _, exists := s.moderationActionsByID[moderationActionKey(action.ID)]; exists {
		return conversation.ModerationAction{}, conversation.ErrConflict
	}

	s.moderationActionsByID[moderationActionKey(action.ID)] = cloneModerationAction(action)
	return cloneModerationAction(action), nil
}

func (s *memoryStore) ModerationActionsByTarget(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string) ([]conversation.ModerationAction, error) {
	if err := s.validateRead(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetKind = conversation.ModerationTargetKind(strings.TrimSpace(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" {
		return nil, conversation.ErrInvalidInput
	}

	actions := make([]conversation.ModerationAction, 0)
	for _, action := range s.moderationActionsByID {
		if action.TargetKind != targetKind || action.TargetID != targetID {
			continue
		}
		actions = append(actions, cloneModerationAction(action))
	}

	sort.Slice(actions, func(i, j int) bool {
		if actions[i].CreatedAt.Equal(actions[j].CreatedAt) {
			return actions[i].ID < actions[j].ID
		}
		return actions[i].CreatedAt.After(actions[j].CreatedAt)
	})

	return actions, nil
}

func (s *memoryStore) SaveModerationRestriction(ctx context.Context, restriction conversation.ModerationRestriction) (conversation.ModerationRestriction, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.ModerationRestriction{}, err
	}
	restriction.TargetKind = conversation.ModerationTargetKind(strings.TrimSpace(string(restriction.TargetKind)))
	restriction.TargetID = strings.TrimSpace(restriction.TargetID)
	restriction.AccountID = strings.TrimSpace(restriction.AccountID)
	restriction.AppliedByAccountID = strings.TrimSpace(restriction.AppliedByAccountID)
	restriction.Reason = strings.TrimSpace(restriction.Reason)
	if restriction.TargetKind == conversation.ModerationTargetKindUnspecified || restriction.TargetID == "" || restriction.AccountID == "" || restriction.State == conversation.ModerationRestrictionStateUnspecified {
		return conversation.ModerationRestriction{}, conversation.ErrInvalidInput
	}
	if restriction.CreatedAt.IsZero() || restriction.UpdatedAt.IsZero() || restriction.UpdatedAt.Before(restriction.CreatedAt) {
		return conversation.ModerationRestriction{}, conversation.ErrInvalidInput
	}
	if !restriction.ExpiresAt.IsZero() && restriction.ExpiresAt.Before(restriction.CreatedAt) {
		return conversation.ModerationRestriction{}, conversation.ErrInvalidInput
	}

	s.moderationRestrictions[moderationRestrictionKey(restriction.TargetKind, restriction.TargetID, restriction.AccountID)] = restriction
	return restriction, nil
}

func (s *memoryStore) ModerationRestrictionByTargetAndAccount(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string, accountID string) (conversation.ModerationRestriction, error) {
	if err := s.validateRead(ctx); err != nil {
		return conversation.ModerationRestriction{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetKind = conversation.ModerationTargetKind(strings.TrimSpace(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	accountID = strings.TrimSpace(accountID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" || accountID == "" {
		return conversation.ModerationRestriction{}, conversation.ErrNotFound
	}

	restriction, ok := s.moderationRestrictions[moderationRestrictionKey(targetKind, targetID, accountID)]
	if !ok {
		return conversation.ModerationRestriction{}, conversation.ErrNotFound
	}

	return restriction, nil
}

func (s *memoryStore) ModerationRestrictionsByTarget(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string) ([]conversation.ModerationRestriction, error) {
	if err := s.validateRead(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetKind = conversation.ModerationTargetKind(strings.TrimSpace(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" {
		return nil, conversation.ErrInvalidInput
	}

	restrictions := make([]conversation.ModerationRestriction, 0)
	for _, restriction := range s.moderationRestrictions {
		if restriction.TargetKind != targetKind || restriction.TargetID != targetID {
			continue
		}
		restrictions = append(restrictions, restriction)
	}

	sort.Slice(restrictions, func(i, j int) bool {
		if restrictions[i].CreatedAt.Equal(restrictions[j].CreatedAt) {
			return restrictions[i].AccountID < restrictions[j].AccountID
		}
		return restrictions[i].CreatedAt.After(restrictions[j].CreatedAt)
	})

	return restrictions, nil
}

func (s *memoryStore) DeleteModerationRestriction(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string, accountID string) error {
	if err := s.validateWrite(ctx); err != nil {
		return err
	}
	targetKind = conversation.ModerationTargetKind(strings.TrimSpace(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	accountID = strings.TrimSpace(accountID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" || accountID == "" {
		return conversation.ErrInvalidInput
	}

	delete(s.moderationRestrictions, moderationRestrictionKey(targetKind, targetID, accountID))
	return nil
}

func (s *memoryStore) SaveModerationRateState(ctx context.Context, state conversation.ModerationRateState) (conversation.ModerationRateState, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.ModerationRateState{}, err
	}
	state.TargetKind = conversation.ModerationTargetKind(strings.TrimSpace(string(state.TargetKind)))
	state.TargetID = strings.TrimSpace(state.TargetID)
	state.AccountID = strings.TrimSpace(state.AccountID)
	if state.TargetKind == conversation.ModerationTargetKindUnspecified || state.TargetID == "" || state.AccountID == "" {
		return conversation.ModerationRateState{}, conversation.ErrInvalidInput
	}
	if state.UpdatedAt.IsZero() {
		return conversation.ModerationRateState{}, conversation.ErrInvalidInput
	}

	s.moderationRateStates[moderationRateStateKey(state.TargetKind, state.TargetID, state.AccountID)] = state
	return state, nil
}

func (s *memoryStore) isShadowed(targetKind conversation.ModerationTargetKind, targetID string, accountID string) bool {
	now := time.Now().UTC()
	for _, restriction := range s.moderationRestrictions {
		if restriction.TargetKind != targetKind || restriction.TargetID != strings.TrimSpace(targetID) || restriction.AccountID != strings.TrimSpace(accountID) {
			continue
		}
		if restriction.State != conversation.ModerationRestrictionStateShadowed {
			continue
		}
		if !restriction.ExpiresAt.IsZero() && !now.Before(restriction.ExpiresAt) {
			continue
		}

		return true
	}

	return false
}

func (s *memoryStore) ModerationRateStateByTargetAndAccount(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string, accountID string) (conversation.ModerationRateState, error) {
	if err := s.validateRead(ctx); err != nil {
		return conversation.ModerationRateState{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetKind = conversation.ModerationTargetKind(strings.TrimSpace(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	accountID = strings.TrimSpace(accountID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" || accountID == "" {
		return conversation.ModerationRateState{}, conversation.ErrNotFound
	}

	state, ok := s.moderationRateStates[moderationRateStateKey(targetKind, targetID, accountID)]
	if !ok {
		return conversation.ModerationRateState{}, conversation.ErrNotFound
	}

	return state, nil
}
