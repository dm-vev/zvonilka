package conversation

import "strings"

// ValidateEncryptedPayload ensures the message payload is opaque ciphertext.
func ValidateEncryptedPayload(payload EncryptedPayload) error {
	if strings.TrimSpace(payload.KeyID) == "" {
		return ErrInvalidInput
	}
	if strings.TrimSpace(payload.Algorithm) == "" {
		return ErrInvalidInput
	}
	if len(payload.Nonce) == 0 || len(payload.Ciphertext) == 0 {
		return ErrInvalidInput
	}

	return nil
}

// SanitizeEncryptedMessage removes plaintext reply hints before persistence.
func SanitizeEncryptedMessage(message *Message) {
	if message == nil {
		return
	}

	message.ReplyTo.Snippet = ""
	for i := range message.Attachments {
		message.Attachments[i].Caption = ""
	}
}
