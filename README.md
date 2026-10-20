<p align="center">
  <img src=".github/brand/beamfall-logo.png" alt="Beamfall" width="96" height="96">
</p>

# Beamfall Podcasts

The Podcasts plugin adds podcast libraries to
[Beamfall Core](https://github.com/beamfall/core). You can find a show,
subscribe to its feed, keep the episodes you have downloaded, and pick up each
one where you left off.

Shows hold their episodes in
publication order, hosts and guests are linked to the shows they appear on, and
topics group related shows. The plugin can import your subscriptions and
listening history from other podcast apps.

It is free and open source under [AGPL-3.0](LICENSE). Like the rest of
Beamfall, it needs no account and has no telemetry.

## Features

- The home screen has Latest Episodes, Continue Listening,
  All Episodes, and Search Archive, with titles in English and French.
- Podcast episodes keep their playback position. Other audio does not by
  default.
- Episode titles, descriptions, publish
  dates, durations, GUIDs, and artwork come straight from each show's own RSS
  feed. Durations in `HH:MM:SS`, `MM:SS`, or seconds are all understood.
- Discovery queries the keyless Apple
  Podcasts (iTunes Search) catalog every time and adds the Podcast Index
  catalog when you supply your own API key and secret. Results are merged, and a
  show listed in both appears once. If Podcast Index is unavailable, the Apple
  results still come back.
- Podcast Index requests are signed per call, so the secret itself is never
  sent. Every directory request goes through the server's SSRF-guarded fetcher;
  the plugin opens no connections of its own.
- The Beamfall Migration Wizard has three import paths:
  - OPML subscription lists, the format most podcast apps export.
  - Overcast extended OPML: subscriptions plus per-episode played,
    progress, and starred state.
  - AntennaPod database exports: per-episode played state and playback
    position, read from AntennaPod's real schema (tested against releases
    2.0.1 and 3.11.4).
- Every importer is additive. AntennaPod
  databases are opened read-only and never written. OPML input is capped in
  size, nesting depth, and outline count; feed URLs must be `http` or `https`
  and may not carry embedded credentials; malformed files are rejected with a
  clear error. Large imports stream in pages and can resume from a checkpoint.

## How it fits into Beamfall

The Podcasts plugin ships with [Beamfall Core](https://github.com/beamfall/core),
so you do not install it separately.

The plugin is written against the
[Beamfall Plugin SDK](https://github.com/beamfall/plugin-sdk)
(`github.com/beamfall/plugin-sdk`). Most of it is declarative: `Bundle()` in
`bundle.go` describes the library shape, home layout, feed and directory
scrapers, and importers, and Core does the rest. The SDK's
[plugin-authoring guide](https://github.com/beamfall/plugin-sdk/blob/main/docs/plugin-authoring.md)
explains how Core, the SDK, and plugins fit together.

| File | What it does |
| --- | --- |
| `bundle.go` | The declarative bundle: Entity Kinds, home layout, scrapers, importers. |
| `directory.go` | `Discover`: merged Apple Podcasts and Podcast Index search. |
| `podindex.go` | Podcast Index client with per-request signed authentication. |
| `source.go` | Migration Wizard sources for OPML, Overcast, and AntennaPod. |
| `opml.go`, `podimport.go` | OPML parsing into a subscription tree. |
| `overcast.go` | Overcast's per-episode state attributes. |
| `antennapod.go`, `sqlite.go` | Read-only AntennaPod database import. |

## Develop

You need Go (see `go.mod`), Python 3, and `golangci-lint`.

```sh
go test ./...          # unit, fixture, and conformance tests
script/gate.sh         # full local gate: gofmt, vet, contract tests,
                       # per-package coverage, golangci-lint, govulncheck
script/gate.sh race    # tests under the race detector
script/gate.sh fuzz    # bounded OPML fuzzing
```

`conformance_test.go` runs the SDK's conformance harness
(`sdktest.RunBundleConformance`) against the real bundle, plus checks that pin
the Entity Kind vocabulary and every scraper and importer field this plugin
relies on. CI runs the same gate in `.github/workflows/conformance.yml`.

## Contributing

Contributions need a signed Contributor License Agreement. Read
[CONTRIBUTING.md](CONTRIBUTING.md) and [CLA.md](CLA.md) before opening a pull
request.

## Security

Please report vulnerabilities privately. See [SECURITY.md](SECURITY.md).

## License

Beamfall Podcasts is licensed under the GNU Affero General Public License,
version 3 only (AGPL-3.0-only). See [LICENSE](LICENSE). Test fixtures derived
from AntennaPod carry their own attribution in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
