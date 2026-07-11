package pgstore

import (
	"database/sql"
	"encoding/json"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPrivacy(row rowScanner) (domainuser.Privacy, error) {
	var privacy domainuser.Privacy
	var rulesJSON []byte
	if err := row.Scan(
		&privacy.AccountID,
		&privacy.PhoneVisibility,
		&privacy.LastSeenVisibility,
		&privacy.MessagePrivacy,
		&privacy.BirthdayVisibility,
		&privacy.AllowContactSync,
		&privacy.AllowUnknownSenders,
		&privacy.AllowUsernameSearch,
		&rulesJSON,
		&privacy.ShowReadDate,
		&privacy.CreatedAt,
		&privacy.UpdatedAt,
	); err != nil {
		return domainuser.Privacy{}, err
	}
	var rules privacyRules
	if err := json.Unmarshal(rulesJSON, &rules); err != nil {
		return domainuser.Privacy{}, err
	}
	applyPrivacyRules(&privacy, rules)
	fillPrivacyRuleDefaults(&privacy)

	privacy.CreatedAt = privacy.CreatedAt.UTC()
	privacy.UpdatedAt = privacy.UpdatedAt.UTC()
	return privacy, nil
}

type privacyRules struct {
	ShowStatus                            domainuser.PrivacyRule `json:"show_status"`
	ShowProfilePhoto                      domainuser.PrivacyRule `json:"show_profile_photo"`
	ShowProfileAudio                      domainuser.PrivacyRule `json:"show_profile_audio"`
	ShowBirthdate                         domainuser.PrivacyRule `json:"show_birthdate"`
	ShowBio                               domainuser.PrivacyRule `json:"show_bio"`
	ShowPhoneNumber                       domainuser.PrivacyRule `json:"show_phone_number"`
	AllowFindingByPhoneNumber             domainuser.PrivacyRule `json:"allow_finding_by_phone_number"`
	ShowLinkInForwardedMessages           domainuser.PrivacyRule `json:"show_link_in_forwarded_messages"`
	AllowChatInvites                      domainuser.PrivacyRule `json:"allow_chat_invites"`
	AllowPrivateVoiceAndVideoNoteMessages domainuser.PrivacyRule `json:"allow_private_voice_and_video_note_messages"`
	AllowCalls                            domainuser.PrivacyRule `json:"allow_calls"`
	AllowPeerToPeerCalls                  domainuser.PrivacyRule `json:"allow_peer_to_peer_calls"`
	AutosaveGifts                         domainuser.PrivacyRule `json:"autosave_gifts"`
}

func privacyRulesFromPrivacy(privacy domainuser.Privacy) privacyRules {
	return privacyRules{
		privacy.ShowStatus, privacy.ShowProfilePhoto, privacy.ShowProfileAudio, privacy.ShowBirthdate,
		privacy.ShowBio, privacy.ShowPhoneNumber, privacy.AllowFindingByPhoneNumber,
		privacy.ShowLinkInForwardedMessages, privacy.AllowChatInvites,
		privacy.AllowPrivateVoiceAndVideoNoteMessages, privacy.AllowCalls,
		privacy.AllowPeerToPeerCalls, privacy.AutosaveGifts,
	}
}

func applyPrivacyRules(privacy *domainuser.Privacy, rules privacyRules) {
	privacy.ShowStatus = rules.ShowStatus
	privacy.ShowProfilePhoto = rules.ShowProfilePhoto
	privacy.ShowProfileAudio = rules.ShowProfileAudio
	privacy.ShowBirthdate = rules.ShowBirthdate
	privacy.ShowBio = rules.ShowBio
	privacy.ShowPhoneNumber = rules.ShowPhoneNumber
	privacy.AllowFindingByPhoneNumber = rules.AllowFindingByPhoneNumber
	privacy.ShowLinkInForwardedMessages = rules.ShowLinkInForwardedMessages
	privacy.AllowChatInvites = rules.AllowChatInvites
	privacy.AllowPrivateVoiceAndVideoNoteMessages = rules.AllowPrivateVoiceAndVideoNoteMessages
	privacy.AllowCalls = rules.AllowCalls
	privacy.AllowPeerToPeerCalls = rules.AllowPeerToPeerCalls
	privacy.AutosaveGifts = rules.AutosaveGifts
}

func fillPrivacyRuleDefaults(privacy *domainuser.Privacy) {
	defaults := []struct {
		rule *domainuser.PrivacyRule
		base domainuser.Visibility
	}{
		{&privacy.ShowStatus, privacy.LastSeenVisibility},
		{&privacy.ShowProfilePhoto, domainuser.VisibilityEveryone},
		{&privacy.ShowProfileAudio, domainuser.VisibilityEveryone},
		{&privacy.ShowBirthdate, privacy.BirthdayVisibility},
		{&privacy.ShowBio, domainuser.VisibilityEveryone},
		{&privacy.ShowPhoneNumber, privacy.PhoneVisibility},
		{&privacy.AllowFindingByPhoneNumber, domainuser.VisibilityEveryone},
		{&privacy.ShowLinkInForwardedMessages, domainuser.VisibilityEveryone},
		{&privacy.AllowChatInvites, domainuser.VisibilityEveryone},
		{&privacy.AllowPrivateVoiceAndVideoNoteMessages, domainuser.VisibilityEveryone},
		{&privacy.AllowCalls, domainuser.VisibilityEveryone},
		{&privacy.AllowPeerToPeerCalls, domainuser.VisibilityContacts},
		{&privacy.AutosaveGifts, domainuser.VisibilityEveryone},
	}
	for _, value := range defaults {
		if value.rule.Base == domainuser.VisibilityUnspecified {
			value.rule.Base = value.base
		}
	}
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
