package controlplane

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
)

func TestBuildAppStorageDisabledReturnsNilArtifacts(t *testing.T) {
	t.Parallel()

	catalog, service, err := buildAppStorage(context.Background(), config.Configuration{})
	if err != nil {
		t.Fatalf("build app storage: %v", err)
	}
	if catalog != nil {
		t.Fatal("expected nil catalog when postgres is disabled")
	}
	if service != nil {
		t.Fatal("expected nil identity service when postgres is disabled")
	}
}

func TestCloseStorageCatalogClosesProvidersInReverseOrder(t *testing.T) {
	t.Parallel()

	order := make([]string, 0, 2)
	catalog, err := domainstorage.NewCatalog(
		&fakeProvider{name: "first", order: &order},
		&fakeProvider{name: "second", order: &order},
	)
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	if err := closeStorageCatalog(context.Background(), catalog); err != nil {
		t.Fatalf("close storage catalog: %v", err)
	}

	if want := []string{"second", "first"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("unexpected close order: got %v want %v", order, want)
	}
}

func TestFinalizeRunJoinsCloseError(t *testing.T) {
	t.Parallel()

	runtimeErr := errors.New("runtime failed")
	closeErr := errors.New("close failed")
	catalog, err := domainstorage.NewCatalog(&fakeProvider{name: "first", closeErr: closeErr})
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	got := finalizeRun(context.Background(), &app{catalog: catalog}, runtimeErr)
	if got == nil {
		t.Fatal("expected joined error")
	}
	if !errors.Is(got, runtimeErr) {
		t.Fatalf("expected runtime error in final error: %v", got)
	}
	if !errors.Is(got, closeErr) {
		t.Fatalf("expected close error in final error: %v", got)
	}
	if got.Error() == "" {
		t.Fatal("expected non-empty joined error")
	}
	if !strings.Contains(got.Error(), runtimeErr.Error()) {
		t.Fatalf("expected runtime error text in final error: %v", got)
	}
	if !strings.Contains(got.Error(), closeErr.Error()) {
		t.Fatalf("expected close error text in final error: %v", got)
	}
}

func TestFinalizeRunReturnsCloseErrorWhenRunSucceeds(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close failed")
	catalog, err := domainstorage.NewCatalog(&fakeProvider{name: "first", closeErr: closeErr})
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	got := finalizeRun(context.Background(), &app{catalog: catalog}, nil)
	if got == nil {
		t.Fatal("expected close error")
	}
	if !errors.Is(got, closeErr) {
		t.Fatalf("expected close error in final error: %v", got)
	}
	if !strings.Contains(got.Error(), "close controlplane app") {
		t.Fatalf("expected wrapped close error: %v", got)
	}
}

func TestCloseStorageCatalogReturnsProviderError(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("provider close failed")
	catalog, err := domainstorage.NewCatalog(&fakeProvider{name: "first", closeErr: closeErr})
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	got := closeStorageCatalog(context.Background(), catalog)
	if got == nil {
		t.Fatal("expected close error")
	}
	if !errors.Is(got, closeErr) {
		t.Fatalf("expected provider close error in final error: %v", got)
	}
	if !strings.Contains(got.Error(), "close storage catalog") {
		t.Fatalf("expected wrapped catalog close error: %v", got)
	}
}

type fakeProvider struct {
	name     string
	closeErr error
	order    *[]string
}

func (p *fakeProvider) Name() string {
	return p.name
}

func (p *fakeProvider) Kind() domainstorage.Kind {
	return domainstorage.KindCustom
}

func (p *fakeProvider) Purpose() domainstorage.Purpose {
	return domainstorage.PurposeCustom
}

func (p *fakeProvider) Capabilities() domainstorage.Capability {
	return domainstorage.CapabilityTransactions
}

func (p *fakeProvider) Close(context.Context) error {
	if p.order != nil {
		*p.order = append(*p.order, p.name)
	}

	return p.closeErr
}
