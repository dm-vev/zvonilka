package postgres

import "strings"

const defaultSchema = "public"

// normalizeSchema returns the configured schema or the default public schema.
func normalizeSchema(schema string) string {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return defaultSchema
	}

	return schema
}

// quoteIdentifier quotes a PostgreSQL identifier safely for DDL and DML.
func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

// qualifiedName returns a schema-qualified identifier.
func qualifiedName(schema string, name string) string {
	return quoteIdentifier(normalizeSchema(schema)) + "." + quoteIdentifier(name)
}
