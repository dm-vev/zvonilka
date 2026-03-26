package user

import "time"

// Visibility describes who can see a profile field or action.
type Visibility string

const (
	// VisibilityUnspecified is the zero value.
	VisibilityUnspecified Visibility = ""
	// VisibilityEveryone exposes the value to everyone.
	VisibilityEveryone Visibility = "everyone"
	// VisibilityContacts exposes the value to contacts only.
	VisibilityContacts Visibility = "contacts"
	// VisibilityNobody hides the value from everyone else.
	VisibilityNobody Visibility = "nobody"
	// VisibilityCustom reserves room for explicit allowlists later.
	VisibilityCustom Visibility = "custom"
)

// Privacy stores per-account visibility preferences.
type Privacy struct {
	AccountID           string
	PhoneVisibility     Visibility
	LastSeenVisibility  Visibility
	MessagePrivacy      Visibility
	BirthdayVisibility  Visibility
	AllowContactSync    bool
	AllowUnknownSenders bool
	AllowUsernameSearch bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ContactSource tracks how a contact entry was introduced.
type ContactSource string

const (
	// ContactSourceUnspecified is the zero value.
	ContactSourceUnspecified ContactSource = ""
	// ContactSourceManual represents a manually added contact.
	ContactSourceManual ContactSource = "manual"
	// ContactSourceImported represents an imported contact entry.
	ContactSourceImported ContactSource = "imported"
	// ContactSourceSynced represents a phonebook-synced contact.
	ContactSourceSynced ContactSource = "synced"
	// ContactSourceInvited represents an invited contact entry.
	ContactSourceInvited ContactSource = "invited"
)

// Contact stores one owner->contact relationship.
type Contact struct {
	OwnerAccountID   string
	ContactAccountID string
	DisplayName      string
	Username         string
	PhoneHash        string
	Source           ContactSource
	Starred          bool
	AddedAt          time.Time
	UpdatedAt        time.Time
	RawContactID     string
	SourceDeviceID   string
	SyncChecksum     string
}

// BlockEntry stores one owner->blocked relationship.
type BlockEntry struct {
	OwnerAccountID   string
	BlockedAccountID string
	Reason           string
	BlockedAt        time.Time
	UpdatedAt        time.Time
}

// SyncedContact is the normalized phonebook payload supplied by the client.
type SyncedContact struct {
	RawContactID string
	DisplayName  string
	PhoneE164    string
	Email        string
	Checksum     string
}

// ContactMatch reports one successful sync match against a platform account.
type ContactMatch struct {
	RawContactID  string
	ContactUserID string
	DisplayName   string
	Username      string
}

// SyncResult reports the materialized sync outcome.
type SyncResult struct {
	Matches         []ContactMatch
	NewContacts     uint32
	UpdatedContacts uint32
	RemovedContacts uint32
}

// Relation describes the viewer's relationship to another account.
type Relation struct {
	IsContact bool
	IsBlocked bool
}
