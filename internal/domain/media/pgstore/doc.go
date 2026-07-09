/*
Package pgstore persists media metadata in PostgreSQL.

The package stores media reservations, readiness state, and object-key mapping
so the media service can combine database-backed metadata with S3-compatible
blob storage.
*/
package pgstore
