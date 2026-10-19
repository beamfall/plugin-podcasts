# AntennaPod fixture provenance

Fixtures in this directory back `antennapod_realschema_test.go`, which exercises
`ReadAntennaPodState` (via the production `NewAntennaPodSource` read-only entry
point) against schema-faithful reconstructions of AntennaPod's real SQLite
export, complementing the hand-minimized schema in `antennapod_test.go`.

## Sources

Schemas were reconstructed by hand from AntennaPod's public `GPL-3.0-only`
source at <https://github.com/AntennaPod/AntennaPod>, fetched 2026-07-10.
Copyright is held by the AntennaPod contributors identified in that repository's
history. Redistribution details are recorded in the repository-root
`THIRD_PARTY_NOTICES.md`.

- `legacy-2.0.1.sql` — release tag `2.0.1`, commit
  [`e1ff4c8763c604c014a62c4c4905737cb92ebebb`](https://github.com/AntennaPod/AntennaPod/tree/e1ff4c8763c604c014a62c4c4905737cb92ebebb)
  (`PodDBAdapter.VERSION = 1090001`), file
  `core/src/main/java/de/danoeh/antennapod/core/storage/PodDBAdapter.java`,
  the `CREATE_TABLE_FEEDS` / `CREATE_TABLE_FEED_ITEMS` / `CREATE_TABLE_FEED_MEDIA`
  constants.
- `current-3.11.4.sql` — release tag `3.11.4`, commit
  [`6c3492af7239b46e8c9933ff04984fcdd07c3a6f`](https://github.com/AntennaPod/AntennaPod/tree/6c3492af7239b46e8c9933ff04984fcdd07c3a6f)
  (`PodDBAdapter.VERSION = 3110000`), file
  `storage/database/src/main/java/de/danoeh/antennapod/storage/database/PodDBAdapter.java`.
  Verified byte-identical schema section to `develop` commit
  [`af577fef0848fe853d1ff960284e9c792f050712`](https://github.com/AntennaPod/AntennaPod/tree/af577fef0848fe853d1ff960284e9c792f050712),
  the branch HEAD at fetch time, so this fixture also represents that pinned
  upstream schema.
- `unsupported-schema.sql` — **not** a real AntennaPod release. A synthetic
  incompatible/corrupted-export fixture; see the comment header in that file.

Only the three tables `ReadAntennaPodState` reads (`Feeds`, `FeedItems`,
`FeedMedia`) are reproduced; other real tables (`DownloadLog`, `Queue`,
`SimpleChapters`, `Favorites`, `FeedImages`) are out of scope for this
importer and intentionally omitted to keep fixtures small.

## Fixture digests

SHA-256 digests cover the retained fixture bytes, including their notice
headers:

| File | SHA-256 |
| --- | --- |
| `current-3.11.4.sql` | `0530100cf948eab462e6c6f6d9bf5164e37c274e7144e646664907336fdd41ef` |
| `legacy-2.0.1.sql` | `527cee53b6d27c19e513cf913cbc7a79c58f7b712f9fbede7581224a486f5aad` |
| `unsupported-schema.sql` | `673fcef171c54e2f0cd94323acc398e367985cd9bf8343d12b91510aceeb09be` |

Recheck with:

```sh
shasum -a 256 testdata/antennapod/*.sql
```

## Licensing

The fixtures contain three reconstructed interoperability-schema constants plus
synthetic rows. GPLv3/AGPLv3 section 13 permits combining them with this
`AGPL-3.0-only` plugin, and a clean-room replacement would reduce their
regression value without resolving any compatibility conflict, so they are
retained with the notices in `THIRD_PARTY_NOTICES.md`.

## Sanitization

All data in these fixtures is synthetic:

- Feed/episode URLs use the `example.test` / `feeds.example.test` reserved
  domains (RFC 2606) — no real feed URLs.
- Titles, authors, and GUIDs are fixture placeholders — no real identities,
  usernames, or passwords, even though the real `Feeds` table has
  `username`/`password` columns (present in the schema for faithfulness,
  never populated here).
- No fixture was built by exporting or sanitizing an actual user database;
  each is a from-scratch reconstruction of the public upstream schema.

## Why schema-faithful fixtures

Building these fixtures surfaced a real defect: `ReadAntennaPodState`'s query
once read `fi.guid` and `fm.played`. Neither column exists in any real AntennaPod release checked (tags `1.2`,
`2.0.1`, `3.11.4`, and `develop`) — the real columns are
`FeedItems.item_identifier` (the RSS guid) and `FeedItems.read` (tri-state:
`NEW=-1`, `UNPLAYED=0`, `PLAYED=1`; the play flag lives on `FeedItems`, not
`FeedMedia`). Every genuine AntennaPod export would have failed the import
with `ErrAntennaPodQueryFailed` ("no such column"). `antennapod.go` now
queries the real columns and treats only `read == 1` as played (a naive `!= 0`
check would have misclassified `NEW(-1)` items as played).

## Unsupported-schema behavior

`unsupported-schema.sql` documents the importer's fail-closed behavior when
pointed at a file that is not a genuine AntennaPod database: the query fails
with `no such column: fi.item_identifier` (or similar), wrapped as
`ErrAntennaPodQueryFailed`. No partial state is returned and no write is
attempted — `openReadOnlySQLite` opens every fixture, including this one,
with `mode=ro&immutable=1`.

## Licensing sign-off

On 2026-10-19 the Beamfall operator reviewed the GPL-3.0-only AntennaPod-derived
fixtures (`legacy-2.0.1.sql` and `current-3.11.4.sql`) and the notice in
`THIRD_PARTY_NOTICES.md`, and accepted their inclusion in this AGPL-3.0-only
repository under GPLv3/AGPLv3 section 13.
