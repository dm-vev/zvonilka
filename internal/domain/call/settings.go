package call

import "time"

// Settings controls call lifecycle timing.
type Settings struct {
	InviteTimeout        time.Duration
	RingingTimeout       time.Duration
	ReconnectGrace       time.Duration
	MaxDuration          time.Duration
	MaxGroupParticipants uint32
	MaxVideoParticipants uint32
}

// DefaultSettings returns the default call settings.
func DefaultSettings() Settings {
	return Settings{
		InviteTimeout:        45 * time.Second,
		RingingTimeout:       45 * time.Second,
		ReconnectGrace:       20 * time.Second,
		MaxDuration:          2 * time.Hour,
		MaxGroupParticipants: 32,
		MaxVideoParticipants: 8,
	}
}

func (s Settings) normalize() Settings {
	defaults := DefaultSettings()
	if s.InviteTimeout <= 0 {
		s.InviteTimeout = defaults.InviteTimeout
	}
	if s.RingingTimeout <= 0 {
		s.RingingTimeout = defaults.RingingTimeout
	}
	if s.ReconnectGrace <= 0 {
		s.ReconnectGrace = defaults.ReconnectGrace
	}
	if s.MaxDuration <= 0 {
		s.MaxDuration = defaults.MaxDuration
	}
	if s.MaxGroupParticipants == 0 {
		s.MaxGroupParticipants = defaults.MaxGroupParticipants
	}
	if s.MaxVideoParticipants == 0 {
		s.MaxVideoParticipants = defaults.MaxVideoParticipants
	}
	if s.MaxVideoParticipants > s.MaxGroupParticipants {
		s.MaxVideoParticipants = s.MaxGroupParticipants
	}

	return s
}
