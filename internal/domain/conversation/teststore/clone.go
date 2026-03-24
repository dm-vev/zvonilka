package teststore

import "github.com/dm-vev/zvonilka/internal/domain/conversation"

func cloneConversations(src map[string]conversation.Conversation) map[string]conversation.Conversation {
	if len(src) == 0 {
		return make(map[string]conversation.Conversation)
	}

	dst := make(map[string]conversation.Conversation, len(src))
	for key, value := range src {
		dst[key] = cloneConversation(value)
	}

	return dst
}

func cloneMembers(src map[string]conversation.ConversationMember) map[string]conversation.ConversationMember {
	if len(src) == 0 {
		return make(map[string]conversation.ConversationMember)
	}

	dst := make(map[string]conversation.ConversationMember, len(src))
	for key, value := range src {
		dst[key] = cloneMember(value)
	}

	return dst
}

func cloneTopics(src map[string]conversation.ConversationTopic) map[string]conversation.ConversationTopic {
	if len(src) == 0 {
		return make(map[string]conversation.ConversationTopic)
	}

	dst := make(map[string]conversation.ConversationTopic, len(src))
	for key, value := range src {
		dst[key] = cloneTopic(value)
	}

	return dst
}

func cloneMessages(src map[string]conversation.Message) map[string]conversation.Message {
	if len(src) == 0 {
		return make(map[string]conversation.Message)
	}

	dst := make(map[string]conversation.Message, len(src))
	for key, value := range src {
		dst[key] = cloneMessage(value)
	}

	return dst
}

func cloneReactions(src map[string]conversation.MessageReaction) map[string]conversation.MessageReaction {
	if len(src) == 0 {
		return make(map[string]conversation.MessageReaction)
	}

	dst := make(map[string]conversation.MessageReaction, len(src))
	for key, value := range src {
		dst[key] = cloneReaction(value)
	}

	return dst
}

func cloneReadStates(src map[string]conversation.ReadState) map[string]conversation.ReadState {
	if len(src) == 0 {
		return make(map[string]conversation.ReadState)
	}

	dst := make(map[string]conversation.ReadState, len(src))
	for key, value := range src {
		dst[key] = cloneReadState(value)
	}

	return dst
}

func cloneSyncStates(src map[string]conversation.SyncState) map[string]conversation.SyncState {
	if len(src) == 0 {
		return make(map[string]conversation.SyncState)
	}

	dst := make(map[string]conversation.SyncState, len(src))
	for key, value := range src {
		dst[key] = cloneSyncState(value)
	}

	return dst
}

func cloneEvents(src map[string]conversation.EventEnvelope) map[string]conversation.EventEnvelope {
	if len(src) == 0 {
		return make(map[string]conversation.EventEnvelope)
	}

	dst := make(map[string]conversation.EventEnvelope, len(src))
	for key, value := range src {
		dst[key] = cloneEvent(value)
	}

	return dst
}

func cloneConversation(value conversation.Conversation) conversation.Conversation {
	return value
}

func cloneTopic(value conversation.ConversationTopic) conversation.ConversationTopic {
	return value
}

func cloneMember(value conversation.ConversationMember) conversation.ConversationMember {
	return value
}

func cloneMessage(value conversation.Message) conversation.Message {
	value.Attachments = append([]conversation.AttachmentRef(nil), value.Attachments...)
	value.MentionAccountIDs = append([]string(nil), value.MentionAccountIDs...)
	value.Reactions = append([]conversation.MessageReaction(nil), value.Reactions...)
	if len(value.Metadata) > 0 {
		metadata := make(map[string]string, len(value.Metadata))
		for key, entry := range value.Metadata {
			metadata[key] = entry
		}
		value.Metadata = metadata
	}
	return value
}

func cloneReadState(value conversation.ReadState) conversation.ReadState {
	return value
}

func cloneSyncState(value conversation.SyncState) conversation.SyncState {
	if len(value.ConversationWatermarks) > 0 {
		watermarks := make(map[string]uint64, len(value.ConversationWatermarks))
		for key, entry := range value.ConversationWatermarks {
			watermarks[key] = entry
		}
		value.ConversationWatermarks = watermarks
	}
	return value
}

func cloneReaction(value conversation.MessageReaction) conversation.MessageReaction {
	return value
}

func cloneEvent(value conversation.EventEnvelope) conversation.EventEnvelope {
	value.Metadata = cloneStringMap(value.Metadata)
	value.Payload.Metadata = cloneStringMap(value.Payload.Metadata)
	return value
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}

	return dst
}
