package identity

import (
	"context"
	"strings"

	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
)

func (s *Service) indexAccount(ctx context.Context, account Account) {
	if s == nil || s.indexer == nil || account.ID == "" {
		return
	}

	document := domainsearch.Document{
		Scope:       domainsearch.SearchScopeUsers,
		EntityType:  "account",
		TargetID:    account.ID,
		Title:       strings.TrimSpace(account.DisplayName),
		Subtitle:    strings.TrimSpace(account.Username),
		Snippet:     strings.TrimSpace(account.Bio),
		UserID:      account.ID,
		AccountKind: strings.ToLower(strings.TrimSpace(string(account.Kind))),
		Metadata:    accountSearchMetadata(account),
		UpdatedAt:   account.UpdatedAt,
		CreatedAt:   account.CreatedAt,
	}
	if document.Title == "" {
		document.Title = account.Username
	}

	_, _ = s.indexer.UpsertDocument(ctx, document)
}

func accountSearchMetadata(account Account) map[string]string {
	metadata := map[string]string{
		"status": strings.ToLower(strings.TrimSpace(string(account.Status))),
	}
	if account.CreatedBy != "" {
		metadata["created_by"] = account.CreatedBy
	}
	if account.CustomBadgeEmoji != "" {
		metadata["badge_emoji"] = account.CustomBadgeEmoji
	}
	if len(account.Roles) > 0 {
		roles := make([]string, 0, len(account.Roles))
		for _, role := range account.Roles {
			roles = append(roles, strings.ToLower(strings.TrimSpace(string(role))))
		}
		metadata["roles"] = strings.Join(roles, ",")
	}

	return metadata
}
