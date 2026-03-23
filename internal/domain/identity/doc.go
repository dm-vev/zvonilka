/*
	Package identity implements the platform's account, login, session, and idempotency rules.

The package keeps the domain semantics close to the core types instead of spreading them
across transport handlers or persistence adapters. That gives the service a single place
for retry behavior, normalization, and rollback policy.

The important invariants are:
  - user-facing identifiers are normalized before lookups and writes;
  - idempotency is scoped per operation and per request fingerprint;
  - approval retries may recover an already persisted account instead of creating a second one;
  - multi-step writes run inside store-managed transactions so the service can keep
    commit and rollback behavior inside the storage boundary.
*/
package identity
