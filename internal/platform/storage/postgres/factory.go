package postgres

import (
	"context"
	"fmt"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
)

// Factory builds a logical storage provider backed by the shared PostgreSQL pool.
type Factory struct {
	bootstrap    *Bootstrap
	name         string
	kind         domainstorage.Kind
	purpose      domainstorage.Purpose
	capabilities domainstorage.Capability
}

// NewFactory constructs a factory for one logical storage binding.
func NewFactory(
	bootstrap *Bootstrap,
	name string,
	kind domainstorage.Kind,
	purpose domainstorage.Purpose,
	capabilities domainstorage.Capability,
) *Factory {
	return &Factory{
		bootstrap:    bootstrap,
		name:         name,
		kind:         kind,
		purpose:      purpose,
		capabilities: capabilities,
	}
}

// Build opens the shared pool if necessary and returns a provider wrapper.
func (f *Factory) Build(ctx context.Context, cfg config.Configuration) (domainstorage.Provider, error) {
	if f == nil || f.bootstrap == nil {
		return nil, fmt.Errorf("configure postgres factory: %w", domainstorage.ErrInvalidInput)
	}

	db, err := f.bootstrap.Open(ctx)
	if err != nil {
		return nil, err
	}

	return &Provider{
		bootstrap:    f.bootstrap,
		db:           db,
		name:         f.name,
		kind:         f.kind,
		purpose:      f.purpose,
		capabilities: f.capabilities,
	}, nil
}
