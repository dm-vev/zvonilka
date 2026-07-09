# Repository Layout

This repository uses a deliberate monorepo layout.

## Rules

- Keep executable code out of the repository root.
- Keep transport contracts under `proto/contracts/`.
- Keep Go business logic under `internal/`.
- Keep Rust realtime/media code under `rust/crates/`.
- Keep deployment artifacts under `deploy/`.
- Keep cross-service operational docs under `docs/`.

## Near-Term Expansion

- Add generated protobuf output under `gen/proto/`.
- Add database migrations under `deploy/migrations/` or a dedicated `migrations/` directory once the storage tool is chosen.
- Add service-local tests next to packages, and cross-service scenarios under `tests/integration/`.
