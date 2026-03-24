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
		return f.provider, f.err
	}

	return f.provider, nil
}

type cancelingFactory struct {
	cancel context.CancelFunc
	err    error
}

func (f cancelingFactory) Build(context.Context, config.Configuration) (domainstorage.Provider, error) {
	if f.cancel != nil {
		f.cancel()
	}

	return nil, f.err
}

type testProvider struct {
	name              string
	kind              domainstorage.Kind
	purpose           domainstorage.Purpose
	caps              domainstorage.Capability
	closed            *[]string
	closeCtxErrs      *[]error
	closeCtxDeadlines *[]bool
	closeErr          error
}

func (p testProvider) Name() string                   { return p.name }
func (p testProvider) Kind() domainstorage.Kind       { return p.kind }
func (p testProvider) Purpose() domainstorage.Purpose { return p.purpose }
func (p testProvider) Capabilities() domainstorage.Capability {
	return p.caps
}
func (p testProvider) Close(ctx context.Context) error {
	if p.closed != nil {
		*p.closed = append(*p.closed, p.name)
	}
	if p.closeCtxErrs != nil {
		*p.closeCtxErrs = append(*p.closeCtxErrs, ctx.Err())
	}
	if p.closeCtxDeadlines != nil {
		_, ok := ctx.Deadline()
		*p.closeCtxDeadlines = append(*p.closeCtxDeadlines, ok)
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

func TestBuilderCleansUpPartialProviderOnFactoryError(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

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
		testFactory{
			provider: testProvider{
				name:     "partial",
				kind:     domainstorage.KindCache,
				purpose:  domainstorage.PurposeCache,
				caps:     domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityKeyValue,
				closed:   &closed,
				closeErr: errors.New("partial close boom"),
			},
			err: errors.New("factory boom"),
		},
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
	if !strings.Contains(err.Error(), "partial close boom") {
		t.Fatalf("expected partial provider cleanup error, got %v", err)
	}
	if len(closed) != 2 {
		t.Fatalf("expected both providers to close, got %v", closed)
	}
	if closed[0] != "partial" || closed[1] != "primary" {
		t.Fatalf("expected partial provider to close before previously built providers, got %v", closed)
	}
}

func TestBuilderUsesBoundedCleanupContextOnFactoryFailure(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	closeCtxErrs := make([]error, 0, 1)
	closeCtxDeadlines := make([]bool, 0, 1)
	ctx, cancel := context.WithCancel(context.Background())

	builder, err := NewBuilder(
		cfg,
		testFactory{provider: testProvider{
			name:              "primary",
			kind:              domainstorage.KindRelational,
			purpose:           domainstorage.PurposePrimary,
			caps:              domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
			closeCtxErrs:      &closeCtxErrs,
			closeCtxDeadlines: &closeCtxDeadlines,
		}},
		cancelingFactory{
			cancel: cancel,
			err:    errors.New("factory boom"),
		},
	)
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}

	_, err = builder.Build(ctx)
	if err == nil {
		t.Fatal("expected build to fail")
	}
	if !strings.Contains(err.Error(), "factory boom") {
		t.Fatalf("unexpected build error: %v", err)
	}
	if len(closeCtxErrs) != 1 {
		t.Fatalf("expected one cleanup close, got %d", len(closeCtxErrs))
	}
	if closeCtxErrs[0] != nil {
		t.Fatalf("expected cleanup context to ignore cancellation, got %v", closeCtxErrs[0])
	}
	if len(closeCtxDeadlines) != 1 || !closeCtxDeadlines[0] {
		t.Fatalf("expected bounded cleanup deadline, got %v", closeCtxDeadlines)
	}
}

func TestBuilderRejectsTypedNilProvider(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	closed := make([]string, 0, 1)
	var nilProvider *testProvider

	builder, err := NewBuilder(
		cfg,
		testFactory{provider: testProvider{
			name:    "primary",
			kind:    domainstorage.KindRelational,
			purpose: domainstorage.PurposePrimary,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
			closed:  &closed,
		}},
		testFactory{provider: domainstorage.Provider(nilProvider)},
	)
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("expected typed nil provider to return error, panicked: %v", r)
		}
	}()

	_, err = builder.Build(context.Background())
	if err == nil {
		t.Fatal("expected build to fail")
	}
	if !strings.Contains(err.Error(), "invalid input") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(closed) != 1 || closed[0] != "primary" {
		t.Fatalf("expected already built provider to be closed, got %v", closed)
	}
}

func TestBuilderRejectsTypedNilFactory(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	var nilFactory *testFactory

	_, err = NewBuilder(cfg, nilFactory)
	if err == nil {
		t.Fatal("expected new builder to fail")
	}
	if !strings.Contains(err.Error(), "invalid input") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuilderBuildRejectsTypedNilFactory(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	closed := make([]string, 0, 1)
	var nilFactory *testFactory

	builder := &Builder{
		cfg: cfg,
		factories: []Factory{
			testFactory{provider: testProvider{
				name:    "primary",
				kind:    domainstorage.KindRelational,
				purpose: domainstorage.PurposePrimary,
				caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
				closed:  &closed,
			}},
			nilFactory,
		},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("expected typed nil factory to return error, panicked: %v", r)
		}
	}()

	_, err = builder.Build(context.Background())
	if err == nil {
		t.Fatal("expected build to fail")
	}
	if !strings.Contains(err.Error(), "invalid input") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(closed) != 1 || closed[0] != "primary" {
		t.Fatalf("expected already built provider to be closed, got %v", closed)
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

func TestBuilderRejectsWrongKind(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	builder, err := NewBuilder(
		cfg,
		testFactory{provider: testProvider{
			name:    "primary",
			kind:    domainstorage.KindObject,
			purpose: domainstorage.PurposePrimary,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
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
	if !strings.Contains(err.Error(), "expected kind") {
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
	if !strings.Contains(err.Error(), "required=read|write|transactions") {
		t.Fatalf("expected required capability details, got %v", err)
	}
	if !strings.Contains(err.Error(), "actual=read|transactions") {
		t.Fatalf("expected actual capability details, got %v", err)
	}
}

func TestBuilderClosesBuiltProvidersOnCatalogConstructionFailure(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load("controlplane")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	closed := make([]string, 0, 1)
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
			name:    "primary",
			kind:    domainstorage.KindRelational,
			purpose: domainstorage.PurposePrimary,
			caps:    domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
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
	if !strings.Contains(err.Error(), "construct storage catalog") {
		t.Fatalf("unexpected build error: %v", err)
	}
	if len(closed) != 2 || closed[0] != "primary" || closed[1] != "primary" {
		t.Fatalf("expected failing provider and partial catalog to close, got %v", closed)
	}
}

func TestBuilderBuildRejectsNilReceiver(t *testing.T) {
	t.Parallel()

	var builder *Builder
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("expected nil builder to return error, panicked: %v", r)
		}
	}()

	_, err := builder.Build(context.Background())
	if err == nil {
		t.Fatal("expected build to fail")
	}
	if !strings.Contains(err.Error(), "invalid input") {
		t.Fatalf("unexpected error: %v", err)
	}
}
