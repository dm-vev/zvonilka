# zvonilka

Zvonilka is a self-hosted messenger platform skeleton.

The repository is organized as a monorepo:

- Go services hold the core domain, admin flows, bot API, and control plane.
- Rust services are reserved for realtime and media-sensitive workloads.
- Protobuf contracts are the source of truth for external and internal APIs.
- Local deployment files and architecture docs live next to the codebase.

## Repository Layout

- `cmd/` Go entrypoints for runnable services.
- `internal/` Go application and platform packages.
- `proto/contracts/` protobuf service contracts.
- `rust/` Rust workspace for realtime and media services.
- `deploy/` local infrastructure manifests.
- `docs/` architecture notes and project layout.
- `tests/` integration test placeholders.

## First Services

- `controlplane` for core domain and admin-facing orchestration.
- `gateway` for client-facing realtime edge.
- `botapi` for external bot integration.
