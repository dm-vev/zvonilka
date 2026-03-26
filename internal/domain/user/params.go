package user

// UpdatePrivacyParams contains one privacy update request.
type UpdatePrivacyParams struct {
	AccountID      string
	Privacy        Privacy
	IdempotencyKey string
}

// ListContactsParams contains one contact-list request.
type ListContactsParams struct {
	AccountID   string
	Query       string
	StarredOnly bool
}

// AddContactParams contains one manual-contact request.
type AddContactParams struct {
	AccountID      string
	ContactUserID  string
	Alias          string
	Starred        bool
	IdempotencyKey string
}

// RemoveContactParams contains one contact-removal request.
type RemoveContactParams struct {
	AccountID      string
	ContactUserID  string
	Reason         string
	IdempotencyKey string
}

// SyncParams contains one phonebook sync request.
type SyncParams struct {
	AccountID      string
	SourceDeviceID string
	Contacts       []SyncedContact
	IdempotencyKey string
}

// BlockParams contains one block request.
type BlockParams struct {
	AccountID      string
	BlockedUserID  string
	Reason         string
	IdempotencyKey string
}

// UnblockParams contains one unblock request.
type UnblockParams struct {
	AccountID      string
	BlockedUserID  string
	Reason         string
	IdempotencyKey string
}
