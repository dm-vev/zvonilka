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
	if value, ok, err := durationValue(serviceName, "CALL_INVITE_TIMEOUT", cfg.Call.InviteTimeout); err != nil {
		return err
	} else if ok {
		cfg.Call.InviteTimeout = value
	}
	if value, ok, err := durationValue(serviceName, "CALL_RINGING_TIMEOUT", cfg.Call.RingingTimeout); err != nil {
		return err
	} else if ok {
		cfg.Call.RingingTimeout = value
	}
	if value, ok, err := durationValue(serviceName, "CALL_RECONNECT_GRACE", cfg.Call.ReconnectGrace); err != nil {
		return err
	} else if ok {
		cfg.Call.ReconnectGrace = value
	}
	if value, ok, err := durationValue(serviceName, "CALL_MAX_DURATION", cfg.Call.MaxDuration); err != nil {
		return err
	} else if ok {
		cfg.Call.MaxDuration = value
	}
	if value, ok, err := intValue(serviceName, "CALL_MAX_GROUP_PARTICIPANTS", int(cfg.Call.MaxGroupParticipants)); err != nil {
		return err
	} else if ok {
		cfg.Call.MaxGroupParticipants = uint32(value)
	}
	if value, ok, err := intValue(serviceName, "CALL_MAX_VIDEO_PARTICIPANTS", int(cfg.Call.MaxVideoParticipants)); err != nil {
		return err
	} else if ok {
		cfg.Call.MaxVideoParticipants = uint32(value)
	}
	if value, ok, err := durationValue(serviceName, "CALL_WORKER_POLL_INTERVAL", cfg.Call.WorkerPollInterval); err != nil {
		return err
	} else if ok {
		cfg.Call.WorkerPollInterval = value
	}
	if value, ok, err := intValue(serviceName, "CALL_WORKER_BATCH_SIZE", cfg.Call.WorkerBatchSize); err != nil {
		return err
	} else if ok {
		cfg.Call.WorkerBatchSize = value
	}
	if value, ok, err := durationValue(serviceName, "CALL_REHOME_POLL_INTERVAL", cfg.Call.RehomePollInterval); err != nil {
		return err
	} else if ok {
		cfg.Call.RehomePollInterval = value
	}
	if value, ok, err := intValue(serviceName, "CALL_REHOME_BATCH_SIZE", cfg.Call.RehomeBatchSize); err != nil {
		return err
	} else if ok {
		cfg.Call.RehomeBatchSize = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "CALL_RECORDING_HOOK_URL", cfg.Call.RecordingHookURL); err != nil {
		return err
	} else if ok {
		cfg.Call.RecordingHookURL = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "CALL_TRANSCRIPTION_HOOK_URL", cfg.Call.TranscriptionHookURL); err != nil {
		return err
	} else if ok {
		cfg.Call.TranscriptionHookURL = value
	}
	if value, ok, err := durationValue(serviceName, "CALL_HOOK_TIMEOUT", cfg.Call.HookTimeout); err != nil {
		return err
	} else if ok {
		cfg.Call.HookTimeout = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "CALL_HOOK_SECRET", cfg.Call.HookSecret); err != nil {
		return err
	} else if ok {
		cfg.Call.HookSecret = value
	}
	if value, ok, err := intValue(serviceName, "CALL_HOOK_MAX_BODY_BYTES", int(cfg.Call.HookMaxBodyBytes)); err != nil {
		return err
	} else if ok {
		cfg.Call.HookMaxBodyBytes = int64(value)
	}
	if value, ok, err := durationValue(serviceName, "CALL_HOOK_LEASE_TTL", cfg.Call.HookLeaseTTL); err != nil {
		return err
	} else if ok {
		cfg.Call.HookLeaseTTL = value
	}
	if value, ok, err := durationValue(serviceName, "CALL_HOOK_RETRY_INITIAL_BACKOFF", cfg.Call.HookRetryInitialBackoff); err != nil {
		return err
	} else if ok {
		cfg.Call.HookRetryInitialBackoff = value
	}
	if value, ok, err := durationValue(serviceName, "CALL_HOOK_RETRY_MAX_BACKOFF", cfg.Call.HookRetryMaxBackoff); err != nil {
		return err
	} else if ok {
		cfg.Call.HookRetryMaxBackoff = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "RTC_PUBLIC_ENDPOINT", cfg.RTC.PublicEndpoint); err != nil {
		return err
	} else if ok {
		cfg.RTC.PublicEndpoint = value
	}
	if value, ok, err := durationValue(serviceName, "RTC_CREDENTIAL_TTL", cfg.RTC.CredentialTTL); err != nil {
		return err
	} else if ok {
		cfg.RTC.CredentialTTL = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "RTC_NODE_ID", cfg.RTC.NodeID); err != nil {
		return err
	} else if ok {
		cfg.RTC.NodeID = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "RTC_CANDIDATE_HOST", cfg.RTC.CandidateHost); err != nil {
		return err
	} else if ok {
		cfg.RTC.CandidateHost = value
	}
	if value, ok, err := intValue(serviceName, "RTC_UDP_PORT_MIN", cfg.RTC.UDPPortMin); err != nil {
		return err
	} else if ok {
		cfg.RTC.UDPPortMin = value
	}
	if value, ok, err := intValue(serviceName, "RTC_UDP_PORT_MAX", cfg.RTC.UDPPortMax); err != nil {
		return err
	} else if ok {
		cfg.RTC.UDPPortMax = value
	}
	if value, ok, err := durationValue(serviceName, "RTC_HEALTH_TTL", cfg.RTC.HealthTTL); err != nil {
		return err
	} else if ok {
		cfg.RTC.HealthTTL = value
	}
	if value, ok, err := durationValue(serviceName, "RTC_HEALTH_TIMEOUT", cfg.RTC.HealthTimeout); err != nil {
		return err
	} else if ok {
		cfg.RTC.HealthTimeout = value
	}
	if value, ok, err := stringSliceValue(serviceName, "RTC_STUN_URLS", cfg.RTC.STUNURLs); err != nil {
		return err
	} else if ok {
		cfg.RTC.STUNURLs = value
	}
	if value, ok, err := stringSliceValue(serviceName, "RTC_TURN_URLS", cfg.RTC.TURNURLs); err != nil {
		return err
	} else if ok {
		cfg.RTC.TURNURLs = value
	}
	if value, ok, err := rtcNodeSliceValue(serviceName, "RTC_CLUSTER_NODES", cfg.RTC.Nodes); err != nil {
		return err
	} else if ok {
		cfg.RTC.Nodes = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "RTC_TURN_SECRET", cfg.RTC.TURNSecret); err != nil {
		return err
	} else if ok {
		cfg.RTC.TURNSecret = value
	}
	if value, ok, err := durationValue(serviceName, "BOT_FANOUT_POLL_INTERVAL", cfg.Bot.FanoutPollInterval); err != nil {
		return err
	} else if ok {
		cfg.Bot.FanoutPollInterval = value
	}
	if value, ok, err := intValue(serviceName, "BOT_FANOUT_BATCH_SIZE", cfg.Bot.FanoutBatchSize); err != nil {
		return err
	} else if ok {
		cfg.Bot.FanoutBatchSize = value
	}
	if value, ok, err := intValue(serviceName, "BOT_GET_UPDATES_MAX_LIMIT", cfg.Bot.GetUpdatesMaxLimit); err != nil {
		return err
	} else if ok {
		cfg.Bot.GetUpdatesMaxLimit = value
	}
	if value, ok, err := durationValue(serviceName, "BOT_LONG_POLL_MAX_TIMEOUT", cfg.Bot.LongPollMaxTimeout); err != nil {
		return err
	} else if ok {
		cfg.Bot.LongPollMaxTimeout = value
	}
	if value, ok, err := durationValue(serviceName, "BOT_LONG_POLL_STEP", cfg.Bot.LongPollStep); err != nil {
		return err
	} else if ok {
		cfg.Bot.LongPollStep = value
	}
	if value, ok, err := durationValue(serviceName, "BOT_WEBHOOK_TIMEOUT", cfg.Bot.WebhookTimeout); err != nil {
		return err
	} else if ok {
		cfg.Bot.WebhookTimeout = value
	}
	if value, ok, err := intValue(serviceName, "BOT_WEBHOOK_BATCH_SIZE", cfg.Bot.WebhookBatchSize); err != nil {
		return err
	} else if ok {
		cfg.Bot.WebhookBatchSize = value
	}
	if value, ok, err := durationValue(serviceName, "BOT_RETRY_INITIAL_BACKOFF", cfg.Bot.RetryInitialBackoff); err != nil {
		return err
	} else if ok {
		cfg.Bot.RetryInitialBackoff = value
	}
	if value, ok, err := durationValue(serviceName, "BOT_RETRY_MAX_BACKOFF", cfg.Bot.RetryMaxBackoff); err != nil {
		return err
	} else if ok {
		cfg.Bot.RetryMaxBackoff = value
	}
	if value, ok, err := intValue(serviceName, "BOT_MAX_ATTEMPTS", cfg.Bot.MaxAttempts); err != nil {
		return err
	} else if ok {
		cfg.Bot.MaxAttempts = value
	}
	if value, ok, err := durationValue(serviceName, "PRESENCE_ONLINE_WINDOW", cfg.Presence.OnlineWindow); err != nil {
		return err
	} else if ok {
		cfg.Presence.OnlineWindow = value
	}
	if value, ok, err := durationValue(serviceName, "MEDIA_UPLOAD_URL_TTL", cfg.Media.UploadURLTTL); err != nil {
		return err
	} else if ok {
		cfg.Media.UploadURLTTL = value
	}
	if value, ok, err := durationValue(serviceName, "MEDIA_DOWNLOAD_URL_TTL", cfg.Media.DownloadURLTTL); err != nil {
		return err
	} else if ok {
		cfg.Media.DownloadURLTTL = value
	}
	if value, ok, err := int64Value(serviceName, "MEDIA_MAX_UPLOAD_SIZE", cfg.Media.MaxUploadSize); err != nil {
		return err
	} else if ok {
		cfg.Media.MaxUploadSize = value
	}
	if value, ok, err := durationValue(serviceName, "PRESENCE_ONLINE_WINDOW", cfg.Presence.OnlineWindow); err != nil {
		return err
	} else if ok {
		cfg.Presence.OnlineWindow = value
	}
	if value, ok, err := durationValue(serviceName, "NOTIFICATION_WORKER_POLL_INTERVAL", cfg.Notification.WorkerPollInterval); err != nil {
		return err
	} else if ok {
		cfg.Notification.WorkerPollInterval = value
	}
	if value, ok, err := durationValue(serviceName, "NOTIFICATION_RETRY_INITIAL_BACKOFF", cfg.Notification.RetryInitialBackoff); err != nil {
		return err
	} else if ok {
		cfg.Notification.RetryInitialBackoff = value
	}
	if value, ok, err := durationValue(serviceName, "NOTIFICATION_RETRY_MAX_BACKOFF", cfg.Notification.RetryMaxBackoff); err != nil {
		return err
	} else if ok {
		cfg.Notification.RetryMaxBackoff = value
	}
	if value, ok, err := durationValue(serviceName, "NOTIFICATION_DELIVERY_LEASE_TTL", cfg.Notification.DeliveryLeaseTTL); err != nil {
		return err
	} else if ok {
		cfg.Notification.DeliveryLeaseTTL = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "NOTIFICATION_DELIVERY_WEBHOOK_URL", cfg.Notification.DeliveryWebhookURL); err != nil {
		return err
	} else if ok {
		cfg.Notification.DeliveryWebhookURL = value
	}
	if value, ok, err := durationValue(serviceName, "NOTIFICATION_DELIVERY_WEBHOOK_TIMEOUT", cfg.Notification.DeliveryWebhookTimeout); err != nil {
		return err
	} else if ok {
		cfg.Notification.DeliveryWebhookTimeout = value
	}
	if value, ok, err := intValue(serviceName, "NOTIFICATION_MAX_ATTEMPTS", cfg.Notification.MaxAttempts); err != nil {
		return err
	} else if ok {
		cfg.Notification.MaxAttempts = value
	}
	if value, ok, err := intValue(serviceName, "NOTIFICATION_BATCH_SIZE", cfg.Notification.BatchSize); err != nil {
		return err
	} else if ok {
		cfg.Notification.BatchSize = value
	}
	if value, ok, err := intValue(serviceName, "SEARCH_DEFAULT_LIMIT", cfg.Search.DefaultLimit); err != nil {
		return err
	} else if ok {
		cfg.Search.DefaultLimit = value
	}
	if value, ok, err := intValue(serviceName, "SEARCH_MAX_LIMIT", cfg.Search.MaxLimit); err != nil {
		return err
	} else if ok {
		cfg.Search.MaxLimit = value
	}
	if value, ok, err := intValue(serviceName, "SEARCH_MIN_QUERY_LENGTH", cfg.Search.MinQueryLength); err != nil {
		return err
	} else if ok {
		cfg.Search.MinQueryLength = value
	}
	if value, ok, err := intValue(serviceName, "SEARCH_SNIPPET_LENGTH", cfg.Search.SnippetLength); err != nil {
		return err
	} else if ok {
		cfg.Search.SnippetLength = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "TRANSLATION_ENDPOINT_URL", cfg.Translation.EndpointURL); err != nil {
		return err
	} else if ok {
		cfg.Translation.EndpointURL = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "TRANSLATION_API_KEY", cfg.Translation.APIKey); err != nil {
		return err
	} else if ok {
		cfg.Translation.APIKey = value
	}
	if value, ok, err := durationValue(serviceName, "TRANSLATION_TIMEOUT", cfg.Translation.Timeout); err != nil {
		return err
	} else if ok {
		cfg.Translation.Timeout = value
	}
	if value, ok, err := intValue(serviceName, "TRANSLATION_MAX_TEXT_BYTES", cfg.Translation.MaxTextBytes); err != nil {
		return err
	} else if ok {
		cfg.Translation.MaxTextBytes = value
	}
	if value, ok, err := stringValueWithPresence(serviceName, "TRANSLATION_PROVIDER_NAME", cfg.Translation.ProviderName); err != nil {
		return err
	} else if ok {
		cfg.Translation.ProviderName = value
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

func int64Value(serviceName string, key string, fallback int64) (int64, bool, error) {
	raw, ok := lookupEnv(serviceName, key)
	if !ok {
		return fallback, false, nil
	}

	value, err := strconv.ParseInt(raw, 10, 64)
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

func stringSliceValue(serviceName string, key string, fallback []string) ([]string, bool, error) {
	raw, ok, err := stringValueWithPresence(serviceName, key, "")
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return append([]string(nil), fallback...), false, nil
	}
	if strings.TrimSpace(raw) == "" {
		return nil, true, nil
	}

	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, part)
	}

	return values, true, nil
}

func rtcNodeSliceValue(serviceName string, key string, fallback []RTCNodeConfig) ([]RTCNodeConfig, bool, error) {
	raw, ok, err := stringValueWithPresence(serviceName, key, "")
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return append([]RTCNodeConfig(nil), fallback...), false, nil
	}

	items := strings.Split(raw, ",")
	result := make([]RTCNodeConfig, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		id, endpoint, found := strings.Cut(item, "=")
		if !found {
			return nil, false, fmt.Errorf("%s must use id=endpoint entries", serviceEnvKey(serviceName, key))
		}
		id = strings.TrimSpace(id)
		endpoint = strings.TrimSpace(endpoint)
		publicEndpoint := endpoint
		controlEndpoint := ""
		if strings.Contains(endpoint, "|") {
			publicEndpoint, controlEndpoint, _ = strings.Cut(endpoint, "|")
			publicEndpoint = strings.TrimSpace(publicEndpoint)
			controlEndpoint = strings.TrimSpace(controlEndpoint)
		}
		if id == "" || publicEndpoint == "" {
			return nil, false, fmt.Errorf("%s must use id=endpoint entries", serviceEnvKey(serviceName, key))
		}
		result = append(result, RTCNodeConfig{
			ID:              id,
			Endpoint:        publicEndpoint,
			ControlEndpoint: controlEndpoint,
		})
	}

	return result, true, nil
}
