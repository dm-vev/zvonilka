package config

import (
	"time"

	domainnotification "github.com/dm-vev/zvonilka/internal/domain/notification"
)

// NotificationConfig defines notification worker cadence and retry settings.
type NotificationConfig struct {
	WorkerPollInterval  time.Duration
	RetryInitialBackoff time.Duration
	RetryMaxBackoff     time.Duration
	MaxAttempts         int
	BatchSize           int
}

// ToSettings converts configuration into domain notification settings.
func (c NotificationConfig) ToSettings() domainnotification.Settings {
	settings := domainnotification.DefaultSettings()
	if c.WorkerPollInterval > 0 {
		settings.WorkerPollInterval = c.WorkerPollInterval
	}
	if c.RetryInitialBackoff > 0 {
		settings.RetryInitialBackoff = c.RetryInitialBackoff
	}
	if c.RetryMaxBackoff > 0 {
		settings.RetryMaxBackoff = c.RetryMaxBackoff
	}
	if c.MaxAttempts > 0 {
		settings.MaxAttempts = c.MaxAttempts
	}
	if c.BatchSize > 0 {
		settings.BatchSize = c.BatchSize
	}

	return settings
}
