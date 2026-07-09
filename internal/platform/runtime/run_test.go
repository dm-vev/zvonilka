package runtime

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

type recordedHealthStatus struct {
	service string
	status  healthgrpc.HealthCheckResponse_ServingStatus
}

type fakeGRPCHealthStatusSetter struct {
	records []recordedHealthStatus
}

func (f *fakeGRPCHealthStatusSetter) SetServingStatus(
	service string,
	status healthgrpc.HealthCheckResponse_ServingStatus,
) {
	f.records = append(f.records, recordedHealthStatus{
		service: service,
		status:  status,
	})
}

func TestSetGRPCHealthStatusTransitions(t *testing.T) {
	t.Parallel()

	healthSetter := &fakeGRPCHealthStatusSetter{}
	setGRPCServingStatus(healthSetter, "gateway")
	setGRPCNotServingStatus(healthSetter, "gateway")

	if len(healthSetter.records) != 4 {
		t.Fatalf("expected 4 health status updates, got %d", len(healthSetter.records))
	}

	if got := healthSetter.records[0]; got.service != "gateway" || got.status != healthgrpc.HealthCheckResponse_SERVING {
		t.Fatalf("unexpected first status update: %+v", got)
	}
	if got := healthSetter.records[1]; got.service != "" || got.status != healthgrpc.HealthCheckResponse_SERVING {
		t.Fatalf("unexpected second status update: %+v", got)
	}
	if got := healthSetter.records[2]; got.service != "gateway" || got.status != healthgrpc.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("unexpected third status update: %+v", got)
	}
	if got := healthSetter.records[3]; got.service != "" || got.status != healthgrpc.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("unexpected fourth status update: %+v", got)
	}
}

func TestShutdownGRPCServerSetsNotServing(t *testing.T) {
	t.Parallel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	healthSetter := &fakeGRPCHealthStatusSetter{}
	grpcServer := grpc.NewServer()

	shutdownGRPCServer(shutdownCtx, grpcServer, healthSetter, "controlplane")

	if len(healthSetter.records) != 2 {
		t.Fatalf("expected 2 health status updates, got %d", len(healthSetter.records))
	}

	if got := healthSetter.records[0]; got.service != "controlplane" || got.status != healthgrpc.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("unexpected first status update: %+v", got)
	}
	if got := healthSetter.records[1]; got.service != "" || got.status != healthgrpc.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("unexpected second status update: %+v", got)
	}
}
