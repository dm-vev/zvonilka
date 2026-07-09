package bot

import "time"

// Settings defines runtime settings for the bot domain.
type Settings struct {
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

// DefaultSettings returns the default bot settings.
func DefaultSettings() Settings {
	return Settings{
		FanoutPollInterval:  500 * time.Millisecond,
		FanoutBatchSize:     256,
		GetUpdatesMaxLimit:  100,
		LongPollMaxTimeout:  50 * time.Second,
		LongPollStep:        250 * time.Millisecond,
		WebhookTimeout:      10 * time.Second,
		WebhookBatchSize:    100,
		RetryInitialBackoff: 1 * time.Second,
		RetryMaxBackoff:     5 * time.Minute,
		MaxAttempts:         20,
	}
}
