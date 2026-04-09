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
	t.Setenv("ZVONILKA_FEATURE_FEDERATION_ENABLED", "true")
	t.Setenv("ZVONILKA_FEDERATION_LOCAL_SERVER_NAME", "alpha.example")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_SHARED_SECRET", "bridge-secret")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_ENDPOINT", "grpc://127.0.0.1:9097")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_PEER_SERVER_NAME", "mesh.example")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_LINK_NAME", "mesh")
	t.Setenv("ZVONILKA_MESHTASTIC_INTERFACE_KIND", "serial")
	t.Setenv("ZVONILKA_MESHTASTIC_DEVICE", "/dev/ttyUSB0")
	t.Setenv("ZVONILKA_MESHCORE_INTERFACE_KIND", "serial")
	t.Setenv("ZVONILKA_MESHCORE_DEVICE", "/dev/ttyUSB1")
	t.Setenv("ZVONILKA_MESHCORE_DESTINATION", "peer-pubkey")

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
		{
			service: "federationworker",
			want: expected{
				http: ":8086",
				grpc: ":9096",
			},
		},
		{
			service: "federationbridge",
			want: expected{
				http: ":8087",
				grpc: ":9097",
			},
		},
		{
			service: "federationmeshtastic",
			want: expected{
				http: ":8088",
				grpc: ":9098",
			},
		},
		{
			service: "federationmeshcore",
			want: expected{
				http: ":8089",
				grpc: ":9099",
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
	t.Setenv("ZVONILKA_NOTIFICATION_DELIVERY_LEASE_TTL", "45s")
	t.Setenv("ZVONILKA_NOTIFICATION_DELIVERY_WEBHOOK_URL", "https://notify.example.com/deliveries")
	t.Setenv("ZVONILKA_NOTIFICATION_DELIVERY_WEBHOOK_TIMEOUT", "12s")
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
	if cfg.Notification.DeliveryLeaseTTL != 45*time.Second {
		t.Fatalf("delivery lease ttl: got %s, want 45s", cfg.Notification.DeliveryLeaseTTL)
	}
	if cfg.Notification.DeliveryWebhookURL != "https://notify.example.com/deliveries" {
		t.Fatalf("delivery webhook url: got %s", cfg.Notification.DeliveryWebhookURL)
	}
	if cfg.Notification.DeliveryWebhookTimeout != 12*time.Second {
		t.Fatalf("delivery webhook timeout: got %s, want 12s", cfg.Notification.DeliveryWebhookTimeout)
	}
	if cfg.Notification.MaxAttempts != 9 {
		t.Fatalf("max attempts: got %d, want 9", cfg.Notification.MaxAttempts)
	}
	if cfg.Notification.BatchSize != 42 {
		t.Fatalf("batch size: got %d, want 42", cfg.Notification.BatchSize)
	}
}

func TestLoadAppliesFederationOverrides(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_FEATURE_FEDERATION_ENABLED", "true")
	t.Setenv("ZVONILKA_FEDERATION_LOCAL_SERVER_NAME", "alpha.example")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_SHARED_SECRET", "bridge-secret")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_ENDPOINT", "grpc://127.0.0.1:9097")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_PEER_SERVER_NAME", "mesh.example")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_LINK_NAME", "mesh")
	t.Setenv("ZVONILKA_FEDERATION_WORKER_POLL_INTERVAL", "1500ms")
	t.Setenv("ZVONILKA_FEDERATION_WORKER_BATCH_SIZE", "25")
	t.Setenv("ZVONILKA_FEDERATION_DIAL_TIMEOUT", "9s")

	cfg, err := Load("federationworker")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Federation.LocalServerName != "alpha.example" {
		t.Fatalf("local server name: got %s", cfg.Federation.LocalServerName)
	}
	if cfg.Federation.BridgeSharedSecret != "bridge-secret" {
		t.Fatalf("bridge shared secret: got %s", cfg.Federation.BridgeSharedSecret)
	}
	if cfg.Federation.BridgeEndpoint != "grpc://127.0.0.1:9097" {
		t.Fatalf("bridge endpoint: got %s", cfg.Federation.BridgeEndpoint)
	}
	if cfg.Federation.BridgePeerServer != "mesh.example" {
		t.Fatalf("bridge peer server: got %s", cfg.Federation.BridgePeerServer)
	}
	if cfg.Federation.BridgeLinkName != "mesh" {
		t.Fatalf("bridge link name: got %s", cfg.Federation.BridgeLinkName)
	}
	if cfg.Federation.WorkerPollInterval != 1500*time.Millisecond {
		t.Fatalf("worker poll interval: got %s, want 1500ms", cfg.Federation.WorkerPollInterval)
	}
	if cfg.Federation.WorkerBatchSize != 25 {
		t.Fatalf("worker batch size: got %d, want 25", cfg.Federation.WorkerBatchSize)
	}
	if cfg.Federation.DialTimeout != 9*time.Second {
		t.Fatalf("dial timeout: got %s, want 9s", cfg.Federation.DialTimeout)
	}
}

func TestLoadRejectsFederationBridgeWithoutSharedSecret(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_FEATURE_FEDERATION_ENABLED", "true")

	_, err := Load("federationbridge")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "federation bridge shared secret is required for federationbridge") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadAppliesMeshtasticOverrides(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_FEATURE_FEDERATION_ENABLED", "true")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_SHARED_SECRET", "bridge-secret")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_ENDPOINT", "grpc://127.0.0.1:9097")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_POLL_INTERVAL", "2s")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_BATCH_SIZE", "12")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_PEER_SERVER_NAME", "mesh.example")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_LINK_NAME", "mesh")
	t.Setenv("ZVONILKA_MESHTASTIC_INTERFACE_KIND", "serial")
	t.Setenv("ZVONILKA_MESHTASTIC_DEVICE", "/dev/ttyUSB0")
	t.Setenv("ZVONILKA_MESHTASTIC_HELPER_PYTHON", "/usr/bin/python3")
	t.Setenv("ZVONILKA_MESHTASTIC_HELPER_SCRIPT_PATH", "/tmp/meshtastic_bridge.py")
	t.Setenv("ZVONILKA_MESHTASTIC_RECEIVE_TIMEOUT", "4s")
	t.Setenv("ZVONILKA_MESHTASTIC_TEXT_PREFIX", "mesh1:")

	cfg, err := Load("federationmeshtastic")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Federation.BridgePollInterval != 2*time.Second {
		t.Fatalf("bridge poll interval: got %s, want 2s", cfg.Federation.BridgePollInterval)
	}
	if cfg.Federation.BridgeBatchSize != 12 {
		t.Fatalf("bridge batch size: got %d, want 12", cfg.Federation.BridgeBatchSize)
	}
	if cfg.Meshtastic.InterfaceKind != "serial" {
		t.Fatalf("meshtastic interface kind: got %s", cfg.Meshtastic.InterfaceKind)
	}
	if cfg.Meshtastic.Device != "/dev/ttyUSB0" {
		t.Fatalf("meshtastic device: got %s", cfg.Meshtastic.Device)
	}
	if cfg.Meshtastic.HelperPython != "/usr/bin/python3" {
		t.Fatalf("meshtastic helper python: got %s", cfg.Meshtastic.HelperPython)
	}
	if cfg.Meshtastic.HelperScriptPath != "/tmp/meshtastic_bridge.py" {
		t.Fatalf("meshtastic helper script path: got %s", cfg.Meshtastic.HelperScriptPath)
	}
	if cfg.Meshtastic.ReceiveTimeout != 4*time.Second {
		t.Fatalf("meshtastic receive timeout: got %s, want 4s", cfg.Meshtastic.ReceiveTimeout)
	}
	if cfg.Meshtastic.TextPrefix != "mesh1:" {
		t.Fatalf("meshtastic text prefix: got %s", cfg.Meshtastic.TextPrefix)
	}
}

func TestLoadRejectsMeshtasticBridgeWithoutDevice(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_FEATURE_FEDERATION_ENABLED", "true")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_SHARED_SECRET", "bridge-secret")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_ENDPOINT", "grpc://127.0.0.1:9097")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_PEER_SERVER_NAME", "mesh.example")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_LINK_NAME", "mesh")
	t.Setenv("ZVONILKA_MESHTASTIC_INTERFACE_KIND", "serial")

	_, err := Load("federationmeshtastic")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "meshtastic device is required for federationmeshtastic") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadAppliesMeshCoreOverrides(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_FEATURE_FEDERATION_ENABLED", "true")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_SHARED_SECRET", "bridge-secret")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_ENDPOINT", "grpc://127.0.0.1:9097")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_POLL_INTERVAL", "2s")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_BATCH_SIZE", "12")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_PEER_SERVER_NAME", "mesh.example")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_LINK_NAME", "meshcore")
	t.Setenv("ZVONILKA_MESHCORE_INTERFACE_KIND", "serial")
	t.Setenv("ZVONILKA_MESHCORE_DEVICE", "/dev/ttyUSB1")
	t.Setenv("ZVONILKA_MESHCORE_HELPER_PYTHON", "/usr/bin/python3")
	t.Setenv("ZVONILKA_MESHCORE_HELPER_SCRIPT_PATH", "/tmp/meshcore_bridge.py")
	t.Setenv("ZVONILKA_MESHCORE_RECEIVE_TIMEOUT", "4s")
	t.Setenv("ZVONILKA_MESHCORE_TEXT_PREFIX", "mesh1:")
	t.Setenv("ZVONILKA_MESHCORE_DESTINATION", "peer-pubkey")

	cfg, err := Load("federationmeshcore")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.MeshCore.InterfaceKind != "serial" {
		t.Fatalf("meshcore interface kind: got %s", cfg.MeshCore.InterfaceKind)
	}
	if cfg.MeshCore.Device != "/dev/ttyUSB1" {
		t.Fatalf("meshcore device: got %s", cfg.MeshCore.Device)
	}
	if cfg.MeshCore.HelperPython != "/usr/bin/python3" {
		t.Fatalf("meshcore helper python: got %s", cfg.MeshCore.HelperPython)
	}
	if cfg.MeshCore.HelperScriptPath != "/tmp/meshcore_bridge.py" {
		t.Fatalf("meshcore helper script path: got %s", cfg.MeshCore.HelperScriptPath)
	}
	if cfg.MeshCore.ReceiveTimeout != 4*time.Second {
		t.Fatalf("meshcore receive timeout: got %s, want 4s", cfg.MeshCore.ReceiveTimeout)
	}
	if cfg.MeshCore.TextPrefix != "mesh1:" {
		t.Fatalf("meshcore text prefix: got %s", cfg.MeshCore.TextPrefix)
	}
	if cfg.MeshCore.Destination != "peer-pubkey" {
		t.Fatalf("meshcore destination: got %s", cfg.MeshCore.Destination)
	}
}

func TestLoadRejectsMeshCoreBridgeWithoutDestination(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_FEATURE_FEDERATION_ENABLED", "true")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_SHARED_SECRET", "bridge-secret")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_ENDPOINT", "grpc://127.0.0.1:9097")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_PEER_SERVER_NAME", "mesh.example")
	t.Setenv("ZVONILKA_FEDERATION_BRIDGE_LINK_NAME", "meshcore")
	t.Setenv("ZVONILKA_MESHCORE_INTERFACE_KIND", "serial")
	t.Setenv("ZVONILKA_MESHCORE_DEVICE", "/dev/ttyUSB1")

	_, err := Load("federationmeshcore")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "meshcore destination is required for federationmeshcore") {
		t.Fatalf("unexpected error: %v", err)
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

func TestLoadRejectsInvalidNotificationWebhookURL(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_NOTIFICATION_DELIVERY_WEBHOOK_URL", "://bad")

	_, err := Load("notificationworker")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "notification delivery webhook url must be absolute") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadAppliesCallHookOverrides(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_CALL_HOOK_SECRET", "shared-secret")
	t.Setenv("ZVONILKA_CALL_HOOK_MAX_BODY_BYTES", "2048")
	t.Setenv("ZVONILKA_CALL_HOOK_LEASE_TTL", "45s")
	t.Setenv("ZVONILKA_CALL_HOOK_RETRY_INITIAL_BACKOFF", "3s")
	t.Setenv("ZVONILKA_CALL_HOOK_RETRY_MAX_BACKOFF", "90s")

	cfg, err := Load("callhooks")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Call.HookSecret != "shared-secret" {
		t.Fatalf("hook secret: got %s", cfg.Call.HookSecret)
	}
	if cfg.Call.HookMaxBodyBytes != 2048 {
		t.Fatalf("hook max body bytes: got %d, want 2048", cfg.Call.HookMaxBodyBytes)
	}
	if cfg.Call.HookLeaseTTL != 45*time.Second {
		t.Fatalf("hook lease ttl: got %s, want 45s", cfg.Call.HookLeaseTTL)
	}
	if cfg.Call.HookRetryInitialBackoff != 3*time.Second {
		t.Fatalf("hook retry initial backoff: got %s, want 3s", cfg.Call.HookRetryInitialBackoff)
	}
	if cfg.Call.HookRetryMaxBackoff != 90*time.Second {
		t.Fatalf("hook retry max backoff: got %s, want 90s", cfg.Call.HookRetryMaxBackoff)
	}
}

func TestLoadRejectsInvalidCallHookRetryWindow(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_CALL_HOOK_RETRY_INITIAL_BACKOFF", "10s")
	t.Setenv("ZVONILKA_CALL_HOOK_RETRY_MAX_BACKOFF", "5s")

	_, err := Load("callhooks")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "call hook retry max backoff must be greater than or equal to the initial backoff") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsInvalidCallHookURL(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_CALL_RECORDING_HOOK_URL", "://bad")

	_, err := Load("callworker")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "call recording hook url must be absolute") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadAppliesRTCHealthOverrides(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_RTC_HEALTH_TTL", "4s")
	t.Setenv("ZVONILKA_RTC_HEALTH_TIMEOUT", "750ms")

	cfg, err := Load("gateway")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.RTC.HealthTTL != 4*time.Second {
		t.Fatalf("rtc health ttl: got %s, want 4s", cfg.RTC.HealthTTL)
	}
	if cfg.RTC.HealthTimeout != 750*time.Millisecond {
		t.Fatalf("rtc health timeout: got %s, want 750ms", cfg.RTC.HealthTimeout)
	}
}

func TestLoadRejectsInvalidRTCHealthWindow(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_RTC_HEALTH_TTL", "0s")

	_, err := Load("gateway")
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "rtc health ttl must be positive") {
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

func TestLoadAppliesCallRehomeOverrides(t *testing.T) {
	resetConfigEnv(t)

	t.Setenv("ZVONILKA_CALL_REHOME_POLL_INTERVAL", "1500ms")
	t.Setenv("ZVONILKA_CALL_REHOME_BATCH_SIZE", "17")

	cfg, err := Load("gateway")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Call.RehomePollInterval != 1500*time.Millisecond {
		t.Fatalf("call rehome poll interval: got %s, want 1500ms", cfg.Call.RehomePollInterval)
	}
	if cfg.Call.RehomeBatchSize != 17 {
		t.Fatalf("call rehome batch size: got %d, want 17", cfg.Call.RehomeBatchSize)
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
		"ZVONILKA_FEDERATIONBRIDGE_ENV",
		"ZVONILKA_FEDERATIONBRIDGE_HTTP_ADDR",
		"ZVONILKA_FEDERATIONBRIDGE_GRPC_ADDR",
		"ZVONILKA_FEDERATIONBRIDGE_SHUTDOWN_TIMEOUT",
		"ZVONILKA_FEDERATIONMESHTASTIC_ENV",
		"ZVONILKA_FEDERATIONMESHTASTIC_HTTP_ADDR",
		"ZVONILKA_FEDERATIONMESHTASTIC_GRPC_ADDR",
		"ZVONILKA_FEDERATIONMESHTASTIC_SHUTDOWN_TIMEOUT",
		"ZVONILKA_FEDERATIONMESHCORE_ENV",
		"ZVONILKA_FEDERATIONMESHCORE_HTTP_ADDR",
		"ZVONILKA_FEDERATIONMESHCORE_GRPC_ADDR",
		"ZVONILKA_FEDERATIONMESHCORE_SHUTDOWN_TIMEOUT",
		"ZVONILKA_FEDERATION_LOCAL_SERVER_NAME",
		"ZVONILKA_FEDERATION_BRIDGE_SHARED_SECRET",
		"ZVONILKA_FEDERATION_BRIDGE_ENDPOINT",
		"ZVONILKA_FEDERATION_BRIDGE_POLL_INTERVAL",
		"ZVONILKA_FEDERATION_BRIDGE_BATCH_SIZE",
		"ZVONILKA_FEDERATION_BRIDGE_PEER_SERVER_NAME",
		"ZVONILKA_FEDERATION_BRIDGE_LINK_NAME",
		"ZVONILKA_FEDERATION_WORKER_POLL_INTERVAL",
		"ZVONILKA_FEDERATION_WORKER_BATCH_SIZE",
		"ZVONILKA_FEDERATION_DIAL_TIMEOUT",
		"ZVONILKA_MESHTASTIC_INTERFACE_KIND",
		"ZVONILKA_MESHTASTIC_DEVICE",
		"ZVONILKA_MESHTASTIC_HELPER_PYTHON",
		"ZVONILKA_MESHTASTIC_HELPER_SCRIPT_PATH",
		"ZVONILKA_MESHTASTIC_RECEIVE_TIMEOUT",
		"ZVONILKA_MESHTASTIC_TEXT_PREFIX",
		"ZVONILKA_MESHCORE_INTERFACE_KIND",
		"ZVONILKA_MESHCORE_DEVICE",
		"ZVONILKA_MESHCORE_HELPER_PYTHON",
		"ZVONILKA_MESHCORE_HELPER_SCRIPT_PATH",
		"ZVONILKA_MESHCORE_RECEIVE_TIMEOUT",
		"ZVONILKA_MESHCORE_TEXT_PREFIX",
		"ZVONILKA_MESHCORE_DESTINATION",
		"ZVONILKA_NOTIFICATION_WORKER_POLL_INTERVAL",
		"ZVONILKA_NOTIFICATION_RETRY_INITIAL_BACKOFF",
		"ZVONILKA_NOTIFICATION_RETRY_MAX_BACKOFF",
		"ZVONILKA_NOTIFICATION_DELIVERY_LEASE_TTL",
		"ZVONILKA_NOTIFICATION_DELIVERY_WEBHOOK_URL",
		"ZVONILKA_NOTIFICATION_DELIVERY_WEBHOOK_TIMEOUT",
		"ZVONILKA_NOTIFICATION_MAX_ATTEMPTS",
		"ZVONILKA_NOTIFICATION_BATCH_SIZE",
		"ZVONILKA_NOTIFICATIONWORKER_NOTIFICATION_WORKER_POLL_INTERVAL",
		"ZVONILKA_NOTIFICATIONWORKER_NOTIFICATION_RETRY_INITIAL_BACKOFF",
		"ZVONILKA_NOTIFICATIONWORKER_NOTIFICATION_RETRY_MAX_BACKOFF",
		"ZVONILKA_NOTIFICATIONWORKER_NOTIFICATION_DELIVERY_LEASE_TTL",
		"ZVONILKA_NOTIFICATIONWORKER_NOTIFICATION_DELIVERY_WEBHOOK_URL",
		"ZVONILKA_NOTIFICATIONWORKER_NOTIFICATION_DELIVERY_WEBHOOK_TIMEOUT",
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
