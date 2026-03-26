package botapi

type replyData struct {
	MessageID textID `json:"message_id"`
	ChatID    textID `json:"chat_id"`
}

type previewData struct {
	IsDisabled bool `json:"is_disabled"`
}

func replyMessageID(legacy textID, reply *replyData) textID {
	if legacy != "" {
		return legacy
	}
	if reply == nil {
		return ""
	}

	return reply.MessageID
}

func disablePreview(legacy bool, preview *previewData) bool {
	if preview == nil {
		return legacy
	}

	return legacy || preview.IsDisabled
}
