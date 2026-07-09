package config

import "time"

// FederationConfig defines federation replication runtime settings.
type FederationConfig struct {
	LocalServerName    string
	BridgeSharedSecret string
	BridgeEndpoint     string
	BridgePollInterval time.Duration
	BridgeBatchSize    int
	BridgePeerServer   string
	BridgeLinkName     string
	WorkerPollInterval time.Duration
	WorkerBatchSize    int
	DialTimeout        time.Duration
}
