package config

import (
	"errors"
	"fmt"
	"strings"
)

// Validate checks that the configuration is internally consistent.
func (c Configuration) Validate() error {
	var errs []error

	if c.Service.Name == "" {
		errs = append(errs, errors.New("service name is required"))
	}
	if c.Service.Environment == "" {
		errs = append(errs, errors.New("service environment is required"))
	}
	if c.Runtime.HTTP.Address == "" && c.Runtime.GRPC.Address == "" {
		errs = append(errs, errors.New("at least one listen address is required"))
	}
	if c.Runtime.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("shutdown timeout must be positive"))
	}
	if err := validateLogLevel(c.Logging.Level); err != nil {
		errs = append(errs, err)
	}
	if err := validateLogFormat(c.Logging.Format); err != nil {
		errs = append(errs, err)
	}
	if c.Identity.JoinRequestTTL <= 0 {
		errs = append(errs, errors.New("identity join request TTL must be positive"))
	}
	if c.Identity.ChallengeTTL <= 0 {
		errs = append(errs, errors.New("identity challenge TTL must be positive"))
	}
	if c.Identity.AccessTokenTTL <= 0 {
		errs = append(errs, errors.New("identity access token TTL must be positive"))
	}
	if c.Identity.RefreshTokenTTL <= 0 {
		errs = append(errs, errors.New("identity refresh token TTL must be positive"))
	}
	if c.Identity.LoginCodeLength <= 0 {
		errs = append(errs, errors.New("identity login code length must be positive"))
	}
	if c.Runtime.HTTP.ReadHeaderTimeout <= 0 {
		errs = append(errs, errors.New("HTTP read header timeout must be positive"))
	}
	if c.Runtime.HTTP.ReadTimeout <= 0 {
		errs = append(errs, errors.New("HTTP read timeout must be positive"))
	}
	if c.Runtime.HTTP.WriteTimeout <= 0 {
		errs = append(errs, errors.New("HTTP write timeout must be positive"))
	}
	if c.Runtime.HTTP.IdleTimeout <= 0 {
		errs = append(errs, errors.New("HTTP idle timeout must be positive"))
	}
	if c.Runtime.HTTP.MaxHeaderBytes <= 0 {
		errs = append(errs, errors.New("HTTP max header bytes must be positive"))
	}

	if c.Infrastructure.Postgres.Enabled {
		if c.Infrastructure.Postgres.DSN == "" {
			errs = append(errs, errors.New("postgres DSN is required when postgres is enabled"))
		}
		if c.Infrastructure.Postgres.MaxOpenConns <= 0 {
			errs = append(errs, errors.New("postgres max open connections must be positive"))
		}
		if c.Infrastructure.Postgres.MaxIdleConns < 0 {
			errs = append(errs, errors.New("postgres max idle connections cannot be negative"))
		}
		if c.Infrastructure.Postgres.ConnMaxLifetime <= 0 {
			errs = append(errs, errors.New("postgres connection max lifetime must be positive"))
		}
		if c.Infrastructure.Postgres.ConnMaxIdleTime <= 0 {
			errs = append(errs, errors.New("postgres connection max idle time must be positive"))
		}
	}

	if c.Infrastructure.Redis.Enabled {
		if c.Infrastructure.Redis.Address == "" {
			errs = append(errs, errors.New("redis address is required when redis is enabled"))
		}
		if c.Infrastructure.Redis.PoolSize <= 0 {
			errs = append(errs, errors.New("redis pool size must be positive"))
		}
		if c.Infrastructure.Redis.DialTimeout <= 0 {
			errs = append(errs, errors.New("redis dial timeout must be positive"))
		}
		if c.Infrastructure.Redis.ReadTimeout <= 0 {
			errs = append(errs, errors.New("redis read timeout must be positive"))
		}
		if c.Infrastructure.Redis.WriteTimeout <= 0 {
			errs = append(errs, errors.New("redis write timeout must be positive"))
		}
		if c.Infrastructure.Redis.ConnMaxIdleTime <= 0 {
			errs = append(errs, errors.New("redis connection max idle time must be positive"))
		}
	}

	if c.Infrastructure.ObjectStore.Enabled {
		if c.Infrastructure.ObjectStore.Endpoint == "" {
			errs = append(errs, errors.New("object storage endpoint is required when object storage is enabled"))
		}
		if c.Infrastructure.ObjectStore.Region == "" {
			errs = append(errs, errors.New("object storage region is required when object storage is enabled"))
		}
		if c.Infrastructure.ObjectStore.Bucket == "" {
			errs = append(errs, errors.New("object storage bucket is required when object storage is enabled"))
		}
	}

	if c.Infrastructure.Telemetry.TracingEnabled && c.Infrastructure.Telemetry.OTLPAddress == "" {
		errs = append(errs, errors.New("OTLP address is required when tracing is enabled"))
	}
	if c.Storage.PrimaryProvider == "" {
		errs = append(errs, errors.New("storage primary provider is required"))
	}
	if c.Storage.CacheProvider == "" {
		errs = append(errs, errors.New("storage cache provider is required"))
	}
	if c.Storage.ObjectProvider == "" {
		errs = append(errs, errors.New("storage object provider is required"))
	}
	if c.Storage.AuditProvider == "" {
		errs = append(errs, errors.New("storage audit provider is required"))
	}
	if c.Storage.SearchProvider == "" {
		errs = append(errs, errors.New("storage search provider is required"))
	}

	if len(errs) == 0 {
		return nil
	}

	return errors.Join(errs...)
}

func validateLogLevel(level string) error {
	switch strings.ToLower(level) {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("invalid logging level %q", level)
	}
}

func validateLogFormat(format string) error {
	switch strings.ToLower(format) {
	case "text", "json":
		return nil
	default:
		return fmt.Errorf("invalid logging format %q", format)
	}
}
