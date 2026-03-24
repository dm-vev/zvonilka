/*
Package conversation implements the core messenger domain.

The package owns conversation membership, message persistence, attachments,
topics, message actions, reactions, read-state, delivery events, and sync
metadata. It stays transport-agnostic so the future API layer can map requests
into these domain operations without embedding protocol details into the
business logic.
*/
package conversation
