package config

import (
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

// CallConfig defines signaling lifecycle settings for direct calls.
type CallConfig struct {
	InviteTimeout        time.Duration
	RingingTimeout       time.Duration
	ReconnectGrace       time.Duration
	MaxDuration          time.Duration
	MaxGroupParticipants uint32
	MaxVideoParticipants uint32
}

// ToSettings converts config values into domain call settings.
func (c CallConfig) ToSettings() domaincall.Settings {
	return domaincall.Settings{
		InviteTimeout:        c.InviteTimeout,
		RingingTimeout:       c.RingingTimeout,
		ReconnectGrace:       c.ReconnectGrace,
		MaxDuration:          c.MaxDuration,
		MaxGroupParticipants: c.MaxGroupParticipants,
		MaxVideoParticipants: c.MaxVideoParticipants,
	}
}
