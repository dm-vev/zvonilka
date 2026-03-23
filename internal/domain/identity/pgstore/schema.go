package pgstore

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

// quoteIdentifier quotes a PostgreSQL identifier safely for SQL statements.
func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

// qualifiedName returns a schema-qualified table name.
func qualifiedName(schema string, table string) string {
	return quoteIdentifier(normalizeSchema(schema)) + "." + quoteIdentifier(table)
}

// lockKey builds a deterministic advisory-lock key for a logical resource.
func lockKey(resource string, value string) string {
	return resource + ":" + value
}
