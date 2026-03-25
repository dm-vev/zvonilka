package notification

import "time"

// Settings controls worker cadence and retry semantics for notification fanout.
type Settings struct {
	WorkerPollInterval  time.Duration
	RetryInitialBackoff time.Duration
	RetryMaxBackoff     time.Duration
	MaxAttempts         int
	BatchSize           int
}

// DefaultSettings returns a conservative baseline for notification processing.
func DefaultSettings() Settings {
	return Settings{
		WorkerPollInterval:  2 * time.Second,
		RetryInitialBackoff: 10 * time.Second,
		RetryMaxBackoff:     5 * time.Minute,
		MaxAttempts:         5,
		BatchSize:           200,
	}
}

func (s Settings) normalize() Settings {
	defaults := DefaultSettings()
	if s.WorkerPollInterval <= 0 {
		s.WorkerPollInterval = defaults.WorkerPollInterval
	}
	if s.RetryInitialBackoff <= 0 {
		s.RetryInitialBackoff = defaults.RetryInitialBackoff
	}
	if s.RetryMaxBackoff <= 0 {
		s.RetryMaxBackoff = defaults.RetryMaxBackoff
	}
	if s.RetryMaxBackoff < s.RetryInitialBackoff {
		s.RetryMaxBackoff = s.RetryInitialBackoff
	}
	if s.MaxAttempts <= 0 {
		s.MaxAttempts = defaults.MaxAttempts
	}
	if s.BatchSize <= 0 {
		s.BatchSize = defaults.BatchSize
	}

	return s
}
