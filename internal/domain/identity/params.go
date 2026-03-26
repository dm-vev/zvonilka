package identity

import "time"

// SubmitJoinRequestParams contains the user-provided join request payload.
type SubmitJoinRequestParams struct {
	Username       string
	DisplayName    string
	Email          string
	Phone          string
	Note           string
	InviteCode     string
	IdempotencyKey string
	RequestedAt    time.Time
}

// ApproveJoinRequestParams contains the moderator decision for a join request.
type ApproveJoinRequestParams struct {
	JoinRequestID  string
	Roles          []Role
	Note           string
	ReviewedBy     string
	DecisionReason string
	IdempotencyKey string
}

// RejectJoinRequestParams contains the moderator rejection for a join request.
type RejectJoinRequestParams struct {
	JoinRequestID  string
	Reason         string
	ReviewedBy     string
	IdempotencyKey string
}

// CreateAccountParams contains the admin-created account payload.
type CreateAccountParams struct {
	Username       string
	DisplayName    string
	Email          string
	Phone          string
	Roles          []Role
	Note           string
	InviteCode     string
	AccountKind    AccountKind
	CreatedBy      string
	IdempotencyKey string
	RequestedAt    time.Time
}

// BeginLoginParams contains the login identifier and delivery settings.
type BeginLoginParams struct {
	Username       string
	Email          string
	Phone          string
	Delivery       LoginDeliveryChannel
	DeviceName     string
	Platform       DevicePlatform
	ClientVersion  string
	Locale         string
	IdempotencyKey string
	RequestedAt    time.Time
}

// VerifyLoginCodeParams contains the code verification payload.
type VerifyLoginCodeParams struct {
	ChallengeID            string
	Code                   string
	TwoFactorCode          string
	RecoveryPassword       string
	EnablePasswordRecovery bool
	DeviceName             string
	Platform               DevicePlatform
	PublicKey              string
	PushToken              string
	IdempotencyKey         string
	RequestedAt            time.Time
}

// AuthenticateBotParams contains the bot token login payload.
type AuthenticateBotParams struct {
	BotToken       string
	DeviceName     string
	Platform       DevicePlatform
	PublicKey      string
	ClientVersion  string
	Locale         string
	IdempotencyKey string
	RequestedAt    time.Time
}

// RefreshSessionParams contains the refresh-token rotation payload.
type RefreshSessionParams struct {
	RefreshToken   string
	DeviceID       string
	IdempotencyKey string
	RequestedAt    time.Time
}

// RegisterDeviceParams contains the device registration payload.
type RegisterDeviceParams struct {
	SessionID      string
	DeviceName     string
	Platform       DevicePlatform
	PublicKey      string
	PushToken      string
	IdempotencyKey string
	RequestedAt    time.Time
}

// RevokeSessionParams contains the session revocation payload.
type RevokeSessionParams struct {
	SessionID      string
	Reason         string
	IdempotencyKey string
	RequestedAt    time.Time
}

// RevokeAllSessionsParams contains the account-wide session revocation payload.
type RevokeAllSessionsParams struct {
	Reason         string
	IdempotencyKey string
	RequestedAt    time.Time
}
