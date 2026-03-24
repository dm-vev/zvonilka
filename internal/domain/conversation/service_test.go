package conversation_test

import (
	"context"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	teststore "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
)

func TestConversationLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	fixedNow := time.Date(2026, time.March, 24, 10, 0, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, members, err := svc.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:  "acc-owner",
		Kind:            conversation.ConversationKindDirect,
		Title:           "Direct",
		MemberAccountIDs: []string{"acc-peer"},
		CreatedAt:       fixedNow,
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if created.LastSequence != 1 {
		t.Fatalf("expected initial event sequence 1, got %d", created.LastSequence)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	message, event, err := svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			ClientMessageID: "client-1",
			Kind:            conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				AAD:        []byte("aad"),
				Metadata:   map[string]string{"format": "v1"},
			},
			Attachments: []conversation.AttachmentRef{
				{
					MediaID:   "media-1",
					Kind:      conversation.AttachmentKindImage,
					FileName:  "photo.jpg",
					MimeType:  "image/jpeg",
					SizeBytes: 1024,
					SHA256Hex: "abc123",
				},
			},
		},
		CreatedAt: fixedNow.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if message.Sequence != event.Sequence {
		t.Fatalf("expected message and event sequence to match, got %d and %d", message.Sequence, event.Sequence)
	}
	if len(message.Attachments) != 1 {
		t.Fatalf("expected attachment to persist, got %d", len(message.Attachments))
	}

	delivered, deliveryEvent, err := svc.RecordDelivery(ctx, conversation.RecordDeliveryParams{
		ConversationID:        created.ID,
		AccountID:             "acc-peer",
		DeviceID:              "dev-peer",
		MessageID:             message.ID,
		DeliveredThroughSequence: event.Sequence,
		CreatedAt:             fixedNow.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("record delivery: %v", err)
	}
	if delivered.LastDeliveredSequence != event.Sequence {
		t.Fatalf("expected delivery watermark %d, got %d", event.Sequence, delivered.LastDeliveredSequence)
	}
	if deliveryEvent.EventType != conversation.EventTypeMessageDelivered {
		t.Fatalf("expected delivery event, got %s", deliveryEvent.EventType)
	}

	readState, readEvent, err := svc.MarkRead(ctx, conversation.MarkReadParams{
		ConversationID:   created.ID,
		AccountID:        "acc-peer",
		DeviceID:         "dev-peer",
		ReadThroughSequence: event.Sequence,
		CreatedAt:        fixedNow.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if readState.LastReadSequence != event.Sequence {
		t.Fatalf("expected read watermark %d, got %d", event.Sequence, readState.LastReadSequence)
	}
	if readEvent.EventType != conversation.EventTypeMessageRead {
		t.Fatalf("expected read event, got %s", readEvent.EventType)
	}

	events, syncState, err := svc.PullEvents(ctx, conversation.PullEventsParams{
		DeviceID:     "dev-peer",
		FromSequence: 0,
		Limit:        100,
	})
	if err != nil {
		t.Fatalf("pull events: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
	if syncState.LastAppliedSequence != events[len(events)-1].Sequence {
		t.Fatalf("expected applied sequence to track latest event")
	}

	acked, err := svc.AcknowledgeEvents(ctx, conversation.AcknowledgeEventsParams{
		DeviceID:      "dev-peer",
		AckedSequence: event.Sequence,
	})
	if err != nil {
		t.Fatalf("acknowledge events: %v", err)
	}
	if acked.LastAckedSequence != event.Sequence {
		t.Fatalf("expected acked sequence %d, got %d", event.Sequence, acked.LastAckedSequence)
	}

	resolved, pending, err := svc.GetSyncState(ctx, conversation.GetSyncStateParams{DeviceID: "dev-peer"})
	if err != nil {
		t.Fatalf("get sync state: %v", err)
	}
	if resolved.DeviceID != "dev-peer" {
		t.Fatalf("unexpected sync device: %s", resolved.DeviceID)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending conversations, got %v", pending)
	}

	messages, err := svc.ListMessages(ctx, conversation.ListMessagesParams{
		AccountID:      "acc-peer",
		ConversationID: created.ID,
	})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected one message, got %d", len(messages))
	}

	conversations, err := svc.ListConversations(ctx, conversation.ListConversationsParams{
		AccountID: "acc-owner",
	})
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("expected one conversation, got %d", len(conversations))
	}
}

func TestCreateConversationRejectsDirectWithoutPeer(t *testing.T) {
	t.Parallel()

	svc, err := conversation.NewService(teststore.NewMemoryStore())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, _, err = svc.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID: "acc-owner",
		Kind:           conversation.ConversationKindDirect,
	})
	if err == nil {
		t.Fatal("expected direct conversation validation error")
	}
}
