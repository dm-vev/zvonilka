package pgstore

import (
	"fmt"
	"strings"
)

const defaultSchema = "public"

func normalizeSchema(schema string) string {
	return strings.TrimSpace(schema)
}

func qualifiedName(schema string, name string) string {
	schema = normalizeSchema(schema)
	if schema == "" {
		schema = defaultSchema
	}

	return fmt.Sprintf(`"%s"."%s"`, strings.ReplaceAll(schema, `"`, `""`), strings.ReplaceAll(name, `"`, `""`))
}
