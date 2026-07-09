package teststore

import (
	"strconv"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/search"
)

func cloneDocument(document search.Document) search.Document {
	document.Metadata = cloneMetadata(document.Metadata)
	return document
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	clone := make(map[string]string, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}

	return clone
}

func hitFromDocument(document search.Document) search.Hit {
	return search.Hit{
		HitID:          document.ID,
		Scope:          document.Scope,
		Title:          document.Title,
		Subtitle:       document.Subtitle,
		Snippet:        document.Snippet,
		TargetID:       document.TargetID,
		ConversationID: document.ConversationID,
		MessageID:      document.MessageID,
		MediaID:        document.MediaID,
		UserID:         document.UserID,
		AccountKind:    document.AccountKind,
		Metadata:       cloneMetadata(document.Metadata),
		UpdatedAt:      document.UpdatedAt,
	}
}

func matchesDocument(document search.Document, params search.SearchParams) bool {
	if len(params.Scopes) > 0 && !containsScope(params.Scopes, document.Scope) {
		return false
	}
	if len(params.ConversationIDs) > 0 && !containsString(params.ConversationIDs, document.ConversationID) {
		return false
	}
	if len(params.UserIDs) > 0 && !containsString(params.UserIDs, document.UserID) {
		return false
	}
	if len(params.AccountKinds) > 0 && !containsString(params.AccountKinds, document.AccountKind) {
		return false
	}
	if !params.UpdatedAfter.IsZero() && !document.UpdatedAt.After(params.UpdatedAfter) {
		return false
	}
	if !params.UpdatedBefore.IsZero() && !document.UpdatedAt.Before(params.UpdatedBefore) {
		return false
	}
	if params.Query == "" {
		return true
	}

	query := strings.ToLower(params.Query)
	haystack := strings.ToLower(strings.Join([]string{
		document.Title,
		document.Subtitle,
		document.Snippet,
		document.SearchText,
		strings.Join(mapValues(document.Metadata), " "),
	}, " "))

	if strings.Contains(haystack, query) {
		return true
	}

	if !params.FuzzyMatch {
		return false
	}

	return strings.Contains(haystack, strings.ReplaceAll(query, " ", ""))
}

func containsScope(scopes []search.SearchScope, scope search.SearchScope) bool {
	for _, candidate := range scopes {
		if candidate == scope {
			return true
		}
	}

	return false
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}

	return false
}

func mapValues(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}

	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}

	return result
}

func nextTokenFromOffset(offset int) string {
	return strconv.Itoa(offset)
}
