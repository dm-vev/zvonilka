/*
Package storage builds storage catalogs from configured provider factories.

The package bridges the platform configuration layer and the domain storage
catalog so services can assemble multiple logical providers, verify bindings,
and close partially constructed providers safely on failure.
*/
package storage
