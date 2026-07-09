package postgres

import (
	"context"
	"database/sql"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

// Provider is a logical storage provider backed by PostgreSQL.
type Provider struct {
	bootstrap    *Bootstrap
	db           *sql.DB
	name         string
	kind         domainstorage.Kind
	purpose      domainstorage.Purpose
	capabilities domainstorage.Capability
}

// Name returns the configured provider name.
func (p *Provider) Name() string {
	return p.name
}

// Kind returns the logical provider kind.
func (p *Provider) Kind() domainstorage.Kind {
	return p.kind
}

// Purpose returns the logical provider purpose.
func (p *Provider) Purpose() domainstorage.Purpose {
	return p.purpose
}

// Capabilities returns the supported capabilities.
func (p *Provider) Capabilities() domainstorage.Capability {
	return p.capabilities
}

// DB exposes the shared PostgreSQL pool for relational consumers.
func (p *Provider) DB() *sql.DB {
	return p.db
}

// Close closes the shared PostgreSQL pool exactly once.
func (p *Provider) Close(ctx context.Context) error {
	if p == nil || p.bootstrap == nil {
		return nil
	}

	return p.bootstrap.Close(ctx)
}

var _ domainstorage.RelationalProvider = (*Provider)(nil)
