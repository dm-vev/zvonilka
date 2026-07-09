package identity

import "time"

// AccountKind distinguishes human accounts from bot accounts.
type AccountKind string

// Account kinds used by the identity layer.
const (
	// AccountKindUnspecified is the zero value.
	AccountKindUnspecified AccountKind = ""
	// AccountKindUser marks a human-operated account.
	AccountKindUser AccountKind = "user"
	// AccountKindBot marks an automation account.
	AccountKindBot AccountKind = "bot"
)

// AccountStatus describes the lifecycle state of an account.
type AccountStatus string

// Account statuses used by the identity layer.
const (
	// AccountStatusActive indicates the account can authenticate.
	AccountStatusActive AccountStatus = "active"
	// AccountStatusSuspended indicates the account is temporarily disabled.
	AccountStatusSuspended AccountStatus = "suspended"
	// AccountStatusRevoked indicates the account was removed or blocked.
	AccountStatusRevoked AccountStatus = "revoked"
)

// Role identifies a coarse-grained account role.
type Role string

// Roles used by the identity layer.
const (
	// RoleOwner grants full ownership.
	RoleOwner Role = "owner"
	// RoleAdmin grants admin capabilities.
	RoleAdmin Role = "admin"
	// RoleModerator grants moderation capabilities.
	RoleModerator Role = "moderator"
	// RoleSupport grants support capabilities.
	RoleSupport Role = "support"
	// RoleAuditor grants read-only audit capabilities.
	RoleAuditor Role = "auditor"
)

// JoinRequestStatus describes the status of a self-service join request.
type JoinRequestStatus string

// Join request states used by the identity layer.
const (
	// JoinRequestStatusPending indicates the request awaits review.
	JoinRequestStatusPending JoinRequestStatus = "pending"
	// JoinRequestStatusApproved indicates the request was approved.
	JoinRequestStatusApproved JoinRequestStatus = "approved"
	// JoinRequestStatusRejected indicates the request was rejected.
	JoinRequestStatusRejected JoinRequestStatus = "rejected"
	// JoinRequestStatusCancelled indicates the request was cancelled by the user.
	JoinRequestStatusCancelled JoinRequestStatus = "cancelled"
	// JoinRequestStatusExpired indicates the request expired before review.
	JoinRequestStatusExpired JoinRequestStatus = "expired"
)

// DevicePlatform identifies the client platform.
type DevicePlatform string

// Device platforms used by the identity layer.
const (
	// DevicePlatformUnspecified is the zero value.
	DevicePlatformUnspecified DevicePlatform = ""
	// DevicePlatformIOS is the iOS platform.
	DevicePlatformIOS DevicePlatform = "ios"
	// DevicePlatformAndroid is the Android platform.
	DevicePlatformAndroid DevicePlatform = "android"
	// DevicePlatformWeb is the web platform.
	DevicePlatformWeb DevicePlatform = "web"
	// DevicePlatformDesktop is the desktop platform.
	DevicePlatformDesktop DevicePlatform = "desktop"
	// DevicePlatformServer is the server platform.
	DevicePlatformServer DevicePlatform = "server"
)

// DeviceStatus describes the lifecycle state of a device.
type DeviceStatus string

// Device statuses used by the identity layer.
const (
	// DeviceStatusActive indicates the device may authenticate.
	DeviceStatusActive DeviceStatus = "active"
	// DeviceStatusSuspended indicates the device is temporarily disabled.
	DeviceStatusSuspended DeviceStatus = "suspended"
	// DeviceStatusRevoked indicates the device was revoked.
	DeviceStatusRevoked DeviceStatus = "revoked"
	// DeviceStatusUnverified indicates the device has not completed trust bootstrap.
	DeviceStatusUnverified DeviceStatus = "unverified"
)

// SessionStatus describes the lifecycle state of a session.
type SessionStatus string

// Session statuses used by the identity layer.
const (
	// SessionStatusActive indicates the session is usable.
	SessionStatusActive SessionStatus = "active"
	// SessionStatusRevoked indicates the session has been revoked.
	SessionStatusRevoked SessionStatus = "revoked"
)

// LoginDeliveryChannel identifies where the login code was sent.
type LoginDeliveryChannel string

// Login delivery channels used by the identity layer.
const (
	// LoginDeliveryChannelUnspecified is the zero value.
	LoginDeliveryChannelUnspecified LoginDeliveryChannel = ""
	// LoginDeliveryChannelSMS sends the code via SMS.
	LoginDeliveryChannelSMS LoginDeliveryChannel = "sms"
	// LoginDeliveryChannelEmail sends the code via email.
	LoginDeliveryChannelEmail LoginDeliveryChannel = "email"
	// LoginDeliveryChannelPush sends the code via push.
	LoginDeliveryChannelPush LoginDeliveryChannel = "push"
	// LoginDeliveryChannelManual is reserved for controlled bootstrap scenarios.
	LoginDeliveryChannelManual LoginDeliveryChannel = "manual"
)

// Account describes a platform account.
type Account struct {
	ID               string
	Kind             AccountKind
	Username         string
	DisplayName      string
	Bio              string
	Email            string
	Phone            string
	Roles            []Role
	Status           AccountStatus
	BotTokenHash     string
	CreatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DisabledAt       time.Time
	LastAuthAt       time.Time
	CustomBadgeEmoji string
}

// JoinRequest describes a pending account request.
type JoinRequest struct {
	ID             string
	Username       string
	DisplayName    string
	Email          string
	Phone          string
	Note           string
	Status         JoinRequestStatus
	RequestedAt    time.Time
	ReviewedAt     time.Time
	ReviewedBy     string
	DecisionReason string
	ExpiresAt      time.Time
}

// LoginTarget describes a channel that can receive a login code.
type LoginTarget struct {
	Channel         LoginDeliveryChannel
	DestinationMask string
	Primary         bool
	Verified        bool
}

// LoginChallenge describes a pending login verification step.
type LoginChallenge struct {
	ID              string
	AccountID       string
	AccountKind     AccountKind
	Purpose         LoginChallengePurpose
	CodeHash        string
	DeliveryChannel LoginDeliveryChannel
	Targets         []LoginTarget
	ExpiresAt       time.Time
	CreatedAt       time.Time
	UsedAt          time.Time
	Used            bool
}

// Device describes a trusted device.
type Device struct {
	ID            string
	AccountID     string
	SessionID     string
	Name          string
	Platform      DevicePlatform
	Status        DeviceStatus
	PublicKey     string
	PushToken     string
	CreatedAt     time.Time
	LastSeenAt    time.Time
	RevokedAt     time.Time
	LastRotatedAt time.Time
}

// Session describes a user session.
type Session struct {
	ID             string
	AccountID      string
	DeviceID       string
	DeviceName     string
	DevicePlatform DevicePlatform
	IPAddress      string
	UserAgent      string
	Status         SessionStatus
	Current        bool
	CreatedAt      time.Time
	LastSeenAt     time.Time
	RevokedAt      time.Time
}

// Tokens describes an issued access token pair.
type Tokens struct {
	AccessToken      string
	RefreshToken     string
	TokenType        string
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
}

// LoginResult bundles the login output.
type LoginResult struct {
	Tokens  Tokens
	Session Session
	Device  Device
}

// LoginChallengePurpose identifies why a challenge was issued.
type LoginChallengePurpose string

// Supported login-challenge purposes.
const (
	// LoginChallengePurposeLogin represents a normal login flow.
	LoginChallengePurposeLogin LoginChallengePurpose = "login"
	// LoginChallengePurposeRecovery represents a password recovery flow.
	LoginChallengePurposeRecovery LoginChallengePurpose = "recovery"
)

// AccountCredentialKind identifies a persisted account secret.
type AccountCredentialKind string

// Supported account-credential kinds.
const (
	// AccountCredentialKindPassword stores the account password hash.
	AccountCredentialKindPassword AccountCredentialKind = "password"
	// AccountCredentialKindRecovery stores the recovery password hash.
	AccountCredentialKindRecovery AccountCredentialKind = "recovery"
)

// AccountCredential stores one hashed account secret.
type AccountCredential struct {
	AccountID  string
	Kind       AccountCredentialKind
	SecretHash string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// LoginFactor identifies one supported login factor.
type LoginFactor string

// Supported login factors.
const (
	// LoginFactorCode represents code-based login.
	LoginFactorCode LoginFactor = "code"
	// LoginFactorPassword represents a stored password.
	LoginFactorPassword LoginFactor = "password"
	// LoginFactorRecoveryPassword represents a stored recovery password.
	LoginFactorRecoveryPassword LoginFactor = "recovery_password"
	// LoginFactorBotToken represents a bot token.
	LoginFactorBotToken LoginFactor = "bot_token"
)

// LoginOption describes one available auth factor.
type LoginOption struct {
	Factor   LoginFactor
	Required bool
	Channels []LoginDeliveryChannel
}

// LoginOptionsResult describes the factors available for one identifier.
type LoginOptionsResult struct {
	Options                 []LoginOption
	PasswordRecoveryEnabled bool
	RequiresAdminApproval   bool
	AccountKind             AccountKind
}
