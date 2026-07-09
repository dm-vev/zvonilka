package pgstore

import (
	"database/sql"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPrivacy(row rowScanner) (domainuser.Privacy, error) {
	var privacy domainuser.Privacy
	if err := row.Scan(
		&privacy.AccountID,
		&privacy.PhoneVisibility,
		&privacy.LastSeenVisibility,
		&privacy.MessagePrivacy,
		&privacy.BirthdayVisibility,
		&privacy.AllowContactSync,
		&privacy.AllowUnknownSenders,
		&privacy.AllowUsernameSearch,
		&privacy.CreatedAt,
		&privacy.UpdatedAt,
	); err != nil {
		return domainuser.Privacy{}, err
	}

	privacy.CreatedAt = privacy.CreatedAt.UTC()
	privacy.UpdatedAt = privacy.UpdatedAt.UTC()
	return privacy, nil
}

func scanContact(row rowScanner) (domainuser.Contact, error) {
	var contact domainuser.Contact
	if err := row.Scan(
		&contact.OwnerAccountID,
		&contact.ContactAccountID,
		&contact.DisplayName,
		&contact.Username,
		&contact.PhoneHash,
		&contact.Source,
		&contact.Starred,
		&contact.RawContactID,
		&contact.SourceDeviceID,
		&contact.SyncChecksum,
		&contact.AddedAt,
		&contact.UpdatedAt,
	); err != nil {
		return domainuser.Contact{}, err
	}

	contact.AddedAt = contact.AddedAt.UTC()
	contact.UpdatedAt = contact.UpdatedAt.UTC()
	return contact, nil
}

func scanBlock(row rowScanner) (domainuser.BlockEntry, error) {
	var block domainuser.BlockEntry
	if err := row.Scan(
		&block.OwnerAccountID,
		&block.BlockedAccountID,
		&block.Reason,
		&block.BlockedAt,
		&block.UpdatedAt,
	); err != nil {
		return domainuser.BlockEntry{}, err
	}

	block.BlockedAt = block.BlockedAt.UTC()
	block.UpdatedAt = block.UpdatedAt.UTC()
	return block, nil
}

func isNoRows(err error) bool {
	return err == sql.ErrNoRows
}
