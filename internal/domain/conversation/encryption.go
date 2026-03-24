package conversation

import "strings"

// ValidateMessagePayload ensures the message payload contains a body and,
// when required, the full E2EE envelope.
func ValidateMessagePayload(payload EncryptedPayload, requireEncrypted bool) error {
	if len(payload.Ciphertext) == 0 {
		return ErrInvalidInput
	}
	if !requireEncrypted {
		return nil
	}

	return ValidateEncryptedPayload(payload)
}

// ValidateEncryptedPayload ensures the message payload has a complete E2EE envelope.
func ValidateEncryptedPayload(payload EncryptedPayload) error {
	if strings.TrimSpace(payload.KeyID) == "" {
		return ErrInvalidInput
	}
	if strings.TrimSpace(payload.Algorithm) == "" {
		return ErrInvalidInput
	}
	if len(payload.Nonce) == 0 {
		return ErrInvalidInput
	}
	if len(payload.Ciphertext) == 0 {
		return ErrInvalidInput
	}

	return nil
}

// StripMessageHints removes plaintext reply hints before persistence.
func StripMessageHints(message *Message) {
	if message == nil {
		return
	}

	message.ReplyTo.Snippet = ""
	for i := range message.Attachments {
		message.Attachments[i].Caption = ""
	}
}
