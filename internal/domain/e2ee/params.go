package e2ee

import "time"

type UploadDevicePreKeysParams struct {
	AccountID            string
	DeviceID             string
	SignedPreKey         SignedPreKey
	OneTimePreKeys       []OneTimePreKey
	ReplaceOneTimePreKey bool
}

type FetchAccountBundlesParams struct {
	RequesterAccountID   string
	RequesterDeviceID    string
	TargetAccountID      string
	ConsumeOneTimePreKey bool
}

type CreateDirectSessionsParams struct {
	InitiatorAccountID string
	InitiatorDeviceID  string
	TargetAccountID    string
	TargetDeviceID     string
	InitiatorEphemeral PublicKey
	Bootstrap          BootstrapPayload
	ExpiresAt          time.Time
}

type ListDeviceSessionsParams struct {
	AccountID           string
	DeviceID            string
	IncludeAcknowledged bool
	PeerAccountID       string
}

type AcknowledgeDirectSessionParams struct {
	SessionID          string
	RecipientAccountID string
	RecipientDeviceID  string
}

type RecipientSenderKey struct {
	RecipientAccountID string
	RecipientDeviceID  string
	Payload            SenderKeyPayload
}

type PublishGroupSenderKeysParams struct {
	ConversationID  string
	SenderAccountID string
	SenderDeviceID  string
	SenderKeyID     string
	Recipients      []RecipientSenderKey
	ExpiresAt       time.Time
}

type ListGroupSenderKeysParams struct {
	ConversationID      string
	RecipientAccountID  string
	RecipientDeviceID   string
	IncludeAcknowledged bool
}

type AcknowledgeGroupSenderKeyParams struct {
	DistributionID     string
	RecipientAccountID string
	RecipientDeviceID  string
}

type ValidateConversationPayloadParams struct {
	ConversationID  string
	SenderAccountID string
	SenderDeviceID  string
	PayloadKeyID    string
	PayloadMetadata map[string]string
}
