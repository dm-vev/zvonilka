package bot

import (
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		cloned[key] = value
	}
	if len(cloned) == 0 {
		return nil
	}

	return cloned
}

func withoutMarkup(metadata map[string]string) map[string]string {
	cloned := cloneMetadata(metadata)
	delete(cloned, metadataReplyMarkupKey)
	if len(cloned) == 0 {
		return nil
	}

	return cloned
}

func withCaption(metadata map[string]string, caption string) map[string]string {
	cloned := cloneMetadata(metadata)
	caption = strings.TrimSpace(caption)
	if caption == "" {
		delete(cloned, metadataCaptionKey)
	} else {
		if cloned == nil {
			cloned = make(map[string]string, 1)
		}
		cloned[metadataCaptionKey] = caption
	}
	if len(cloned) == 0 {
		return nil
	}

	return cloned
}

func supportsCaption(message conversation.Message) bool {
	return len(message.Attachments) > 0
}

func copyDraft(message conversation.Message, threadID string, silent bool, metadata map[string]string) conversation.MessageDraft {
	return conversation.MessageDraft{
		Kind:                message.Kind,
		Payload:             message.Payload,
		Attachments:         append([]conversation.AttachmentRef(nil), message.Attachments...),
		MentionAccountIDs:   append([]string(nil), message.MentionAccountIDs...),
		ThreadID:            strings.TrimSpace(threadID),
		Silent:              silent,
		DisableLinkPreviews: message.DisableLinkPreviews,
		Metadata:            cloneMetadata(metadata),
	}
}
