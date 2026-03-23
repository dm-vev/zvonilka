package config

import "testing"

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

		if cfg.HTTPAddr != tc.want.http {
			t.Fatalf("http addr for %s: got %s, want %s", tc.service, cfg.HTTPAddr, tc.want.http)
		}
		if cfg.GRPCAddr != tc.want.grpc {
			t.Fatalf("grpc addr for %s: got %s, want %s", tc.service, cfg.GRPCAddr, tc.want.grpc)
		}

		if owner, ok := httpOwners[cfg.HTTPAddr]; ok {
			t.Fatalf("http addr %s reused by %s and %s", cfg.HTTPAddr, owner, tc.service)
		}
		httpOwners[cfg.HTTPAddr] = tc.service

		if owner, ok := grpcOwners[cfg.GRPCAddr]; ok {
			t.Fatalf("grpc addr %s reused by %s and %s", cfg.GRPCAddr, owner, tc.service)
		}
		grpcOwners[cfg.GRPCAddr] = tc.service
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
	if gatewayCfg.HTTPAddr != ":28080" {
		t.Fatalf("gateway http addr: got %s, want :28080", gatewayCfg.HTTPAddr)
	}
	if gatewayCfg.GRPCAddr != ":29090" {
		t.Fatalf("gateway grpc addr: got %s, want :29090", gatewayCfg.GRPCAddr)
	}

	botAPICfg, err := FromEnv("botapi")
	if err != nil {
		t.Fatalf("from env for botapi: %v", err)
	}
	if botAPICfg.HTTPAddr != ":18080" {
		t.Fatalf("botapi http addr: got %s, want :18080", botAPICfg.HTTPAddr)
	}
	if botAPICfg.GRPCAddr != ":19090" {
		t.Fatalf("botapi grpc addr: got %s, want :19090", botAPICfg.GRPCAddr)
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
