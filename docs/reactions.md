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

The current seed contains eight reactions. Each reaction has its own 100x100
WebP icon extracted from the ordinary Telegram Android client's
`assets/emoji/<section>_<index>.png` files, with the client's alpha masks
applied. The picker fields use that matching icon, while
`shared/custom_emoji_reaction.tgs` is the Telegram Lottie activation effect.
TGS and WebM assets are supported by the validator and can replace the static
picker fields when per-reaction animation files are available.
