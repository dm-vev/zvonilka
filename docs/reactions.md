# Reaction Catalog

The gateway exposes the authenticated `ConversationService/GetReactionCatalog` RPC.
The client sends its cached `known_version`; an equal version returns
`not_modified=true` with no reaction definitions.

The seed manifest is `assets/reactions/catalog.json`. It uses the existing
`MediaService` to create ready `STICKER_ASSET` media records with
`public_access=true`, then upserts the catalog in one PostgreSQL transaction.
The same file content is reused by SHA-256 on subsequent runs.

```sh
ZVONILKA_REACTION_SEED_OWNER_ACCOUNT_ID=<existing-account-id> \
  go run ./cmd/reactionseed \
  -manifest assets/reactions/catalog.json
```

Use `-replace` only when reactions absent from the manifest should be marked
inactive. Ordinary media upload RPCs cannot set `public_access`; only this
privileged seed command can do so.

The current seed contains eight reactions. It uses one own 100x100 WebP as a
temporary asset for all five required Telegram-compatible fields. TGS and WebM
assets are supported by the validator and can be added to the manifest later.
