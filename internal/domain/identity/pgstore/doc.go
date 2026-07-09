/*
Package pgstore implements the identity store on top of PostgreSQL.

The package keeps transactional semantics, uniqueness checks, and stale-row
cleanup aligned with the domain store contract.
*/
package pgstore
