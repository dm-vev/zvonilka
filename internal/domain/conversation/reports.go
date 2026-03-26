package conversation

import (
	"context"
	"fmt"
	"strings"
)

// SubmitModerationReport persists a complaint for later review.
func (s *Service) SubmitModerationReport(
	ctx context.Context,
	params SubmitModerationReportParams,
) (ModerationReport, error) {
	if err := s.validateContext(ctx, "submit moderation report"); err != nil {
		return ModerationReport{}, err
	}

	params.TargetID = strings.TrimSpace(params.TargetID)
	params.ReporterAccountID = strings.TrimSpace(params.ReporterAccountID)
	params.TargetAccountID = strings.TrimSpace(params.TargetAccountID)
	params.Reason = strings.TrimSpace(params.Reason)
	params.Details = strings.TrimSpace(params.Details)
	if params.TargetKind == ModerationTargetKindUnspecified || params.TargetID == "" || params.ReporterAccountID == "" || params.Reason == "" {
		return ModerationReport{}, ErrInvalidInput
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var saved ModerationReport
	err := s.runTx(ctx, func(tx Store) error {
		conversation, err := s.lookupModerationConversation(ctx, tx, params.TargetKind, params.TargetID)
		if err != nil {
			return err
		}

		member, err := tx.ConversationMemberByConversationAndAccount(ctx, conversation.ID, params.ReporterAccountID)
		if err != nil {
			return fmt.Errorf(
				"authorize report submission for account %s in conversation %s: %w",
				params.ReporterAccountID,
				conversation.ID,
				err,
			)
		}
		if !isActiveMember(member) {
			return ErrForbidden
		}

		reportID, idErr := newID("rep")
		if idErr != nil {
			return fmt.Errorf("generate moderation report id: %w", idErr)
		}

		report := ModerationReport{
			ID:                reportID,
			TargetKind:        params.TargetKind,
			TargetID:          params.TargetID,
			ReporterAccountID: params.ReporterAccountID,
			TargetAccountID:   params.TargetAccountID,
			Reason:            params.Reason,
			Details:           params.Details,
			Status:            ModerationReportStatusPending,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		saved, err = tx.SaveModerationReport(ctx, report)
		if err != nil {
			return fmt.Errorf("save moderation report %s: %w", report.ID, err)
		}

		actionID, actionErr := newID("mod")
		if actionErr != nil {
			return fmt.Errorf("generate moderation action id: %w", actionErr)
		}
		if _, err := tx.SaveModerationAction(ctx, ModerationAction{
			ID:              actionID,
			TargetKind:      params.TargetKind,
			TargetID:        params.TargetID,
			ActorAccountID:  params.ReporterAccountID,
			TargetAccountID: params.TargetAccountID,
			Type:            ModerationActionTypeReportSet,
			Reason:          params.Reason,
			Metadata: map[string]string{
				"report_id": report.ID,
			},
			CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("log moderation report %s: %w", report.ID, err)
		}

		return nil
	})
	if err != nil {
		return ModerationReport{}, err
	}

	return saved, nil
}

// ModerationReportByID resolves a single moderation report.
func (s *Service) ModerationReportByID(ctx context.Context, reportID string) (ModerationReport, error) {
	if err := s.validateContext(ctx, "load moderation report"); err != nil {
		return ModerationReport{}, err
	}

	reportID = strings.TrimSpace(reportID)
	if reportID == "" {
		return ModerationReport{}, ErrInvalidInput
	}

	report, err := s.store.ModerationReportByID(ctx, reportID)
	if err != nil {
		return ModerationReport{}, fmt.Errorf("load moderation report %s: %w", reportID, err)
	}

	return report, nil
}

// ModerationReportsByTarget lists moderation reports for one target.
func (s *Service) ModerationReportsByTarget(
	ctx context.Context,
	targetKind ModerationTargetKind,
	targetID string,
) ([]ModerationReport, error) {
	if err := s.validateContext(ctx, "list moderation reports"); err != nil {
		return nil, err
	}

	targetID = strings.TrimSpace(targetID)
	if targetKind == ModerationTargetKindUnspecified || targetID == "" {
		return nil, ErrInvalidInput
	}

	reports, err := s.store.ModerationReportsByTarget(ctx, targetKind, targetID)
	if err != nil {
		return nil, fmt.Errorf("list moderation reports for %s:%s: %w", targetKind, targetID, err)
	}

	return reports, nil
}

// ResolveModerationReport marks a report resolved or rejected.
func (s *Service) ResolveModerationReport(
	ctx context.Context,
	params ResolveModerationReportParams,
) (ModerationReport, error) {
	if err := s.validateContext(ctx, "resolve moderation report"); err != nil {
		return ModerationReport{}, err
	}

	params.ReportID = strings.TrimSpace(params.ReportID)
	params.ResolverAccountID = strings.TrimSpace(params.ResolverAccountID)
	params.Resolution = strings.TrimSpace(params.Resolution)
	if params.ReportID == "" || params.ResolverAccountID == "" {
		return ModerationReport{}, ErrInvalidInput
	}

	now := params.ReviewedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var saved ModerationReport
	err := s.runTx(ctx, func(tx Store) error {
		report, err := tx.ModerationReportByID(ctx, params.ReportID)
		if err != nil {
			return fmt.Errorf("load moderation report %s: %w", params.ReportID, err)
		}

		conversation, err := s.lookupModerationConversation(ctx, tx, report.TargetKind, report.TargetID)
		if err != nil {
			return err
		}
		member, err := tx.ConversationMemberByConversationAndAccount(ctx, conversation.ID, params.ResolverAccountID)
		if err != nil {
			return fmt.Errorf(
				"authorize report resolution for account %s in conversation %s: %w",
				params.ResolverAccountID,
				conversation.ID,
				err,
			)
		}
		if !s.canModerate(member, params.ResolverAccountID) {
			return ErrForbidden
		}

		if params.Resolved {
			report.Status = ModerationReportStatusResolved
		} else {
			report.Status = ModerationReportStatusRejected
		}
		report.ReviewedByAccountID = params.ResolverAccountID
		report.ReviewedAt = now
		report.Resolution = params.Resolution
		report.UpdatedAt = now
		saved, err = tx.SaveModerationReport(ctx, report)
		if err != nil {
			return fmt.Errorf("update moderation report %s: %w", report.ID, err)
		}

		actionID, idErr := newID("mod")
		if idErr != nil {
			return fmt.Errorf("generate moderation action id: %w", idErr)
		}
		actionType := ModerationActionTypeReportResolve
		if !params.Resolved {
			actionType = ModerationActionTypeReportReject
		}
		if _, err := tx.SaveModerationAction(ctx, ModerationAction{
			ID:              actionID,
			TargetKind:      report.TargetKind,
			TargetID:        report.TargetID,
			ActorAccountID:  params.ResolverAccountID,
			TargetAccountID: report.TargetAccountID,
			Type:            actionType,
			Reason:          params.Resolution,
			Metadata: map[string]string{
				"report_id": report.ID,
			},
			CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("log moderation report resolution %s: %w", report.ID, err)
		}

		return nil
	})
	if err != nil {
		return ModerationReport{}, err
	}

	return saved, nil
}
