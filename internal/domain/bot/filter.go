package bot

import (
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func updateTypeForEvent(eventType conversation.EventType, conv conversation.Conversation) UpdateType {
	switch eventType {
	case conversation.EventTypeMessageCreated:
		if conv.Kind == conversation.ConversationKindChannel {
			return UpdateTypeChannelPost
		}
		return UpdateTypeMessage
	case conversation.EventTypeMessageEdited:
		if conv.Kind == conversation.ConversationKindChannel {
			return UpdateTypeEditedChannelPost
		}
		return UpdateTypeEditedMessage
	case conversation.EventTypeConversationMembers:
		return UpdateTypeChatMember
	default:
		return UpdateTypeUnspecified
	}
}

func shouldRoute(conv conversation.Conversation, msg conversation.Message, botAccountID string) bool {
	switch conv.Kind {
	case conversation.ConversationKindDirect:
		return true
	case conversation.ConversationKindChannel:
		return true
	case conversation.ConversationKindGroup:
		if mentionsBot(msg, botAccountID) {
			return true
		}
		if strings.TrimSpace(msg.ReplyTo.SenderAccountID) == botAccountID {
			return true
		}
		return commandMessage(msg)
	default:
		return false
	}
}

func mentionsBot(msg conversation.Message, botAccountID string) bool {
	for _, accountID := range msg.MentionAccountIDs {
		if accountID == botAccountID {
			return true
		}
	}

	return false
}

func commandMessage(msg conversation.Message) bool {
	return strings.HasPrefix(strings.TrimSpace(plainText(msg)), "/")
}
