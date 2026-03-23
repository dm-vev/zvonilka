package storage

import (
	"context"
	"errors"
	"testing"
)

type testProvider struct {
	name         string
	kind         Kind
	purpose      Purpose
	capabilities Capability
	closed       *[]string
}

func (p testProvider) Name() string             { return p.name }
func (p testProvider) Kind() Kind               { return p.kind }
func (p testProvider) Purpose() Purpose         { return p.purpose }
func (p testProvider) Capabilities() Capability { return p.capabilities }
func (p testProvider) Close(context.Context) error {
	if p.closed != nil {
		*p.closed = append(*p.closed, p.name)
	}
	return nil
}

func TestCatalogRegistersAndSelectsProviders(t *testing.T) {
	t.Parallel()

	closed := make([]string, 0, 3)
	catalog, err := NewCatalog(
		testProvider{name: "primary", kind: KindRelational, purpose: PurposePrimary, capabilities: CapabilityRead | CapabilityWrite | CapabilityTransactions, closed: &closed},
		testProvider{name: "cache", kind: KindCache, purpose: PurposeCache, capabilities: CapabilityRead | CapabilityWrite | CapabilityKeyValue, closed: &closed},
		testProvider{name: "object", kind: KindObject, purpose: PurposeObject, capabilities: CapabilityRead | CapabilityWrite | CapabilityBlob | CapabilityListing, closed: &closed},
	)
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	provider, err := catalog.Select(PurposePrimary, CapabilityTransactions)
	if err != nil {
		t.Fatalf("select provider: %v", err)
	}
	if provider.Name() != "primary" {
		t.Fatalf("expected primary provider, got %s", provider.Name())
	}

	cacheProviders := catalog.ProvidersByPurpose(PurposeCache)
	if len(cacheProviders) != 1 || cacheProviders[0].Name() != "cache" {
		t.Fatalf("expected cache provider in purpose list, got %+v", cacheProviders)
	}

	objectProviders := catalog.ProvidersByKind(KindObject)
	if len(objectProviders) != 1 || objectProviders[0].Name() != "object" {
		t.Fatalf("expected object provider in kind list, got %+v", objectProviders)
	}

	if err := catalog.Close(context.Background()); err != nil {
		t.Fatalf("close catalog: %v", err)
	}
	if len(closed) != 3 {
		t.Fatalf("expected 3 closed providers, got %d", len(closed))
	}
	if closed[0] != "object" || closed[1] != "cache" || closed[2] != "primary" {
		t.Fatalf("expected reverse close order, got %v", closed)
	}
}

func TestCatalogRejectsDuplicateName(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{}
	err := catalog.Register(testProvider{name: "primary", kind: KindRelational, purpose: PurposePrimary, capabilities: CapabilityTransactions})
	if err != nil {
		t.Fatalf("register provider: %v", err)
	}
	err = catalog.Register(testProvider{name: "primary", kind: KindCache, purpose: PurposeCache, capabilities: CapabilityKeyValue})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestCatalogNormalizesProviderNames(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{}
	err := catalog.Register(testProvider{name: " Primary ", kind: KindRelational, purpose: PurposePrimary, capabilities: CapabilityTransactions})
	if err != nil {
		t.Fatalf("register provider: %v", err)
	}

	provider, err := catalog.Provider("PRIMARY")
	if err != nil {
		t.Fatalf("lookup provider: %v", err)
	}
	if provider.Name() != " Primary " {
		t.Fatalf("expected original provider name to stay intact, got %q", provider.Name())
	}
}

func TestCatalogNormalizesProviderMetadata(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{}
	err := catalog.Register(testProvider{
		name:         "primary",
		kind:         " Relational ",
		purpose:      " PRIMARY ",
		capabilities: CapabilityTransactions,
	})
	if err != nil {
		t.Fatalf("register provider: %v", err)
	}

	byPurpose := catalog.ProvidersByPurpose(PurposePrimary)
	if len(byPurpose) != 1 || byPurpose[0].Name() != "primary" {
		t.Fatalf("expected normalized purpose lookup to find provider, got %+v", byPurpose)
	}

	byKind := catalog.ProvidersByKind(KindRelational)
	if len(byKind) != 1 || byKind[0].Name() != "primary" {
		t.Fatalf("expected normalized kind lookup to find provider, got %+v", byKind)
	}

	selected, err := catalog.Select(Purpose(" primary "), CapabilityTransactions)
	if err != nil {
		t.Fatalf("select provider: %v", err)
	}
	if selected.Name() != "primary" {
		t.Fatalf("expected normalized selection to find provider, got %s", selected.Name())
	}
}

func TestCatalogRejectsUnsupportedProviderMetadata(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{}
	err := catalog.Register(testProvider{
		name:         "primary",
		kind:         Kind("bucket"),
		purpose:      PurposePrimary,
		capabilities: CapabilityTransactions,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestCommitMarksErrors(t *testing.T) {
	t.Parallel()

	err := errors.New("boom")
	committed := Commit(err)
	if committed == nil {
		t.Fatal("expected committed error wrapper")
	}
	if !IsCommit(committed) {
		t.Fatal("expected commit marker")
	}
	if UnwrapCommit(committed) != err {
		t.Fatalf("expected wrapped error %v, got %v", err, UnwrapCommit(committed))
	}
}

func TestCommitIsIdempotent(t *testing.T) {
	t.Parallel()

	err := errors.New("boom")
	committed := Commit(err)
	nested := Commit(committed)
	if nested != committed {
		t.Fatalf("expected commit wrapper to be idempotent")
	}
	if UnwrapCommit(nested) != err {
		t.Fatalf("expected nested unwrap to return original error, got %v", UnwrapCommit(nested))
	}
}
