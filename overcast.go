package podcasts

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Sentinel errors distinguishing Overcast OPML state failure paths. Callers
// can use errors.Is to tell a missing feed URL from a malformed attribute.
var (
	// ErrOvercastMissingFeedURL is returned when an item outline carries a
	// guid but no enclosing xmlUrl was found to scope it to a feed.
	ErrOvercastMissingFeedURL = errors.New("overcast: item guid has no enclosing feed URL")
	// ErrOvercastInvalidPlayed is returned when a played attribute is present
	// but not a recognized boolean value.
	ErrOvercastInvalidPlayed = errors.New("overcast: invalid played attribute")
	// ErrOvercastInvalidProgress is returned when a progress attribute is
	// present but not a parseable float.
	ErrOvercastInvalidProgress = errors.New("overcast: invalid progress attribute")
	// ErrOvercastInvalidStarred is returned when a starred attribute is
	// present but not a recognized boolean value.
	ErrOvercastInvalidStarred = errors.New("overcast: invalid starred attribute")
)

// ParseOvercastState reads Overcast's extended OPML state and returns
// feed-scoped item state rows.
func ParseOvercastState(r io.Reader) ([]ItemState, error) {
	var raw opmlDocument
	if err := xml.NewDecoder(io.LimitReader(r, maxOPMLBytes)).Decode(&raw); err != nil {
		return nil, err
	}
	var states []ItemState
	for _, outline := range raw.Body.Outlines {
		if err := collectState(outline, "", &states); err != nil {
			return nil, err
		}
	}
	return states, nil
}

func collectState(in opmlOutline, feedURL string, states *[]ItemState) error {
	if in.XMLURL != "" {
		parsed, err := readSafeFeedURL(strings.TrimSpace(in.XMLURL))
		if err != nil {
			return err
		}
		feedURL = parsed
	}
	guid := strings.TrimSpace(in.GUID)
	if guid != "" {
		if feedURL == "" {
			return ErrOvercastMissingFeedURL
		}
		state := ItemState{FeedURL: feedURL, GUID: guid}
		if played, ok := parseBoolAttr(in.Played); ok {
			state.Played = played
		} else if strings.TrimSpace(in.Played) != "" {
			return ErrOvercastInvalidPlayed
		}
		if progress, ok := parseFloatAttr(in.Progress); ok {
			state.Progress = progress
			state.HasProgress = true
		} else if raw := strings.TrimSpace(in.Progress); raw != "" {
			// A value that fails to parse as a float is malformed input; a value
			// that parses but is NaN or falls outside [0, 1] is treated as absent
			// progress (HasProgress stays false) rather than an error.
			if _, parseErr := strconv.ParseFloat(raw, 64); parseErr != nil {
				return fmt.Errorf("%w: %w", ErrOvercastInvalidProgress, parseErr)
			}
		}
		if starred, ok := parseBoolAttr(in.Starred); ok {
			state.Starred = starred
		} else if strings.TrimSpace(in.Starred) != "" {
			return ErrOvercastInvalidStarred
		}
		*states = append(*states, state)
	}
	for _, child := range in.Outlines {
		if err := collectState(child, feedURL, states); err != nil {
			return err
		}
	}
	return nil
}

func parseBoolAttr(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "yes", "1":
		return true, true
	case "false", "no", "0":
		return false, true
	default:
		return false, false
	}
}

// parseFloatAttr parses a progress attribute as a float in [0, 1]. A NaN or a
// value outside that range is treated as absent (HasProgress=false) rather
// than clamped, since an out-of-range progress is more likely a malformed
// upstream value than a valid boundary reading. NaN must be rejected
// explicitly: it compares false against both range bounds, so without the
// check a progress="NaN" attribute would flow through as present progress.
func parseFloatAttr(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	if math.IsNaN(value) || value < 0 || value > 1 {
		return 0, false
	}
	return value, true
}
