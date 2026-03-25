package search

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func normalizeScope(scope SearchScope) SearchScope {
	scope = SearchScope(strings.ToLower(strings.TrimSpace(string(scope))))
	switch scope {
	case SearchScopeUsers, SearchScopeConversations, SearchScopeMessages, SearchScopeMedia:
		return scope
	default:
		return SearchScopeUnspecified
	}
}

// NormalizeScope canonicalizes a search scope value.
func NormalizeScope(scope SearchScope) SearchScope {
	return normalizeScope(scope)
}

func normalizeScopes(scopes []SearchScope) []SearchScope {
	if len(scopes) == 0 {
		return nil
	}

	seen := make(map[SearchScope]struct{}, len(scopes))
	result := make([]SearchScope, 0, len(scopes))
	for _, scope := range scopes {
		scope = normalizeScope(scope)
		if scope == SearchScopeUnspecified {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })

	return result
}

// NormalizeScopes canonicalizes a set of search scopes.
func NormalizeScopes(scopes []SearchScope) []SearchScope {
	return normalizeScopes(scopes)
}

func normalizeIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)

	return result
}

// NormalizeIDs canonicalizes a set of string identifiers.
func NormalizeIDs(values []string) []string {
	return normalizeIDs(values)
}

func normalizeAccountKinds(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)

	return result
}

// NormalizeAccountKinds canonicalizes a set of account kinds.
func NormalizeAccountKinds(values []string) []string {
	return normalizeAccountKinds(values)
}

func normalizeSearchText(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parts = append(parts, value)
	}

	return strings.Join(parts, " ")
}

func normalizeQuery(query string) string {
	return strings.TrimSpace(strings.ToLower(query))
}

// NormalizeQuery canonicalizes a search query.
func NormalizeQuery(query string) string {
	return normalizeQuery(query)
}

func normalizeMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		key = strings.TrimSpace(strings.ToLower(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		result[key] = value
	}

	return result
}

// NormalizeMetadata canonicalizes search metadata.
func NormalizeMetadata(metadata map[string]string) map[string]string {
	return normalizeMetadata(metadata)
}

func documentID(scope SearchScope, targetID string) string {
	return string(scope) + ":" + strings.TrimSpace(targetID)
}

// DocumentID builds a stable identifier for a search document.
func DocumentID(scope SearchScope, targetID string) string {
	return documentID(scope, targetID)
}

func normalizeDocument(document Document, now time.Time) (Document, error) {
	document.Scope = normalizeScope(document.Scope)
	document.EntityType = strings.TrimSpace(strings.ToLower(document.EntityType))
	document.TargetID = strings.TrimSpace(document.TargetID)
	document.Title = strings.TrimSpace(document.Title)
	document.Subtitle = strings.TrimSpace(document.Subtitle)
	document.Snippet = strings.TrimSpace(document.Snippet)
	document.SearchText = strings.TrimSpace(document.SearchText)
	document.ConversationID = strings.TrimSpace(document.ConversationID)
	document.MessageID = strings.TrimSpace(document.MessageID)
	document.MediaID = strings.TrimSpace(document.MediaID)
	document.UserID = strings.TrimSpace(document.UserID)
	document.AccountKind = strings.TrimSpace(strings.ToLower(document.AccountKind))
	document.Metadata = normalizeMetadata(document.Metadata)
	if document.Scope == SearchScopeUnspecified || document.TargetID == "" {
		return Document{}, ErrInvalidInput
	}
	if document.EntityType == "" {
		document.EntityType = string(document.Scope)
	}
	if document.ID == "" {
		document.ID = documentID(document.Scope, document.TargetID)
	}
	if document.SearchText == "" {
		document.SearchText = normalizeSearchText(
			document.EntityType,
			document.Title,
			document.Subtitle,
			document.Snippet,
			document.ConversationID,
			document.MessageID,
			document.MediaID,
			document.UserID,
			document.AccountKind,
		)
		for key, value := range document.Metadata {
			document.SearchText = normalizeSearchText(document.SearchText, key, value)
		}
	}
	if document.CreatedAt.IsZero() {
		document.CreatedAt = now.UTC()
	}
	if document.UpdatedAt.IsZero() {
		document.UpdatedAt = document.CreatedAt
	}

	return document, nil
}

// NormalizeDocument canonicalizes a search document.
func NormalizeDocument(document Document, now time.Time) (Document, error) {
	return normalizeDocument(document, now)
}

func nextPageToken(offset int) string {
	if offset <= 0 {
		return ""
	}

	return strconv.Itoa(offset)
}

// NextPageToken formats a pagination offset as a token.
func NextPageToken(offset int) string {
	return nextPageToken(offset)
}

func offsetFromPageToken(pageToken string) (int, error) {
	pageToken = strings.TrimSpace(pageToken)
	if pageToken == "" {
		return 0, nil
	}

	offset, err := strconv.Atoi(pageToken)
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("%w", ErrInvalidInput)
	}

	return offset, nil
}

// OffsetFromPageToken parses a pagination token into an offset.
func OffsetFromPageToken(pageToken string) (int, error) {
	return offsetFromPageToken(pageToken)
}
