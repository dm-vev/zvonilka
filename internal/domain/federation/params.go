package federation

import "time"

// CreatePeerParams captures peer creation input.
type CreatePeerParams struct {
	ServerName    string
	BaseURL       string
	Capabilities  []Capability
	Trusted       bool
	SharedSecret  string
	SigningSecret string
}

// UpdatePeerParams captures mutable peer fields.
type UpdatePeerParams struct {
	PeerID       string
	ServerName   *string
	BaseURL      *string
	Capabilities *[]Capability
	Trusted      *bool
	State        *PeerState
}

// CreateLinkParams captures link creation input.
type CreateLinkParams struct {
	PeerID                   string
	Name                     string
	Endpoint                 string
	TransportKind            TransportKind
	DeliveryClass            DeliveryClass
	DiscoveryMode            DiscoveryMode
	MediaPolicy              MediaPolicy
	MaxBundleBytes           int
	MaxFragmentBytes         int
	AllowedConversationKinds []ConversationKind
	AllowedEventFamilies     []EventFamily
	AllowedMessageKinds      []MessageKind
}

// UpdateLinkParams captures mutable link fields.
type UpdateLinkParams struct {
	LinkID                   string
	Name                     *string
	Endpoint                 *string
	TransportKind            *TransportKind
	DeliveryClass            *DeliveryClass
	DiscoveryMode            *DiscoveryMode
	MediaPolicy              *MediaPolicy
	State                    *LinkState
	MaxBundleBytes           *int
	MaxFragmentBytes         *int
	AllowedConversationKinds *[]ConversationKind
	AllowedEventFamilies     *[]EventFamily
	AllowedMessageKinds      *[]MessageKind
	LastHealthyAt            *time.Time
	LastError                *string
}

// SaveBundleParams captures input for inbound and outbound bundle persistence.
type SaveBundleParams struct {
	PeerID        string
	LinkID        string
	DedupKey      string
	Direction     BundleDirection
	CursorFrom    uint64
	CursorTo      uint64
	EventCount    int
	PayloadType   string
	Payload       []byte
	Compression   CompressionKind
	IntegrityHash string
	AuthTag       string
	AvailableAt   time.Time
	ExpiresAt     time.Time
}

// AcknowledgeBundlesParams captures bundle acknowledgement input.
type AcknowledgeBundlesParams struct {
	PeerID         string
	LinkID         string
	UpToCursor     uint64
	BundleIDs      []string
	AcknowledgedAt time.Time
}

// SaveFragmentParams captures input for inbound and outbound fragment persistence.
type SaveFragmentParams struct {
	PeerID        string
	LinkID        string
	BundleID      string
	DedupKey      string
	Direction     BundleDirection
	CursorFrom    uint64
	CursorTo      uint64
	EventCount    int
	PayloadType   string
	Compression   CompressionKind
	IntegrityHash string
	AuthTag       string
	FragmentIndex int
	FragmentCount int
	Payload       []byte
	AvailableAt   time.Time
}

// ClaimFragmentsParams captures outbound bridge fragment claim input.
type ClaimFragmentsParams struct {
	PeerID         string
	LinkID         string
	Limit          int
	ClaimedAt      time.Time
	LeaseToken     string
	LeaseExpiresAt time.Time
}

// AcknowledgeFragmentsParams captures fragment acknowledgement input.
type AcknowledgeFragmentsParams struct {
	PeerID         string
	LinkID         string
	FragmentIDs    []string
	LeaseToken     string
	AcknowledgedAt time.Time
}
