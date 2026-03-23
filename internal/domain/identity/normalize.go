package identity

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

func normalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizePhone(value string) string {
	return strings.TrimSpace(value)
}

func maskEmail(value string) string {
	value = normalizeEmail(value)
	if value == "" {
		return ""
	}

	parts := strings.SplitN(value, "@", 2)
	if len(parts) != 2 || parts[0] == "" {
		return value
	}

	localPart := parts[0]
	if len(localPart) <= 2 {
		return "***@" + parts[1]
	}

	return localPart[:1] + "***@" + parts[1]
}

func maskPhone(value string) string {
	value = normalizePhone(value)
	if value == "" {
		return ""
	}

	if len(value) <= 4 {
		return "***" + value
	}

	return value[:2] + "***" + value[len(value)-2:]
}

func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}
