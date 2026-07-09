package config

import (
	"time"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

// BotConfig defines Bot API worker and delivery settings.
type BotConfig struct {
	FanoutPollInterval  time.Duration
	FanoutBatchSize     int
	GetUpdatesMaxLimit  int
	LongPollMaxTimeout  time.Duration
	LongPollStep        time.Duration
	WebhookTimeout      time.Duration
	WebhookBatchSize    int
	RetryInitialBackoff time.Duration
	RetryMaxBackoff     time.Duration
	MaxAttempts         int
}

// ToSettings converts configuration into domain bot settings.
func (c BotConfig) ToSettings() domainbot.Settings {
	settings := domainbot.DefaultSettings()
	if c.FanoutPollInterval > 0 {
		settings.FanoutPollInterval = c.FanoutPollInterval
	}
	if c.FanoutBatchSize > 0 {
		settings.FanoutBatchSize = c.FanoutBatchSize
	}
	if c.GetUpdatesMaxLimit > 0 {
		settings.GetUpdatesMaxLimit = c.GetUpdatesMaxLimit
	}
	if c.LongPollMaxTimeout > 0 {
		settings.LongPollMaxTimeout = c.LongPollMaxTimeout
	}
	if c.LongPollStep > 0 {
		settings.LongPollStep = c.LongPollStep
	}
	if c.WebhookTimeout > 0 {
		settings.WebhookTimeout = c.WebhookTimeout
	}
	if c.WebhookBatchSize > 0 {
		settings.WebhookBatchSize = c.WebhookBatchSize
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

	return settings
}
