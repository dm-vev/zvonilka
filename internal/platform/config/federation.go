package config

import "time"

// FederationConfig defines federation replication runtime settings.
type FederationConfig struct {
	LocalServerName    string
	WorkerPollInterval time.Duration
	WorkerBatchSize    int
	DialTimeout        time.Duration
}
