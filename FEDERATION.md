# Federation

## Direction

Federation is a single core replication system that supports both:

- full server-to-server federation over ordinary IP networks
- constrained, delay-tolerant transports through external bridge adapters

Peer always means a remote `zvonilka` server. LoRa and other mesh networks are transports, not peers.

## Goals

- Support full chat federation between ordinary internet-connected servers.
- Support constrained `chat + receipts` delivery over DTN-style transports.
- Keep the federation core transport-agnostic.
- Allow multiple discovery and media strategies through policy, not hardcoded behavior.
- Treat mesh transports as untrusted and protect replication end-to-end.

## Non-Goals For V1

- Calls over constrained transports.
- Presence flood replication over constrained transports.
- Search mirroring over constrained transports.
- Media blob transfer over constrained transports.
- Tight coupling between the main server and radio protocol stacks.

## Core Model

Federation is built from three separate concepts:

- `Peer`: a remote server, its trust state, capabilities, and discovery metadata
- `Link`: a concrete delivery path to a peer with transport kind, delivery class, limits, and strategy overrides
- `Bundle`: a transportable replication unit with cursor ranges, integrity metadata, encryption metadata, compression, fragments, and acknowledgements

This replaces an event-only transport shape. Federation transports bundles, not raw business RPC calls.

## Addressing

Federation uses stable global addresses:

- users: `user@server`
- conversations: `chat:slug@server`

Local internal IDs remain internal. Federation payloads carry stable addresses plus any mapping metadata needed by the receiver.

## Replication Model

The system replicates a normalized federation event log rather than replaying internal service calls.

Supported event families:

- `conversation.created`
- `conversation.updated`
- `conversation.member_joined`
- `conversation.member_left`
- `message.created`
- `message.edited`
- `message.deleted`
- `message.reaction_added`
- `message.reaction_removed`
- `receipt.delivered`
- `receipt.read`
- `directory.user_stub_updated`
- `media.manifest_announced`

Supported conversation kinds:

- direct
- group
- channel

## Delivery Classes

The federation core distinguishes delivery classes:

- `REALTIME` for ordinary internet-connected peers
- `DELAY_TOLERANT` for intermittently connected links
- `ULTRA_CONSTRAINED` for LoRa and similar low-bandwidth transports

Different delivery classes use the same core model but different policy limits, retry behavior, fragment sizing, batching, and allowed event families.

## Security Model

Federation uses opaque ciphertext relay for end-to-end encrypted messages:

- federation does not decrypt message payloads
- ciphertext, metadata, and key-routing data are replicated as opaque envelopes
- transport security is not trusted as the sole protection layer

Every bundle must:

- be signed by the sending server federation key
- be encrypted for the receiving peer or peer-group context
- carry replay-safe identifiers bound to `bundle_id`, `link_id`, and fragment identity

Every acknowledgement must:

- be signed
- be bound to a peer, link, and cursor range
- be rejected when stale, duplicated, or outside the replay window

Mesh transports are always treated as untrusted.

## Discovery And Trust

Discovery is strategy-based and configurable. Supported modes:

- manual registration
- DNS-based lookup
- `.well-known` discovery
- bridge-announced endpoints

Discovery can create pending peers or links, but trust activation is always a separate explicit server-side decision.

## Policy Hierarchy

Federation behavior is controlled by three levels:

- global server defaults
- per-link overrides
- per-conversation overrides

The lower level can only further restrict what the higher level allows. A conversation policy must not widen link or global limits.

Configurable policy dimensions include:

- allowed delivery classes
- allowed event families
- allowed conversation kinds
- max bundle bytes
- max fragment bytes
- TTL
- retry window
- receipt behavior
- media strategy
- discovery strategy

## Media Strategy

Media behavior is policy-driven and can vary by link and conversation.

Supported strategies:

- `REFERENCE_PROXY`: send manifests and signed fetch references
- `BACKGROUND_REPLICATION`: replicate media blobs asynchronously for suitable links
- `DISABLED`: do not federate media

Expected defaults:

- ordinary internet peers may use `REFERENCE_PROXY` or `BACKGROUND_REPLICATION`
- constrained transports use references or stubs only
- constrained transports do not transfer blobs in v1

## Transport Architecture

The federation core must not embed protocol-specific logic for Meshtastic, MeshCore, or future transports.

Recommended structure:

- `internal/domain/federation` for peer, link, policy, routing, bundle, cursor, and trust logic
- `internal/app/federationworker` for outbox dispatch, inbox apply, retry, deduplication, and reassembly
- `internal/platform/federation/transport/http` for ordinary server federation
- `internal/platform/federation/transport/bridgeclient` for bridge-based constrained transports

Bridge processes are external transport adapters, for example:

- `zvonilka-federation-bridge-meshtastic`
- `zvonilka-federation-bridge-meshcore`

Bridges are responsible for:

- converting bundles to transport frames
- converting transport frames back to bundles
- transport-specific fragmentation and reassembly
- local transport session state
- health and delivery telemetry

The federation core only depends on a generic transport adapter contract.

## Constrained Transport Rules

For LoRa-class and other highly constrained links, v1 supports:

- text-oriented chat delivery
- delivery and read receipts
- policy-filtered room replication
- batching and compression
- fragment-based store-and-forward delivery
- resumable retransmission after partial transfer

For these links, v1 does not support:

- calls
- live presence streams
- media blob transfer
- high-volume sync mirroring
- unrestricted event replay

## Public API Direction

The federation public contract should evolve from peer-only RPCs to peer, link, and bundle administration.

Expected API groups:

- peer lifecycle: create, get, list, update
- link lifecycle: create, get, list, update, pause, resume, delete
- replication: push bundles, pull bundles, acknowledge bundles, get replication cursor
- inspection: preview routing policy, link health, replication status

The replication API should be bundle-centric rather than event-centric.

## Persistence Direction

Federation storage should include durable records for:

- peers
- links
- discovery endpoints
- outbox bundles
- outbox fragments
- inbox bundles
- replication cursors
- policy overrides
- bundle receipts

Each durable replication record should preserve:

- lease state
- retry state
- cursor range
- deduplication identity
- signature and encryption metadata
- fragment completeness
- transport error history

## Acceptance Criteria

The design is considered complete when:

- one federation core can serve both HTTP peers and constrained bridge transports
- ordinary peers support full chat federation for direct, group, and channel conversations
- constrained links support `chat + receipts` with policy-controlled limits
- E2EE payloads are relayed without federation-layer decryption
- media strategy is configurable by defaults, links, and conversations
- discovery supports multiple strategies without coupling trust activation to auto-discovery
- a new constrained transport can be added through a bridge adapter without changing core federation semantics
