package gateway

import (
	"encoding/base64"
	"strings"
	"time"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	mediav1 "github.com/dm-vev/zvonilka/gen/proto/contracts/media/v1"
	searchv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/search/v1"
	usersv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/users/v1"
	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func protoTime(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}

	return timestamppb.New(value.UTC())
}

func protoDuration(value time.Duration) *durationpb.Duration {
	if value <= 0 {
		return nil
	}

	return durationpb.New(value)
}

func identityPlatformFromProto(platform commonv1.DevicePlatform) domainidentity.DevicePlatform {
	switch platform {
	case commonv1.DevicePlatform_DEVICE_PLATFORM_IOS:
		return domainidentity.DevicePlatformIOS
	case commonv1.DevicePlatform_DEVICE_PLATFORM_ANDROID:
		return domainidentity.DevicePlatformAndroid
	case commonv1.DevicePlatform_DEVICE_PLATFORM_WEB:
		return domainidentity.DevicePlatformWeb
	case commonv1.DevicePlatform_DEVICE_PLATFORM_DESKTOP:
		return domainidentity.DevicePlatformDesktop
	case commonv1.DevicePlatform_DEVICE_PLATFORM_SERVER:
		return domainidentity.DevicePlatformServer
	default:
		return domainidentity.DevicePlatformUnspecified
	}
}

func identityPlatformToProto(platform domainidentity.DevicePlatform) commonv1.DevicePlatform {
	switch platform {
	case domainidentity.DevicePlatformIOS:
		return commonv1.DevicePlatform_DEVICE_PLATFORM_IOS
	case domainidentity.DevicePlatformAndroid:
		return commonv1.DevicePlatform_DEVICE_PLATFORM_ANDROID
	case domainidentity.DevicePlatformWeb:
		return commonv1.DevicePlatform_DEVICE_PLATFORM_WEB
	case domainidentity.DevicePlatformDesktop:
		return commonv1.DevicePlatform_DEVICE_PLATFORM_DESKTOP
	case domainidentity.DevicePlatformServer:
		return commonv1.DevicePlatform_DEVICE_PLATFORM_SERVER
	default:
		return commonv1.DevicePlatform_DEVICE_PLATFORM_UNSPECIFIED
	}
}

func identityDeviceStatusToProto(status domainidentity.DeviceStatus) commonv1.DeviceStatus {
	switch status {
	case domainidentity.DeviceStatusActive:
		return commonv1.DeviceStatus_DEVICE_STATUS_ACTIVE
	case domainidentity.DeviceStatusSuspended:
		return commonv1.DeviceStatus_DEVICE_STATUS_SUSPENDED
	case domainidentity.DeviceStatusRevoked:
		return commonv1.DeviceStatus_DEVICE_STATUS_REVOKED
	case domainidentity.DeviceStatusUnverified:
		return commonv1.DeviceStatus_DEVICE_STATUS_UNVERIFIED
	default:
		return commonv1.DeviceStatus_DEVICE_STATUS_UNSPECIFIED
	}
}

func conversationKindFromProto(kind commonv1.ConversationKind) domainconversation.ConversationKind {
	switch kind {
	case commonv1.ConversationKind_CONVERSATION_KIND_DIRECT:
		return domainconversation.ConversationKindDirect
	case commonv1.ConversationKind_CONVERSATION_KIND_GROUP:
		return domainconversation.ConversationKindGroup
	case commonv1.ConversationKind_CONVERSATION_KIND_CHANNEL:
		return domainconversation.ConversationKindChannel
	case commonv1.ConversationKind_CONVERSATION_KIND_SAVED_MESSAGES:
		return domainconversation.ConversationKindSavedMessages
	default:
		return domainconversation.ConversationKindUnspecified
	}
}

func conversationKindToProto(kind domainconversation.ConversationKind) commonv1.ConversationKind {
	switch kind {
	case domainconversation.ConversationKindDirect:
		return commonv1.ConversationKind_CONVERSATION_KIND_DIRECT
	case domainconversation.ConversationKindGroup:
		return commonv1.ConversationKind_CONVERSATION_KIND_GROUP
	case domainconversation.ConversationKindChannel:
		return commonv1.ConversationKind_CONVERSATION_KIND_CHANNEL
	case domainconversation.ConversationKindSavedMessages:
		return commonv1.ConversationKind_CONVERSATION_KIND_SAVED_MESSAGES
	default:
		return commonv1.ConversationKind_CONVERSATION_KIND_UNSPECIFIED
	}
}

func memberRoleToProto(role domainconversation.MemberRole) commonv1.MemberRole {
	switch role {
	case domainconversation.MemberRoleOwner:
		return commonv1.MemberRole_MEMBER_ROLE_OWNER
	case domainconversation.MemberRoleAdmin:
		return commonv1.MemberRole_MEMBER_ROLE_ADMIN
	case domainconversation.MemberRoleMember:
		return commonv1.MemberRole_MEMBER_ROLE_MEMBER
	case domainconversation.MemberRoleGuest:
		return commonv1.MemberRole_MEMBER_ROLE_GUEST
	default:
		return commonv1.MemberRole_MEMBER_ROLE_UNSPECIFIED
	}
}

func messageKindFromProto(kind commonv1.MessageKind) domainconversation.MessageKind {
	switch kind {
	case commonv1.MessageKind_MESSAGE_KIND_TEXT:
		return domainconversation.MessageKindText
	case commonv1.MessageKind_MESSAGE_KIND_IMAGE:
		return domainconversation.MessageKindImage
	case commonv1.MessageKind_MESSAGE_KIND_VIDEO:
		return domainconversation.MessageKindVideo
	case commonv1.MessageKind_MESSAGE_KIND_DOCUMENT:
		return domainconversation.MessageKindDocument
	case commonv1.MessageKind_MESSAGE_KIND_VOICE:
		return domainconversation.MessageKindVoice
	case commonv1.MessageKind_MESSAGE_KIND_STICKER:
		return domainconversation.MessageKindSticker
	case commonv1.MessageKind_MESSAGE_KIND_GIF:
		return domainconversation.MessageKindGIF
	case commonv1.MessageKind_MESSAGE_KIND_SYSTEM:
		return domainconversation.MessageKindSystem
	default:
		return domainconversation.MessageKindUnspecified
	}
}

func messageKindToProto(kind domainconversation.MessageKind) commonv1.MessageKind {
	switch kind {
	case domainconversation.MessageKindText:
		return commonv1.MessageKind_MESSAGE_KIND_TEXT
	case domainconversation.MessageKindImage:
		return commonv1.MessageKind_MESSAGE_KIND_IMAGE
	case domainconversation.MessageKindVideo:
		return commonv1.MessageKind_MESSAGE_KIND_VIDEO
	case domainconversation.MessageKindDocument:
		return commonv1.MessageKind_MESSAGE_KIND_DOCUMENT
	case domainconversation.MessageKindVoice:
		return commonv1.MessageKind_MESSAGE_KIND_VOICE
	case domainconversation.MessageKindSticker:
		return commonv1.MessageKind_MESSAGE_KIND_STICKER
	case domainconversation.MessageKindGIF:
		return commonv1.MessageKind_MESSAGE_KIND_GIF
	case domainconversation.MessageKindSystem:
		return commonv1.MessageKind_MESSAGE_KIND_SYSTEM
	default:
		return commonv1.MessageKind_MESSAGE_KIND_UNSPECIFIED
	}
}

func messageStatusToProto(status domainconversation.MessageStatus) commonv1.MessageStatus {
	switch status {
	case domainconversation.MessageStatusPending:
		return commonv1.MessageStatus_MESSAGE_STATUS_PENDING
	case domainconversation.MessageStatusSent:
		return commonv1.MessageStatus_MESSAGE_STATUS_SENT
	case domainconversation.MessageStatusDelivered:
		return commonv1.MessageStatus_MESSAGE_STATUS_DELIVERED
	case domainconversation.MessageStatusRead:
		return commonv1.MessageStatus_MESSAGE_STATUS_READ
	case domainconversation.MessageStatusFailed:
		return commonv1.MessageStatus_MESSAGE_STATUS_FAILED
	case domainconversation.MessageStatusDeleted:
		return commonv1.MessageStatus_MESSAGE_STATUS_DELETED
	default:
		return commonv1.MessageStatus_MESSAGE_STATUS_UNSPECIFIED
	}
}

func attachmentKindFromProto(kind commonv1.AttachmentKind) domainconversation.AttachmentKind {
	switch kind {
	case commonv1.AttachmentKind_ATTACHMENT_KIND_IMAGE:
		return domainconversation.AttachmentKindImage
	case commonv1.AttachmentKind_ATTACHMENT_KIND_VIDEO:
		return domainconversation.AttachmentKindVideo
	case commonv1.AttachmentKind_ATTACHMENT_KIND_DOCUMENT:
		return domainconversation.AttachmentKindDocument
	case commonv1.AttachmentKind_ATTACHMENT_KIND_VOICE:
		return domainconversation.AttachmentKindVoice
	case commonv1.AttachmentKind_ATTACHMENT_KIND_STICKER:
		return domainconversation.AttachmentKindSticker
	case commonv1.AttachmentKind_ATTACHMENT_KIND_GIF:
		return domainconversation.AttachmentKindGIF
	case commonv1.AttachmentKind_ATTACHMENT_KIND_AVATAR:
		return domainconversation.AttachmentKindAvatar
	case commonv1.AttachmentKind_ATTACHMENT_KIND_FILE:
		return domainconversation.AttachmentKindFile
	default:
		return domainconversation.AttachmentKindUnspecified
	}
}

func attachmentKindToProto(kind domainconversation.AttachmentKind) commonv1.AttachmentKind {
	switch kind {
	case domainconversation.AttachmentKindImage:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_IMAGE
	case domainconversation.AttachmentKindVideo:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_VIDEO
	case domainconversation.AttachmentKindDocument:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_DOCUMENT
	case domainconversation.AttachmentKindVoice:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_VOICE
	case domainconversation.AttachmentKindSticker:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_STICKER
	case domainconversation.AttachmentKindGIF:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_GIF
	case domainconversation.AttachmentKindAvatar:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_AVATAR
	case domainconversation.AttachmentKindFile:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_FILE
	default:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_UNSPECIFIED
	}
}

func presenceStateFromProto(state commonv1.PresenceState) domainpresence.PresenceState {
	switch state {
	case commonv1.PresenceState_PRESENCE_STATE_OFFLINE:
		return domainpresence.PresenceStateOffline
	case commonv1.PresenceState_PRESENCE_STATE_ONLINE:
		return domainpresence.PresenceStateOnline
	case commonv1.PresenceState_PRESENCE_STATE_AWAY:
		return domainpresence.PresenceStateAway
	case commonv1.PresenceState_PRESENCE_STATE_BUSY:
		return domainpresence.PresenceStateBusy
	case commonv1.PresenceState_PRESENCE_STATE_INVISIBLE:
		return domainpresence.PresenceStateInvisible
	default:
		return domainpresence.PresenceStateUnspecified
	}
}

func presenceStateToProto(state domainpresence.PresenceState) commonv1.PresenceState {
	switch state {
	case domainpresence.PresenceStateOffline:
		return commonv1.PresenceState_PRESENCE_STATE_OFFLINE
	case domainpresence.PresenceStateOnline:
		return commonv1.PresenceState_PRESENCE_STATE_ONLINE
	case domainpresence.PresenceStateAway:
		return commonv1.PresenceState_PRESENCE_STATE_AWAY
	case domainpresence.PresenceStateBusy:
		return commonv1.PresenceState_PRESENCE_STATE_BUSY
	case domainpresence.PresenceStateInvisible:
		return commonv1.PresenceState_PRESENCE_STATE_INVISIBLE
	default:
		return commonv1.PresenceState_PRESENCE_STATE_UNSPECIFIED
	}
}

func accountKindToProto(kind domainidentity.AccountKind) commonv1.AccountKind {
	switch kind {
	case domainidentity.AccountKindUser:
		return commonv1.AccountKind_ACCOUNT_KIND_USER
	case domainidentity.AccountKindBot:
		return commonv1.AccountKind_ACCOUNT_KIND_BOT
	default:
		return commonv1.AccountKind_ACCOUNT_KIND_UNSPECIFIED
	}
}

func searchScopeFromProto(scope commonv1.SearchScope) domainsearch.SearchScope {
	switch scope {
	case commonv1.SearchScope_SEARCH_SCOPE_USERS:
		return domainsearch.SearchScopeUsers
	case commonv1.SearchScope_SEARCH_SCOPE_CONVERSATIONS:
		return domainsearch.SearchScopeConversations
	case commonv1.SearchScope_SEARCH_SCOPE_MESSAGES:
		return domainsearch.SearchScopeMessages
	case commonv1.SearchScope_SEARCH_SCOPE_MEDIA:
		return domainsearch.SearchScopeMedia
	default:
		return domainsearch.SearchScopeUnspecified
	}
}

func searchScopeToProto(scope domainsearch.SearchScope) commonv1.SearchScope {
	switch scope {
	case domainsearch.SearchScopeUsers:
		return commonv1.SearchScope_SEARCH_SCOPE_USERS
	case domainsearch.SearchScopeConversations:
		return commonv1.SearchScope_SEARCH_SCOPE_CONVERSATIONS
	case domainsearch.SearchScopeMessages:
		return commonv1.SearchScope_SEARCH_SCOPE_MESSAGES
	case domainsearch.SearchScopeMedia:
		return commonv1.SearchScope_SEARCH_SCOPE_MEDIA
	default:
		return commonv1.SearchScope_SEARCH_SCOPE_UNSPECIFIED
	}
}

func mediaPurposeToKind(purpose commonv1.MediaPurpose) domainmedia.MediaKind {
	switch purpose {
	case commonv1.MediaPurpose_MEDIA_PURPOSE_MESSAGE_ATTACHMENT:
		return domainmedia.MediaKindFile
	case commonv1.MediaPurpose_MEDIA_PURPOSE_PROFILE_AVATAR:
		return domainmedia.MediaKindAvatar
	case commonv1.MediaPurpose_MEDIA_PURPOSE_CHAT_AVATAR:
		return domainmedia.MediaKindAvatar
	case commonv1.MediaPurpose_MEDIA_PURPOSE_BOT_AVATAR:
		return domainmedia.MediaKindAvatar
	case commonv1.MediaPurpose_MEDIA_PURPOSE_STICKER_ASSET:
		return domainmedia.MediaKindSticker
	default:
		return domainmedia.MediaKindFile
	}
}

func mediaObject(asset domainmedia.MediaAsset) *mediav1.MediaObject {
	return &mediav1.MediaObject{
		MediaId:     asset.ID,
		OwnerUserId: asset.OwnerAccountID,
		Purpose:     mediaPurposeFromKind(asset.Kind),
		FileName:    asset.FileName,
		MimeType:    asset.ContentType,
		SizeBytes:   asset.SizeBytes,
		Sha256Hex:   asset.SHA256Hex,
		StorageKey:  asset.ObjectKey,
		CreatedAt:   protoTime(asset.CreatedAt),
		CompletedAt: protoTime(asset.ReadyAt),
		DeletedAt:   protoTime(asset.DeletedAt),
	}
}

func mediaKindToProto(kind domainmedia.MediaKind) commonv1.AttachmentKind {
	switch kind {
	case domainmedia.MediaKindImage:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_IMAGE
	case domainmedia.MediaKindVideo:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_VIDEO
	case domainmedia.MediaKindDocument:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_DOCUMENT
	case domainmedia.MediaKindVoice:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_VOICE
	case domainmedia.MediaKindSticker:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_STICKER
	case domainmedia.MediaKindGIF:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_GIF
	case domainmedia.MediaKindAvatar:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_AVATAR
	case domainmedia.MediaKindFile:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_FILE
	default:
		return commonv1.AttachmentKind_ATTACHMENT_KIND_UNSPECIFIED
	}
}

func mediaPurposeFromKind(kind domainmedia.MediaKind) commonv1.MediaPurpose {
	switch kind {
	case domainmedia.MediaKindAvatar:
		return commonv1.MediaPurpose_MEDIA_PURPOSE_PROFILE_AVATAR
	case domainmedia.MediaKindSticker:
		return commonv1.MediaPurpose_MEDIA_PURPOSE_STICKER_ASSET
	default:
		return commonv1.MediaPurpose_MEDIA_PURPOSE_MESSAGE_ATTACHMENT
	}
}

func authDevice(device domainidentity.Device) *authv1.Device {
	return &authv1.Device{
		DeviceId:   device.ID,
		UserId:     device.AccountID,
		DeviceName: device.Name,
		Platform:   identityPlatformToProto(device.Platform),
		Status:     identityDeviceStatusToProto(device.Status),
		PublicKey:  protoPublicKey(device.PublicKey),
		PushToken:  device.PushToken,
		CreatedAt:  protoTime(device.CreatedAt),
		LastSeenAt: protoTime(device.LastSeenAt),
		RevokedAt:  protoTime(device.RevokedAt),
	}
}

func authSession(session domainidentity.Session) *authv1.Session {
	return &authv1.Session{
		SessionId:      session.ID,
		UserId:         session.AccountID,
		DeviceId:       session.DeviceID,
		DeviceName:     session.DeviceName,
		DevicePlatform: identityPlatformToProto(session.DevicePlatform),
		IpAddress:      session.IPAddress,
		UserAgent:      session.UserAgent,
		CreatedAt:      protoTime(session.CreatedAt),
		LastSeenAt:     protoTime(session.LastSeenAt),
		RevokedAt:      protoTime(session.RevokedAt),
		Current:        session.Current,
		Revoked:        !session.RevokedAt.IsZero() || session.Status == domainidentity.SessionStatusRevoked,
	}
}

func authTargets(targets []domainidentity.LoginTarget) []*authv1.LoginTarget {
	result := make([]*authv1.LoginTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, &authv1.LoginTarget{
			Channel:         loginChannelToProto(target.Channel),
			DestinationMask: target.DestinationMask,
			Primary:         target.Primary,
			Verified:        target.Verified,
		})
	}

	return result
}

func loginChannelFromProto(channel authv1.LoginDeliveryChannel) domainidentity.LoginDeliveryChannel {
	switch channel {
	case authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_SMS:
		return domainidentity.LoginDeliveryChannelSMS
	case authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL:
		return domainidentity.LoginDeliveryChannelEmail
	case authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_PUSH:
		return domainidentity.LoginDeliveryChannelPush
	case authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_MANUAL:
		return domainidentity.LoginDeliveryChannelManual
	default:
		return domainidentity.LoginDeliveryChannelUnspecified
	}
}

func loginChannelToProto(channel domainidentity.LoginDeliveryChannel) authv1.LoginDeliveryChannel {
	switch channel {
	case domainidentity.LoginDeliveryChannelSMS:
		return authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_SMS
	case domainidentity.LoginDeliveryChannelEmail:
		return authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL
	case domainidentity.LoginDeliveryChannelPush:
		return authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_PUSH
	case domainidentity.LoginDeliveryChannelManual:
		return authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_MANUAL
	default:
		return authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_UNSPECIFIED
	}
}

func protoPublicKey(value string) *commonv1.PublicKeyBundle {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	decoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		decoded = []byte(value)
	}

	return &commonv1.PublicKeyBundle{
		PublicKey: decoded,
	}
}

func publicKeyFromProto(bundle *commonv1.PublicKeyBundle) string {
	if bundle == nil || len(bundle.PublicKey) == 0 {
		return ""
	}

	return base64.RawStdEncoding.EncodeToString(bundle.PublicKey)
}

func protoPayload(payload domainconversation.EncryptedPayload) *commonv1.EncryptedPayload {
	if payload.KeyID == "" && payload.Algorithm == "" && len(payload.Nonce) == 0 && len(payload.Ciphertext) == 0 && len(payload.AAD) == 0 && len(payload.Metadata) == 0 {
		return nil
	}

	return &commonv1.EncryptedPayload{
		KeyId:      payload.KeyID,
		Algorithm:  payload.Algorithm,
		Nonce:      payload.Nonce,
		Ciphertext: payload.Ciphertext,
		Aad:        payload.AAD,
		Metadata:   payload.Metadata,
	}
}

func payloadFromProto(payload *commonv1.EncryptedPayload) domainconversation.EncryptedPayload {
	if payload == nil {
		return domainconversation.EncryptedPayload{}
	}

	return domainconversation.EncryptedPayload{
		KeyID:      strings.TrimSpace(payload.KeyId),
		Algorithm:  strings.TrimSpace(payload.Algorithm),
		Nonce:      append([]byte(nil), payload.Nonce...),
		Ciphertext: append([]byte(nil), payload.Ciphertext...),
		AAD:        append([]byte(nil), payload.Aad...),
		Metadata:   payload.Metadata,
	}
}

func attachmentFromProto(attachment *commonv1.AttachmentRef) domainconversation.AttachmentRef {
	if attachment == nil {
		return domainconversation.AttachmentRef{}
	}

	duration := time.Duration(0)
	if attachment.Duration != nil {
		duration = attachment.Duration.AsDuration()
	}

	return domainconversation.AttachmentRef{
		MediaID:   strings.TrimSpace(attachment.MediaId),
		Kind:      attachmentKindFromProto(attachment.Kind),
		FileName:  strings.TrimSpace(attachment.FileName),
		MimeType:  strings.TrimSpace(attachment.MimeType),
		SizeBytes: attachment.SizeBytes,
		SHA256Hex: strings.TrimSpace(attachment.Sha256Hex),
		Width:     attachment.Width,
		Height:    attachment.Height,
		Duration:  duration,
		Caption:   strings.TrimSpace(attachment.Caption),
	}
}

func attachmentToProto(attachment domainconversation.AttachmentRef) *commonv1.AttachmentRef {
	return &commonv1.AttachmentRef{
		MediaId:   attachment.MediaID,
		Kind:      attachmentKindToProto(attachment.Kind),
		FileName:  attachment.FileName,
		MimeType:  attachment.MimeType,
		SizeBytes: attachment.SizeBytes,
		Sha256Hex: attachment.SHA256Hex,
		Width:     attachment.Width,
		Height:    attachment.Height,
		Duration:  protoDuration(attachment.Duration),
		Caption:   attachment.Caption,
	}
}

func referenceFromProto(reference *commonv1.MessageReference) domainconversation.MessageReference {
	if reference == nil {
		return domainconversation.MessageReference{}
	}

	return domainconversation.MessageReference{
		ConversationID:  strings.TrimSpace(reference.ConversationId),
		MessageID:       strings.TrimSpace(reference.MessageId),
		SenderAccountID: strings.TrimSpace(reference.SenderUserId),
		MessageKind:     messageKindFromProto(reference.MessageKind),
		Snippet:         strings.TrimSpace(reference.Snippet),
	}
}

func referenceToProto(reference domainconversation.MessageReference) *commonv1.MessageReference {
	if reference.MessageID == "" {
		return nil
	}

	return &commonv1.MessageReference{
		ConversationId: reference.ConversationID,
		MessageId:      reference.MessageID,
		SenderUserId:   reference.SenderAccountID,
		MessageKind:    messageKindToProto(reference.MessageKind),
		Snippet:        reference.Snippet,
	}
}

func eventTypeToProto(eventType domainconversation.EventType) commonv1.EventType {
	switch eventType {
	case domainconversation.EventTypeMessageCreated:
		return commonv1.EventType_EVENT_TYPE_MESSAGE_CREATED
	case domainconversation.EventTypeMessageEdited:
		return commonv1.EventType_EVENT_TYPE_MESSAGE_EDITED
	case domainconversation.EventTypeMessageDeleted:
		return commonv1.EventType_EVENT_TYPE_MESSAGE_DELETED
	case domainconversation.EventTypeMessageReactionAdded, domainconversation.EventTypeMessageReactionUpdated:
		return commonv1.EventType_EVENT_TYPE_MESSAGE_REACTION_ADDED
	case domainconversation.EventTypeMessageReactionRemoved:
		return commonv1.EventType_EVENT_TYPE_MESSAGE_REACTION_REMOVED
	case domainconversation.EventTypeConversationCreated:
		return commonv1.EventType_EVENT_TYPE_CONVERSATION_CREATED
	case domainconversation.EventTypeConversationUpdated:
		return commonv1.EventType_EVENT_TYPE_CONVERSATION_UPDATED
	case domainconversation.EventTypeConversationMembers:
		return commonv1.EventType_EVENT_TYPE_CONVERSATION_MEMBERS_CHANGED
	default:
		return commonv1.EventType_EVENT_TYPE_UNSPECIFIED
	}
}

func profile(account domainidentity.Account, snapshot domainpresence.Snapshot) *usersv1.UserProfile {
	return &usersv1.UserProfile{
		UserId:           account.ID,
		Username:         account.Username,
		DisplayName:      account.DisplayName,
		Bio:              account.Bio,
		Phone:            account.Phone,
		Email:            account.Email,
		Verified:         false,
		CustomBadgeEmoji: account.CustomBadgeEmoji,
		Presence:         presenceStateToProto(snapshot.State),
		CustomStatus:     snapshot.CustomStatus,
		LastSeenAt:       protoTime(snapshot.LastSeenAt),
		CreatedAt:        protoTime(account.CreatedAt),
		UpdatedAt:        protoTime(account.UpdatedAt),
		AccountKind:      accountKindToProto(account.Kind),
	}
}

func searchHit(hit domainsearch.Hit) *searchv1.SearchHit {
	return &searchv1.SearchHit{
		HitId:          hit.HitID,
		Scope:          searchScopeToProto(hit.Scope),
		Title:          hit.Title,
		Subtitle:       hit.Subtitle,
		Snippet:        hit.Snippet,
		TargetId:       hit.TargetID,
		ConversationId: hit.ConversationID,
		MessageId:      hit.MessageID,
		MediaId:        hit.MediaID,
		UserId:         hit.UserID,
		AccountKind:    accountKindToProto(domainidentity.AccountKind(hit.AccountKind)),
		Metadata:       hit.Metadata,
		UpdatedAt:      protoTime(hit.UpdatedAt),
	}
}
