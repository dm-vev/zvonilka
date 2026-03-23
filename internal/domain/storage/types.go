package storage

import "context"

// Kind identifies the backing technology of a storage provider.
type Kind string

// Storage kinds used by the storage catalog.
const (
	// KindUnspecified is the zero value.
	KindUnspecified Kind = ""
	// KindRelational represents transactional relational storage.
	KindRelational Kind = "relational"
	// KindCache represents ephemeral or cache-oriented storage.
	KindCache Kind = "cache"
	// KindObject represents blob/object storage.
	KindObject Kind = "object"
	// KindIndex represents a search or indexing backend.
	KindIndex Kind = "index"
	// KindCustom covers provider kinds that are specific to a deployment.
	KindCustom Kind = "custom"
)

// Purpose identifies the logical role a storage provider fills.
type Purpose string

// Storage purposes used by the catalog.
const (
	// PurposeUnspecified is the zero value.
	PurposeUnspecified Purpose = ""
	// PurposePrimary is the main transactional storage for core domain data.
	PurposePrimary Purpose = "primary"
	// PurposeCache is the ephemeral cache or rate-limit backend.
	PurposeCache Purpose = "cache"
	// PurposeObject is the blob backend for attachments and media.
	PurposeObject Purpose = "object"
	// PurposeAudit is the append-only audit or action-log backend.
	PurposeAudit Purpose = "audit"
	// PurposeSearch is the indexing backend for search and discovery.
	PurposeSearch Purpose = "search"
	// PurposeCustom covers purpose tags that are specific to a deployment.
	PurposeCustom Purpose = "custom"
)

// Capability identifies an operation family that a provider can support.
type Capability uint64

// Provider capabilities used by the catalog.
const (
	// CapabilityRead indicates the provider can serve read operations.
	CapabilityRead Capability = 1 << iota
	// CapabilityWrite indicates the provider can serve write operations.
	CapabilityWrite
	// CapabilityTransactions indicates the provider can execute atomic units of work.
	CapabilityTransactions
	// CapabilityBlob indicates the provider can store opaque binary blobs.
	CapabilityBlob
	// CapabilityKeyValue indicates the provider can store key-value records.
	CapabilityKeyValue
	// CapabilityListing indicates the provider can enumerate records or objects.
	CapabilityListing
)

// Has reports whether the capability set includes every requested capability.
func (c Capability) Has(required Capability) bool {
	return c&required == required
}

// Provider describes a registered storage backend.
type Provider interface {
	Name() string
	Kind() Kind
	Purpose() Purpose
	Capabilities() Capability
	Close(ctx context.Context) error
}

// Transactioner executes a closure within a single atomic write unit.
type Transactioner[T any] interface {
	WithinTx(ctx context.Context, fn func(T) error) error
}
