package callhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	// SignatureHeader carries the shared-secret HMAC for call hook requests.
	SignatureHeader = "X-Zvonilka-Signature"
	signaturePrefix = "sha256="
)

// SignPayload returns the canonical HMAC header value for one payload.
func SignPayload(secret string, body []byte) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)

	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature verifies one HMAC header against the payload body.
func VerifySignature(secret string, body []byte, signature string) bool {
	secret = strings.TrimSpace(secret)
	signature = strings.TrimSpace(signature)
	if secret == "" {
		return true
	}
	if !strings.HasPrefix(signature, signaturePrefix) {
		return false
	}

	expected := SignPayload(secret, body)
	return hmac.Equal([]byte(expected), []byte(signature))
}
