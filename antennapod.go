package podcasts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// rowQuerier is the subset of *sql.DB (or *sql.Conn/*sql.Tx) ReadAntennaPodState needs.
type rowQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Sentinel errors distinguishing AntennaPod driver failure paths. Callers can
// use errors.Is to tell a missing database from a malformed one, and errors.Unwrap
// to reach the underlying driver/scan error.
var (
	// ErrAntennaPodDatabaseRequired is returned when a nil database handle is provided.
	ErrAntennaPodDatabaseRequired = errors.New("antennapod: database is required")
	// ErrAntennaPodQueryFailed is returned when the state query itself fails.
	ErrAntennaPodQueryFailed = errors.New("antennapod: state query failed")
	// ErrAntennaPodScanFailed is returned when scanning a result row fails.
	ErrAntennaPodScanFailed = errors.New("antennapod: row scan failed")
	// ErrAntennaPodInvalidRow is returned when a row's feed URL or GUID fails validation.
	ErrAntennaPodInvalidRow = errors.New("antennapod: invalid feed URL or GUID")
	// ErrAntennaPodRowIteration is returned when row iteration ends in an error.
	ErrAntennaPodRowIteration = errors.New("antennapod: row iteration failed")
)

// ReadAntennaPodState reads AntennaPod state rows and returns feed-scoped rows.
//
// Column names match AntennaPod's real PodDBAdapter schema (verified against
// the public AntennaPod source at release tags 2.0.1 and 3.11.4, and current
// develop — VERSION 1090001 and 3110000 respectively): the RSS guid lives in
// FeedItems.item_identifier (there is no FeedItems.guid), and play state
// lives in FeedItems.read (there is no FeedMedia.played). FeedItems.read is
// tri-state — NEW(-1), UNPLAYED(0), PLAYED(1) — so only the exact value 1
// counts as played; a naive "nonzero" check would misclassify NEW(-1) items
// as played.
func ReadAntennaPodState(ctx context.Context, db rowQuerier) ([]ItemState, error) {
	if db == nil {
		return nil, ErrAntennaPodDatabaseRequired
	}
	rows, err := db.QueryContext(ctx, `
		SELECT f.download_url, fi.item_identifier, fi.read, fm.position, fm.duration
		FROM FeedItems fi
		JOIN Feeds f ON f.id = fi.feed
		JOIN FeedMedia fm ON fm.feeditem = fi.id
		ORDER BY f.download_url, fi.item_identifier
	`)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAntennaPodQueryFailed, err)
	}
	defer func() { _ = rows.Close() }()

	var states []ItemState
	for rows.Next() {
		var feedURL string
		var guid string
		var read int64
		var position int64
		var duration int64
		if err := rows.Scan(&feedURL, &guid, &read, &position, &duration); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrAntennaPodScanFailed, err)
		}
		parsedFeedURL, err := readSafeFeedURL(feedURL)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrAntennaPodInvalidRow, err)
		}
		if parsedFeedURL == "" {
			return nil, fmt.Errorf("%w: empty feed URL", ErrAntennaPodInvalidRow)
		}
		// Trim the guid so it matches the Overcast path's GUID handling
		// (collectState, overcast.go) — the two sources feed the same
		// feed-URL+GUID replay ledger, so a padded guid must not dodge
		// duplicate detection against the same GUID from the feed itself.
		guid = strings.TrimSpace(guid)
		if guid == "" {
			return nil, fmt.Errorf("%w: empty guid", ErrAntennaPodInvalidRow)
		}
		state := ItemState{FeedURL: parsedFeedURL, GUID: guid, Played: read == 1}
		// A position/duration pair whose ratio falls outside [0, 1] (a corrupt
		// or truncated row) is treated as absent progress rather than exported
		// unclamped — the same policy the Overcast path applies to an
		// out-of-range progress attribute (parseFloatAttr, overcast.go).
		if duration > 0 {
			progress := float64(position) / float64(duration)
			if progress >= 0 && progress <= 1 {
				state.Progress = progress
				state.HasProgress = true
			}
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAntennaPodRowIteration, err)
	}
	return states, nil
}
