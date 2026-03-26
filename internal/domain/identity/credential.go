package identity

import "time"

// SessionCredentialKind identifies the bearer token family persisted for a session.
type SessionCredentialKind string

// Supported persisted session credential kinds.
const (
	// SessionCredentialKindUnspecified is the zero value.
	SessionCredentialKindUnspecified SessionCredentialKind = ""
	// SessionCredentialKindAccess stores the short-lived bearer token.
	SessionCredentialKindAccess SessionCredentialKind = "access"
	// SessionCredentialKindRefresh stores the long-lived refresh token.
	SessionCredentialKindRefresh SessionCredentialKind = "refresh"
)

// SessionCredential stores one hashed bearer token bound to a session/device/account tuple.
type SessionCredential struct {
	SessionID string
	AccountID string
	DeviceID  string
	Kind      SessionCredentialKind
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AuthContext describes the authenticated bearer principal resolved from an access token.
type AuthContext struct {
	Account Account
	Device  Device
	Session Session
}
