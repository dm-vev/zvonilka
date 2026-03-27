package call

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// GetDiagnostics returns one client-facing diagnostics report for a visible call.
func (s *Service) GetDiagnostics(ctx context.Context, params GetParams) (Diagnostics, error) {
	if err := s.validateContext(ctx, "get call diagnostics"); err != nil {
		return Diagnostics{}, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.CallID == "" || params.AccountID == "" {
		return Diagnostics{}, ErrInvalidInput
	}

	var result Diagnostics
	err := s.runTx(ctx, func(store Store) error {
		callRow, loadErr := s.visibleCall(ctx, store, params.CallID, params.AccountID)
		if loadErr != nil {
			return loadErr
		}
		events, loadErr := store.EventsAfterSequence(ctx, 0, callRow.ID, "", 5000)
		if loadErr != nil {
			return fmt.Errorf("load diagnostics events for call %s: %w", callRow.ID, loadErr)
		}

		result = buildDiagnostics(s.currentTime(), callRow, events)
		return nil
	})
	if err != nil {
		return Diagnostics{}, err
	}

	return cloneDiagnostics(result), nil
}

func buildDiagnostics(now time.Time, callRow Call, events []Event) Diagnostics {
	diagnostics := Diagnostics{
		Call:              cloneCall(callRow),
		PeakQoSEscalation: "normal",
	}

	endedAt := callRow.EndedAt
	if endedAt.IsZero() {
		endedAt = now
	}
	if !callRow.StartedAt.IsZero() && endedAt.After(callRow.StartedAt) {
		diagnostics.DurationSeconds = uint32(endedAt.Sub(callRow.StartedAt) / time.Second)
	}
	if !callRow.AnsweredAt.IsZero() && endedAt.After(callRow.AnsweredAt) {
		diagnostics.ActiveDurationSeconds = uint32(endedAt.Sub(callRow.AnsweredAt) / time.Second)
	}

	for _, participant := range callRow.Participants {
		stats := participant.Transport
		if stats.ReconnectAttempt > diagnostics.MaxReconnectAttempt {
			diagnostics.MaxReconnectAttempt = stats.ReconnectAttempt
		}
		if qosEscalationRankForDiagnostics(stats.QoSEscalation) > qosEscalationRankForDiagnostics(diagnostics.PeakQoSEscalation) {
			diagnostics.PeakQoSEscalation = normalizeQoSEscalation(stats.QoSEscalation)
		}
		if !stats.AppliedAt.IsZero() && stats.AppliedAt.After(diagnostics.LastAppliedAt) {
			diagnostics.LastAppliedAt = stats.AppliedAt
			diagnostics.LastAppliedProfile = stats.AppliedProfile
		}
	}

	adaptationRevisions := make(map[string]struct{})
	adaptationAcks := make(map[string]struct{})
	for _, event := range events {
		if event.EventType != EventTypeMediaUpdated {
			continue
		}

		escalation := normalizeQoSEscalation(event.Metadata["qos_escalation"])
		if qosEscalationRankForDiagnostics(escalation) > qosEscalationRankForDiagnostics(diagnostics.PeakQoSEscalation) {
			diagnostics.PeakQoSEscalation = escalation
		}

		targetKey := strings.TrimSpace(event.Metadata[callMetadataTargetAccountID]) + "|" +
			strings.TrimSpace(event.Metadata[callMetadataTargetDeviceID])
		if targetKey == "|" {
			targetKey = strings.TrimSpace(event.ActorAccountID) + "|" + strings.TrimSpace(event.ActorDeviceID)
		}
		if revision := strings.TrimSpace(event.Metadata["adaptation_revision"]); revision != "" {
			adaptationRevisions[targetKey+"|"+revision] = struct{}{}
		}
		if revision := strings.TrimSpace(event.Metadata["acked_adaptation_revision"]); revision != "" {
			adaptationAcks[targetKey+"|"+revision] = struct{}{}
		}
		if profile := strings.TrimSpace(event.Metadata["applied_profile"]); profile != "" && event.CreatedAt.After(diagnostics.LastAppliedAt) {
			diagnostics.LastAppliedAt = event.CreatedAt
			diagnostics.LastAppliedProfile = profile
		}
	}
	diagnostics.TotalAdaptationRevisions = uint32(len(adaptationRevisions))
	diagnostics.TotalAdaptationAcks = uint32(len(adaptationAcks))

	return diagnostics
}

func normalizeQoSEscalation(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "normal"
	}

	return value
}

func qosEscalationRankForDiagnostics(value string) int {
	switch normalizeQoSEscalation(value) {
	case "critical":
		return 2
	case "elevated":
		return 1
	case "normal":
		return 0
	default:
		return -1
	}
}
