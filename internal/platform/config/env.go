package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func applyEnvOverrides(cfg *Configuration, serviceName string) error {
	cfg.Service.Environment = stringValue(serviceName, "ENV", cfg.Service.Environment)

	logLevelSet := false
	if value, ok, err := stringValueWithPresence(serviceName, "LOG_LEVEL", cfg.Logging.Level); err != nil {
		return err
	} else if ok {
		cfg.Logging.Level = strings.ToLower(value)
		logLevelSet = true
	}

	logFormatSet := false
	if value, ok, err := stringValueWithPresence(serviceName, "LOG_FORMAT", cfg.Logging.Format); err != nil {
		return err
	} else if ok {
		cfg.Logging.Format = strings.ToLower(value)
		logFormatSet = true
	}

	logAddSourceSet := false
	if value, ok, err := boolValue(serviceName, "LOG_ADD_SOURCE", cfg.Logging.AddSource); err != nil {
		return err
	} else if ok {
		cfg.Logging.AddSource = value
		logAddSourceSet = true
	}

	if value, ok, err := durationValue(serviceName, "SHUTDOWN_TIMEOUT", cfg.Runtime.ShutdownTimeout); err != nil {
		return err
	} else if ok {
		cfg.Runtime.ShutdownTimeout = value
	}

	if value, ok, err := stringValueWithPresence(serviceName, "HTTP_ADDR", cfg.Runtime.HTTP.Address); err != nil {
		return err
	} else if ok {
		cfg.Runtime.HTTP.Address = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "GRPC_ADDR", cfg.Runtime.GRPC.Address); err != nil {
		return err
	} else if ok {
		cfg.Runtime.GRPC.Address = value
	}
	if value, ok, err := durationValue(serviceName, "HTTP_READ_HEADER_TIMEOUT", cfg.Runtime.HTTP.ReadHeaderTimeout); err != nil {
		return err
	} else if ok {
		cfg.Runtime.HTTP.ReadHeaderTimeout = value
	}
	if value, ok, err := durationValue(serviceName, "HTTP_READ_TIMEOUT", cfg.Runtime.HTTP.ReadTimeout); err != nil {
		return err
	} else if ok {
		cfg.Runtime.HTTP.ReadTimeout = value
	}
	if value, ok, err := durationValue(serviceName, "HTTP_WRITE_TIMEOUT", cfg.Runtime.HTTP.WriteTimeout); err != nil {
		return err
	} else if ok {
		cfg.Runtime.HTTP.WriteTimeout = value
	}
	if value, ok, err := durationValue(serviceName, "HTTP_IDLE_TIMEOUT", cfg.Runtime.HTTP.IdleTimeout); err != nil {
		return err
	} else if ok {
		cfg.Runtime.HTTP.IdleTimeout = value
	}
	if value, ok, err := intValue(serviceName, "HTTP_MAX_HEADER_BYTES", cfg.Runtime.HTTP.MaxHeaderBytes); err != nil {
		return err
	} else if ok {
		cfg.Runtime.HTTP.MaxHeaderBytes = value
	}
	grpcReflectionSet := false
	if value, ok, err := boolValue(serviceName, "GRPC_REFLECTION_ENABLED", cfg.Runtime.GRPC.ReflectionEnabled); err != nil {
		return err
	} else if ok {
		cfg.Runtime.GRPC.ReflectionEnabled = value
		grpcReflectionSet = true
	}

	if value, ok, err := durationValue(serviceName, "IDENTITY_JOIN_REQUEST_TTL", cfg.Identity.JoinRequestTTL); err != nil {
		return err
	} else if ok {
		cfg.Identity.JoinRequestTTL = value
	}
	if value, ok, err := durationValue(serviceName, "IDENTITY_CHALLENGE_TTL", cfg.Identity.ChallengeTTL); err != nil {
		return err
	} else if ok {
		cfg.Identity.ChallengeTTL = value
	}
	if value, ok, err := durationValue(serviceName, "IDENTITY_ACCESS_TOKEN_TTL", cfg.Identity.AccessTokenTTL); err != nil {
		return err
	} else if ok {
		cfg.Identity.AccessTokenTTL = value
	}
	if value, ok, err := durationValue(serviceName, "IDENTITY_REFRESH_TOKEN_TTL", cfg.Identity.RefreshTokenTTL); err != nil {
		return err
	} else if ok {
		cfg.Identity.RefreshTokenTTL = value
	}
	if value, ok, err := intValue(serviceName, "IDENTITY_LOGIN_CODE_LENGTH", cfg.Identity.LoginCodeLength); err != nil {
		return err
	} else if ok {
		cfg.Identity.LoginCodeLength = value
	}

	postgresEnabledSet := false
	if value, ok, err := boolValue(serviceName, "POSTGRES_ENABLED", cfg.Infrastructure.Postgres.Enabled); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Postgres.Enabled = value
		postgresEnabledSet = true
	}
	if value, ok, err := stringValueWithPresence(serviceName, "POSTGRES_DSN", cfg.Infrastructure.Postgres.DSN); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Postgres.DSN = value
	}
	if value, ok, err := intValue(serviceName, "POSTGRES_MAX_OPEN_CONNS", cfg.Infrastructure.Postgres.MaxOpenConns); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Postgres.MaxOpenConns = value
	}
	if value, ok, err := intValue(serviceName, "POSTGRES_MAX_IDLE_CONNS", cfg.Infrastructure.Postgres.MaxIdleConns); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Postgres.MaxIdleConns = value
	}
	if value, ok, err := durationValue(serviceName, "POSTGRES_CONN_MAX_LIFETIME", cfg.Infrastructure.Postgres.ConnMaxLifetime); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Postgres.ConnMaxLifetime = value
	}
	if value, ok, err := durationValue(serviceName, "POSTGRES_CONN_MAX_IDLE_TIME", cfg.Infrastructure.Postgres.ConnMaxIdleTime); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Postgres.ConnMaxIdleTime = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "POSTGRES_MIGRATIONS_PATH", cfg.Infrastructure.Postgres.MigrationsPath); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Postgres.MigrationsPath = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "POSTGRES_SCHEMA", cfg.Infrastructure.Postgres.Schema); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Postgres.Schema = value
	}

	redisEnabledSet := false
	if value, ok, err := boolValue(serviceName, "REDIS_ENABLED", cfg.Infrastructure.Redis.Enabled); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Redis.Enabled = value
		redisEnabledSet = true
	}
	if value, ok, err := stringValueWithPresence(serviceName, "REDIS_ADDR", cfg.Infrastructure.Redis.Address); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Redis.Address = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "REDIS_USERNAME", cfg.Infrastructure.Redis.Username); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Redis.Username = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "REDIS_PASSWORD", cfg.Infrastructure.Redis.Password); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Redis.Password = value
	}
	if value, ok, err := intValue(serviceName, "REDIS_DB", cfg.Infrastructure.Redis.DB); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Redis.DB = value
	}
	if value, ok, err := intValue(serviceName, "REDIS_POOL_SIZE", cfg.Infrastructure.Redis.PoolSize); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Redis.PoolSize = value
	}
	if value, ok, err := durationValue(serviceName, "REDIS_DIAL_TIMEOUT", cfg.Infrastructure.Redis.DialTimeout); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Redis.DialTimeout = value
	}
	if value, ok, err := durationValue(serviceName, "REDIS_READ_TIMEOUT", cfg.Infrastructure.Redis.ReadTimeout); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Redis.ReadTimeout = value
	}
	if value, ok, err := durationValue(serviceName, "REDIS_WRITE_TIMEOUT", cfg.Infrastructure.Redis.WriteTimeout); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Redis.WriteTimeout = value
	}
	if value, ok, err := durationValue(serviceName, "REDIS_CONN_MAX_IDLE_TIME", cfg.Infrastructure.Redis.ConnMaxIdleTime); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Redis.ConnMaxIdleTime = value
	}

	objectStoreEnabledSet := false
	if value, ok, err := boolValue(serviceName, "OBJECT_STORAGE_ENABLED", cfg.Infrastructure.ObjectStore.Enabled); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.ObjectStore.Enabled = value
		objectStoreEnabledSet = true
	}
	if value, ok, err := stringValueWithPresence(serviceName, "OBJECT_STORAGE_ENDPOINT", cfg.Infrastructure.ObjectStore.Endpoint); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.ObjectStore.Endpoint = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "OBJECT_STORAGE_REGION", cfg.Infrastructure.ObjectStore.Region); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.ObjectStore.Region = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "OBJECT_STORAGE_BUCKET", cfg.Infrastructure.ObjectStore.Bucket); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.ObjectStore.Bucket = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "OBJECT_STORAGE_ACCESS_KEY_ID", cfg.Infrastructure.ObjectStore.AccessKeyID); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.ObjectStore.AccessKeyID = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "OBJECT_STORAGE_SECRET_ACCESS_KEY", cfg.Infrastructure.ObjectStore.SecretAccessKey); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.ObjectStore.SecretAccessKey = value
	}
	if value, ok, err := boolValue(serviceName, "OBJECT_STORAGE_USE_SSL", cfg.Infrastructure.ObjectStore.UseSSL); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.ObjectStore.UseSSL = value
	}
	if value, ok, err := boolValue(serviceName, "OBJECT_STORAGE_FORCE_PATH_STYLE", cfg.Infrastructure.ObjectStore.ForcePathStyle); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.ObjectStore.ForcePathStyle = value
	}

	telemetryTracingSet := false
	if value, ok, err := boolValue(serviceName, "TELEMETRY_METRICS_ENABLED", cfg.Infrastructure.Telemetry.MetricsEnabled); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Telemetry.MetricsEnabled = value
	}
	if value, ok, err := boolValue(serviceName, "TELEMETRY_TRACING_ENABLED", cfg.Infrastructure.Telemetry.TracingEnabled); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Telemetry.TracingEnabled = value
		telemetryTracingSet = true
	}
	if value, ok, err := stringValueWithPresence(serviceName, "TELEMETRY_OTLP_ADDRESS", cfg.Infrastructure.Telemetry.OTLPAddress); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Telemetry.OTLPAddress = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "TELEMETRY_SENTRY_DSN", cfg.Infrastructure.Telemetry.SentryDSN); err != nil {
		return err
	} else if ok {
		cfg.Infrastructure.Telemetry.SentryDSN = value
	}

	if value, ok, err := stringValueWithPresence(serviceName, "STORAGE_PRIMARY_PROVIDER", cfg.Storage.PrimaryProvider); err != nil {
		return err
	} else if ok {
		cfg.Storage.PrimaryProvider = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "STORAGE_CACHE_PROVIDER", cfg.Storage.CacheProvider); err != nil {
		return err
	} else if ok {
		cfg.Storage.CacheProvider = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "STORAGE_OBJECT_PROVIDER", cfg.Storage.ObjectProvider); err != nil {
		return err
	} else if ok {
		cfg.Storage.ObjectProvider = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "STORAGE_AUDIT_PROVIDER", cfg.Storage.AuditProvider); err != nil {
		return err
	} else if ok {
		cfg.Storage.AuditProvider = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "STORAGE_SEARCH_PROVIDER", cfg.Storage.SearchProvider); err != nil {
		return err
	} else if ok {
		cfg.Storage.SearchProvider = value
	}

	if value, ok, err := boolValue(serviceName, "FEATURE_FEDERATION_ENABLED", cfg.Features.FederationEnabled); err != nil {
		return err
	} else if ok {
		cfg.Features.FederationEnabled = value
	}
	if value, ok, err := boolValue(serviceName, "FEATURE_CALLS_ENABLED", cfg.Features.CallsEnabled); err != nil {
		return err
	} else if ok {
		cfg.Features.CallsEnabled = value
	}
	if value, ok, err := boolValue(serviceName, "FEATURE_SEARCH_ENABLED", cfg.Features.SearchEnabled); err != nil {
		return err
	} else if ok {
		cfg.Features.SearchEnabled = value
	}
	if value, ok, err := boolValue(serviceName, "FEATURE_SCHEDULED_MESSAGES_ENABLED", cfg.Features.ScheduledMessagesEnabled); err != nil {
		return err
	} else if ok {
		cfg.Features.ScheduledMessagesEnabled = value
	}
	if value, ok, err := boolValue(serviceName, "FEATURE_TRANSLATION_ENABLED", cfg.Features.TranslationEnabled); err != nil {
		return err
	} else if ok {
		cfg.Features.TranslationEnabled = value
	}

	if !postgresEnabledSet && cfg.Infrastructure.Postgres.DSN != "" {
		cfg.Infrastructure.Postgres.Enabled = true
	}
	if !redisEnabledSet && cfg.Infrastructure.Redis.Address != "" {
		cfg.Infrastructure.Redis.Enabled = true
	}
	if !objectStoreEnabledSet && (cfg.Infrastructure.ObjectStore.Endpoint != "" || cfg.Infrastructure.ObjectStore.Bucket != "") {
		cfg.Infrastructure.ObjectStore.Enabled = true
	}
	if !telemetryTracingSet && (cfg.Infrastructure.Telemetry.OTLPAddress != "" || cfg.Infrastructure.Telemetry.SentryDSN != "") {
		cfg.Infrastructure.Telemetry.TracingEnabled = cfg.Infrastructure.Telemetry.TracingEnabled || cfg.Infrastructure.Telemetry.OTLPAddress != ""
	}

	derivedLogging := defaultLogging(cfg.Service.Environment)
	if !logLevelSet {
		cfg.Logging.Level = derivedLogging.Level
	}
	if !logFormatSet {
		cfg.Logging.Format = derivedLogging.Format
	}
	if !logAddSourceSet {
		cfg.Logging.AddSource = derivedLogging.AddSource
	}
	if !grpcReflectionSet {
		cfg.Runtime.GRPC.ReflectionEnabled = isDevelopmentLikeEnvironment(cfg.Service.Environment)
	}

	cfg.normalize()

	return nil
}

func lookupEnv(serviceName string, key string) (string, bool) {
	if serviceName != "" {
		if value, ok := os.LookupEnv(serviceEnvKey(serviceName, key)); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), true
		}
	}

	value, ok := os.LookupEnv("ZVONILKA_" + key)
	if !ok {
		return "", false
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}

	return value, true
}

func serviceEnvKey(serviceName string, key string) string {
	serviceName = strings.ToUpper(strings.TrimSpace(serviceName))
	key = strings.ToUpper(strings.TrimSpace(key))
	if serviceName == "" {
		return "ZVONILKA_" + key
	}

	return "ZVONILKA_" + serviceName + "_" + key
}

func stringValue(serviceName string, key string, fallback string) string {
	if value, ok := lookupEnv(serviceName, key); ok {
		return value
	}

	return fallback
}

func stringValueWithPresence(serviceName string, key string, fallback string) (string, bool, error) {
	if value, ok := lookupEnv(serviceName, key); ok {
		return value, true, nil
	}

	return fallback, false, nil
}

func boolValue(serviceName string, key string, fallback bool) (bool, bool, error) {
	raw, ok := lookupEnv(serviceName, key)
	if !ok {
		return fallback, false, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, false, fmt.Errorf("parse %s: %w", key, err)
	}

	return value, true, nil
}

func intValue(serviceName string, key string, fallback int) (int, bool, error) {
	raw, ok := lookupEnv(serviceName, key)
	if !ok {
		return fallback, false, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false, fmt.Errorf("parse %s: %w", key, err)
	}

	return value, true, nil
}

func durationValue(serviceName string, key string, fallback time.Duration) (time.Duration, bool, error) {
	raw, ok := lookupEnv(serviceName, key)
	if !ok {
		return fallback, false, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, false, fmt.Errorf("parse %s: %w", key, err)
	}

	return value, true, nil
}
