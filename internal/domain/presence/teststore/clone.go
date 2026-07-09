package teststore

import "github.com/dm-vev/zvonilka/internal/domain/presence"

func clonePresences(src map[string]presence.Presence) map[string]presence.Presence {
	if len(src) == 0 {
		return make(map[string]presence.Presence)
	}

	dst := make(map[string]presence.Presence, len(src))
	for key, value := range src {
		dst[key] = clonePresence(value)
	}

	return dst
}

func clonePresence(value presence.Presence) presence.Presence {
	return value
}
