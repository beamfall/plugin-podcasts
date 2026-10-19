package podcasts

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestOpenReadOnlySQLite_RejectsWrite verifies openReadOnlySQLite honors
// mode=ro even when the operator-supplied path contains characters
// (`?`, `&`) that would otherwise inject extra DSN query parameters and
// override mode=ro if path were interpolated unescaped.
func TestOpenReadOnlySQLite_RejectsWrite(t *testing.T) {
	real := filepath.Join(t.TempDir(), "antennapod.db")
	seed, err := sql.Open("sqlite", "file:"+real)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	if _, err := seed.ExecContext(context.Background(), `CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("seed schema: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	// Craft a path that, if concatenated unescaped, injects "&mode=rwc" and
	// would let a caller override the trailing mode=ro&immutable=1 clause.
	crafted := real + "?x=1&mode=rwc"

	db, closeFn, err := openReadOnlySQLite(crafted)
	if err != nil {
		t.Fatalf("openReadOnlySQLite: %v", err)
	}
	defer func() { _ = closeFn() }()

	if _, err := db.ExecContext(context.Background(), `INSERT INTO t (id) VALUES (1)`); err == nil {
		t.Fatalf("write succeeded against crafted path %q; mode=ro was undermined by DSN injection", crafted)
	}
}

// TestOpenReadOnlySQLite_PathWithHashAndSpace verifies openReadOnlySQLite
// opens a real database file whose path contains a '#' and a space —
// characters that are legal in filesystem paths but, like '?', are
// structurally significant to the DSN parser's query/fragment split. The
// escaper must percent-encode '#' (and leave the space and '/' alone) so the
// path round-trips through the SQLite URI filename parser intact.
//
// The fixture is created at a safe name and moved into place with a plain
// filesystem rename: seeding it directly at the '#'-bearing path would hit
// the same unescaped-DSN truncation this test guards against, in the seed
// open rather than openReadOnlySQLite.
func TestOpenReadOnlySQLite_PathWithHashAndSpace(t *testing.T) {
	dir := t.TempDir()
	safe := filepath.Join(dir, "safe.db")
	seed, err := sql.Open("sqlite", "file:"+safe)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	if _, err := seed.ExecContext(context.Background(), `CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("seed schema: %v", err)
	}
	if _, err := seed.ExecContext(context.Background(), `INSERT INTO t (id) VALUES (1)`); err != nil {
		t.Fatalf("seed row: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	real := filepath.Join(dir, "anten na#pod.db")
	if err := os.Rename(safe, real); err != nil {
		t.Fatalf("rename fixture to '#'/space path: %v", err)
	}

	db, closeFn, err := openReadOnlySQLite(real)
	if err != nil {
		t.Fatalf("openReadOnlySQLite: %v", err)
	}
	defer func() { _ = closeFn() }()

	var count int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM t`).Scan(&count); err != nil {
		t.Fatalf("read via escaped path %q: %v", real, err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

// TestOpenReadOnlySQLite_PathWithLiteralPercentHash verifies openReadOnlySQLite
// opens a real database file whose basename contains a literal "%23" and a
// space — SQLite's URI filename parser percent-decodes the path, so an
// unescaped literal "%23" would decode back to '#' and resolve the wrong
// file. The escaper must percent-encode the '%' itself so "%23" round-trips
// through the parser as the literal three characters, not as an escaped '#'.
//
// The fixture is created at a safe name and moved into place with a plain
// filesystem rename: seeding it directly at the literal-"%23" path would hit
// the same percent-decoding bug this test guards against, in the seed open
// rather than openReadOnlySQLite.
func TestOpenReadOnlySQLite_PathWithLiteralPercentHash(t *testing.T) {
	dir := t.TempDir()
	safe := filepath.Join(dir, "safe.db")
	seed, err := sql.Open("sqlite", "file:"+safe)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	if _, err := seed.ExecContext(context.Background(), `CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("seed schema: %v", err)
	}
	if _, err := seed.ExecContext(context.Background(), `INSERT INTO t (id) VALUES (1)`); err != nil {
		t.Fatalf("seed row: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	real := filepath.Join(dir, "50%23done pod.db")
	if err := os.Rename(safe, real); err != nil {
		t.Fatalf("rename fixture to literal '%%23'/space path: %v", err)
	}

	db, closeFn, err := openReadOnlySQLite(real)
	if err != nil {
		t.Fatalf("openReadOnlySQLite: %v", err)
	}
	defer func() { _ = closeFn() }()

	var count int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM t`).Scan(&count); err != nil {
		t.Fatalf("read via escaped path %q: %v", real, err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

// TestEscapeSQLiteURIPath verifies the escaper encodes only '?', '#', and
// '%', leaving every other byte — including '/' and spaces — untouched. The
// '%' cases guard against the SQLite URI filename parser percent-decoding a
// literal "%23"/"%3F" already present in a real path back into '#'/'?'; a
// single-pass strings.Replacer doesn't rescan its own output, so encoding
// '%' first does not get re-encoded by the '?'/'#' replacements that follow.
func TestEscapeSQLiteURIPath(t *testing.T) {
	cases := map[string]string{
		"/a/b/c.db":       "/a/b/c.db",
		"/a b/c.db":       "/a b/c.db",
		"/a?b.db":         "/a%3Fb.db",
		"/a#b.db":         "/a%23b.db",
		"/a?b#c d.db":     "/a%3Fb%23c d.db",
		"/a%b.db":         "/a%25b.db",
		"/50%23done/x.db": "/50%2523done/x.db",
		"/a%3Fb.db":       "/a%253Fb.db",
	}
	for in, want := range cases {
		if got := escapeSQLiteURIPath(in); got != want {
			t.Errorf("escapeSQLiteURIPath(%q) = %q, want %q", in, got, want)
		}
	}
}
