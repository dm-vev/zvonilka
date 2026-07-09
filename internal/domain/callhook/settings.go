package callhook

import "time"

const (
	defaultExecutorPollInterval = 2 * time.Second
	defaultExecutorBatchSize    = 100
	defaultExecutorLeaseTTL     = 30 * time.Second
	defaultExecutorRetryInitial = 5 * time.Second
	defaultExecutorRetryMax     = 5 * time.Minute
)

// ExecutorSettings control background job execution cadence.
type ExecutorSettings struct {
	PollInterval        time.Duration
	BatchSize           int
	LeaseTTL            time.Duration
	RetryInitialBackoff time.Duration
	RetryMaxBackoff     time.Duration
}

func (s ExecutorSettings) normalize() ExecutorSettings {
	if s.PollInterval <= 0 {
		s.PollInterval = defaultExecutorPollInterval
	}
	if s.BatchSize <= 0 {
		s.BatchSize = defaultExecutorBatchSize
	}
	if s.LeaseTTL <= 0 {
		s.LeaseTTL = defaultExecutorLeaseTTL
	}
	if s.RetryInitialBackoff <= 0 {
		s.RetryInitialBackoff = defaultExecutorRetryInitial
	}
	if s.RetryMaxBackoff <= 0 {
		s.RetryMaxBackoff = defaultExecutorRetryMax
	}
	if s.RetryMaxBackoff < s.RetryInitialBackoff {
		s.RetryMaxBackoff = s.RetryInitialBackoff
	}

	return s
}
