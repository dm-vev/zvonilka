package storage

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
)

// Factory builds a storage provider from the current service configuration.
type Factory interface {
	Build(ctx context.Context, cfg config.Configuration) (domainstorage.Provider, error)
}

// Builder assembles a domain storage catalog from provider factories.
type Builder struct {
	cfg       config.Configuration
	factories []Factory
}

// NewBuilder constructs a builder from the service configuration and factories.
func NewBuilder(cfg config.Configuration, factories ...Factory) (*Builder, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate configuration: %w", err)
	}
	if len(factories) == 0 {
		return nil, fmt.Errorf("configure storage builder: %w", domainstorage.ErrInvalidInput)
	}
	for _, factory := range factories {
		if isNilFactory(factory) {
			return nil, fmt.Errorf("configure storage builder: %w", domainstorage.ErrInvalidInput)
		}
	}

	return &Builder{
		cfg:       cfg,
		factories: append([]Factory(nil), factories...),
	}, nil
}

// Build constructs and validates a storage catalog.
func (b *Builder) Build(ctx context.Context) (*domainstorage.Catalog, error) {
	if b == nil {
		return nil, fmt.Errorf("build storage catalog: %w", domainstorage.ErrInvalidInput)
	}
	if ctx == nil {
		return nil, fmt.Errorf("build storage catalog: %w", domainstorage.ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build storage catalog: %w", err)
	}

	providers := make([]domainstorage.Provider, 0, len(b.factories))
	for _, factory := range b.factories {
		provider, err := factory.Build(ctx, b.cfg)
		if err != nil {
			cleanupProviders := append([]domainstorage.Provider(nil), providers...)
			if !isNilProvider(provider) {
				cleanupProviders = append(cleanupProviders, provider)
			}

			return nil, errors.Join(fmt.Errorf("build storage provider: %w", err), closeProviders(ctx, cleanupProviders))
		}
		if isNilProvider(provider) {
			return nil, errors.Join(
				fmt.Errorf("build storage provider: %w", domainstorage.ErrInvalidInput),
				closeProviders(ctx, providers),
			)
		}

		providers = append(providers, provider)
	}

	catalog, err := domainstorage.NewCatalog(providers...)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("construct storage catalog: %w", err), closeProviders(ctx, providers))
	}

	if err := b.validateBindings(catalog); err != nil {
		return nil, errors.Join(err, closeProviders(ctx, providers))
	}

	return catalog, nil
}

// validateBindings verifies that every configured logical binding resolves to a registered provider.
func (b *Builder) validateBindings(catalog *domainstorage.Catalog) error {
	bindings := []struct {
		logical  string
		name     string
		purpose  domainstorage.Purpose
		required domainstorage.Capability
	}{
		{
			logical:  "primary",
			name:     b.cfg.Storage.PrimaryProvider,
			purpose:  domainstorage.PurposePrimary,
			required: domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
		},
		{
			logical:  "cache",
			name:     b.cfg.Storage.CacheProvider,
			purpose:  domainstorage.PurposeCache,
			required: domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityKeyValue,
		},
		{
			logical:  "object",
			name:     b.cfg.Storage.ObjectProvider,
			purpose:  domainstorage.PurposeObject,
			required: domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityBlob | domainstorage.CapabilityListing,
		},
		{
			logical:  "audit",
			name:     b.cfg.Storage.AuditProvider,
			purpose:  domainstorage.PurposeAudit,
			required: domainstorage.CapabilityWrite | domainstorage.CapabilityListing,
		},
		{
			logical:  "search",
			name:     b.cfg.Storage.SearchProvider,
			purpose:  domainstorage.PurposeSearch,
			required: domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityListing,
		},
	}

	for _, binding := range bindings {
		provider, err := catalog.Provider(binding.name)
		if err != nil {
			return fmt.Errorf("resolve %s storage binding %q: %w", binding.logical, binding.name, err)
		}
		if provider.Purpose() != binding.purpose {
			return fmt.Errorf(
				"resolve %s storage binding %q: expected purpose %q, got %q",
				binding.logical,
				binding.name,
				binding.purpose,
				provider.Purpose(),
			)
		}
		if !provider.Capabilities().Has(binding.required) {
			return fmt.Errorf(
				"resolve %s storage binding %q: provider lacks required capabilities: required=%s actual=%s",
				binding.logical,
				binding.name,
				describeCapabilities(binding.required),
				describeCapabilities(provider.Capabilities()),
			)
		}
	}

	return nil
}

// closeProviders closes already built providers when a later step fails.
func closeProviders(ctx context.Context, providers []domainstorage.Provider) error {
	var closeErr error
	for i := len(providers) - 1; i >= 0; i-- {
		if isNilProvider(providers[i]) {
			continue
		}
		if err := providers[i].Close(ctx); err != nil {
			closeErr = errors.Join(
				closeErr,
				fmt.Errorf("close storage provider %q: %w", providers[i].Name(), err),
			)
		}
	}

	return closeErr
}

func describeCapabilities(c domainstorage.Capability) string {
	if c == 0 {
		return "none"
	}

	parts := make([]string, 0, 6)
	add := func(required domainstorage.Capability, label string) {
		if c.Has(required) {
			parts = append(parts, label)
		}
	}

	add(domainstorage.CapabilityRead, "read")
	add(domainstorage.CapabilityWrite, "write")
	add(domainstorage.CapabilityTransactions, "transactions")
	add(domainstorage.CapabilityBlob, "blob")
	add(domainstorage.CapabilityKeyValue, "key-value")
	add(domainstorage.CapabilityListing, "listing")

	if len(parts) == 0 {
		return fmt.Sprintf("unknown(%d)", c)
	}

	return strings.Join(parts, "|")
}

func isNilProvider(provider domainstorage.Provider) bool {
	if provider == nil {
		return true
	}

	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func isNilFactory(factory Factory) bool {
	if factory == nil {
		return true
	}

	value := reflect.ValueOf(factory)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
