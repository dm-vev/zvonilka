package search

import "time"

// SearchScope identifies the searchable entity family.
type SearchScope string

// Supported search scopes.
const (
	SearchScopeUnspecified   SearchScope = ""
	SearchScopeUsers         SearchScope = "users"
	SearchScopeConversations SearchScope = "conversations"
	SearchScopeMessages      SearchScope = "messages"
	SearchScopeMedia         SearchScope = "media"
)

// Document describes one denormalized search row.
type Document struct {
	ID            string
	Scope         SearchScope
	EntityType    string
	TargetID      string
	Title         string
	Subtitle      string
	Snippet       string
	SearchText    string
	ConversationID string
	MessageID     string
	MediaID       string
	UserID        string
	AccountKind   string
	Metadata      map[string]string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Hit describes a search result.
type Hit struct {
	HitID         string
	Scope         SearchScope
	Title         string
	Subtitle      string
	Snippet       string
	TargetID      string
	ConversationID string
	MessageID     string
	MediaID       string
	UserID        string
	AccountKind   string
	Metadata      map[string]string
	UpdatedAt     time.Time
}

// SearchParams controls search query filtering and pagination.
type SearchParams struct {
	Query           string
	Scopes          []SearchScope
	ConversationIDs []string
	UserIDs         []string
	AccountKinds    []string
	IncludeSnippets bool
	FuzzyMatch      bool
	UpdatedAfter    time.Time
	UpdatedBefore   time.Time
	Limit           int
	Offset          int
}

// SearchResult contains search hits and pagination metadata.
type SearchResult struct {
	Hits          []Hit
	TotalSize     uint64
	NextPageToken string
}
