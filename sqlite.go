package podcasts

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// openReadOnlySQLite opens an operator-supplied AntennaPod SQLite export file
// read-only (mode=ro&immutable=1 — never writes the original).
func openReadOnlySQLite(path string) (*sql.DB, func() error, error) {
	// Percent-encode a literal '?' or '#' in the operator-supplied path before
	// splicing it into the DSN: modernc.org/sqlite splits the DSN at the first
	// '?' in the whole string, so an unescaped '?' or '#' in the path would
	// truncate or corrupt the trailing "mode=ro&immutable=1" query and could
	// silently drop the read-only guarantee. The SQLite URI filename parser
	// percent-decodes the path back to its literal bytes, so this round-trips
	// correctly; unlike url.PathEscape, it leaves '/' (and everything else)
	// untouched.
	db, err := sql.Open("sqlite", "file:"+escapeSQLiteURIPath(path)+"?mode=ro&immutable=1")
	if err != nil {
		return nil, nil, fmt.Errorf("open foreign db: %w", err)
	}
	return db, db.Close, nil
}

// escapeSQLiteURIPath percent-encodes the '?', '#', and '%' characters in a
// file path so it can be safely embedded in a SQLite file: DSN ahead of a
// query string. '?' and '#' are structurally significant to the DSN parser's
// query/fragment split; every other byte, including '/', passes through
// unchanged. '%' must also be encoded because the SQLite URI filename parser
// percent-decodes the path back to its literal bytes, so a literal "%23" or
// "%3F" already present in a real path would otherwise decode to '#' or '?'
// and resolve the wrong file.
func escapeSQLiteURIPath(path string) string {
	r := strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23")
	return r.Replace(path)
}
