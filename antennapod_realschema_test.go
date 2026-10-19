package podcasts

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	sdk "github.com/beamfall/plugin-sdk"
	_ "modernc.org/sqlite"
)

// TestReadAntennaPodState_RealSchemaFixtures exercises the importer's
// production read-only entry point (NewAntennaPodSource, which opens the
// file via openReadOnlySQLite's mode=ro&immutable=1) against schema-faithful
// AntennaPod fixtures reconstructed from real AntennaPod releases — see
// testdata/antennapod/PROVENANCE.md. Building these fixtures surfaced a real
// bug (fixed alongside this test): the importer previously queried
// fi.guid/fm.played, columns no real AntennaPod release has ever had.
func TestReadAntennaPodState_RealSchemaFixtures(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		want    map[string]sdk.ItemState
	}{
		{
			name:    "legacy 2.0.1",
			fixture: "legacy-2.0.1.sql",
			want: map[string]sdk.ItemState{
				// position=1800000 duration=3600000 -> 0.5
				"legacy-guid-played": {Present: true, FeedURL: "https://feeds.example.test/legacy-a.xml", GUID: "legacy-guid-played", Played: true, HasProgress: true, Progress: 0.5},
				// position=300000 duration=1200000 -> 0.25
				"legacy-guid-unplayed": {Present: true, FeedURL: "https://feeds.example.test/legacy-a.xml", GUID: "legacy-guid-unplayed", Played: false, HasProgress: true, Progress: 0.25},
				// position=0 duration=900000 -> 0
				"legacy-guid-new": {Present: true, FeedURL: "https://feeds.example.test/legacy-b.xml", GUID: "legacy-guid-new", Played: false, HasProgress: true, Progress: 0},
			},
		},
		{
			name:    "current 3.11.4",
			fixture: "current-3.11.4.sql",
			want: map[string]sdk.ItemState{
				// position=2700000 duration=3600000 -> 0.75
				"current-guid-played": {Present: true, FeedURL: "https://feeds.example.test/current-a.xml", GUID: "current-guid-played", Played: true, HasProgress: true, Progress: 0.75},
				// position=600000 duration=1200000 -> 0.5
				"current-guid-unplayed": {Present: true, FeedURL: "https://feeds.example.test/current-a.xml", GUID: "current-guid-unplayed", Played: false, HasProgress: true, Progress: 0.5},
				// position=0 duration=900000 -> 0
				"current-guid-new": {Present: true, FeedURL: "https://feeds.example.test/current-b.xml", GUID: "current-guid-new", Played: false, HasProgress: true, Progress: 0},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := seedFixtureDB(t, tc.fixture)

			source := NewAntennaPodSource(path, nil)
			items, err := source.Items(context.Background()) //nolint:staticcheck // deliberate SDK 0.x compatibility assertion
			if err != nil {
				t.Fatalf("Items() against %s: %v", tc.fixture, err)
			}
			if len(items) != len(tc.want) {
				t.Fatalf("Items() returned %d items, want %d", len(items), len(tc.want))
			}

			got := map[string]sdk.ItemState{}
			for _, item := range items {
				if !item.ItemState.Present {
					t.Fatalf("item %+v: ItemState.Present = false, want true", item)
				}
				got[item.ItemState.GUID] = item.ItemState
			}
			for guid, want := range tc.want {
				state, ok := got[guid]
				if !ok {
					t.Fatalf("guid %q missing from result: %+v", guid, got)
				}
				if state != want {
					t.Fatalf("guid %q = %+v, want %+v", guid, state, want)
				}
			}
		})
	}
}

// TestReadAntennaPodState_UnsupportedSchemaFixture verifies the importer
// fails closed — a typed, wrapped error, no partial state, no write — when
// pointed at a database shaped unlike any real AntennaPod export. See
// testdata/antennapod/unsupported-schema.sql.
func TestReadAntennaPodState_UnsupportedSchemaFixture(t *testing.T) {
	path := seedFixtureDB(t, "unsupported-schema.sql")

	source := NewAntennaPodSource(path, nil)
	items, err := source.Items(context.Background()) //nolint:staticcheck // deliberate SDK 0.x compatibility assertion
	if !errors.Is(err, ErrAntennaPodQueryFailed) {
		t.Fatalf("Items() against unsupported-schema.sql: err = %v, want errors.Is ErrAntennaPodQueryFailed", err)
	}
	if items != nil {
		t.Fatalf("Items() against unsupported-schema.sql returned %d items on error, want none", len(items))
	}
}

// fixtureStringLiteral matches a single-quoted SQL string literal so the
// sanitization lint below inspects only actual data values (INSERT
// arguments), not schema identifiers or the prose in header comments (which
// legitimately references real upstream URLs like github.com/AntennaPod).
var fixtureStringLiteral = regexp.MustCompile(`'([^']*)'`)

// fixtureEmailLike matches anything that looks like an email address —
// PROVENANCE.md guarantees fixtures carry no real identities.
var fixtureEmailLike = regexp.MustCompile(`^[\w.+-]+@[\w.-]+\.[a-zA-Z]{2,}$`)

// fixtureInsertColumns matches an `INSERT INTO Feeds (...)` column list, so
// the lint can catch a future fixture edit that populates the real Feeds
// schema's username/password columns — PROVENANCE.md documents that no
// fixture ever populates them, even though the columns exist for schema
// faithfulness.
var fixtureInsertColumns = regexp.MustCompile(`(?is)INSERT\s+INTO\s+Feeds\s*\(([^)]*)\)`)

// fixtureHexToken and fixtureBase64Token match a literal that is *entirely*
// an unbroken run of hex or base64 alphabet characters, at least 20 chars
// long — the shape of a real secret (API key, bearer token, hex/base64
// blob), not fixture prose. Both are anchored full-string matches, so a
// hyphenated placeholder GUID like "legacy-guid-unplayed" (letters outside
// the hex set, and a hyphen outside both charsets) or a title with spaces
// never matches; only a solid block of hex/base64 characters does.
var fixtureHexToken = regexp.MustCompile(`^[0-9a-fA-F]{20,}$`)
var fixtureBase64Token = regexp.MustCompile(`^[A-Za-z0-9+/]{20,}={0,2}$`)

// fixtureBearerLike matches an embedded "Bearer <token>" credential inside
// an otherwise-innocuous literal (e.g. a header value or URL query string).
var fixtureBearerLike = regexp.MustCompile(`(?i)\bbearer\s+\S{10,}`)

// TestFixturesAreSanitized is a fixture safety lint: it scans every committed
// AntennaPod SQL fixture for the unsanitized/PII-like content PROVENANCE.md
// (testdata/antennapod/PROVENANCE.md, "Sanitization") claims was never
// present — real-looking email addresses, feed/episode URLs outside the
// RFC 2606 example.test reserved domain, populated username/password
// values, URL userinfo credentials (even on an allowed host), and
// token-like high-entropy secrets (hex/base64 blobs, bearer tokens) — so a
// future fixture edit that reintroduces real data fails the build instead
// of silently shipping.
func TestFixturesAreSanitized(t *testing.T) {
	dir := filepath.Join("testdata", "antennapod")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		checked++
		path := filepath.Join(dir, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(contents)

		for _, cols := range fixtureInsertColumns.FindAllStringSubmatch(text, -1) {
			lower := strings.ToLower(cols[1])
			if strings.Contains(lower, "username") || strings.Contains(lower, "password") {
				t.Errorf("%s: INSERT INTO Feeds populates username/password (%s); PROVENANCE.md says fixtures never populate these columns", path, strings.TrimSpace(cols[1]))
			}
		}

		for _, m := range fixtureStringLiteral.FindAllStringSubmatch(text, -1) {
			literal := m[1]
			if fixtureEmailLike.MatchString(literal) {
				t.Errorf("%s: fixture data literal %q looks like a real email address; PROVENANCE.md says fixtures carry no real identities", path, literal)
				continue
			}
			if fixtureHexToken.MatchString(literal) || fixtureBase64Token.MatchString(literal) {
				t.Errorf("%s: fixture data literal %q looks like a high-entropy secret token (hex/base64 blob); PROVENANCE.md says fixtures carry no secrets", path, literal)
				continue
			}
			if fixtureBearerLike.MatchString(literal) {
				t.Errorf("%s: fixture data literal %q embeds what looks like a bearer token; PROVENANCE.md says fixtures carry no secrets", path, literal)
				continue
			}
			if !strings.HasPrefix(literal, "http://") && !strings.HasPrefix(literal, "https://") {
				continue
			}
			parsed, err := url.Parse(literal)
			if err != nil {
				t.Errorf("%s: fixture URL literal %q failed to parse: %v", path, literal, err)
				continue
			}
			if parsed.User != nil {
				t.Errorf("%s: fixture URL literal %q embeds userinfo credentials in the URL; PROVENANCE.md says fixtures carry no credentials, even on an allowed host", path, literal)
				continue
			}
			host := parsed.Hostname()
			if host != "example.test" && !strings.HasSuffix(host, ".example.test") {
				t.Errorf("%s: fixture URL literal %q uses host %q, want example.test or a subdomain of it (RFC 2606 reserved domain); PROVENANCE.md says fixtures use no real feed URLs", path, literal, host)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("no .sql fixtures found under %s", dir)
	}
}

// seedFixtureDB creates a fresh writable SQLite file, executes the named
// testdata/antennapod fixture's schema+data statements against it, and
// returns the file path for a caller to open read-only through the
// production import path.
func seedFixtureDB(t *testing.T, fixtureName string) string {
	t.Helper()
	sqlBytes, err := os.ReadFile(filepath.Join("testdata", "antennapod", fixtureName))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixtureName, err)
	}

	path := filepath.Join(t.TempDir(), fixtureName+".db")
	seed, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open seed db for %s: %v", fixtureName, err)
	}
	if _, err := seed.ExecContext(context.Background(), string(sqlBytes)); err != nil {
		t.Fatalf("seed fixture %s: %v", fixtureName, err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed db for %s: %v", fixtureName, err)
	}
	return path
}
