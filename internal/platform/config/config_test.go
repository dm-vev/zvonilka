package config

import (
	"strings"
	"testing"

	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
)

func TestFromEnvUsesDistinctServiceDefaults(t *testing.T) {
	resetConfigEnv(t)

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
	}

	for _, key := range keys {
		t.Setenv(key, "")
	}
}
