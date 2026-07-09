/*
Package pgstore persists presence snapshots in PostgreSQL.

The package stores explicit presence settings while deriving last-seen from the
identity store at read time. This keeps the table small and avoids duplicating
session activity state.
*/
package pgstore
