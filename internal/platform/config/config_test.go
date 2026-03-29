package config

import (
	"strings"
	"testing"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	domainnotification "github.com/dm-vev/zvonilka/internal/domain/notification"
	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
)

func TestFromEnvUsesDistinctServiceDefaults(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("ZVONILKA_CALL_RECORDING_HOOK_URL", "http://127.0.0.1/recording")

	type expected struct {
		http string
		grpc string
	}

	cases := []struct {
		service string
		want    expected
	}{
		{
			service: "controlplane",
			want: expected{
				http: ":8080",
				grpc: ":9090",
			},
		},
		{
			service: "gateway",
			want: expected{
				http: ":8081",
				grpc: ":9091",
			},
		},
		{
			service: "botapi",
			want: expected{
				http: ":8082",
				grpc: ":9092",
			},
		},
		{
			service: "notificationworker",
			want: expected{
				http: ":8083",
				grpc: ":9093",
			},
		},
		{
			service: "callworker",
			want: expected{
				http: ":8084",
				grpc: ":9094",
			},
		},
		{
			service: "callhooks",
			want: expected{
				http: ":8085",
				grpc: ":9095",
			},
		},
	}

	httpOwners := make(map[string]string, len(cases))
	grpcOwners := make(map[string]string, len(cases))

	for _, tc := range cases {
		cfg, err := FromEnv(tc.service)
		if err != nil {
			t.Fatalf("from env for %s: %v", tc.service, err)
		}

		if cfg.Runtime.HTTP.Address != tc.want.http {
			t.Fatalf("http addr for %s: got %s, want %s", tc.service, cfg.Runtime.HTTP.Address, tc.want.http)
		}
		if cfg.Runtime.GRPC.Address != tc.want.grpc {
			t.Fatalf("grpc addr for %s: got %s, want %s", tc.service, cfg.Runtime.GRPC.Address, tc.want.grpc)
		}

		if owner, ok := httpOwners[cfg.Runtime.HTTP.Address]; ok {
			t.Fatalf("http addr %s reused by %s and %s", cfg.Runtime.HTTP.Address, owner, tc.service)
		}
		httpOwners[cfg.Runtime.HTTP.Address] = tc.service

		if owner, ok := grpcOwners[cfg.Runtime.GRPC.Address]; ok {
			t.Fatalf("grpc addr %s reused by %s and %s", cfg.Runtime.GRPC.Address, owner, tc.service)
		}
		grpcOwners[cfg.Runtime.GRPC.Address] = tc.service
	}
}

func TestFromEnvExplicitOverridesWin(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_HTTP_ADDR", ":18080")
	t.Setenv("ZVONILKA_GRPC_ADDR", ":19090")
	t.Setenv("ZVONILKA_GATEWAY_HTTP_ADDR", ":28080")
	t.Setenv("ZVONILKA_GATEWAY_GRPC_ADDR", ":29090")

	gatewayCfg, err := FromEnv("gateway")
	if err != nil {
		t.Fatalf("from env for gateway: %v", err)
	}
	if gatewayCfg.Runtime.HTTP.Address != ":28080" {
		t.Fatalf("gateway http addr: got %s, want :28080", gatewayCfg.Runtime.HTTP.Address)
	}
	if gatewayCfg.Runtime.GRPC.Address != ":29090" {
		t.Fatalf("gateway grpc addr: got %s, want :29090", gatewayCfg.Runtime.GRPC.Address)
	}

	botAPICfg, err := FromEnv("botapi")
	if err != nil {
		t.Fatalf("from env for botapi: %v", err)
	}
	if botAPICfg.Runtime.HTTP.Address != ":18080" {
		t.Fatalf("botapi http addr: got %s, want :18080", botAPICfg.Runtime.HTTP.Address)
	}
	if botAPICfg.Runtime.GRPC.Address != ":19090" {
		t.Fatalf("botapi grpc addr: got %s, want :19090", botAPICfg.Runtime.GRPC.Address)
	}
}

func TestLoadRejectsExplicitZeroIdentityValue(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_IDENTITY_LOGIN_CODE_LENGTH", "0")

	_, err := Load("controlplane")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "identity login code length must be positive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsUnsupportedServiceName(t *testing.T) {
	resetConfigEnv(t)

	_, err := Load("worker")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "unsupported service name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadNotificationDefaultsMatchDomainSettings(t *testing.T) {
	resetConfigEnv(t)

	cfg, err := Load("notificationworker")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got, want := cfg.Notification.ToSettings(), domainnotification.DefaultSettings(); got != want {
		t.Fatalf("notification settings mismatch: got %+v, want %+v", got, want)
	}
}

func TestLoadAppliesNotificationOverrides(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_NOTIFICATION_WORKER_POLL_INTERVAL", "750ms")
	t.Setenv("ZVONILKA_NOTIFICATION_RETRY_INITIAL_BACKOFF", "2s")
	t.Setenv("ZVONILKA_NOTIFICATION_RETRY_MAX_BACKOFF", "30s")
	t.Setenv("ZVONILKA_NOTIFICATION_MAX_ATTEMPTS", "9")
	t.Setenv("ZVONILKA_NOTIFICATION_BATCH_SIZE", "42")

	cfg, err := Load("notificationworker")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Notification.WorkerPollInterval != 750*time.Millisecond {
		t.Fatalf("worker poll interval: got %s, want 750ms", cfg.Notification.WorkerPollInterval)
	}
	if cfg.Notification.RetryInitialBackoff != 2*time.Second {
		t.Fatalf("retry initial backoff: got %s, want 2s", cfg.Notification.RetryInitialBackoff)
	}
	if cfg.Notification.RetryMaxBackoff != 30*time.Second {
		t.Fatalf("retry max backoff: got %s, want 30s", cfg.Notification.RetryMaxBackoff)
	}
	if cfg.Notification.MaxAttempts != 9 {
		t.Fatalf("max attempts: got %d, want 9", cfg.Notification.MaxAttempts)
	}
	if cfg.Notification.BatchSize != 42 {
		t.Fatalf("batch size: got %d, want 42", cfg.Notification.BatchSize)
	}
}

func TestLoadRejectsInvalidNotificationRetryWindow(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_NOTIFICATION_RETRY_INITIAL_BACKOFF", "10s")
	t.Setenv("ZVONILKA_NOTIFICATION_RETRY_MAX_BACKOFF", "5s")

	_, err := Load("notificationworker")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "notification retry max backoff must be greater than or equal to the initial backoff") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadIdentityDefaultsMatchDomainSettings(t *testing.T) {
	resetConfigEnv(t)

	cfg, err := Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got, want := cfg.Identity.ToSettings(), domainidentity.DefaultSettings(); got != want {
		t.Fatalf("identity settings mismatch: got %+v, want %+v", got, want)
	}
}

func TestLoadCallDefaultsMatchDomainSettings(t *testing.T) {
	resetConfigEnv(t)

	cfg, err := Load("gateway")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got, want := cfg.Call.ToSettings(), domaincall.DefaultSettings(); got != want {
		t.Fatalf("call settings mismatch: got %+v, want %+v", got, want)
	}
}

func TestLoadAppliesCallReconnectGraceOverride(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_CALL_RECONNECT_GRACE", "12s")

	cfg, err := Load("gateway")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Call.ReconnectGrace != 12*time.Second {
		t.Fatalf("call reconnect grace: got %s, want 12s", cfg.Call.ReconnectGrace)
	}
}

func TestLoadSearchDefaultsMatchDomainSettings(t *testing.T) {
	resetConfigEnv(t)

	cfg, err := Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got, want := cfg.Search.ToSettings(), domainsearch.DefaultSettings(); got != want {
		t.Fatalf("search settings mismatch: got %+v, want %+v", got, want)
	}
}

func TestLoadAppliesSearchOverrides(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_SEARCH_DEFAULT_LIMIT", "11")
	t.Setenv("ZVONILKA_SEARCH_MAX_LIMIT", "44")
	t.Setenv("ZVONILKA_SEARCH_MIN_QUERY_LENGTH", "3")
	t.Setenv("ZVONILKA_SEARCH_SNIPPET_LENGTH", "240")

	cfg, err := Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Search.DefaultLimit != 11 {
		t.Fatalf("search default limit: got %d, want 11", cfg.Search.DefaultLimit)
	}
	if cfg.Search.MaxLimit != 44 {
		t.Fatalf("search max limit: got %d, want 44", cfg.Search.MaxLimit)
	}
	if cfg.Search.MinQueryLength != 3 {
		t.Fatalf("search min query length: got %d, want 3", cfg.Search.MinQueryLength)
	}
	if cfg.Search.SnippetLength != 240 {
		t.Fatalf("search snippet length: got %d, want 240", cfg.Search.SnippetLength)
	}
}

func TestLoadRejectsInvalidSearchLimitWindow(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_SEARCH_DEFAULT_LIMIT", "50")
	t.Setenv("ZVONILKA_SEARCH_MAX_LIMIT", "10")

	_, err := Load("controlplane")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "search max limit must be greater than or equal to the default limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadPresenceDefaultsMatchDomainSettings(t *testing.T) {
	resetConfigEnv(t)

	cfg, err := Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got, want := cfg.Presence.ToSettings(), domainpresence.DefaultSettings(); got != want {
		t.Fatalf("presence settings mismatch: got %+v, want %+v", got, want)
	}
}

func TestLoadUsesProductionDefaultsForRuntimeAndLogging(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_ENV", "production")

	cfg, err := Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Logging.Level != "info" {
		t.Fatalf("logging level: got %s, want info", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Fatalf("logging format: got %s, want json", cfg.Logging.Format)
	}
	if cfg.Logging.AddSource {
		t.Fatal("expected add source to be disabled in production")
	}
	if cfg.Runtime.GRPC.ReflectionEnabled {
		t.Fatal("expected grpc reflection to be disabled in production")
	}
}

func TestLoadUsesProductionDefaultsWhenEnvironmentUnset(t *testing.T) {
	resetConfigEnv(t)

	cfg, err := Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Service.Environment != "production" {
		t.Fatalf("service environment: got %s, want production", cfg.Service.Environment)
	}
	if cfg.Logging.Level != "info" {
		t.Fatalf("logging level: got %s, want info", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Fatalf("logging format: got %s, want json", cfg.Logging.Format)
	}
	if cfg.Logging.AddSource {
		t.Fatal("expected add source to be disabled by default")
	}
	if cfg.Runtime.GRPC.ReflectionEnabled {
		t.Fatal("expected grpc reflection to be disabled by default")
	}
}

func TestLoadUsesStorageProviderDefaults(t *testing.T) {
	resetConfigEnv(t)

	cfg, err := Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Storage.PrimaryProvider != "primary" {
		t.Fatalf("storage primary provider: got %s, want primary", cfg.Storage.PrimaryProvider)
	}
	if cfg.Storage.CacheProvider != "cache" {
		t.Fatalf("storage cache provider: got %s, want cache", cfg.Storage.CacheProvider)
	}
	if cfg.Storage.ObjectProvider != "object" {
		t.Fatalf("storage object provider: got %s, want object", cfg.Storage.ObjectProvider)
	}
	if cfg.Storage.AuditProvider != "audit" {
		t.Fatalf("storage audit provider: got %s, want audit", cfg.Storage.AuditProvider)
	}
	if cfg.Storage.SearchProvider != "search" {
		t.Fatalf("storage search provider: got %s, want search", cfg.Storage.SearchProvider)
	}
}

func TestLoadAppliesPresenceOnlineWindowOverride(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_PRESENCE_ONLINE_WINDOW", "12m")

	cfg, err := Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Presence.OnlineWindow != 12*time.Minute {
		t.Fatalf("presence online window: got %s, want 12m", cfg.Presence.OnlineWindow)
	}
}

func TestLoadRejectsInvalidPresenceOnlineWindow(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_PRESENCE_ONLINE_WINDOW", "0s")

	_, err := Load("controlplane")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "presence online window must be positive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadAllowsIndependentStorageInfraFlagsForGateway(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_POSTGRES_ENABLED", "true")
	t.Setenv("ZVONILKA_POSTGRES_DSN", "postgres://localhost/app")
	t.Setenv("ZVONILKA_OBJECT_STORAGE_ENABLED", "false")

	cfg, err := Load("gateway")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !cfg.Infrastructure.Postgres.Enabled {
		t.Fatal("expected postgres to stay enabled")
	}
	if cfg.Infrastructure.ObjectStore.Enabled {
		t.Fatal("expected object storage to stay disabled")
	}
}

func TestLoadHonorsExplicitDisableForDerivedInfrastructureFlags(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_POSTGRES_ENABLED", "false")
	t.Setenv("ZVONILKA_POSTGRES_DSN", "postgres://localhost/app")
	t.Setenv("ZVONILKA_REDIS_ENABLED", "false")
	t.Setenv("ZVONILKA_REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("ZVONILKA_OBJECT_STORAGE_ENABLED", "false")
	t.Setenv("ZVONILKA_OBJECT_STORAGE_ENDPOINT", "http://minio:9000")
	t.Setenv("ZVONILKA_OBJECT_STORAGE_REGION", "eu-west-1")
	t.Setenv("ZVONILKA_OBJECT_STORAGE_BUCKET", "zvonilka")
	t.Setenv("ZVONILKA_OBJECT_STORAGE_ACCESS_KEY_ID", "test-access")
	t.Setenv("ZVONILKA_OBJECT_STORAGE_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("ZVONILKA_TELEMETRY_TRACING_ENABLED", "false")
	t.Setenv("ZVONILKA_TELEMETRY_OTLP_ADDRESS", "otel:4317")

	cfg, err := Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Infrastructure.Postgres.Enabled {
		t.Fatal("expected postgres to stay disabled when explicitly set to false")
	}
	if cfg.Infrastructure.Redis.Enabled {
		t.Fatal("expected redis to stay disabled when explicitly set to false")
	}
	if cfg.Infrastructure.ObjectStore.Enabled {
		t.Fatal("expected object storage to stay disabled when explicitly set to false")
	}
	if cfg.Infrastructure.Telemetry.TracingEnabled {
		t.Fatal("expected telemetry tracing to stay disabled when explicitly set to false")
	}
}

func TestLoadAutoEnablesDerivedInfrastructureFlagsWhenUnset(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_POSTGRES_DSN", "postgres://localhost/app")
	t.Setenv("ZVONILKA_REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("ZVONILKA_OBJECT_STORAGE_ENDPOINT", "http://minio:9000")
	t.Setenv("ZVONILKA_OBJECT_STORAGE_REGION", "eu-west-1")
	t.Setenv("ZVONILKA_OBJECT_STORAGE_BUCKET", "zvonilka")
	t.Setenv("ZVONILKA_OBJECT_STORAGE_ACCESS_KEY_ID", "test-access")
	t.Setenv("ZVONILKA_OBJECT_STORAGE_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("ZVONILKA_TELEMETRY_OTLP_ADDRESS", "otel:4317")

	cfg, err := Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !cfg.Infrastructure.Postgres.Enabled {
		t.Fatal("expected postgres to auto-enable from DSN")
	}
	if !cfg.Infrastructure.Redis.Enabled {
		t.Fatal("expected redis to auto-enable from address")
	}
	if !cfg.Infrastructure.ObjectStore.Enabled {
		t.Fatal("expected object storage to auto-enable from endpoint")
	}
	if !cfg.Infrastructure.Telemetry.TracingEnabled {
		t.Fatal("expected telemetry tracing to auto-enable from otlp address")
	}
}

func TestLoadNormalizesStorageProviderNames(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_STORAGE_PRIMARY_PROVIDER", " Primary ")
	t.Setenv("ZVONILKA_STORAGE_CACHE_PROVIDER", "CACHE")
	t.Setenv("ZVONILKA_STORAGE_OBJECT_PROVIDER", "Object")
	t.Setenv("ZVONILKA_STORAGE_AUDIT_PROVIDER", " Audit ")
	t.Setenv("ZVONILKA_STORAGE_SEARCH_PROVIDER", "SEARCH")

	cfg, err := Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Storage.PrimaryProvider != "primary" {
		t.Fatalf("storage primary provider: got %s, want primary", cfg.Storage.PrimaryProvider)
	}
	if cfg.Storage.CacheProvider != "cache" {
		t.Fatalf("storage cache provider: got %s, want cache", cfg.Storage.CacheProvider)
	}
	if cfg.Storage.ObjectProvider != "object" {
		t.Fatalf("storage object provider: got %s, want object", cfg.Storage.ObjectProvider)
	}
	if cfg.Storage.AuditProvider != "audit" {
		t.Fatalf("storage audit provider: got %s, want audit", cfg.Storage.AuditProvider)
	}
	if cfg.Storage.SearchProvider != "search" {
		t.Fatalf("storage search provider: got %s, want search", cfg.Storage.SearchProvider)
	}
}

func TestLoadRejectsDuplicateStorageProviderBindings(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_STORAGE_PRIMARY_PROVIDER", "shared")
	t.Setenv("ZVONILKA_STORAGE_CACHE_PROVIDER", "SHARED")

	_, err := Load("controlplane")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "storage provider bindings must be distinct") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadUsesServiceSpecificStorageProviderOverrides(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_STORAGE_PRIMARY_PROVIDER", "primary")
	t.Setenv("ZVONILKA_STORAGE_CACHE_PROVIDER", "cache")
	t.Setenv("ZVONILKA_STORAGE_OBJECT_PROVIDER", "object")
	t.Setenv("ZVONILKA_STORAGE_AUDIT_PROVIDER", "audit")
	t.Setenv("ZVONILKA_STORAGE_SEARCH_PROVIDER", "search")
	t.Setenv("ZVONILKA_GATEWAY_STORAGE_PRIMARY_PROVIDER", "gateway-primary")
	t.Setenv("ZVONILKA_GATEWAY_STORAGE_CACHE_PROVIDER", "gateway-cache")
	t.Setenv("ZVONILKA_GATEWAY_STORAGE_OBJECT_PROVIDER", "gateway-object")
	t.Setenv("ZVONILKA_GATEWAY_STORAGE_AUDIT_PROVIDER", "gateway-audit")
	t.Setenv("ZVONILKA_GATEWAY_STORAGE_SEARCH_PROVIDER", "gateway-search")

	cfg, err := Load("gateway")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Storage.PrimaryProvider != "gateway-primary" {
		t.Fatalf("storage primary provider: got %s, want gateway-primary", cfg.Storage.PrimaryProvider)
	}
	if cfg.Storage.CacheProvider != "gateway-cache" {
		t.Fatalf("storage cache provider: got %s, want gateway-cache", cfg.Storage.CacheProvider)
	}
	if cfg.Storage.ObjectProvider != "gateway-object" {
		t.Fatalf("storage object provider: got %s, want gateway-object", cfg.Storage.ObjectProvider)
	}
	if cfg.Storage.AuditProvider != "gateway-audit" {
		t.Fatalf("storage audit provider: got %s, want gateway-audit", cfg.Storage.AuditProvider)
	}
	if cfg.Storage.SearchProvider != "gateway-search" {
		t.Fatalf("storage search provider: got %s, want gateway-search", cfg.Storage.SearchProvider)
	}
}

func TestLoadNormalizesServiceAndEnvironmentCase(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_ENV", "DEV")

	cfg, err := Load("GATEWAY")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Service.Name != "gateway" {
		t.Fatalf("service name: got %s, want gateway", cfg.Service.Name)
	}
	if cfg.Runtime.HTTP.Address != ":8081" {
		t.Fatalf("gateway http addr: got %s, want :8081", cfg.Runtime.HTTP.Address)
	}
	if cfg.Runtime.GRPC.Address != ":9091" {
		t.Fatalf("gateway grpc addr: got %s, want :9091", cfg.Runtime.GRPC.Address)
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("logging level: got %s, want debug", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Fatalf("logging format: got %s, want text", cfg.Logging.Format)
	}
	if !cfg.Runtime.GRPC.ReflectionEnabled {
		t.Fatal("expected grpc reflection to stay enabled in development profile")
	}
}

func TestLoadAcceptsPresenceOnlineWindowOverride(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_PRESENCE_ONLINE_WINDOW", "9m")

	cfg, err := Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Presence.OnlineWindow.String() != "9m0s" {
		t.Fatalf("presence online window: got %s, want 9m0s", cfg.Presence.OnlineWindow)
	}
}

func TestValidateNormalizesStorageProviderBindingsBeforeDistinctnessCheck(t *testing.T) {
	cfg := defaultConfiguration("controlplane")
	cfg.Storage.PrimaryProvider = " Primary "
	cfg.Storage.CacheProvider = "PRIMARY"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validate to fail")
	}
	if !strings.Contains(err.Error(), "storage provider bindings must be distinct") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsExplicitZeroPresenceWindow(t *testing.T) {
	cfg := defaultConfiguration("controlplane")
	cfg.Presence.OnlineWindow = 0

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validate to fail")
	}
	if !strings.Contains(err.Error(), "presence online window must be positive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func resetConfigEnv(t *testing.T) {
	t.Helper()

	keys := []string{
		"ZVONILKA_ENV",
		"ZVONILKA_HTTP_ADDR",
		"ZVONILKA_GRPC_ADDR",
		"ZVONILKA_SHUTDOWN_TIMEOUT",
		"ZVONILKA_CONTROLPLANE_ENV",
		"ZVONILKA_CONTROLPLANE_HTTP_ADDR",
		"ZVONILKA_CONTROLPLANE_GRPC_ADDR",
		"ZVONILKA_CONTROLPLANE_SHUTDOWN_TIMEOUT",
		"ZVONILKA_GATEWAY_ENV",
		"ZVONILKA_GATEWAY_HTTP_ADDR",
		"ZVONILKA_GATEWAY_GRPC_ADDR",
		"ZVONILKA_GATEWAY_SHUTDOWN_TIMEOUT",
		"ZVONILKA_BOTAPI_ENV",
		"ZVONILKA_BOTAPI_HTTP_ADDR",
		"ZVONILKA_BOTAPI_GRPC_ADDR",
		"ZVONILKA_BOTAPI_SHUTDOWN_TIMEOUT",
		"ZVONILKA_NOTIFICATIONWORKER_ENV",
		"ZVONILKA_NOTIFICATIONWORKER_HTTP_ADDR",
		"ZVONILKA_NOTIFICATIONWORKER_GRPC_ADDR",
		"ZVONILKA_NOTIFICATIONWORKER_SHUTDOWN_TIMEOUT",
		"ZVONILKA_NOTIFICATION_WORKER_POLL_INTERVAL",
		"ZVONILKA_NOTIFICATION_RETRY_INITIAL_BACKOFF",
		"ZVONILKA_NOTIFICATION_RETRY_MAX_BACKOFF",
		"ZVONILKA_NOTIFICATION_MAX_ATTEMPTS",
		"ZVONILKA_NOTIFICATION_BATCH_SIZE",
		"ZVONILKA_NOTIFICATIONWORKER_NOTIFICATION_WORKER_POLL_INTERVAL",
		"ZVONILKA_NOTIFICATIONWORKER_NOTIFICATION_RETRY_INITIAL_BACKOFF",
		"ZVONILKA_NOTIFICATIONWORKER_NOTIFICATION_RETRY_MAX_BACKOFF",
		"ZVONILKA_NOTIFICATIONWORKER_NOTIFICATION_MAX_ATTEMPTS",
		"ZVONILKA_NOTIFICATIONWORKER_NOTIFICATION_BATCH_SIZE",
		"ZVONILKA_CONTROLPLANE_STORAGE_PRIMARY_PROVIDER",
		"ZVONILKA_CONTROLPLANE_STORAGE_CACHE_PROVIDER",
		"ZVONILKA_CONTROLPLANE_STORAGE_OBJECT_PROVIDER",
		"ZVONILKA_CONTROLPLANE_STORAGE_AUDIT_PROVIDER",
		"ZVONILKA_CONTROLPLANE_STORAGE_SEARCH_PROVIDER",
		"ZVONILKA_GATEWAY_STORAGE_PRIMARY_PROVIDER",
		"ZVONILKA_GATEWAY_STORAGE_CACHE_PROVIDER",
		"ZVONILKA_GATEWAY_STORAGE_OBJECT_PROVIDER",
		"ZVONILKA_GATEWAY_STORAGE_AUDIT_PROVIDER",
		"ZVONILKA_GATEWAY_STORAGE_SEARCH_PROVIDER",
		"ZVONILKA_BOTAPI_STORAGE_PRIMARY_PROVIDER",
		"ZVONILKA_BOTAPI_STORAGE_CACHE_PROVIDER",
		"ZVONILKA_BOTAPI_STORAGE_OBJECT_PROVIDER",
		"ZVONILKA_BOTAPI_STORAGE_AUDIT_PROVIDER",
		"ZVONILKA_BOTAPI_STORAGE_SEARCH_PROVIDER",
		"ZVONILKA_NOTIFICATIONWORKER_STORAGE_PRIMARY_PROVIDER",
		"ZVONILKA_NOTIFICATIONWORKER_STORAGE_CACHE_PROVIDER",
		"ZVONILKA_NOTIFICATIONWORKER_STORAGE_OBJECT_PROVIDER",
		"ZVONILKA_NOTIFICATIONWORKER_STORAGE_AUDIT_PROVIDER",
		"ZVONILKA_NOTIFICATIONWORKER_STORAGE_SEARCH_PROVIDER",
		"ZVONILKA_STORAGE_PRIMARY_PROVIDER",
		"ZVONILKA_STORAGE_CACHE_PROVIDER",
		"ZVONILKA_STORAGE_OBJECT_PROVIDER",
		"ZVONILKA_STORAGE_AUDIT_PROVIDER",
		"ZVONILKA_STORAGE_SEARCH_PROVIDER",
		"ZVONILKA_PRESENCE_ONLINE_WINDOW",
		"ZVONILKA_CONTROLPLANE_PRESENCE_ONLINE_WINDOW",
		"ZVONILKA_GATEWAY_PRESENCE_ONLINE_WINDOW",
		"ZVONILKA_BOTAPI_PRESENCE_ONLINE_WINDOW",
	}

	for _, key := range keys {
		t.Setenv(key, "")
	}
}
