package config

import (
	"errors"
	"fmt"
	"strings"
)

// Validate checks that the configuration is internally consistent.
func (c Configuration) Validate() error {
	c.normalize()

	var errs []error

	if err := validateServiceName(c.Service.Name); err != nil {
		errs = append(errs, err)
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
	if c.Call.InviteTimeout <= 0 {
		errs = append(errs, errors.New("call invite timeout must be positive"))
	}
	if c.Call.RingingTimeout <= 0 {
		errs = append(errs, errors.New("call ringing timeout must be positive"))
	}
	if c.Call.ReconnectGrace <= 0 {
		errs = append(errs, errors.New("call reconnect grace must be positive"))
	}
	if c.Call.MaxDuration <= 0 {
		errs = append(errs, errors.New("call max duration must be positive"))
	}
	if c.Call.MaxGroupParticipants == 0 {
		errs = append(errs, errors.New("call max group participants must be positive"))
	}
	if c.Call.MaxVideoParticipants == 0 {
		errs = append(errs, errors.New("call max video participants must be positive"))
	}
	if c.Call.MaxVideoParticipants > c.Call.MaxGroupParticipants {
		errs = append(errs, errors.New("call max video participants must be less than or equal to max group participants"))
	}
	if c.Call.WorkerPollInterval <= 0 {
		errs = append(errs, errors.New("call worker poll interval must be positive"))
	}
	if c.Call.WorkerBatchSize <= 0 {
		errs = append(errs, errors.New("call worker batch size must be positive"))
	}
	if c.Call.HookTimeout <= 0 {
		errs = append(errs, errors.New("call hook timeout must be positive"))
	}
	if c.Service.Name == "callworker" && strings.TrimSpace(c.Call.RecordingHookURL) == "" && strings.TrimSpace(c.Call.TranscriptionHookURL) == "" {
		errs = append(errs, errors.New("call recording or transcription hook url is required"))
	}
	if c.RTC.CredentialTTL <= 0 {
		errs = append(errs, errors.New("rtc credential ttl must be positive"))
	}
	if strings.TrimSpace(c.RTC.NodeID) == "" {
		errs = append(errs, errors.New("rtc node id is required"))
	}
	if c.RTC.UDPPortMin <= 0 {
		errs = append(errs, errors.New("rtc udp port min must be positive"))
	}
	if c.RTC.UDPPortMax <= 0 {
		errs = append(errs, errors.New("rtc udp port max must be positive"))
	}
	if c.RTC.UDPPortMax < c.RTC.UDPPortMin {
		errs = append(errs, errors.New("rtc udp port max must be greater than or equal to min"))
	}
	if len(c.RTC.TURNURLs) > 0 && c.RTC.TURNSecret == "" {
		errs = append(errs, errors.New("rtc turn secret is required when turn urls are configured"))
	}
	if err := validateRTCNodes(c.RTC); err != nil {
		errs = append(errs, err)
	}
	if c.Bot.FanoutPollInterval <= 0 {
		errs = append(errs, errors.New("bot fanout poll interval must be positive"))
	}
	if c.Bot.FanoutBatchSize <= 0 {
		errs = append(errs, errors.New("bot fanout batch size must be positive"))
	}
	if c.Bot.GetUpdatesMaxLimit <= 0 {
		errs = append(errs, errors.New("bot getUpdates max limit must be positive"))
	}
	if c.Bot.LongPollMaxTimeout <= 0 {
		errs = append(errs, errors.New("bot long-poll max timeout must be positive"))
	}
	if c.Bot.LongPollStep <= 0 {
		errs = append(errs, errors.New("bot long-poll step must be positive"))
	}
	if c.Bot.WebhookTimeout <= 0 {
		errs = append(errs, errors.New("bot webhook timeout must be positive"))
	}
	if c.Bot.WebhookBatchSize <= 0 {
		errs = append(errs, errors.New("bot webhook batch size must be positive"))
	}
	if c.Bot.RetryInitialBackoff <= 0 {
		errs = append(errs, errors.New("bot retry initial backoff must be positive"))
	}
	if c.Bot.RetryMaxBackoff <= 0 {
		errs = append(errs, errors.New("bot retry max backoff must be positive"))
	}
	if c.Bot.RetryMaxBackoff < c.Bot.RetryInitialBackoff {
		errs = append(errs, errors.New("bot retry max backoff must be greater than or equal to the initial backoff"))
	}
	if c.Bot.MaxAttempts <= 0 {
		errs = append(errs, errors.New("bot max attempts must be positive"))
	}
	if c.Presence.OnlineWindow <= 0 {
		errs = append(errs, errors.New("presence online window must be positive"))
	}
	if c.Media.UploadURLTTL <= 0 {
		errs = append(errs, errors.New("media upload URL TTL must be positive"))
	}
	if c.Media.DownloadURLTTL <= 0 {
		errs = append(errs, errors.New("media download URL TTL must be positive"))
	}
	if c.Media.MaxUploadSize <= 0 {
		errs = append(errs, errors.New("media max upload size must be positive"))
	}
	if c.Presence.OnlineWindow <= 0 {
		errs = append(errs, errors.New("presence online window must be positive"))
	}
	if c.Notification.WorkerPollInterval <= 0 {
		errs = append(errs, errors.New("notification worker poll interval must be positive"))
	}
	if c.Notification.RetryInitialBackoff <= 0 {
		errs = append(errs, errors.New("notification retry initial backoff must be positive"))
	}
	if c.Notification.RetryMaxBackoff <= 0 {
		errs = append(errs, errors.New("notification retry max backoff must be positive"))
	}
	if c.Notification.RetryMaxBackoff < c.Notification.RetryInitialBackoff {
		errs = append(errs, errors.New("notification retry max backoff must be greater than or equal to the initial backoff"))
	}
	if c.Notification.MaxAttempts <= 0 {
		errs = append(errs, errors.New("notification max attempts must be positive"))
	}
	if c.Notification.BatchSize <= 0 {
		errs = append(errs, errors.New("notification batch size must be positive"))
	}
	if c.Search.DefaultLimit <= 0 {
		errs = append(errs, errors.New("search default limit must be positive"))
	}
	if c.Search.MaxLimit <= 0 {
		errs = append(errs, errors.New("search max limit must be positive"))
	}
	if c.Search.MaxLimit < c.Search.DefaultLimit {
		errs = append(errs, errors.New("search max limit must be greater than or equal to the default limit"))
	}
	if c.Search.MinQueryLength <= 0 {
		errs = append(errs, errors.New("search minimum query length must be positive"))
	}
	if c.Search.SnippetLength <= 0 {
		errs = append(errs, errors.New("search snippet length must be positive"))
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
		if c.Infrastructure.ObjectStore.AccessKeyID == "" {
			errs = append(errs, errors.New("object storage access key ID is required when object storage is enabled"))
		}
		if c.Infrastructure.ObjectStore.SecretAccessKey == "" {
			errs = append(errs, errors.New("object storage secret access key is required when object storage is enabled"))
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
	if err := validateDistinctStorageProviders(c.Storage); err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		return nil
	}

	return errors.Join(errs...)
}

func validateRTCNodes(cfg RTCConfig) error {
	if len(cfg.Nodes) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(cfg.Nodes))
	for _, node := range cfg.Nodes {
		id := strings.TrimSpace(node.ID)
		endpoint := strings.TrimSpace(node.Endpoint)
		if id == "" || endpoint == "" {
			return errors.New("rtc cluster nodes must define id and endpoint")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("rtc cluster nodes must be unique: %s", id)
		}
		seen[id] = struct{}{}
	}

	if _, ok := seen[strings.TrimSpace(cfg.NodeID)]; !ok {
		return fmt.Errorf("rtc node id %q must be present in rtc cluster nodes", strings.TrimSpace(cfg.NodeID))
	}

	return nil
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

func validateServiceName(serviceName string) error {
	serviceName = strings.ToLower(strings.TrimSpace(serviceName))
	switch serviceName {
	case "controlplane", "gateway", "botapi", "notificationworker", "callworker", "callhooks":
		return nil
	case "":
		return errors.New("service name is required")
	default:
		return fmt.Errorf("unsupported service name %q", serviceName)
	}
}

func validateDistinctStorageProviders(storage StorageConfig) error {
	type binding struct {
		name  string
		value string
	}

	bindings := []binding{
		{name: "primary", value: storage.PrimaryProvider},
		{name: "cache", value: storage.CacheProvider},
		{name: "object", value: storage.ObjectProvider},
		{name: "audit", value: storage.AuditProvider},
		{name: "search", value: storage.SearchProvider},
	}

	seen := make(map[string]string, len(bindings))
	var errs []error

	for _, binding := range bindings {
		binding.value = strings.ToLower(strings.TrimSpace(binding.value))
		if binding.value == "" {
			continue
		}

		if previous, ok := seen[binding.value]; ok {
			errs = append(
				errs,
				fmt.Errorf(
					"storage provider bindings must be distinct: %s and %s both use %q",
					previous,
					binding.name,
					binding.value,
				),
			)
			continue
		}

		seen[binding.value] = binding.name
	}

	if len(errs) == 0 {
		return nil
	}

	return errors.Join(errs...)
}
