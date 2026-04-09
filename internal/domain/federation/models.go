package federation

import (
	"errors"
	"sort"
	"strings"
	"time"
)

var (
	// ErrInvalidInput indicates that the caller supplied malformed federation input.
	ErrInvalidInput = errors.New("invalid input")
	// ErrNotFound indicates that no federation row exists for the requested key.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates that the requested change conflicts with existing state.
	ErrConflict = errors.New("conflict")
	// ErrUnauthorized indicates that federation authentication failed.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden indicates that the current actor may not perform the operation.
	ErrForbidden = errors.New("forbidden")
)

// Capability identifies an optional federation capability announced by a peer.
type Capability string

// Peer capabilities supported by the federation domain.
const (
	CapabilityUnspecified      Capability = ""
	CapabilityEventReplication Capability = "event_replication"
	CapabilityDirectorySync    Capability = "directory_sync"
	CapabilityMediaProxy       Capability = "media_proxy"
	CapabilityBotBridge        Capability = "bot_bridge"
	CapabilitySearchMirror     Capability = "search_mirror"
)

// ConversationKind identifies which conversation families a link may carry.
type ConversationKind string

// Conversation kinds supported by federation policy.
const (
	ConversationKindUnspecified ConversationKind = ""
	ConversationKindDirect      ConversationKind = "direct"
	ConversationKindGroup       ConversationKind = "group"
	ConversationKindChannel     ConversationKind = "channel"
)

// PeerState identifies the trust and lifecycle state of a peer.
type PeerState string

// Peer states used by the federation domain.
const (
	PeerStateUnspecified PeerState = ""
	PeerStatePending     PeerState = "pending"
	PeerStateActive      PeerState = "active"
	PeerStateDegraded    PeerState = "degraded"
	PeerStateRevoked     PeerState = "revoked"
)

// LinkState identifies the lifecycle of a transport link.
type LinkState string

// Link states used by the federation domain.
const (
	LinkStateUnspecified LinkState = ""
	LinkStateActive      LinkState = "active"
	LinkStatePaused      LinkState = "paused"
	LinkStateDegraded    LinkState = "degraded"
	LinkStateDeleted     LinkState = "deleted"
)

// TransportKind identifies the transport adapter used by a link.
type TransportKind string

// Transport kinds supported by the federation domain.
const (
	TransportKindUnspecified TransportKind = ""
	TransportKindHTTPS       TransportKind = "https"
	TransportKindMeshtastic  TransportKind = "meshtastic"
	TransportKindMeshCore    TransportKind = "meshcore"
	TransportKindCustomDTN   TransportKind = "custom_dtn"
)

// DeliveryClass identifies the delivery envelope for a link.
type DeliveryClass string

// Delivery classes supported by the federation domain.
const (
	DeliveryClassUnspecified      DeliveryClass = ""
	DeliveryClassRealtime         DeliveryClass = "realtime"
	DeliveryClassDelayTolerant    DeliveryClass = "delay_tolerant"
	DeliveryClassUltraConstrained DeliveryClass = "ultra_constrained"
)

// DiscoveryMode identifies how a link endpoint is discovered.
type DiscoveryMode string

// Discovery modes supported by the federation domain.
const (
	DiscoveryModeUnspecified     DiscoveryMode = ""
	DiscoveryModeManual          DiscoveryMode = "manual"
	DiscoveryModeDNS             DiscoveryMode = "dns"
	DiscoveryModeWellKnown       DiscoveryMode = "well_known"
	DiscoveryModeBridgeAnnounced DiscoveryMode = "bridge_announced"
)

// MediaPolicy identifies how a link federates media.
type MediaPolicy string

// Media policies supported by the federation domain.
const (
	MediaPolicyUnspecified           MediaPolicy = ""
	MediaPolicyReferenceProxy        MediaPolicy = "reference_proxy"
	MediaPolicyBackgroundReplication MediaPolicy = "background_replication"
	MediaPolicyDisabled              MediaPolicy = "disabled"
)

// BundleDirection identifies whether a bundle is inbound or outbound.
type BundleDirection string

// Bundle directions supported by the federation domain.
const (
	BundleDirectionUnspecified BundleDirection = ""
	BundleDirectionInbound     BundleDirection = "inbound"
	BundleDirectionOutbound    BundleDirection = "outbound"
)

// BundleState identifies the lifecycle of a stored bundle.
type BundleState string

// Bundle states supported by the federation domain.
const (
	BundleStateUnspecified  BundleState = ""
	BundleStateQueued       BundleState = "queued"
	BundleStateAccepted     BundleState = "accepted"
	BundleStateAcknowledged BundleState = "acknowledged"
	BundleStateFailed       BundleState = "failed"
)

// CompressionKind identifies the payload compression mode for a bundle.
type CompressionKind string

// Compression kinds supported by the federation domain.
const (
	CompressionKindUnspecified CompressionKind = ""
	CompressionKindNone        CompressionKind = "none"
	CompressionKindGzip        CompressionKind = "gzip"
)

// FragmentState identifies the lifecycle of one durable bundle fragment.
type FragmentState string

// Fragment states supported by the federation domain.
const (
	FragmentStateUnspecified  FragmentState = ""
	FragmentStateQueued       FragmentState = "queued"
	FragmentStateClaimed      FragmentState = "claimed"
	FragmentStateAccepted     FragmentState = "accepted"
	FragmentStateAcknowledged FragmentState = "acknowledged"
	FragmentStateAssembled    FragmentState = "assembled"
	FragmentStateFailed       FragmentState = "failed"
)

// Peer stores one remote server federation identity.
type Peer struct {
	ID                      string
	ServerName              string
	BaseURL                 string
	Capabilities            []Capability
	Trusted                 bool
	State                   PeerState
	VerificationFingerprint string
	SharedSecret            string
	SharedSecretHash        string
	CreatedAt               time.Time
	UpdatedAt               time.Time
	LastSeenAt              time.Time
}

// Link stores one concrete delivery path to a peer.
type Link struct {
	ID                       string
	PeerID                   string
	Name                     string
	Endpoint                 string
	TransportKind            TransportKind
	DeliveryClass            DeliveryClass
	DiscoveryMode            DiscoveryMode
	MediaPolicy              MediaPolicy
	State                    LinkState
	MaxBundleBytes           int
	MaxFragmentBytes         int
	AllowedConversationKinds []ConversationKind
	CreatedAt                time.Time
	UpdatedAt                time.Time
	LastHealthyAt            time.Time
	LastError                string
}

// Bundle stores one durable replication unit.
type Bundle struct {
	ID            string
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
	State         BundleState
	CreatedAt     time.Time
	AvailableAt   time.Time
	ExpiresAt     time.Time
	AckedAt       time.Time
}

// BundleFragment stores one durable transport fragment for a constrained link.
type BundleFragment struct {
	ID             string
	PeerID         string
	LinkID         string
	BundleID       string
	DedupKey       string
	Direction      BundleDirection
	CursorFrom     uint64
	CursorTo       uint64
	EventCount     int
	PayloadType    string
	Compression    CompressionKind
	IntegrityHash  string
	AuthTag        string
	FragmentIndex  int
	FragmentCount  int
	Payload        []byte
	State          FragmentState
	LeaseToken     string
	LeaseExpiresAt time.Time
	AttemptCount   int
	CreatedAt      time.Time
	AvailableAt    time.Time
	AckedAt        time.Time
}

// ReplicationCursor stores the durable replication watermarks for one peer/link pair.
type ReplicationCursor struct {
	LastReceivedCursor uint64
	PeerID             string
	LinkID             string
	LastInboundCursor  uint64
	LastOutboundCursor uint64
	LastAckedCursor    uint64
	UpdatedAt          time.Time
}

// NormalizePeer validates and normalizes a peer snapshot.
func NormalizePeer(peer Peer, now time.Time) (Peer, error) {
	return peer.normalize(now)
}

// NormalizeLink validates and normalizes a link snapshot.
func NormalizeLink(link Link, now time.Time) (Link, error) {
	return link.normalize(now)
}

// NormalizeBundle validates and normalizes a bundle snapshot.
func NormalizeBundle(bundle Bundle, now time.Time) (Bundle, error) {
	return bundle.normalize(now)
}

// NormalizeBundleFragment validates and normalizes a fragment snapshot.
func NormalizeBundleFragment(fragment BundleFragment, now time.Time) (BundleFragment, error) {
	return fragment.normalize(now)
}

// NormalizeReplicationCursor validates and normalizes a cursor snapshot.
func NormalizeReplicationCursor(cursor ReplicationCursor, now time.Time) (ReplicationCursor, error) {
	return cursor.normalize(now)
}

func defaultConversationKinds() []ConversationKind {
	return []ConversationKind{
		ConversationKindDirect,
		ConversationKindGroup,
		ConversationKindChannel,
	}
}

func normalizeCapabilities(values []Capability) []Capability {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[Capability]struct{}, len(values))
	normalized := make([]Capability, 0, len(values))
	for _, value := range values {
		value = Capability(strings.TrimSpace(strings.ToLower(string(value))))
		if value == CapabilityUnspecified {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})

	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func normalizeConversationKinds(values []ConversationKind) []ConversationKind {
	if len(values) == 0 {
		return defaultConversationKinds()
	}

	seen := make(map[ConversationKind]struct{}, len(values))
	normalized := make([]ConversationKind, 0, len(values))
	for _, value := range values {
		value = ConversationKind(strings.TrimSpace(strings.ToLower(string(value))))
		switch value {
		case ConversationKindDirect, ConversationKindGroup, ConversationKindChannel:
		default:
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})

	if len(normalized) == 0 {
		return defaultConversationKinds()
	}

	return normalized
}

func (p Peer) normalize(now time.Time) (Peer, error) {
	p.ID = strings.TrimSpace(p.ID)
	p.ServerName = strings.TrimSpace(strings.ToLower(p.ServerName))
	p.BaseURL = strings.TrimSpace(p.BaseURL)
	p.VerificationFingerprint = strings.TrimSpace(strings.ToLower(p.VerificationFingerprint))
	p.SharedSecret = strings.TrimSpace(p.SharedSecret)
	p.SharedSecretHash = strings.TrimSpace(strings.ToLower(p.SharedSecretHash))
	if p.ID == "" || p.ServerName == "" {
		return Peer{}, ErrInvalidInput
	}
	if p.SharedSecret == "" || p.SharedSecretHash == "" {
		return Peer{}, ErrInvalidInput
	}

	p.Capabilities = normalizeCapabilities(p.Capabilities)
	switch p.State {
	case PeerStateUnspecified:
		if p.Trusted {
			p.State = PeerStateActive
		} else {
			p.State = PeerStatePending
		}
	case PeerStateActive:
		p.Trusted = true
	case PeerStateRevoked:
		p.Trusted = false
	case PeerStatePending, PeerStateDegraded:
	default:
		return Peer{}, ErrInvalidInput
	}

	if p.CreatedAt.IsZero() {
		p.CreatedAt = now.UTC()
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = p.CreatedAt
	}

	p.CreatedAt = p.CreatedAt.UTC()
	p.UpdatedAt = p.UpdatedAt.UTC()
	p.LastSeenAt = p.LastSeenAt.UTC()

	return p, nil
}

func (l Link) normalize(now time.Time) (Link, error) {
	l.ID = strings.TrimSpace(l.ID)
	l.PeerID = strings.TrimSpace(l.PeerID)
	l.Name = strings.TrimSpace(strings.ToLower(l.Name))
	l.Endpoint = strings.TrimSpace(l.Endpoint)
	l.LastError = strings.TrimSpace(l.LastError)
	if l.ID == "" || l.PeerID == "" || l.Name == "" {
		return Link{}, ErrInvalidInput
	}

	if l.TransportKind == TransportKindUnspecified {
		l.TransportKind = TransportKindHTTPS
	}
	switch l.TransportKind {
	case TransportKindHTTPS, TransportKindMeshtastic, TransportKindMeshCore, TransportKindCustomDTN:
	default:
		return Link{}, ErrInvalidInput
	}

	if l.DeliveryClass == DeliveryClassUnspecified {
		l.DeliveryClass = DeliveryClassRealtime
	}
	switch l.DeliveryClass {
	case DeliveryClassRealtime, DeliveryClassDelayTolerant, DeliveryClassUltraConstrained:
	default:
		return Link{}, ErrInvalidInput
	}

	if l.DiscoveryMode == DiscoveryModeUnspecified {
		l.DiscoveryMode = DiscoveryModeManual
	}
	switch l.DiscoveryMode {
	case DiscoveryModeManual, DiscoveryModeDNS, DiscoveryModeWellKnown, DiscoveryModeBridgeAnnounced:
	default:
		return Link{}, ErrInvalidInput
	}

	if l.MediaPolicy == MediaPolicyUnspecified {
		if l.DeliveryClass == DeliveryClassUltraConstrained {
			l.MediaPolicy = MediaPolicyDisabled
		} else {
			l.MediaPolicy = MediaPolicyReferenceProxy
		}
	}
	switch l.MediaPolicy {
	case MediaPolicyReferenceProxy, MediaPolicyBackgroundReplication, MediaPolicyDisabled:
	default:
		return Link{}, ErrInvalidInput
	}

	if l.State == LinkStateUnspecified {
		l.State = LinkStateActive
	}
	switch l.State {
	case LinkStateActive, LinkStatePaused, LinkStateDegraded, LinkStateDeleted:
	default:
		return Link{}, ErrInvalidInput
	}

	if l.MaxBundleBytes <= 0 {
		switch l.DeliveryClass {
		case DeliveryClassRealtime:
			l.MaxBundleBytes = 256 * 1024
		case DeliveryClassDelayTolerant:
			l.MaxBundleBytes = 64 * 1024
		default:
			l.MaxBundleBytes = 2 * 1024
		}
	}
	if l.MaxFragmentBytes <= 0 {
		switch l.DeliveryClass {
		case DeliveryClassRealtime:
			l.MaxFragmentBytes = 64 * 1024
		case DeliveryClassDelayTolerant:
			l.MaxFragmentBytes = 8 * 1024
		default:
			l.MaxFragmentBytes = 192
		}
	}
	if l.MaxFragmentBytes > l.MaxBundleBytes {
		return Link{}, ErrInvalidInput
	}

	l.AllowedConversationKinds = normalizeConversationKinds(l.AllowedConversationKinds)
	if l.CreatedAt.IsZero() {
		l.CreatedAt = now.UTC()
	}
	if l.UpdatedAt.IsZero() {
		l.UpdatedAt = l.CreatedAt
	}

	l.CreatedAt = l.CreatedAt.UTC()
	l.UpdatedAt = l.UpdatedAt.UTC()
	l.LastHealthyAt = l.LastHealthyAt.UTC()

	return l, nil
}

func (b Bundle) normalize(now time.Time) (Bundle, error) {
	b.ID = strings.TrimSpace(b.ID)
	b.PeerID = strings.TrimSpace(b.PeerID)
	b.LinkID = strings.TrimSpace(b.LinkID)
	b.DedupKey = strings.TrimSpace(b.DedupKey)
	b.IntegrityHash = strings.TrimSpace(strings.ToLower(b.IntegrityHash))
	b.AuthTag = strings.TrimSpace(strings.ToLower(b.AuthTag))
	b.PayloadType = strings.TrimSpace(strings.ToLower(b.PayloadType))
	if b.ID == "" || b.PeerID == "" || b.LinkID == "" || b.DedupKey == "" {
		return Bundle{}, ErrInvalidInput
	}

	switch b.Direction {
	case BundleDirectionInbound, BundleDirectionOutbound:
	default:
		return Bundle{}, ErrInvalidInput
	}
	if b.CursorTo < b.CursorFrom {
		return Bundle{}, ErrInvalidInput
	}
	if b.EventCount < 0 {
		return Bundle{}, ErrInvalidInput
	}

	if b.Compression == CompressionKindUnspecified {
		b.Compression = CompressionKindNone
	}
	switch b.Compression {
	case CompressionKindNone, CompressionKindGzip:
	default:
		return Bundle{}, ErrInvalidInput
	}

	if b.State == BundleStateUnspecified {
		if b.Direction == BundleDirectionInbound {
			b.State = BundleStateAccepted
		} else {
			b.State = BundleStateQueued
		}
	}
	switch b.State {
	case BundleStateQueued, BundleStateAccepted, BundleStateAcknowledged, BundleStateFailed:
	default:
		return Bundle{}, ErrInvalidInput
	}

	if b.CreatedAt.IsZero() {
		b.CreatedAt = now.UTC()
	}
	if b.AvailableAt.IsZero() {
		b.AvailableAt = b.CreatedAt
	}

	b.CreatedAt = b.CreatedAt.UTC()
	b.AvailableAt = b.AvailableAt.UTC()
	b.ExpiresAt = b.ExpiresAt.UTC()
	b.AckedAt = b.AckedAt.UTC()

	return b, nil
}

func (f BundleFragment) normalize(now time.Time) (BundleFragment, error) {
	f.ID = strings.TrimSpace(f.ID)
	f.PeerID = strings.TrimSpace(f.PeerID)
	f.LinkID = strings.TrimSpace(f.LinkID)
	f.BundleID = strings.TrimSpace(f.BundleID)
	f.DedupKey = strings.TrimSpace(f.DedupKey)
	f.LeaseToken = strings.TrimSpace(f.LeaseToken)
	f.IntegrityHash = strings.TrimSpace(strings.ToLower(f.IntegrityHash))
	f.AuthTag = strings.TrimSpace(strings.ToLower(f.AuthTag))
	f.PayloadType = strings.TrimSpace(strings.ToLower(f.PayloadType))
	if f.ID == "" || f.PeerID == "" || f.LinkID == "" || f.BundleID == "" || f.DedupKey == "" {
		return BundleFragment{}, ErrInvalidInput
	}

	switch f.Direction {
	case BundleDirectionInbound, BundleDirectionOutbound:
	default:
		return BundleFragment{}, ErrInvalidInput
	}
	if f.CursorTo < f.CursorFrom {
		return BundleFragment{}, ErrInvalidInput
	}
	if f.EventCount < 0 {
		return BundleFragment{}, ErrInvalidInput
	}
	if f.AttemptCount < 0 {
		return BundleFragment{}, ErrInvalidInput
	}
	if f.FragmentCount <= 0 || f.FragmentIndex < 0 || f.FragmentIndex >= f.FragmentCount {
		return BundleFragment{}, ErrInvalidInput
	}
	if f.IntegrityHash == "" || f.AuthTag == "" {
		return BundleFragment{}, ErrInvalidInput
	}

	if f.Compression == CompressionKindUnspecified {
		f.Compression = CompressionKindNone
	}
	switch f.Compression {
	case CompressionKindNone, CompressionKindGzip:
	default:
		return BundleFragment{}, ErrInvalidInput
	}

	if f.State == FragmentStateUnspecified {
		if f.Direction == BundleDirectionInbound {
			f.State = FragmentStateAccepted
		} else {
			f.State = FragmentStateQueued
		}
	}
	switch f.State {
	case FragmentStateQueued, FragmentStateClaimed, FragmentStateAccepted, FragmentStateAcknowledged, FragmentStateAssembled, FragmentStateFailed:
	default:
		return BundleFragment{}, ErrInvalidInput
	}
	if f.State == FragmentStateClaimed {
		if f.Direction != BundleDirectionOutbound || f.LeaseToken == "" || f.LeaseExpiresAt.IsZero() {
			return BundleFragment{}, ErrInvalidInput
		}
	} else {
		f.LeaseToken = ""
		f.LeaseExpiresAt = time.Time{}
	}

	if f.CreatedAt.IsZero() {
		f.CreatedAt = now.UTC()
	}
	if f.AvailableAt.IsZero() {
		f.AvailableAt = f.CreatedAt
	}

	f.CreatedAt = f.CreatedAt.UTC()
	f.AvailableAt = f.AvailableAt.UTC()
	f.LeaseExpiresAt = f.LeaseExpiresAt.UTC()
	f.AckedAt = f.AckedAt.UTC()

	return f, nil
}

func (c ReplicationCursor) normalize(now time.Time) (ReplicationCursor, error) {
	c.PeerID = strings.TrimSpace(c.PeerID)
	c.LinkID = strings.TrimSpace(c.LinkID)
	if c.PeerID == "" || c.LinkID == "" {
		return ReplicationCursor{}, ErrInvalidInput
	}
	if c.LastInboundCursor > c.LastReceivedCursor {
		return ReplicationCursor{}, ErrInvalidInput
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now.UTC()
	}

	c.UpdatedAt = c.UpdatedAt.UTC()

	return c, nil
}
