package e2ee

import "time"

type PublicKey struct {
	KeyID     string
	Algorithm string
	PublicKey []byte
	CreatedAt time.Time
	RotatedAt time.Time
	ExpiresAt time.Time
}

type SignedPreKey struct {
	Key       PublicKey
	Signature []byte
}

type OneTimePreKey struct {
	Key                PublicKey
	ClaimedAt          time.Time
	ClaimedByAccountID string
	ClaimedByDeviceID  string
}

type DeviceBundle struct {
	AccountID           string
	DeviceID            string
	IdentityKey         PublicKey
	SignedPreKey        SignedPreKey
	OneTimePreKey       OneTimePreKey
	OneTimePreKeysAvail uint32
	DeviceLastSeenAt    time.Time
}

type DeviceTrustState string

const (
	DeviceTrustStateUnspecified DeviceTrustState = ""
	DeviceTrustStateTrusted     DeviceTrustState = "trusted"
	DeviceTrustStateUntrusted   DeviceTrustState = "untrusted"
	DeviceTrustStateCompromised DeviceTrustState = "compromised"
)

type DeviceTrust struct {
	ObserverAccountID string
	ObserverDeviceID  string
	TargetAccountID   string
	TargetDeviceID    string
	State             DeviceTrustState
	KeyFingerprint    string
	Note              string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type DeviceVerificationCode struct {
	ObserverAccountID    string
	ObserverDeviceID     string
	TargetAccountID      string
	TargetDeviceID       string
	TargetKeyFingerprint string
	SafetyNumber         string
	CurrentTrustState    DeviceTrustState
}

type VerificationRequiredDevice struct {
	AccountID          string
	DeviceID           string
	TrustState         DeviceTrustState
	KeyFingerprint     string
	ConversationIDs    []string
	DirectConversation bool
}

type ConversationKeyCoverageState string

const (
	ConversationKeyCoverageStateUnspecified ConversationKeyCoverageState = ""
	ConversationKeyCoverageStateReady       ConversationKeyCoverageState = "ready"
	ConversationKeyCoverageStatePending     ConversationKeyCoverageState = "pending"
	ConversationKeyCoverageStateExpired     ConversationKeyCoverageState = "expired"
	ConversationKeyCoverageStateMissing     ConversationKeyCoverageState = "missing"
)

type ConversationKeyCoverageEntry struct {
	AccountID            string
	DeviceID             string
	State                ConversationKeyCoverageState
	ReferenceID          string
	ExpiresAt            time.Time
	TrustState           DeviceTrustState
	KeyFingerprint       string
	VerificationRequired bool
}

type UpdateType string

const (
	UpdateTypeUnspecified                 UpdateType = ""
	UpdateTypeDevicePreKeysUpdated        UpdateType = "device_prekeys.updated"
	UpdateTypeDeviceTrustUpdated          UpdateType = "device_trust.updated"
	UpdateTypeDirectSessionCreated        UpdateType = "direct_session.created"
	UpdateTypeDirectSessionAcknowledged   UpdateType = "direct_session.acknowledged"
	UpdateTypeGroupSenderKeyPublished     UpdateType = "group_sender_key.published"
	UpdateTypeGroupSenderKeyAcknowledged  UpdateType = "group_sender_key.acknowledged"
	UpdateTypeConversationCoverageChanged UpdateType = "conversation_key_coverage.changed"
	UpdateTypeDeviceVerified              UpdateType = "device.verified"
)

type Update struct {
	ID                   string
	Type                 UpdateType
	ActorAccountID       string
	ActorDeviceID        string
	TargetAccountID      string
	TargetDeviceID       string
	ConversationID       string
	SessionID            string
	DistributionID       string
	SenderKeyID          string
	Metadata             map[string]string
	CreatedAt            time.Time
	CurrentTrustState    DeviceTrustState
	TargetKeyFingerprint string
	VerificationRequired bool
	ConversationIDs      []string
}

type BootstrapPayload struct {
	Algorithm  string
	Nonce      []byte
	Ciphertext []byte
	Metadata   map[string]string
}

type DirectSessionState string

const (
	DirectSessionStateUnspecified  DirectSessionState = ""
	DirectSessionStatePending      DirectSessionState = "pending"
	DirectSessionStateAcknowledged DirectSessionState = "acknowledged"
)

type DirectSession struct {
	ID                 string
	InitiatorAccountID string
	InitiatorDeviceID  string
	RecipientAccountID string
	RecipientDeviceID  string
	InitiatorEphemeral PublicKey
	IdentityKey        PublicKey
	SignedPreKey       SignedPreKey
	OneTimePreKey      OneTimePreKey
	Bootstrap          BootstrapPayload
	State              DirectSessionState
	CreatedAt          time.Time
	AcknowledgedAt     time.Time
	ExpiresAt          time.Time
}

type SenderKeyPayload struct {
	KeyID      string
	Algorithm  string
	Nonce      []byte
	Ciphertext []byte
	AAD        []byte
	Metadata   map[string]string
}

type GroupSenderKeyState string

const (
	GroupSenderKeyStateUnspecified  GroupSenderKeyState = ""
	GroupSenderKeyStatePending      GroupSenderKeyState = "pending"
	GroupSenderKeyStateAcknowledged GroupSenderKeyState = "acknowledged"
)

type GroupSenderKeyDistribution struct {
	ID                 string
	ConversationID     string
	SenderAccountID    string
	SenderDeviceID     string
	RecipientAccountID string
	RecipientDeviceID  string
	SenderKeyID        string
	Payload            SenderKeyPayload
	State              GroupSenderKeyState
	CreatedAt          time.Time
	AcknowledgedAt     time.Time
	ExpiresAt          time.Time
}

type DeviceLinkTransfer struct {
	ID             string
	AccountID      string
	SourceDeviceID string
	TargetDeviceID string
	Payload        SenderKeyPayload
	CreatedAt      time.Time
}
