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

// normalizePhone strips all non-digit characters from a phone number and
// canonicalizes all Unicode decimal digits to ASCII digits.
func normalizePhone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	digits := make([]byte, 0, len(value))
	for _, symbol := range value {
		if digit, ok := normalizeDigit(symbol); ok {
			digits = append(digits, digit)
		}
	}

	return string(digits)
}

// normalizeDigit maps a Unicode decimal digit to its ASCII representation.
func normalizeDigit(symbol rune) (byte, bool) {
	if symbol >= '0' && symbol <= '9' {
		return byte(symbol), true
	}

	for _, digitRange := range unicode.Digit.R16 {
		if digitRange.Stride != 1 {
			continue
		}
		if symbol < rune(digitRange.Lo) || symbol > rune(digitRange.Hi) {
			continue
		}

		offset := symbol - rune(digitRange.Lo)
		if offset >= 0 && offset < 10 {
			return byte('0' + offset), true
		}
	}

	for _, digitRange := range unicode.Digit.R32 {
		if digitRange.Stride != 1 {
			continue
		}
		if symbol < rune(digitRange.Lo) || symbol > rune(digitRange.Hi) {
			continue
		}

		offset := symbol - rune(digitRange.Lo)
		if offset >= 0 && offset < 10 {
			return byte('0' + offset), true
		}
	}

	return 0, false
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

	localPartRunes := []rune(parts[0])
	if len(localPartRunes) <= 2 {
		return "***@" + parts[1]
	}

	return string(localPartRunes[:1]) + "***@" + parts[1]
}

// maskPhone returns a partially redacted phone number for UI and logs.
func maskPhone(value string) string {
	value = normalizePhone(value)
	if value == "" {
		return ""
	}

	phoneRunes := []rune(value)
	if len(phoneRunes) <= 4 {
		return "***" + string(phoneRunes)
	}

	return string(phoneRunes[:2]) + "***" + string(phoneRunes[len(phoneRunes)-2:])
}

// hashSecret hashes a secret value into a stable opaque string.
func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}
