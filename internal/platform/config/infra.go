package config

import "time"

// InfrastructureConfig groups storage, cache, and telemetry adapters.
type InfrastructureConfig struct {
	Postgres    PostgresConfig
	Redis       RedisConfig
	ObjectStore ObjectStorageConfig
	Telemetry   TelemetryConfig
}

// PostgresConfig defines relational storage settings.
type PostgresConfig struct {
	Enabled         bool
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	MigrationsPath  string
	Schema          string
}

// RedisConfig defines cache and ephemeral-state settings.
type RedisConfig struct {
	Enabled         bool
	Address         string
	Username        string
	Password        string
	DB              int
	PoolSize        int
	DialTimeout     time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ConnMaxIdleTime time.Duration
}

// ObjectStorageConfig defines S3-compatible blob storage settings.
type ObjectStorageConfig struct {
	Enabled         bool
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	ForcePathStyle  bool
}

// TelemetryConfig defines tracing and error-reporting settings.
type TelemetryConfig struct {
	MetricsEnabled bool
	TracingEnabled bool
	OTLPAddress    string
	SentryDSN      string
}
