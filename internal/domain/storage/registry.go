package storage

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Catalog stores and resolves named storage providers.
type Catalog struct {
	mu        sync.RWMutex
	providers map[string]Provider
	order     []string
	byKind    map[Kind][]string
	byPurpose map[Purpose][]string
}

// NewCatalog builds a catalog from the provided providers.
func NewCatalog(providers ...Provider) (*Catalog, error) {
	catalog := &Catalog{}
	for _, provider := range providers {
		if err := catalog.Register(provider); err != nil {
			return nil, err
		}
	}

	return catalog, nil
}

// Register adds a provider to the catalog.
func (c *Catalog) Register(provider Provider) error {
	if provider == nil {
		return ErrInvalidInput
	}

	name := normalizeName(provider.Name())
	if name == "" {
		return ErrInvalidInput
	}
	if provider.Kind() == KindUnspecified {
		return ErrInvalidInput
	}
	if provider.Purpose() == PurposeUnspecified {
		return ErrInvalidInput
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureMapsLocked()

	if _, exists := c.providers[name]; exists {
		return fmt.Errorf("register storage provider %q: %w", name, ErrConflict)
	}

	c.providers[name] = provider
	c.order = append(c.order, name)
	c.byKind[provider.Kind()] = append(c.byKind[provider.Kind()], name)
	c.byPurpose[provider.Purpose()] = append(c.byPurpose[provider.Purpose()], name)

	return nil
}

// Provider resolves a provider by name.
func (c *Catalog) Provider(name string) (Provider, error) {
	name = normalizeName(name)
	if name == "" {
		return nil, ErrInvalidInput
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	provider, ok := c.providers[name]
	if !ok {
		return nil, fmt.Errorf("storage provider %q: %w", name, ErrNotFound)
	}

	return provider, nil
}

// Select resolves the first provider that matches the requested purpose and capabilities.
func (c *Catalog) Select(purpose Purpose, required Capability) (Provider, error) {
	if purpose == PurposeUnspecified {
		return nil, ErrInvalidInput
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	names := c.byPurpose[purpose]
	for _, name := range names {
		provider := c.providers[name]
		if provider == nil {
			continue
		}
		if provider.Capabilities().Has(required) {
			return provider, nil
		}
	}

	return nil, fmt.Errorf("storage provider for purpose %q: %w", purpose, ErrNotFound)
}

// ProvidersByPurpose returns all providers registered for a purpose.
func (c *Catalog) ProvidersByPurpose(purpose Purpose) []Provider {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := c.byPurpose[purpose]
	providers := make([]Provider, 0, len(names))
	for _, name := range names {
		if provider := c.providers[name]; provider != nil {
			providers = append(providers, provider)
		}
	}

	return slices.Clone(providers)
}

// ProvidersByKind returns all providers registered for a kind.
func (c *Catalog) ProvidersByKind(kind Kind) []Provider {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := c.byKind[kind]
	providers := make([]Provider, 0, len(names))
	for _, name := range names {
		if provider := c.providers[name]; provider != nil {
			providers = append(providers, provider)
		}
	}

	return slices.Clone(providers)
}

// Close closes all registered providers in reverse registration order.
func (c *Catalog) Close(ctx context.Context) error {
	c.mu.RLock()
	names := append([]string(nil), c.order...)
	providers := make(map[string]Provider, len(c.providers))
	for name, provider := range c.providers {
		providers[name] = provider
	}
	c.mu.RUnlock()

	var closeErr error
	for i := len(names) - 1; i >= 0; i-- {
		provider := providers[names[i]]
		if provider == nil {
			continue
		}
		if err := provider.Close(ctx); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close storage provider %q: %w", names[i], err))
		}
	}

	return closeErr
}

func (c *Catalog) ensureMapsLocked() {
	if c.providers == nil {
		c.providers = make(map[string]Provider)
	}
	if c.byKind == nil {
		c.byKind = make(map[Kind][]string)
	}
	if c.byPurpose == nil {
		c.byPurpose = make(map[Purpose][]string)
	}
}

func normalizeName(name string) string {
	return strings.ToLower(trimSpaces(name))
}
