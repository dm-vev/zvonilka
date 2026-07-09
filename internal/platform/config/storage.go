package config

// StorageConfig defines logical provider bindings for the universal storage catalog.
type StorageConfig struct {
	PrimaryProvider string
	CacheProvider   string
	ObjectProvider  string
	AuditProvider   string
	SearchProvider  string
}
