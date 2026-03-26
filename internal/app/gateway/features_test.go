package gateway

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	mediav1 "github.com/dm-vev/zvonilka/gen/proto/contracts/media/v1"
	syncv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/sync/v1"
	usersv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/users/v1"
	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/media"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
	presencetest "github.com/dm-vev/zvonilka/internal/domain/presence/teststore"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestCreateThreadAcceptsRootMessageID(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	account, authCtx := fixture.mustCreateUserAndLogin(t, "threads-owner", "threads-owner@example.com")
	created, err := fixture.api.CreateConversation(authCtx, &conversationv1.CreateConversationRequest{
		Kind:  commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title: "Threads",
		Settings: &conversationv1.ConversationSettings{
			AllowThreads: true,
		},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	sent, err := fixture.api.SendMessage(authCtx, &conversationv1.SendMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		Draft:          testMessageDraft("root"),
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}

	thread, err := fixture.api.CreateThread(authCtx, &conversationv1.CreateThreadRequest{
		ConversationId: created.Conversation.ConversationId,
		RootMessageId:  sent.Message.MessageId,
		Title:          "Announcements",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if thread.Thread.RootMessageId != sent.Message.MessageId {
		t.Fatalf("expected root message id %s, got %s", sent.Message.MessageId, thread.Thread.RootMessageId)
	}

	loaded, err := fixture.api.GetThread(authCtx, &conversationv1.GetThreadRequest{
		ConversationId: created.Conversation.ConversationId,
		ThreadId:       thread.Thread.ThreadId,
	})
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if loaded.Thread.RootMessageId != sent.Message.MessageId {
		t.Fatalf("expected persisted root message id %s, got %s", sent.Message.MessageId, loaded.Thread.RootMessageId)
	}

	if loaded.Thread.ConversationId != created.Conversation.ConversationId || account.ID == "" {
		t.Fatalf("unexpected loaded thread: %+v", loaded.Thread)
	}
}

func TestMediaFiltersVariantAndHardDelete(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	account, authCtx := fixture.mustCreateUserAndLogin(t, "media-owner", "media-owner@example.com")

	first, err := fixture.api.InitiateUpload(authCtx, &mediav1.InitiateUploadRequest{
		Purpose:        commonv1.MediaPurpose_MEDIA_PURPOSE_MESSAGE_ATTACHMENT,
		FileName:       "photo.jpg",
		MimeType:       "image/jpeg",
		SizeBytes:      1024,
		ConversationId: "conv-1",
	})
	if err != nil {
		t.Fatalf("initiate first upload: %v", err)
	}

	_, err = fixture.api.InitiateUpload(authCtx, &mediav1.InitiateUploadRequest{
		Purpose:        commonv1.MediaPurpose_MEDIA_PURPOSE_STICKER_ASSET,
		FileName:       "sticker.webp",
		MimeType:       "image/webp",
		SizeBytes:      512,
		ConversationId: "conv-2",
	})
	if err != nil {
		t.Fatalf("initiate second upload: %v", err)
	}

	listed, err := fixture.api.ListMedia(authCtx, &mediav1.ListMediaRequest{
		Purposes:       []commonv1.MediaPurpose{commonv1.MediaPurpose_MEDIA_PURPOSE_MESSAGE_ATTACHMENT},
		ConversationId: "conv-1",
	})
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(listed.Media) != 1 || listed.Media[0].MediaId != first.Media.MediaId {
		t.Fatalf("unexpected filtered media result: %+v", listed.Media)
	}

	now := fixture.now()
	if _, err := fixture.mediaStore.SaveMediaAsset(context.Background(), media.MediaAsset{
		ID:              "media-variant",
		OwnerAccountID:  account.ID,
		Kind:            media.MediaKindImage,
		Status:          media.MediaStatusReady,
		StorageProvider: "object",
		Bucket:          fixture.mediaBlob.bucket,
		ObjectKey:       "media/" + account.ID + "/media-variant",
		FileName:        "variant.jpg",
		ContentType:     "image/jpeg",
		SizeBytes:       2048,
		Metadata: map[string]string{
			"variant_object_key.thumb": "media/" + account.ID + "/media-variant-thumb",
			media.MetadataPurposeKey:   "message_attachment",
		},
		UploadExpiresAt: now.Add(time.Minute),
		ReadyAt:         now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("seed variant asset: %v", err)
	}

	download, err := fixture.api.GetDownloadUrl(authCtx, &mediav1.GetDownloadUrlRequest{
		MediaId: "media-variant",
		Variant: "thumb",
	})
	if err != nil {
		t.Fatalf("get variant download url: %v", err)
	}
	if !strings.Contains(download.Url, "media-variant-thumb") {
		t.Fatalf("expected variant object key in download url, got %s", download.Url)
	}

	if _, err := fixture.api.DeleteMedia(authCtx, &mediav1.DeleteMediaRequest{
		MediaId:    first.Media.MediaId,
		HardDelete: true,
	}); err != nil {
		t.Fatalf("hard delete media: %v", err)
	}

	_, err = fixture.api.GetMedia(authCtx, &mediav1.GetMediaRequest{MediaId: first.Media.MediaId})
	if status.Code(err).String() == "OK" || err == nil {
		t.Fatal("expected deleted media to disappear after hard delete")
	}
}

func TestSubscribeEventsWakesOnConversationChanges(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	_, authCtx := fixture.mustCreateUserAndLogin(t, "sync-owner", "sync-owner@example.com")
	created, err := fixture.api.CreateConversation(authCtx, &conversationv1.CreateConversationRequest{
		Kind:  commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title: "Sync",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	stream := newTestSubscribeEventsStream(authCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeEvents(&syncv1.SubscribeEventsRequest{
			FromSequence:    created.Conversation.LastSequence,
			ConversationIds: []string{created.Conversation.ConversationId},
		}, stream)
	}()

	time.Sleep(20 * time.Millisecond)

	if _, err := fixture.api.SendMessage(authCtx, &conversationv1.SendMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		Draft:          testMessageDraft("sync"),
	}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	select {
	case response := <-stream.responses:
		if response.GetEvent() == nil || response.GetEvent().GetConversationId() != created.Conversation.ConversationId {
			t.Fatalf("unexpected subscribe response: %+v", response)
		}
		stream.cancel()
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timed out waiting for subscribe event")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe events returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscribe loop to stop")
	}
}

func TestPullEventsPresenceFilteringAdvancesSequence(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "presence-owner", "presence-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "presence-peer", "presence-peer@example.com")
	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Presence",
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	baseline := created.Conversation.LastSequence
	if _, err := fixture.api.SetPresence(ownerCtx, &usersv1.SetPresenceRequest{
		Presence: commonv1.PresenceState_PRESENCE_STATE_ONLINE,
	}); err != nil {
		t.Fatalf("set presence: %v", err)
	}

	skipped, err := fixture.api.PullEvents(peerCtx, &syncv1.PullEventsRequest{
		FromSequence:    baseline,
		ConversationIds: []string{created.Conversation.ConversationId},
	})
	if err != nil {
		t.Fatalf("pull events without presence: %v", err)
	}
	if len(skipped.Events) != 0 {
		t.Fatalf("expected presence events to be skipped, got %+v", skipped.Events)
	}
	if skipped.NextSequence <= baseline {
		t.Fatalf("expected next sequence to advance beyond %d, got %d", baseline, skipped.NextSequence)
	}

	if _, err := fixture.api.SetPresence(ownerCtx, &usersv1.SetPresenceRequest{
		Presence: commonv1.PresenceState_PRESENCE_STATE_AWAY,
	}); err != nil {
		t.Fatalf("set second presence: %v", err)
	}

	included, err := fixture.api.PullEvents(peerCtx, &syncv1.PullEventsRequest{
		FromSequence:    skipped.NextSequence,
		ConversationIds: []string{created.Conversation.ConversationId},
		IncludePresence: true,
	})
	if err != nil {
		t.Fatalf("pull events with presence: %v", err)
	}
	if len(included.Events) != 1 {
		t.Fatalf("expected one presence event, got %+v", included.Events)
	}
	if included.Events[0].GetEventType() != commonv1.EventType_EVENT_TYPE_USER_UPDATED {
		t.Fatalf("expected user updated event, got %s", included.Events[0].GetEventType())
	}
	if included.Events[0].GetPayloadType() != "presence" {
		t.Fatalf("expected presence payload type, got %s", included.Events[0].GetPayloadType())
	}
	if included.Events[0].GetMetadata()["user_id"] != owner.ID {
		t.Fatalf("expected presence metadata to reference %s, got %+v", owner.ID, included.Events[0].GetMetadata())
	}
}

func TestPullEventsModerationFilteringAdvancesSequence(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "moderation-owner", "moderation-owner@example.com")
	peer, _ := fixture.mustCreateUserAndLogin(t, "moderation-peer", "moderation-peer@example.com")
	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Moderation",
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	baseline := created.Conversation.LastSequence
	if _, err := fixture.api.conversation.ApplyModerationRestriction(context.Background(), conversation.ApplyModerationRestrictionParams{
		TargetKind:      conversation.ModerationTargetKindConversation,
		TargetID:        created.Conversation.ConversationId,
		ActorAccountID:  owner.ID,
		TargetAccountID: peer.ID,
		State:           conversation.ModerationRestrictionStateMuted,
		CreatedAt:       fixture.now(),
	}); err != nil {
		t.Fatalf("apply moderation restriction: %v", err)
	}

	skipped, err := fixture.api.PullEvents(ownerCtx, &syncv1.PullEventsRequest{
		FromSequence:    baseline,
		ConversationIds: []string{created.Conversation.ConversationId},
	})
	if err != nil {
		t.Fatalf("pull events without moderation: %v", err)
	}
	if len(skipped.Events) != 0 {
		t.Fatalf("expected moderation events to be skipped, got %+v", skipped.Events)
	}
	if skipped.NextSequence <= baseline {
		t.Fatalf("expected next sequence to advance beyond %d, got %d", baseline, skipped.NextSequence)
	}

	if err := fixture.api.conversation.LiftModerationRestriction(context.Background(), conversation.LiftModerationRestrictionParams{
		TargetKind:      conversation.ModerationTargetKindConversation,
		TargetID:        created.Conversation.ConversationId,
		ActorAccountID:  owner.ID,
		TargetAccountID: peer.ID,
		Reason:          "resolved",
		CreatedAt:       fixture.now().Add(time.Minute),
	}); err != nil {
		t.Fatalf("lift moderation restriction: %v", err)
	}

	included, err := fixture.api.PullEvents(ownerCtx, &syncv1.PullEventsRequest{
		FromSequence:      skipped.NextSequence,
		ConversationIds:   []string{created.Conversation.ConversationId},
		IncludeModeration: true,
	})
	if err != nil {
		t.Fatalf("pull events with moderation: %v", err)
	}
	if len(included.Events) != 1 {
		t.Fatalf("expected one moderation event, got %+v", included.Events)
	}
	if included.Events[0].GetEventType() != commonv1.EventType_EVENT_TYPE_ADMIN_ACTION_RECORDED {
		t.Fatalf("expected admin action event, got %s", included.Events[0].GetEventType())
	}
	if included.Events[0].GetPayloadType() != "moderation_action" {
		t.Fatalf("expected moderation payload type, got %s", included.Events[0].GetPayloadType())
	}
	if included.Events[0].GetMetadata()["action_type"] != string(conversation.ModerationActionTypeUnmute) {
		t.Fatalf("expected unmute action metadata, got %+v", included.Events[0].GetMetadata())
	}
}

func TestSubscribeEventsFiltersPresenceUntilRequested(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "subscribe-presence-owner", "subscribe-presence-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "subscribe-presence-peer", "subscribe-presence-peer@example.com")
	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Subscribe Presence",
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	stream := newTestSubscribeEventsStream(peerCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeEvents(&syncv1.SubscribeEventsRequest{
			FromSequence:    created.Conversation.LastSequence,
			ConversationIds: []string{created.Conversation.ConversationId},
		}, stream)
	}()

	time.Sleep(20 * time.Millisecond)

	if _, err := fixture.api.SetPresence(ownerCtx, &usersv1.SetPresenceRequest{
		Presence: commonv1.PresenceState_PRESENCE_STATE_BUSY,
	}); err != nil {
		t.Fatalf("set presence: %v", err)
	}

	select {
	case response := <-stream.responses:
		t.Fatalf("expected presence event to be filtered, got %+v", response)
	case <-time.After(150 * time.Millisecond):
	}

	stream.cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe events returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for filtered subscribe loop to stop")
	}

	includingStream := newTestSubscribeEventsStream(peerCtx)
	errCh = make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeEvents(&syncv1.SubscribeEventsRequest{
			FromSequence:    created.Conversation.LastSequence,
			ConversationIds: []string{created.Conversation.ConversationId},
			IncludePresence: true,
		}, includingStream)
	}()

	time.Sleep(20 * time.Millisecond)

	if _, err := fixture.api.SetPresence(ownerCtx, &usersv1.SetPresenceRequest{
		Presence: commonv1.PresenceState_PRESENCE_STATE_AWAY,
	}); err != nil {
		t.Fatalf("set second presence: %v", err)
	}

	select {
	case response := <-includingStream.responses:
		if response.GetEvent() == nil {
			t.Fatal("expected sync event")
		}
		if response.GetEvent().GetEventType() != commonv1.EventType_EVENT_TYPE_USER_UPDATED {
			t.Fatalf("expected user updated event, got %s", response.GetEvent().GetEventType())
		}
		includingStream.cancel()
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timed out waiting for included presence event")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe events returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscribe loop to stop")
	}
	_ = owner
}

type gatewayFeatureFixture struct {
	api        *api
	sender     *recordingSender
	mediaStore *gatewayMediaStore
	mediaBlob  *gatewayBlobStore
	nowFunc    func() time.Time
}

func newGatewayFeatureFixture(t *testing.T) *gatewayFeatureFixture {
	t.Helper()

	now := time.Date(2026, time.March, 26, 15, 0, 0, 0, time.UTC)
	nowFunc := func() time.Time { return now }

	identityStore := identitytest.NewMemoryStore()
	sender := &recordingSender{}
	identityService, err := identity.NewService(identityStore, sender, identity.WithNow(nowFunc))
	if err != nil {
		t.Fatalf("new identity service: %v", err)
	}

	presenceService, err := presence.NewService(presencetest.NewMemoryStore(), identityStore, presence.WithNow(nowFunc))
	if err != nil {
		t.Fatalf("new presence service: %v", err)
	}

	conversationService, err := conversation.NewService(
		conversationtest.NewMemoryStore(),
		conversation.WithNow(nowFunc),
	)
	if err != nil {
		t.Fatalf("new conversation service: %v", err)
	}

	mediaStore := newGatewayMediaStore()
	mediaBlob := newGatewayBlobStore("media-bucket")
	mediaService, err := media.NewService(
		mediaStore,
		mediaBlob,
		media.WithNow(nowFunc),
		media.WithSettings(media.Settings{
			UploadURLTTL:   15 * time.Minute,
			DownloadURLTTL: 15 * time.Minute,
			MaxUploadSize:  10 << 20,
		}),
	)
	if err != nil {
		t.Fatalf("new media service: %v", err)
	}

	return &gatewayFeatureFixture{
		api: &api{
			identity:     identityService,
			presence:     presenceService,
			conversation: conversationService,
			media:        mediaService,
			syncNotifier: newSyncNotifier(),
		},
		sender:     sender,
		mediaStore: mediaStore,
		mediaBlob:  mediaBlob,
		nowFunc:    nowFunc,
	}
}

func (f *gatewayFeatureFixture) now() time.Time {
	return f.nowFunc()
}

func (f *gatewayFeatureFixture) mustCreateUserAndLogin(
	t *testing.T,
	username string,
	email string,
) (identity.Account, context.Context) {
	t.Helper()

	account, _, err := f.api.identity.CreateAccount(context.Background(), identity.CreateAccountParams{
		Username:    username,
		DisplayName: username,
		Email:       email,
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	begin, err := f.api.BeginLogin(context.Background(), &authv1.BeginLoginRequest{
		Identifier:      &authv1.BeginLoginRequest_Username{Username: account.Username},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
		DeviceName:      "test-device",
		DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}

	verify, err := f.api.VerifyLoginCode(context.Background(), &authv1.VerifyLoginCodeRequest{
		ChallengeId:    begin.ChallengeId,
		Code:           f.sender.code(begin.Targets[0].DestinationMask),
		DeviceName:     "test-device",
		DevicePlatform: commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		DeviceKey:      &commonv1.PublicKeyBundle{PublicKey: []byte("device-key")},
	})
	if err != nil {
		t.Fatalf("verify login: %v", err)
	}

	authCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"authorization",
		"Bearer "+verify.Tokens.AccessToken,
	))

	return account, authCtx
}

func testMessageDraft(id string) *commonv1.MessageDraft {
	return &commonv1.MessageDraft{
		ClientMessageId: id,
		Kind:            commonv1.MessageKind_MESSAGE_KIND_TEXT,
		Payload: &commonv1.EncryptedPayload{
			KeyId:      "key-" + id,
			Algorithm:  "xchacha20poly1305",
			Nonce:      []byte("nonce-" + id),
			Ciphertext: []byte("ciphertext-" + id),
		},
	}
}

type testSubscribeEventsStream struct {
	ctx       context.Context
	cancel    context.CancelFunc
	responses chan *syncv1.SubscribeEventsResponse
}

func newTestSubscribeEventsStream(ctx context.Context) *testSubscribeEventsStream {
	streamCtx, cancel := context.WithCancel(ctx)
	return &testSubscribeEventsStream{
		ctx:       streamCtx,
		cancel:    cancel,
		responses: make(chan *syncv1.SubscribeEventsResponse, 4),
	}
}

func (s *testSubscribeEventsStream) Context() context.Context { return s.ctx }
func (s *testSubscribeEventsStream) Send(resp *syncv1.SubscribeEventsResponse) error {
	s.responses <- resp
	return nil
}
func (*testSubscribeEventsStream) SetHeader(metadata.MD) error  { return nil }
func (*testSubscribeEventsStream) SendHeader(metadata.MD) error { return nil }
func (*testSubscribeEventsStream) SetTrailer(metadata.MD)       {}
func (*testSubscribeEventsStream) SendMsg(any) error            { return nil }
func (*testSubscribeEventsStream) RecvMsg(any) error            { return nil }

type gatewayMediaStore struct {
	mu     sync.Mutex
	assets map[string]media.MediaAsset
}

func newGatewayMediaStore() *gatewayMediaStore {
	return &gatewayMediaStore{assets: make(map[string]media.MediaAsset)}
}

func (s *gatewayMediaStore) WithinTx(_ context.Context, fn func(media.Store) error) error {
	return fn(s)
}

func (s *gatewayMediaStore) SaveMediaAsset(_ context.Context, asset media.MediaAsset) (media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.assets == nil {
		s.assets = make(map[string]media.MediaAsset)
	}
	s.assets[asset.ID] = cloneGatewayMediaAsset(asset)
	return cloneGatewayMediaAsset(asset), nil
}

func (s *gatewayMediaStore) MediaAssetByID(_ context.Context, mediaID string) (media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	asset, ok := s.assets[mediaID]
	if !ok {
		return media.MediaAsset{}, media.ErrNotFound
	}
	return cloneGatewayMediaAsset(asset), nil
}

func (s *gatewayMediaStore) MediaAssetsByOwner(_ context.Context, ownerAccountID string, limit int) ([]media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	assets := make([]media.MediaAsset, 0, len(s.assets))
	for _, asset := range s.assets {
		if asset.OwnerAccountID != ownerAccountID {
			continue
		}
		assets = append(assets, cloneGatewayMediaAsset(asset))
	}
	if limit > 0 && len(assets) > limit {
		assets = assets[:limit]
	}

	return assets, nil
}

func (s *gatewayMediaStore) MediaActiveAssetsByOwner(_ context.Context, ownerAccountID string, limit int) ([]media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	assets := make([]media.MediaAsset, 0, len(s.assets))
	for _, asset := range s.assets {
		if asset.OwnerAccountID != ownerAccountID || asset.Status == media.MediaStatusDeleted {
			continue
		}
		assets = append(assets, cloneGatewayMediaAsset(asset))
	}
	if limit > 0 && len(assets) > limit {
		assets = assets[:limit]
	}

	return assets, nil
}

func (s *gatewayMediaStore) MediaAssetByObjectKey(_ context.Context, objectKey string) (media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, asset := range s.assets {
		if asset.ObjectKey == objectKey {
			return cloneGatewayMediaAsset(asset), nil
		}
	}

	return media.MediaAsset{}, media.ErrNotFound
}

func (s *gatewayMediaStore) DeleteMediaAsset(_ context.Context, mediaID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.assets[mediaID]; !ok {
		return media.ErrNotFound
	}
	delete(s.assets, mediaID)
	return nil
}

type gatewayBlobStore struct {
	mu      sync.Mutex
	bucket  string
	payload map[string][]byte
}

func newGatewayBlobStore(bucket string) *gatewayBlobStore {
	return &gatewayBlobStore{
		bucket:  bucket,
		payload: make(map[string][]byte),
	}
}

func (*gatewayBlobStore) Name() string                   { return "object" }
func (*gatewayBlobStore) Kind() domainstorage.Kind       { return domainstorage.KindObject }
func (*gatewayBlobStore) Purpose() domainstorage.Purpose { return domainstorage.PurposeObject }
func (*gatewayBlobStore) Capabilities() domainstorage.Capability {
	return domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityBlob
}
func (*gatewayBlobStore) Close(context.Context) error { return nil }
func (s *gatewayBlobStore) Bucket() string            { return s.bucket }

func (s *gatewayBlobStore) PutObject(
	_ context.Context,
	key string,
	body io.Reader,
	_ int64,
	_ domainstorage.PutObjectOptions,
) (domainstorage.BlobObject, error) {
	payload, _ := io.ReadAll(body)
	s.mu.Lock()
	s.payload[key] = append([]byte(nil), payload...)
	s.mu.Unlock()

	return domainstorage.BlobObject{Bucket: s.bucket, Key: key}, nil
}

func (s *gatewayBlobStore) GetObject(_ context.Context, key string) (io.ReadCloser, domainstorage.BlobObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, ok := s.payload[key]
	if !ok {
		return nil, domainstorage.BlobObject{}, domainstorage.ErrNotFound
	}

	return io.NopCloser(bytes.NewReader(payload)), domainstorage.BlobObject{Bucket: s.bucket, Key: key}, nil
}

func (s *gatewayBlobStore) HeadObject(_ context.Context, key string) (domainstorage.BlobObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.payload[key]; !ok {
		return domainstorage.BlobObject{}, domainstorage.ErrNotFound
	}

	return domainstorage.BlobObject{Bucket: s.bucket, Key: key}, nil
}

func (s *gatewayBlobStore) DeleteObject(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.payload, key)
	return nil
}

func (s *gatewayBlobStore) PresignPutObject(
	_ context.Context,
	key string,
	expires time.Duration,
	options domainstorage.PutObjectOptions,
) (domainstorage.PresignedRequest, error) {
	return domainstorage.PresignedRequest{
		URL:       "https://example.invalid/upload/" + key,
		Method:    http.MethodPut,
		Headers:   map[string]string{"content-type": options.ContentType},
		ExpiresAt: time.Now().UTC().Add(expires),
		Bucket:    s.bucket,
		ObjectKey: key,
	}, nil
}

func (s *gatewayBlobStore) PresignGetObject(
	_ context.Context,
	key string,
	expires time.Duration,
) (domainstorage.PresignedRequest, error) {
	return domainstorage.PresignedRequest{
		URL:       "https://example.invalid/download/" + key,
		Method:    http.MethodGet,
		ExpiresAt: time.Now().UTC().Add(expires),
		Bucket:    s.bucket,
		ObjectKey: key,
	}, nil
}

func cloneGatewayMediaAsset(asset media.MediaAsset) media.MediaAsset {
	clone := asset
	if len(asset.Metadata) > 0 {
		clone.Metadata = make(map[string]string, len(asset.Metadata))
		for key, value := range asset.Metadata {
			clone.Metadata[key] = value
		}
	}
	return clone
}
