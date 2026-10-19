package podcasts

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestReadAntennaPodState(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "antennapod.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := []string{
		`CREATE TABLE Feeds (id INTEGER PRIMARY KEY, download_url TEXT NOT NULL)`,
		`CREATE TABLE FeedItems (id INTEGER PRIMARY KEY, feed INTEGER NOT NULL, item_identifier TEXT NOT NULL, read INTEGER NOT NULL)`,
		`CREATE TABLE FeedMedia (id INTEGER PRIMARY KEY, feeditem INTEGER NOT NULL, position INTEGER NOT NULL, duration INTEGER NOT NULL)`,
		`INSERT INTO Feeds(id, download_url) VALUES (101, 'https://feeds.example.com/a.xml')`,
		`INSERT INTO FeedItems(id, feed, item_identifier, read) VALUES (1, 101, 'guid-a', 1), (2, 101, 'guid-b', 0)`,
		`INSERT INTO FeedMedia(id, feeditem, position, duration) VALUES (11, 1, 180, 240), (12, 2, 20, 200)`,
	}
	for _, stmt := range schema {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("seed statement %q failed: %v", stmt, err)
		}
	}

	got, err := ReadAntennaPodState(context.Background(), db)
	if err != nil {
		t.Fatalf("ReadAntennaPodEpisodeState returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("state count = %d, want 2", len(got))
	}
	byGUID := map[string]ItemState{}
	for _, state := range got {
		byGUID[state.GUID] = state
	}
	if state := byGUID["guid-a"]; state.FeedURL != "https://feeds.example.com/a.xml" || !state.Played || !state.HasProgress || state.Progress != 0.75 {
		t.Fatalf("guid-a = %+v, want played/progress=0.75", state)
	}
	if state := byGUID["guid-b"]; state.FeedURL != "https://feeds.example.com/a.xml" || state.Played || !state.HasProgress || state.Progress != 0.10 {
		t.Fatalf("guid-b = %+v, want unplayed/progress=0.10", state)
	}
}

// TestReadAntennaPodStateNewIsNotPlayed verifies FeedItems.read is treated
// as tri-state, matching AntennaPod's real FeedItem model (NEW=-1,
// UNPLAYED=0, PLAYED=1): only read=1 counts as played. A naive "nonzero"
// check would misclassify a NEW(-1) item — one the user has never opened —
// as played.
func TestReadAntennaPodStateNewIsNotPlayed(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "antennapod.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := []string{
		`CREATE TABLE Feeds (id INTEGER PRIMARY KEY, download_url TEXT NOT NULL)`,
		`CREATE TABLE FeedItems (id INTEGER PRIMARY KEY, feed INTEGER NOT NULL, item_identifier TEXT NOT NULL, read INTEGER NOT NULL)`,
		`CREATE TABLE FeedMedia (id INTEGER PRIMARY KEY, feeditem INTEGER NOT NULL, position INTEGER NOT NULL, duration INTEGER NOT NULL)`,
		`INSERT INTO Feeds(id, download_url) VALUES (101, 'https://feeds.example.com/a.xml')`,
		`INSERT INTO FeedItems(id, feed, item_identifier, read) VALUES (1, 101, 'guid-new', -1)`,
		`INSERT INTO FeedMedia(id, feeditem, position, duration) VALUES (11, 1, 0, 240)`,
	}
	for _, stmt := range schema {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("seed statement %q failed: %v", stmt, err)
		}
	}

	got, err := ReadAntennaPodState(context.Background(), db)
	if err != nil {
		t.Fatalf("ReadAntennaPodState returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("state count = %d, want 1", len(got))
	}
	if got[0].Played {
		t.Fatalf("NEW(-1) item state = %+v, want Played=false", got[0])
	}
}

// TestReadAntennaPodStateNormalizesCorruptRows verifies two Overcast-parity
// behaviors on hostile/corrupt export rows: a position/duration ratio outside
// [0, 1] (position past the duration, or negative) yields HasProgress=false
// rather than an unclamped Progress, and a whitespace-padded guid is trimmed
// so it cannot dodge feed-URL+GUID duplicate detection in the replay ledger.
func TestReadAntennaPodStateNormalizesCorruptRows(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "antennapod.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := []string{
		`CREATE TABLE Feeds (id INTEGER PRIMARY KEY, download_url TEXT NOT NULL)`,
		`CREATE TABLE FeedItems (id INTEGER PRIMARY KEY, feed INTEGER NOT NULL, item_identifier TEXT NOT NULL, read INTEGER NOT NULL)`,
		`CREATE TABLE FeedMedia (id INTEGER PRIMARY KEY, feeditem INTEGER NOT NULL, position INTEGER NOT NULL, duration INTEGER NOT NULL)`,
		`INSERT INTO Feeds(id, download_url) VALUES (101, 'https://feeds.example.com/a.xml')`,
		`INSERT INTO FeedItems(id, feed, item_identifier, read) VALUES (1, 101, '  guid-over  ', 0), (2, 101, 'guid-negative', 0)`,
		`INSERT INTO FeedMedia(id, feeditem, position, duration) VALUES (11, 1, 500, 200), (12, 2, -20, 200)`,
	}
	for _, stmt := range schema {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("seed statement %q failed: %v", stmt, err)
		}
	}

	got, err := ReadAntennaPodState(context.Background(), db)
	if err != nil {
		t.Fatalf("ReadAntennaPodState returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("state count = %d, want 2", len(got))
	}
	byGUID := map[string]ItemState{}
	for _, state := range got {
		byGUID[state.GUID] = state
	}
	over, ok := byGUID["guid-over"]
	if !ok {
		t.Fatalf("padded guid was not trimmed to %q; states = %+v", "guid-over", got)
	}
	if over.HasProgress || over.Progress != 0 {
		t.Fatalf("guid-over = %+v, want absent progress for position > duration", over)
	}
	negative := byGUID["guid-negative"]
	if negative.HasProgress || negative.Progress != 0 {
		t.Fatalf("guid-negative = %+v, want absent progress for negative position", negative)
	}
}

func TestReadAntennaPodStateWrappedErrors(t *testing.T) {
	if _, err := ReadAntennaPodState(context.Background(), nil); !errors.Is(err, ErrAntennaPodDatabaseRequired) {
		t.Fatalf("nil db: err = %v, want errors.Is ErrAntennaPodDatabaseRequired", err)
	}

	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "antennapod.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := ReadAntennaPodState(context.Background(), db); !errors.Is(err, ErrAntennaPodQueryFailed) {
		t.Fatalf("missing tables: err = %v, want errors.Is ErrAntennaPodQueryFailed", err)
	}

	schema := []string{
		`CREATE TABLE Feeds (id INTEGER PRIMARY KEY, download_url TEXT NOT NULL)`,
		`CREATE TABLE FeedItems (id INTEGER PRIMARY KEY, feed INTEGER NOT NULL, item_identifier TEXT NOT NULL, read INTEGER NOT NULL)`,
		`CREATE TABLE FeedMedia (id INTEGER PRIMARY KEY, feeditem INTEGER NOT NULL, position INTEGER NOT NULL, duration INTEGER NOT NULL)`,
		`INSERT INTO Feeds(id, download_url) VALUES (101, 'not a url')`,
		`INSERT INTO FeedItems(id, feed, item_identifier, read) VALUES (1, 101, 'guid-a', 1)`,
		`INSERT INTO FeedMedia(id, feeditem, position, duration) VALUES (11, 1, 180, 240)`,
	}
	for _, stmt := range schema {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("seed statement %q failed: %v", stmt, err)
		}
	}
	if _, err := ReadAntennaPodState(context.Background(), db); !errors.Is(err, ErrAntennaPodInvalidRow) {
		t.Fatalf("invalid feed URL: err = %v, want errors.Is ErrAntennaPodInvalidRow", err)
	}

	emptyGUIDDB, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "antennapod-empty-guid.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = emptyGUIDDB.Close() })
	emptyGUIDSchema := []string{
		`CREATE TABLE Feeds (id INTEGER PRIMARY KEY, download_url TEXT NOT NULL)`,
		`CREATE TABLE FeedItems (id INTEGER PRIMARY KEY, feed INTEGER NOT NULL, item_identifier TEXT NOT NULL, read INTEGER NOT NULL)`,
		`CREATE TABLE FeedMedia (id INTEGER PRIMARY KEY, feeditem INTEGER NOT NULL, position INTEGER NOT NULL, duration INTEGER NOT NULL)`,
		`INSERT INTO Feeds(id, download_url) VALUES (201, 'https://feeds.example.com/b.xml')`,
		`INSERT INTO FeedItems(id, feed, item_identifier, read) VALUES (2, 201, '', 0)`,
		`INSERT INTO FeedMedia(id, feeditem, position, duration) VALUES (12, 2, 20, 200)`,
	}
	for _, stmt := range emptyGUIDSchema {
		if _, err := emptyGUIDDB.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("seed statement %q failed: %v", stmt, err)
		}
	}
	if _, err := ReadAntennaPodState(context.Background(), emptyGUIDDB); !errors.Is(err, ErrAntennaPodInvalidRow) {
		t.Fatalf("empty guid: err = %v, want errors.Is ErrAntennaPodInvalidRow", err)
	}
}
