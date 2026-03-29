package call

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// GetIceConfig returns RTC transport configuration for one visible call.
func (s *Service) GetIceConfig(ctx context.Context, params IceParams) ([]IceServer, time.Time, string, error) {
	if err := s.validateContext(ctx, "get ice config"); err != nil {
		return nil, time.Time{}, "", err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.CallID == "" || params.AccountID == "" {
		return nil, time.Time{}, "", ErrInvalidInput
	}

	callRow, err := s.GetCall(ctx, GetParams{CallID: params.CallID, AccountID: params.AccountID})
	if err != nil {
		return nil, time.Time{}, "", err
	}
	if callRow.State == StateEnded {
		return nil, time.Time{}, "", ErrConflict
	}

	servers, expiresAt, err := s.iceServersForAccount(params.AccountID, time.Time{})
	if err != nil {
		return nil, time.Time{}, "", err
	}

	return servers, expiresAt, s.resolveRuntimeEndpoint(s.rtc.endpointForSession(callRow.ActiveSessionID)), nil
}

func (s *Service) iceServersForAccount(
	accountID string,
	runtimeExpiresAt time.Time,
) ([]IceServer, time.Time, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, time.Time{}, ErrInvalidInput
	}

	servers := make([]IceServer, 0, len(s.rtc.STUNURLs)+len(s.rtc.TURNURLs))
	for _, url := range s.rtc.STUNURLs {
		if url == "" {
			continue
		}
		servers = append(servers, IceServer{URLs: []string{url}})
	}

	expiresAt := runtimeExpiresAt
	if expiresAt.IsZero() {
		expiresAt = s.currentTime().Add(s.rtc.CredentialTTL)
	}
	if len(s.rtc.TURNURLs) > 0 {
		server := IceServer{
			URLs:      append([]string(nil), s.rtc.TURNURLs...),
			ExpiresAt: expiresAt,
		}
		if s.rtc.TURNSecret != "" {
			username, credential, err := turnCredential(s.rtc.TURNSecret, accountID, expiresAt)
			if err != nil {
				return nil, time.Time{}, fmt.Errorf("generate turn credential for %s: %w", accountID, err)
			}
			server.Username = username
			server.Credential = credential
		}
		servers = append(servers, server)
	}

	return cloneIceServers(servers), expiresAt, nil
}

func (s *Service) resolveRuntimeEndpoint(runtimeEndpoint string) string {
	runtimeEndpoint = strings.TrimSpace(runtimeEndpoint)
	if runtimeEndpoint != "" {
		return runtimeEndpoint
	}

	return s.rtc.PublicEndpoint
}
