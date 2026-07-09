package s3

import (
	"context"
	"fmt"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
)

// Factory builds a logical S3-compatible storage provider.
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

// Build opens the shared client if necessary and returns a provider wrapper.
func (f *Factory) Build(ctx context.Context, cfg config.Configuration) (domainstorage.Provider, error) {
	if f == nil || f.bootstrap == nil {
		return nil, fmt.Errorf("configure s3 factory: %w", domainstorage.ErrInvalidInput)
	}

	provider, err := f.bootstrap.Open(ctx)
	if err != nil {
		return nil, err
	}

	return &Provider{
		client:       provider.client,
		presign:      provider.presign,
		bucket:       provider.bucket,
		name:         f.name,
		kind:         f.kind,
		purpose:      f.purpose,
		capabilities: f.capabilities,
	}, nil
}
