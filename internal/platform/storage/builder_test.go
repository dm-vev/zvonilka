package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
)

type testFactory struct {
	provider domainstorage.Provider
	err      error
}

func (f testFactory) Build(context.Context, config.Configuration) (domainstorage.Provider, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.provider, nil
}

type testProvider struct {
	name     string
	kind     domainstorage.Kind
	purpose  domainstorage.Purpose
	caps     domainstorage.Capability
	closed   *[]string
	closeErr error
}

func (p testProvider) Name() string                   { return p.name }
func (p testProvider) Kind() domainstorage.Kind       { return p.kind }
func (p testProvider) Purpose() domainstorage.Purpose { return p.purpose }
func (p testProvider) Capabilities() domainstorage.Capability {
	return p.caps
}
func (p testProvider) Close(context.Context) error {
	if p.closed != nil {
		*p.closed = append(*p.closed, p.name)
	}
	return p.closeErr
}

func TestBuilderBuildsCatalogAndValidatesBindings(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	closed := make([]string, 0, 5)
	builder, err := NewBuilder(
		cfg,
		testFactory{provider: testProvider{
			name:    "primary",
			kind:    domainstorage.KindRelational,
			purpose: domainstorage.PurposePrimary,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
			closed:  &closed,
		}},
		testFactory{provider: testProvider{
			name:    "cache",
			kind:    domainstorage.KindCache,
			purpose: domainstorage.PurposeCache,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityKeyValue,
			closed:  &closed,
		}},
		testFactory{provider: testProvider{
			name:    "object",
			kind:    domainstorage.KindObject,
			purpose: domainstorage.PurposeObject,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityBlob | domainstorage.CapabilityListing,
			closed:  &closed,
		}},
		testFactory{provider: testProvider{
			name:    "audit",
			kind:    domainstorage.KindIndex,
			purpose: domainstorage.PurposeAudit,
			caps:    domainstorage.CapabilityWrite | domainstorage.CapabilityListing,
			closed:  &closed,
		}},
		testFactory{provider: testProvider{
			name:    "search",
			kind:    domainstorage.KindIndex,
			purpose: domainstorage.PurposeSearch,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityListing,
			closed:  &closed,
		}},
	)
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}

	catalog, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}

	if _, err := catalog.Provider("PRIMARY"); err != nil {
		t.Fatalf("lookup primary provider: %v", err)
	}
	if _, err := catalog.Select(domainstorage.PurposeCache, domainstorage.CapabilityKeyValue); err != nil {
		t.Fatalf("select cache provider: %v", err)
	}

	if err := catalog.Close(context.Background()); err != nil {
		t.Fatalf("close catalog: %v", err)
	}

	if len(closed) != 5 {
		t.Fatalf("expected 5 providers to close, got %d", len(closed))
	}
	if closed[0] != "search" || closed[4] != "primary" {
		t.Fatalf("expected reverse close order, got %v", closed)
	}
}

func TestBuilderRejectsMissingBinding(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Storage.ObjectProvider = "missing"

	closed := make([]string, 0, 2)
	builder, err := NewBuilder(
		cfg,
		testFactory{provider: testProvider{
			name:    "primary",
			kind:    domainstorage.KindRelational,
			purpose: domainstorage.PurposePrimary,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
			closed:  &closed,
		}},
		testFactory{provider: testProvider{
			name:    "cache",
			kind:    domainstorage.KindCache,
			purpose: domainstorage.PurposeCache,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityKeyValue,
			closed:  &closed,
		}},
	)
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}

	_, err = builder.Build(context.Background())
	if err == nil {
		t.Fatal("expected build to fail")
	}
	if !strings.Contains(err.Error(), "resolve object storage binding") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(closed) != 2 {
		t.Fatalf("expected both providers to close, got %v", closed)
	}
}

func TestBuilderPropagatesCleanupErrors(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	closeErr := errors.New("close boom")
	builder, err := NewBuilder(
		cfg,
		testFactory{provider: testProvider{
			name:     "primary",
			kind:     domainstorage.KindRelational,
			purpose:  domainstorage.PurposePrimary,
			caps:     domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
			closeErr: closeErr,
		}},
		testFactory{err: errors.New("factory boom")},
	)
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}

	_, err = builder.Build(context.Background())
	if err == nil {
		t.Fatal("expected build to fail")
	}
	if !strings.Contains(err.Error(), "factory boom") {
		t.Fatalf("unexpected build error: %v", err)
	}
	if !strings.Contains(err.Error(), "close storage provider") {
		t.Fatalf("expected cleanup error in wrapped error, got %v", err)
	}
}

func TestBuilderRejectsNilFactory(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	_, err = NewBuilder(cfg, nil)
	if err == nil {
		t.Fatal("expected new builder to fail")
	}
}

func TestBuilderRejectsWrongPurpose(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	builder, err := NewBuilder(
		cfg,
		testFactory{provider: testProvider{
			name:    "primary",
			kind:    domainstorage.KindCache,
			purpose: domainstorage.PurposeCache,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityKeyValue,
		}},
		testFactory{provider: testProvider{
			name:    "cache",
			kind:    domainstorage.KindCache,
			purpose: domainstorage.PurposeCache,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityKeyValue,
		}},
		testFactory{provider: testProvider{
			name:    "object",
			kind:    domainstorage.KindObject,
			purpose: domainstorage.PurposeObject,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityBlob | domainstorage.CapabilityListing,
		}},
		testFactory{provider: testProvider{
			name:    "audit",
			kind:    domainstorage.KindIndex,
			purpose: domainstorage.PurposeAudit,
			caps:    domainstorage.CapabilityWrite | domainstorage.CapabilityListing,
		}},
		testFactory{provider: testProvider{
			name:    "search",
			kind:    domainstorage.KindIndex,
			purpose: domainstorage.PurposeSearch,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityListing,
		}},
	)
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}

	_, err = builder.Build(context.Background())
	if err == nil {
		t.Fatal("expected build to fail")
	}
	if !strings.Contains(err.Error(), "expected purpose") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuilderRejectsWrongCapabilities(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Storage.PrimaryProvider = "primary"

	builder, err := NewBuilder(
		cfg,
		testFactory{provider: testProvider{
			name:    "primary",
			kind:    domainstorage.KindRelational,
			purpose: domainstorage.PurposePrimary,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityTransactions,
		}},
		testFactory{provider: testProvider{
			name:    "cache",
			kind:    domainstorage.KindCache,
			purpose: domainstorage.PurposeCache,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityKeyValue,
		}},
		testFactory{provider: testProvider{
			name:    "object",
			kind:    domainstorage.KindObject,
			purpose: domainstorage.PurposeObject,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityBlob | domainstorage.CapabilityListing,
		}},
		testFactory{provider: testProvider{
			name:    "audit",
			kind:    domainstorage.KindIndex,
			purpose: domainstorage.PurposeAudit,
			caps:    domainstorage.CapabilityWrite | domainstorage.CapabilityListing,
		}},
		testFactory{provider: testProvider{
			name:    "search",
			kind:    domainstorage.KindIndex,
			purpose: domainstorage.PurposeSearch,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityListing,
		}},
	)
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}

	_, err = builder.Build(context.Background())
	if err == nil {
		t.Fatal("expected build to fail")
	}
	if !strings.Contains(err.Error(), "lacks required capabilities") {
		t.Fatalf("unexpected error: %v", err)
	}
}
