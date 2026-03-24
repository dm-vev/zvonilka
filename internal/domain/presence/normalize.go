package presence

import "strings"

// TrimID canonicalizes presence identifiers.
func TrimID(value string) string {
	return strings.TrimSpace(value)
}

// Normalize canonicalizes a presence record before persistence.
func Normalize(value Presence) (Presence, error) {
	return normalizePresence(value)
}
