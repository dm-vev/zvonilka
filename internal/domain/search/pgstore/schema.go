package pgstore

import "strings"

const defaultSchema = "public"

func normalizeSchema(schema string) string {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return defaultSchema
	}

	return schema
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func qualifiedName(schema string, table string) string {
	return quoteIdentifier(normalizeSchema(schema)) + "." + quoteIdentifier(table)
}
