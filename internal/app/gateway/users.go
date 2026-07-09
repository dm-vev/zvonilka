package gateway

import (
	"context"
	"strings"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	usersv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/users/v1"
	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

// GetMyProfile returns the authenticated account profile.
func (a *api) GetMyProfile(
	ctx context.Context,
	_ *usersv1.GetMyProfileRequest,
) (*usersv1.GetMyProfileResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	userProfile, err := a.profileByID(ctx, authContext.Account.ID, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &usersv1.GetMyProfileResponse{Profile: userProfile}, nil
}

// UpdateMyProfile updates the authenticated account's mutable profile fields.
func (a *api) UpdateMyProfile(
	ctx context.Context,
	req *usersv1.UpdateMyProfileRequest,
) (*usersv1.UpdateMyProfileResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetProfile() == nil {
		return nil, grpcError(domainuser.ErrInvalidInput)
	}

	params := identityProfileUpdate(authContext.Account.ID, req)
	account, err := a.identity.UpdateProfile(ctx, params)
	if err != nil {
		return nil, grpcError(err)
	}

	userProfile, err := a.profileByID(ctx, account.ID, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	events, err := a.conversation.PublishUserUpdate(ctx, domainconversation.PublishUserUpdateParams{
		AccountID:   account.ID,
		DeviceID:    authContext.Device.ID,
		PayloadType: "profile",
		Metadata: map[string]string{
			"scope": "profile",
		},
		CreatedAt: account.UpdatedAt,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvents(events...)

	return &usersv1.UpdateMyProfileResponse{Profile: userProfile}, nil
}

// UpdatePrivacySettings stores privacy settings for the authenticated account.
func (a *api) UpdatePrivacySettings(
	ctx context.Context,
	req *usersv1.UpdatePrivacySettingsRequest,
) (*usersv1.UpdatePrivacySettingsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetPrivacy() == nil {
		return nil, grpcError(domainuser.ErrInvalidInput)
	}

	privacy, err := a.user.UpdatePrivacy(ctx, domainuser.UpdatePrivacyParams{
		AccountID: authContext.Account.ID,
		Privacy: domainuser.Privacy{
			PhoneVisibility:     visibilityFromProto(req.GetPrivacy().GetPhoneVisibility()),
			LastSeenVisibility:  visibilityFromProto(req.GetPrivacy().GetLastSeenVisibility()),
			MessagePrivacy:      visibilityFromProto(req.GetPrivacy().GetMessagePrivacy()),
			BirthdayVisibility:  visibilityFromProto(req.GetPrivacy().GetBirthdayVisibility()),
			AllowContactSync:    req.GetPrivacy().GetAllowContactSync(),
			AllowUnknownSenders: req.GetPrivacy().GetAllowUnknownSenders(),
			AllowUsernameSearch: req.GetPrivacy().GetAllowUsernameSearch(),
		},
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &usersv1.UpdatePrivacySettingsResponse{Privacy: privacyToProto(privacy)}, nil
}

// SetPresence stores an explicit presence preference for the authenticated account.
func (a *api) SetPresence(
	ctx context.Context,
	req *usersv1.SetPresenceRequest,
) (*usersv1.SetPresenceResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	hideLastSeenFor := timeDuration(req.GetHideLastSeenFor())
	state, err := a.presence.SetPresence(ctx, domainpresence.SetParams{
		AccountID:       authContext.Account.ID,
		State:           presenceStateFromProto(req.GetPresence()),
		CustomStatus:    req.GetCustomStatus(),
		HideLastSeenFor: hideLastSeenFor,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	events, err := a.conversation.PublishUserUpdate(ctx, domainconversation.PublishUserUpdateParams{
		AccountID:   authContext.Account.ID,
		DeviceID:    authContext.Device.ID,
		PayloadType: "presence",
		Metadata: map[string]string{
			"scope": "presence",
		},
		CreatedAt: state.UpdatedAt,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvents(events...)

	userProfile, err := a.profileByID(ctx, authContext.Account.ID, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}
	return &usersv1.SetPresenceResponse{Profile: userProfile}, nil
}

// GetUser returns one account profile by ID or username.
func (a *api) GetUser(
	ctx context.Context,
	req *usersv1.GetUserRequest,
) (*usersv1.GetUserResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	var profileID string
	switch lookup := req.Lookup.(type) {
	case *usersv1.GetUserRequest_UserId:
		profileID = lookup.UserId
	case *usersv1.GetUserRequest_Username:
		account, loadErr := a.identity.AccountByUsername(ctx, lookup.Username)
		if loadErr != nil {
			return nil, grpcError(loadErr)
		}
		profileID = account.ID
	default:
		return nil, grpcError(domainsearch.ErrInvalidInput)
	}

	userProfile, err := a.profileByID(ctx, profileID, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &usersv1.GetUserResponse{Profile: userProfile}, nil
}

// SearchUsers returns indexed user hits together with filtered profile projections.
func (a *api) SearchUsers(
	ctx context.Context,
	req *usersv1.SearchUsersRequest,
) (*usersv1.SearchUsersResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	offset, err := decodeOffset(req.GetPage(), "users")
	if err != nil {
		return nil, grpcError(domainsearch.ErrInvalidInput)
	}

	result, err := a.search.Search(ctx, domainsearch.SearchParams{
		Query:  req.GetQuery(),
		Scopes: []domainsearch.SearchScope{domainsearch.SearchScopeUsers},
		Limit:  pageSize(req.GetPage()),
		Offset: offset,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	userIDs := make([]string, 0, len(result.Hits))
	seen := make(map[string]struct{}, len(result.Hits))
	for _, hit := range result.Hits {
		userID := strings.TrimSpace(hit.UserID)
		if userID == "" {
			userID = strings.TrimSpace(hit.TargetID)
		}
		if userID == "" {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		userIDs = append(userIDs, userID)
	}

	relations, err := a.user.Relations(ctx, authContext.Account.ID, userIDs)
	if err != nil {
		return nil, grpcError(err)
	}
	profiles, err := a.profilesByID(ctx, userIDs, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	responseProfiles := make([]*usersv1.UserProfile, 0, len(result.Hits))
	for _, hit := range result.Hits {
		userID := strings.TrimSpace(hit.UserID)
		if userID == "" {
			userID = strings.TrimSpace(hit.TargetID)
		}
		profile, ok := profiles[userID]
		if !ok {
			continue
		}
		if !req.GetIncludeBlocked() && relations[userID].IsBlocked {
			continue
		}
		if req.GetIncludeContactsOnly() && !relations[userID].IsContact {
			continue
		}
		responseProfiles = append(responseProfiles, profile)
	}

	return &usersv1.SearchUsersResponse{
		Profiles: responseProfiles,
		Page: &commonv1.PageResponse{
			NextPageToken: result.NextPageToken,
			TotalSize:     result.TotalSize,
		},
	}, nil
}

// ListContacts returns the authenticated account's contacts.
func (a *api) ListContacts(
	ctx context.Context,
	req *usersv1.ListContactsRequest,
) (*usersv1.ListContactsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	contacts, err := a.user.ListContacts(ctx, domainuser.ListContactsParams{
		AccountID:   authContext.Account.ID,
		Query:       req.GetQuery(),
		StarredOnly: req.GetStarredOnly(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	offset, err := decodeOffset(req.GetPage(), "contacts")
	if err != nil {
		return nil, grpcError(domainuser.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(contacts) {
		end = len(contacts)
	}

	page := contacts
	if offset < len(contacts) {
		page = contacts[offset:end]
	} else {
		page = nil
	}

	result := make([]*usersv1.Contact, 0, len(page))
	for _, contact := range page {
		result = append(result, userContact(contact))
	}

	nextToken := ""
	if end < len(contacts) {
		nextToken = offsetToken("contacts", end)
	}

	return &usersv1.ListContactsResponse{
		Contacts: result,
		Page: &commonv1.PageResponse{
			NextPageToken: nextToken,
			TotalSize:     uint64(len(contacts)),
		},
	}, nil
}

// SyncContacts matches a device phonebook snapshot against platform accounts.
func (a *api) SyncContacts(
	ctx context.Context,
	req *usersv1.SyncContactsRequest,
) (*usersv1.SyncContactsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	contacts := make([]domainuser.SyncedContact, 0, len(req.GetContacts()))
	for _, contact := range req.GetContacts() {
		contacts = append(contacts, domainuser.SyncedContact{
			RawContactID: contact.GetRawContactId(),
			DisplayName:  contact.GetDisplayName(),
			PhoneE164:    contact.GetPhoneE164(),
			Email:        contact.GetEmail(),
			Checksum:     contact.GetChecksum(),
		})
	}

	result, err := a.user.SyncContacts(ctx, domainuser.SyncParams{
		AccountID:      authContext.Account.ID,
		SourceDeviceID: req.GetSourceDeviceId(),
		Contacts:       contacts,
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	matches := make([]*usersv1.ContactMatch, 0, len(result.Matches))
	for _, match := range result.Matches {
		matches = append(matches, &usersv1.ContactMatch{
			RawContactId:  match.RawContactID,
			ContactUserId: match.ContactUserID,
			DisplayName:   match.DisplayName,
			Username:      match.Username,
		})
	}

	return &usersv1.SyncContactsResponse{
		Matches:         matches,
		NewContacts:     result.NewContacts,
		UpdatedContacts: result.UpdatedContacts,
		RemovedContacts: result.RemovedContacts,
	}, nil
}

// AddContact creates or updates one manual contact entry.
func (a *api) AddContact(
	ctx context.Context,
	req *usersv1.AddContactRequest,
) (*usersv1.AddContactResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	contact, err := a.user.AddContact(ctx, domainuser.AddContactParams{
		AccountID:      authContext.Account.ID,
		ContactUserID:  req.GetUserId(),
		Alias:          req.GetAlias(),
		Starred:        req.GetStarred(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &usersv1.AddContactResponse{Contact: userContact(contact)}, nil
}

// RemoveContact removes one manual or synced contact entry.
func (a *api) RemoveContact(
	ctx context.Context,
	req *usersv1.RemoveContactRequest,
) (*usersv1.RemoveContactResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	removed, err := a.user.RemoveContact(ctx, domainuser.RemoveContactParams{
		AccountID:      authContext.Account.ID,
		ContactUserID:  req.GetUserId(),
		Reason:         req.GetReason(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &usersv1.RemoveContactResponse{Removed: removed}, nil
}

// BlockUser stores one blocklist entry for the authenticated account.
func (a *api) BlockUser(
	ctx context.Context,
	req *usersv1.BlockUserRequest,
) (*usersv1.BlockUserResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	entry, err := a.user.BlockUser(ctx, domainuser.BlockParams{
		AccountID:      authContext.Account.ID,
		BlockedUserID:  req.GetUserId(),
		Reason:         req.GetReason(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &usersv1.BlockUserResponse{Entry: userBlock(entry)}, nil
}

// UnblockUser removes one blocklist entry for the authenticated account.
func (a *api) UnblockUser(
	ctx context.Context,
	req *usersv1.UnblockUserRequest,
) (*usersv1.UnblockUserResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	removed, err := a.user.UnblockUser(ctx, domainuser.UnblockParams{
		AccountID:      authContext.Account.ID,
		BlockedUserID:  req.GetUserId(),
		Reason:         req.GetReason(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &usersv1.UnblockUserResponse{Removed: removed}, nil
}

// ListBlockedUsers returns the authenticated account's blocklist.
func (a *api) ListBlockedUsers(
	ctx context.Context,
	req *usersv1.ListBlockedUsersRequest,
) (*usersv1.ListBlockedUsersResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	entries, err := a.user.ListBlockedUsers(ctx, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	offset, err := decodeOffset(req.GetPage(), "blocks")
	if err != nil {
		return nil, grpcError(domainuser.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(entries) {
		end = len(entries)
	}

	page := entries
	if offset < len(entries) {
		page = entries[offset:end]
	} else {
		page = nil
	}

	result := make([]*usersv1.BlockEntry, 0, len(page))
	for _, entry := range page {
		result = append(result, userBlock(entry))
	}

	nextToken := ""
	if end < len(entries) {
		nextToken = offsetToken("blocks", end)
	}

	return &usersv1.ListBlockedUsersResponse{
		BlockedUsers: result,
		Page: &commonv1.PageResponse{
			NextPageToken: nextToken,
			TotalSize:     uint64(len(entries)),
		},
	}, nil
}

func (a *api) profileByID(ctx context.Context, accountID string, viewerID string) (*usersv1.UserProfile, error) {
	profiles, err := a.profilesByID(ctx, []string{accountID}, viewerID)
	if err != nil {
		return nil, err
	}

	userProfile, ok := profiles[accountID]
	if !ok {
		return nil, domainuser.ErrNotFound
	}

	return userProfile, nil
}

func (a *api) profilesByID(
	ctx context.Context,
	accountIDs []string,
	viewerID string,
) (map[string]*usersv1.UserProfile, error) {
	if len(accountIDs) == 0 {
		return map[string]*usersv1.UserProfile{}, nil
	}

	presenceSnapshots, err := a.presence.ListPresence(ctx, accountIDs, viewerID)
	if err != nil {
		return nil, err
	}

	relations, err := a.user.Relations(ctx, viewerID, accountIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*usersv1.UserProfile, len(accountIDs))
	for _, accountID := range accountIDs {
		accountID = strings.TrimSpace(accountID)
		if accountID == "" {
			continue
		}

		account, loadErr := a.identity.AccountByID(ctx, accountID)
		if loadErr != nil {
			continue
		}
		privacy, loadErr := a.user.GetPrivacy(ctx, accountID)
		if loadErr != nil {
			return nil, loadErr
		}

		relation := relations[accountID]
		snapshot := presenceSnapshots[accountID]
		result[accountID] = userProfile(account, snapshot, privacy, relation, viewerID == accountID)
	}

	return result, nil
}

func identityProfileUpdate(accountID string, req *usersv1.UpdateMyProfileRequest) domainidentity.UpdateProfileParams {
	profile := req.GetProfile()
	params := domainidentity.UpdateProfileParams{
		AccountID:        accountID,
		IdempotencyKey:   req.GetIdempotencyKey(),
		Username:         profile.GetUsername(),
		DisplayName:      profile.GetDisplayName(),
		Bio:              profile.GetBio(),
		Email:            profile.GetEmail(),
		Phone:            profile.GetPhone(),
		CustomBadgeEmoji: profile.GetCustomBadgeEmoji(),
	}

	if req.GetUpdateMask() == nil || len(req.GetUpdateMask().GetPaths()) == 0 {
		return params
	}

	masked := domainidentity.UpdateProfileParams{
		AccountID:      accountID,
		IdempotencyKey: req.GetIdempotencyKey(),
	}
	for _, path := range req.GetUpdateMask().GetPaths() {
		switch path {
		case "username":
			masked.Username = profile.GetUsername()
		case "display_name":
			masked.DisplayName = profile.GetDisplayName()
		case "bio":
			masked.Bio = profile.GetBio()
		case "email":
			masked.Email = profile.GetEmail()
		case "phone":
			masked.Phone = profile.GetPhone()
		case "custom_badge_emoji":
			masked.CustomBadgeEmoji = profile.GetCustomBadgeEmoji()
		}
	}

	return masked
}
