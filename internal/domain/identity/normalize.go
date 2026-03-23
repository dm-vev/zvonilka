package identity

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"unicode"
)

// normalizeUsername canonicalizes usernames for lookup and uniqueness checks.
func normalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// normalizeEmail canonicalizes email addresses for lookup and uniqueness checks.
func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// normalizePhone strips all non-digit characters from a phone number.
func normalizePhone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	digits := make([]rune, 0, len(value))
	for _, symbol := range value {
		if unicode.IsDigit(symbol) {
			digits = append(digits, symbol)
		}
	}

	return string(digits)
}

// maskEmail returns a partially redacted email address for UI and logs.
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

// maskPhone returns a partially redacted phone number for UI and logs.
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

// hashSecret hashes a secret value into a stable opaque string.
func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}
