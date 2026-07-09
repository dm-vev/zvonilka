/*
Package postgres opens and migrates PostgreSQL-backed storage providers.

The package owns the physical database pool, migration execution, and shared
close semantics for the logical storage providers that the catalog registers.
*/
package postgres
