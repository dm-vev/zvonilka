package gateway

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	mediav1 "github.com/dm-vev/zvonilka/gen/proto/contracts/media/v1"
	usersv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/users/v1"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestIdentityProfileUpdateValidatesFieldMaskAndAvatarPresence(t *testing.T) {
	t.Parallel()

	avatar := &usersv1.ProfileAvatarUpdate{Clear: true}
	params, err := identityProfileUpdate("account-1", &usersv1.UpdateMyProfileRequest{
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"avatar"}},
		Avatar:     avatar,
	})
	if err != nil {
		t.Fatalf("clear avatar update: %v", err)
	}
	if params.AvatarMediaID == nil || *params.AvatarMediaID != "" {
		t.Fatalf("unexpected clear avatar params: %+v", params)
	}

	_, err = identityProfileUpdate("account-1", &usersv1.UpdateMyProfileRequest{
		Profile:    &usersv1.UserProfile{},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"email"}},
	})
	if !errors.Is(err, identity.ErrInvalidInput) {
		t.Fatalf("unknown profile path error = %v", err)
	}

	_, err = identityProfileUpdate("account-1", &usersv1.UpdateMyProfileRequest{
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"avatar"}},
		Avatar:     &usersv1.ProfileAvatarUpdate{},
	})
	if !errors.Is(err, identity.ErrInvalidInput) {
		t.Fatalf("empty avatar media error = %v", err)
	}
}

func TestUpdateMyProfileAndPrivacyRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	account, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "profile-user",
		DisplayName: "Profile User",
		Email:       "profile@example.com",
		Phone:       "+1 (555) 123-0000",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	begin, err := api.BeginLogin(ctx, &authv1.BeginLoginRequest{
		Identifier:      &authv1.BeginLoginRequest_Username{Username: account.Username},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
		DeviceName:      "phone",
		DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	verify, err := api.VerifyLoginCode(ctx, &authv1.VerifyLoginCodeRequest{
		ChallengeId:    begin.ChallengeId,
		Code:           sender.code(begin.Targets[0].DestinationMask),
		DeviceName:     "phone",
		DevicePlatform: commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		DeviceKey:      &commonv1.PublicKeyBundle{PublicKey: []byte("device-key")},
	})
	if err != nil {
		t.Fatalf("verify login: %v", err)
	}

	authCtx := metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+verify.Tokens.AccessToken))
	updated, err := api.UpdateMyProfile(authCtx, &usersv1.UpdateMyProfileRequest{
		Profile: &usersv1.UserProfile{
			Username:         "profile-user",
			DisplayName:      "Updated Name",
			Bio:              "Updated bio",
			Phone:            "+1 (555) 123-9999",
			CustomBadgeEmoji: "🔥",
		},
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.Profile.DisplayName != "Updated Name" || updated.Profile.CustomBadgeEmoji != "🔥" {
		t.Fatalf("unexpected updated profile: %+v", updated.Profile)
	}

	privacy, err := api.UpdatePrivacySettings(authCtx, &usersv1.UpdatePrivacySettingsRequest{
		Privacy: &usersv1.PrivacySettings{
			PhoneVisibility:     commonv1.Visibility_VISIBILITY_CONTACTS,
			LastSeenVisibility:  commonv1.Visibility_VISIBILITY_NOBODY,
			MessagePrivacy:      commonv1.Visibility_VISIBILITY_EVERYONE,
			BirthdayVisibility:  commonv1.Visibility_VISIBILITY_NOBODY,
			AllowContactSync:    true,
			AllowUnknownSenders: true,
			AllowUsernameSearch: false,
		},
	})
	if err != nil {
		t.Fatalf("update privacy: %v", err)
	}
	if !privacy.Privacy.AllowContactSync || privacy.Privacy.AllowUsernameSearch {
		t.Fatalf("unexpected privacy settings: %+v", privacy.Privacy)
	}

	me, err := api.GetMyProfile(authCtx, &usersv1.GetMyProfileRequest{})
	if err != nil {
		t.Fatalf("get my profile: %v", err)
	}
	if me.Profile.Privacy == nil || me.Profile.Phone == "" {
		t.Fatalf("expected own profile to expose privacy and phone: %+v", me.Profile)
	}
}

func TestProfileAvatarUpdateReturnsAvatarAndAllowsPublicDownload(t *testing.T) {
	fixture := newGatewayFeatureFixture(t)
	ctx := context.Background()

	owner, _, err := fixture.api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "avatar-owner",
		Email:       "avatar-owner@example.com",
		AccountKind: identity.AccountKindUser,
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	ownerCtx, _ := mustLoginOnDevice(t, fixture.api, fixture.sender, ctx, owner.Username, "owner-phone", "owner-key")
	var payload bytes.Buffer
	if err := png.Encode(&payload, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode avatar: %v", err)
	}
	initiated, err := fixture.api.InitiateUpload(ownerCtx, &mediav1.InitiateUploadRequest{
		Purpose:   commonv1.MediaPurpose_MEDIA_PURPOSE_PROFILE_AVATAR,
		FileName:  "avatar.png",
		MimeType:  "image/png",
		SizeBytes: uint64(payload.Len()),
	})
	if err != nil {
		t.Fatalf("initiate avatar upload: %v", err)
	}
	reserved, err := fixture.mediaStore.MediaAssetByID(ctx, initiated.Media.MediaId)
	if err != nil {
		t.Fatalf("load reserved avatar: %v", err)
	}
	fixture.mediaBlob.seedObject(reserved.ObjectKey, domainstorage.BlobObject{
		Bucket:        reserved.Bucket,
		Key:           reserved.ObjectKey,
		ContentLength: int64(payload.Len()),
		ContentType:   "image/png",
	}, payload.Bytes())
	completed, err := fixture.api.CompleteUpload(ownerCtx, &mediav1.CompleteUploadRequest{
		UploadId: initiated.Upload.UploadId,
		MediaId:  initiated.Upload.MediaId,
	})
	if err != nil {
		t.Fatalf("complete avatar upload: %v", err)
	}
	mediaID := completed.Media.MediaId
	completedAsset, err := fixture.mediaStore.MediaAssetByID(ctx, mediaID)
	if err != nil {
		t.Fatalf("load completed avatar: %v", err)
	}
	if !completed.Media.PublicAccess || completedAsset.Width != 2 || completedAsset.Height != 2 {
		t.Fatalf("unexpected completed avatar: %+v", completed.Media)
	}

	updated, err := fixture.api.UpdateMyProfile(ownerCtx, &usersv1.UpdateMyProfileRequest{
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"avatar"}},
		Avatar:     &usersv1.ProfileAvatarUpdate{MediaId: mediaID},
	})
	if err != nil {
		t.Fatalf("update avatar: %v", err)
	}
	if len(updated.Profile.Avatars) != 1 || updated.Profile.Avatars[0].MediaId != mediaID {
		t.Fatalf("unexpected avatar profile: %+v", updated.Profile.Avatars)
	}
	if _, err := fixture.api.DeleteMedia(ownerCtx, &mediav1.DeleteMediaRequest{MediaId: mediaID}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("delete current avatar error = %v", err)
	}

	viewer, _, err := fixture.api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "avatar-viewer",
		Email:       "avatar-viewer@example.com",
		AccountKind: identity.AccountKindUser,
	})
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	viewerCtx, _ := mustLoginOnDevice(t, fixture.api, fixture.sender, ctx, viewer.Username, "viewer-phone", "viewer-key")
	download, err := fixture.api.GetDownloadUrl(viewerCtx, &mediav1.GetDownloadUrlRequest{MediaId: mediaID})
	if err != nil {
		t.Fatalf("download public avatar: %v", err)
	}
	if download.GetUrl() == "" {
		t.Fatal("expected public avatar download URL")
	}

	if _, err := fixture.api.UpdateMyProfile(ownerCtx, &usersv1.UpdateMyProfileRequest{
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"avatar"}},
		Avatar:     &usersv1.ProfileAvatarUpdate{Clear: true},
	}); err != nil {
		t.Fatalf("clear avatar: %v", err)
	}
	if _, err := fixture.api.DeleteMedia(ownerCtx, &mediav1.DeleteMediaRequest{MediaId: mediaID}); err != nil {
		t.Fatalf("delete cleared avatar: %v", err)
	}
}

func TestContactsBlocksAndSearchRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	alice, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice-users",
		DisplayName: "Alice",
		Email:       "alice-users@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob-users",
		DisplayName: "Bob",
		Email:       "bob-users@example.com",
		Phone:       "+1 (555) 999-1000",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	begin, err := api.BeginLogin(ctx, &authv1.BeginLoginRequest{
		Identifier:      &authv1.BeginLoginRequest_Username{Username: alice.Username},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
		DeviceName:      "phone",
		DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	verify, err := api.VerifyLoginCode(ctx, &authv1.VerifyLoginCodeRequest{
		ChallengeId:    begin.ChallengeId,
		Code:           sender.code(begin.Targets[0].DestinationMask),
		DeviceName:     "phone",
		DevicePlatform: commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		DeviceKey:      &commonv1.PublicKeyBundle{PublicKey: []byte("device-key")},
	})
	if err != nil {
		t.Fatalf("verify login: %v", err)
	}
	authCtx := metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+verify.Tokens.AccessToken))

	added, err := api.AddContact(authCtx, &usersv1.AddContactRequest{
		UserId:  bob.ID,
		Alias:   "Bobby",
		Starred: true,
	})
	if err != nil {
		t.Fatalf("add contact: %v", err)
	}
	if added.Contact.DisplayName != "Bobby" || !added.Contact.Starred {
		t.Fatalf("unexpected contact: %+v", added.Contact)
	}

	listed, err := api.ListContacts(authCtx, &usersv1.ListContactsRequest{})
	if err != nil {
		t.Fatalf("list contacts: %v", err)
	}
	if len(listed.Contacts) != 1 || listed.Contacts[0].ContactUserId != bob.ID {
		t.Fatalf("unexpected contacts: %+v", listed.Contacts)
	}

	blocked, err := api.BlockUser(authCtx, &usersv1.BlockUserRequest{
		UserId: bob.ID,
		Reason: "spam",
	})
	if err != nil {
		t.Fatalf("block user: %v", err)
	}
	if blocked.Entry.BlockedUserId != bob.ID {
		t.Fatalf("unexpected block entry: %+v", blocked.Entry)
	}

	blockList, err := api.ListBlockedUsers(authCtx, &usersv1.ListBlockedUsersRequest{})
	if err != nil {
		t.Fatalf("list blocked: %v", err)
	}
	if len(blockList.BlockedUsers) != 1 {
		t.Fatalf("unexpected blocked users: %+v", blockList.BlockedUsers)
	}

	hidden, err := api.SearchUsers(authCtx, &usersv1.SearchUsersRequest{Query: "bob-users"})
	if err != nil {
		t.Fatalf("search users without blocked: %v", err)
	}
	if len(hidden.Profiles) != 0 {
		t.Fatalf("expected blocked user to be hidden, got %+v", hidden.Profiles)
	}

	marked, err := api.SearchUsers(authCtx, &usersv1.SearchUsersRequest{
		Query:          "@bob-users",
		IncludeBlocked: true,
	})
	if err != nil {
		t.Fatalf("search users with username marker: %v", err)
	}
	if len(marked.Profiles) != 1 || marked.Profiles[0].Username != bob.Username {
		t.Fatalf("expected username marker search result, got %+v", marked.Profiles)
	}

	included, err := api.SearchUsers(authCtx, &usersv1.SearchUsersRequest{
		Query:          "bob-users",
		IncludeBlocked: true,
	})
	if err != nil {
		t.Fatalf("search users with blocked: %v", err)
	}
	if len(included.Profiles) != 1 || !included.Profiles[0].IsBlocked {
		t.Fatalf("expected blocked profile in search, got %+v", included.Profiles)
	}

	synced, err := api.SyncContacts(authCtx, &usersv1.SyncContactsRequest{
		SourceDeviceId: verify.Device.DeviceId,
		Contacts: []*usersv1.SyncedContact{{
			RawContactId: "raw-bob",
			DisplayName:  "Robert",
			PhoneE164:    bob.Phone,
			Checksum:     "v1",
		}},
	})
	if err != nil {
		t.Fatalf("sync contacts: %v", err)
	}
	if len(synced.Matches) != 1 || synced.Matches[0].ContactUserId != bob.ID {
		t.Fatalf("unexpected sync result: %+v", synced)
	}
}
