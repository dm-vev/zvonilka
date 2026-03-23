# Architecture

## Direction

Zvonilka is split into a Go control plane and Rust realtime edge services.

## Layers

- `cmd/` contains runnable binaries only.
- `internal/app/` wires service startup and transport boundaries.
- `internal/domain/` will hold business rules and aggregate boundaries.
- `internal/platform/` will hold technical capabilities such as config, logging, storage, and messaging adapters.
- `proto/contracts/` is the contract source of truth for both client-facing and internal APIs.
- `rust/crates/` holds independent binaries for latency-sensitive workloads.

## Service Boundaries

- `controlplane` owns account lifecycle, directory, chats, groups, channels, moderation, and admin workflows.
- `gateway` is the client-facing event edge and sync entrypoint.
- `botapi` is the external automation boundary.
- `realtime-gateway`, `presence-service`, and `media-worker` are isolated Rust services reserved for high-frequency workloads.
