package user

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

func trimmed(value string) string {
	return strings.TrimSpace(value)
}

func normalizeVisibility(value Visibility) Visibility {
	switch strings.ToLower(strings.TrimSpace(string(value))) {
	case string(VisibilityEveryone):
		return VisibilityEveryone
	case string(VisibilityContacts):
		return VisibilityContacts
	case string(VisibilityNobody):
		return VisibilityNobody
	case string(VisibilityCustom):
		return VisibilityCustom
	default:
		return VisibilityUnspecified
	}
}

func normalizePrivacyRule(rule PrivacyRule) (PrivacyRule, bool) {
	rule.Base = normalizeVisibility(rule.Base)
	if rule.Base != VisibilityEveryone && rule.Base != VisibilityContacts && rule.Base != VisibilityNobody {
		return PrivacyRule{}, false
	}
	rule.AllowUserIDs = normalizedIDs(rule.AllowUserIDs)
	rule.RestrictUserIDs = normalizedIDs(rule.RestrictUserIDs)
	rule.AllowChatIDs = normalizedIDs(rule.AllowChatIDs)
	rule.RestrictChatIDs = normalizedIDs(rule.RestrictChatIDs)
	if rule.AllowBots && rule.RestrictBots {
		return PrivacyRule{}, false
	}
	return rule, true
}

func normalizedIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizePhone(value string) string {
	var builder strings.Builder
	for _, r := range strings.TrimSpace(value) {
		if !unicode.IsDigit(r) {
			continue
		}

		digit := -1
		switch {
		case r >= '0' && r <= '9':
			digit = int(r - '0')
		case unicode.Is(unicode.Arabic, r):
			switch r {
			case '٠':
				digit = 0
			case '١':
				digit = 1
			case '٢':
				digit = 2
			case '٣':
				digit = 3
			case '٤':
				digit = 4
			case '٥':
				digit = 5
			case '٦':
				digit = 6
			case '٧':
				digit = 7
			case '٨':
				digit = 8
			case '٩':
				digit = 9
			}
		}

		if digit >= 0 {
			builder.WriteByte(byte('0' + digit))
			continue
		}

		builder.WriteRune(r)
	}

	return builder.String()
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func phoneHash(value string) string {
	value = normalizePhone(value)
	if value == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
