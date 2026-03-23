package config

import (
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

type listenDefaults struct {
	http string
	grpc string
}

var serviceListenDefaults = map[string]listenDefaults{
	"controlplane": {
		http: ":8080",
		grpc: ":9090",
	},
	"gateway": {
		http: ":8081",
		grpc: ":9091",
	},
	"botapi": {
		http: ":8082",
		grpc: ":9092",
	},
}

const (
	defaultEnvironment          = "production"
	defaultShutdownTimeout      = 10 * time.Second
	defaultReadHeaderTimeout    = 5 * time.Second
	defaultReadTimeout          = 10 * time.Second
	defaultWriteTimeout         = 10 * time.Second
	defaultIdleTimeout          = 60 * time.Second
	defaultMaxHeaderBytes       = 1 << 20
	defaultPostgresMaxOpen      = 10
	defaultPostgresMaxIdle      = 5
	defaultPostgresConnLifetime = 30 * time.Minute
	defaultPostgresConnIdle     = 5 * time.Minute
	defaultRedisPoolSize        = 10
	defaultRedisDialTimeout     = 5 * time.Second
	defaultRedisReadTimeout     = 3 * time.Second
	defaultRedisWriteTimeout    = 3 * time.Second
	defaultRedisConnIdle        = 5 * time.Minute
)

func defaultConfiguration(serviceName string) Configuration {
	environment := defaultEnvironment
	runtimeDefaults := defaultRuntime(serviceName, environment)
	identityDefaults := identity.DefaultSettings()

	cfg := Configuration{
		Service: ServiceConfig{
			Name:        serviceName,
			Environment: environment,
		},
		Logging: defaultLogging(environment),
		Runtime: RuntimeConfig{
			ShutdownTimeout: defaultShutdownTimeout,
			HTTP:            runtimeDefaults.HTTP,
			GRPC:            runtimeDefaults.GRPC,
		},
		Identity: IdentityConfig{
			JoinRequestTTL:  identityDefaults.JoinRequestTTL,
			ChallengeTTL:    identityDefaults.ChallengeTTL,
			AccessTokenTTL:  identityDefaults.AccessTokenTTL,
			RefreshTokenTTL: identityDefaults.RefreshTokenTTL,
			LoginCodeLength: identityDefaults.LoginCodeLength,
		},
		Infrastructure: InfrastructureConfig{
			Postgres: PostgresConfig{
				MaxOpenConns:    defaultPostgresMaxOpen,
				MaxIdleConns:    defaultPostgresMaxIdle,
				ConnMaxLifetime: defaultPostgresConnLifetime,
				ConnMaxIdleTime: defaultPostgresConnIdle,
				MigrationsPath:  "deploy/migrations/postgres",
			},
			Redis: RedisConfig{
				PoolSize:        defaultRedisPoolSize,
				DialTimeout:     defaultRedisDialTimeout,
				ReadTimeout:     defaultRedisReadTimeout,
				WriteTimeout:    defaultRedisWriteTimeout,
				ConnMaxIdleTime: defaultRedisConnIdle,
			},
			ObjectStore: ObjectStorageConfig{
				UseSSL:         true,
				ForcePathStyle: false,
			},
		},
		Storage: StorageConfig{
			PrimaryProvider: "primary",
			CacheProvider:   "cache",
			ObjectProvider:  "object",
			AuditProvider:   "audit",
			SearchProvider:  "search",
		},
	}

	return cfg
}

func defaultRuntime(serviceName string, environment string) RuntimeConfig {
	defaults := defaultsForService(serviceName)
	reflectionEnabled := false
	if isDevelopmentLikeEnvironment(environment) {
		reflectionEnabled = true
	}

	return RuntimeConfig{
		HTTP: HTTPConfig{
			Address:           defaults.http,
			ReadHeaderTimeout: defaultReadHeaderTimeout,
			ReadTimeout:       defaultReadTimeout,
			WriteTimeout:      defaultWriteTimeout,
			IdleTimeout:       defaultIdleTimeout,
			MaxHeaderBytes:    defaultMaxHeaderBytes,
		},
		GRPC: GRPCConfig{
			Address:           defaults.grpc,
			ReflectionEnabled: reflectionEnabled,
		},
	}
}

func defaultLogging(environment string) LoggingConfig {
	environment = strings.ToLower(strings.TrimSpace(environment))
	level := "info"
	format := "json"
	addSource := false

	if isDevelopmentLikeEnvironment(environment) {
		level = "debug"
		format = "text"
		addSource = true
	}

	return LoggingConfig{
		Level:     level,
		Format:    format,
		AddSource: addSource,
	}
}

func isDevelopmentLikeEnvironment(environment string) bool {
	environment = strings.ToLower(strings.TrimSpace(environment))
	switch environment {
	case "development", "dev", "local", "test":
		return true
	default:
		return false
	}
}

func defaultsForService(serviceName string) listenDefaults {
	defaults, ok := serviceListenDefaults[serviceName]
	if ok {
		return defaults
	}

	return listenDefaults{
		http: ":8080",
		grpc: ":9090",
	}
}
