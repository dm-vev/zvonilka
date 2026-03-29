package call

import (
	"context"
	"fmt"
	"strings"
)

// RehomeActiveCalls proactively migrates active sessions from unavailable runtime nodes.
func (s *Service) RehomeActiveCalls(ctx context.Context, limit int) ([]Event, error) {
	if err := s.validateContext(ctx, "rehome active calls"); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}

	var events []Event
	err := s.runTx(ctx, func(store Store) error {
		rows, err := store.ActiveCalls(ctx, limit)
		if err != nil {
			return fmt.Errorf("list active calls: %w", err)
		}

		for _, row := range rows {
			if row.State != StateActive || strings.TrimSpace(row.ActiveSessionID) == "" {
				continue
			}
			if _, err := s.runtime.SessionStats(ctx, row.ActiveSessionID); err == nil {
				continue
			} else if !runtimeUnavailable(err) {
				return fmt.Errorf("load runtime stats for active call %s: %w", row.ID, err)
			}

			now := s.currentTime()
			migratedCall, _, migratedEvents, err := s.failoverActiveSession(ctx, store, row, now, "proactive")
			if err != nil {
				return fmt.Errorf("fail over active call %s: %w", row.ID, err)
			}
			for i := range migratedEvents {
				migratedEvents[i].Call = cloneCall(migratedCall)
			}
			events = append(events, migratedEvents...)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return cloneEvents(events), nil
}
