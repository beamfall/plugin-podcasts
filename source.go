package podcasts

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	sdk "github.com/beamfall/plugin-sdk"
)

const podcastImportPageItems = 256

// NewOPMLSource builds a plain OPML subscription-list import source.
func NewOPMLSource(path string) sdk.Source { //nolint:staticcheck // deliberate SDK 0.x compatibility path for pre-streaming consumers
	return opmlSource{path: path, categories: []string{sdk.CategorySubscriptions}}
}

// NewOvercastSource builds an Overcast extended-OPML import source (subscriptions + item state).
func NewOvercastSource(path string, requested []string) sdk.Source { //nolint:staticcheck // deliberate SDK 0.x compatibility path for pre-streaming consumers
	return opmlSource{
		path:         path,
		categories:   selectedCategories(requested, []string{sdk.CategorySubscriptions, sdk.CategoryItemState}),
		includeState: true,
	}
}

// NewAntennaPodSource builds an AntennaPod SQLite item-state import source.
func NewAntennaPodSource(path string, requested []string) sdk.Source { //nolint:staticcheck // deliberate SDK 0.x compatibility path for pre-streaming consumers
	return antennaPodSource{
		path:       path,
		categories: selectedCategories(requested, []string{sdk.CategoryItemState}),
	}
}

type opmlSource struct {
	path         string
	categories   []string
	includeState bool
}

func (s opmlSource) Categories() []string { return s.categories }

func (s opmlSource) Items(ctx context.Context) ([]sdk.ForeignItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := readCappedFile(s.path)
	if err != nil {
		return nil, err
	}
	items := []sdk.ForeignItem{}
	if containsString(s.categories, sdk.CategorySubscriptions) {
		doc, err := ParseOPML(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		appendSubscriptions(&items, doc.Outlines)
	}
	if s.includeState && containsString(s.categories, sdk.CategoryItemState) {
		states, err := ParseOvercastState(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		appendItemStates(&items, states)
	}
	return items, nil
}

func (s opmlSource) Stream(
	ctx context.Context,
	checkpoint string,
	emit func(context.Context, []sdk.ForeignItem, string) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	pager, err := newPodcastPageEmitter(ctx, checkpoint, emit)
	if err != nil {
		return err
	}
	if containsString(s.categories, sdk.CategorySubscriptions) {
		if err := streamOPMLFile(ctx, s.path, opmlSubscriptions, pager); err != nil {
			return err
		}
	}
	if s.includeState && containsString(s.categories, sdk.CategoryItemState) {
		if err := streamOPMLFile(ctx, s.path, opmlItemStates, pager); err != nil {
			return err
		}
	}
	return pager.finish()
}

// readCappedFile opens path and reads at most maxOPMLBytes+1 bytes, mirroring
// the decode cap ParseOPML/ParseOvercastState apply (opml.go), so an
// oversized-on-disk file is rejected before it is ever fully read into memory
// rather than only bounded at decode time.
func readCappedFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxOPMLBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxOPMLBytes {
		return nil, fmt.Errorf("opml: input exceeds %d byte limit", maxOPMLBytes)
	}
	return data, nil
}

type antennaPodSource struct {
	path       string
	categories []string
}

func (s antennaPodSource) Categories() []string { return s.categories }

func (s antennaPodSource) Items(ctx context.Context) ([]sdk.ForeignItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db, closeDB, err := openReadOnlySQLite(s.path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeDB() }()
	states, err := ReadAntennaPodState(ctx, db)
	if err != nil {
		return nil, err
	}
	items := make([]sdk.ForeignItem, 0, len(states))
	appendItemStates(&items, states)
	return items, nil
}

func (s antennaPodSource) Stream(
	ctx context.Context,
	checkpoint string,
	emit func(context.Context, []sdk.ForeignItem, string) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	pager, err := newPodcastPageEmitter(ctx, checkpoint, emit)
	if err != nil {
		return err
	}
	db, closeDB, err := openReadOnlySQLite(s.path)
	if err != nil {
		return err
	}
	defer func() { _ = closeDB() }()
	rows, err := db.QueryContext(ctx, `
		SELECT f.download_url, fi.item_identifier, fi.read, fm.position, fm.duration
		FROM FeedItems fi
		JOIN Feeds f ON f.id = fi.feed
		JOIN FeedMedia fm ON fm.feeditem = fi.id
		ORDER BY f.download_url, fi.item_identifier
	`)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrAntennaPodQueryFailed, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var feedURL, guid string
		var read, position, duration int64
		if err := rows.Scan(&feedURL, &guid, &read, &position, &duration); err != nil {
			return fmt.Errorf("%w: %w", ErrAntennaPodScanFailed, err)
		}
		state, err := antennaPodItemState(feedURL, guid, read, position, duration)
		if err != nil {
			return err
		}
		if err := pager.add(itemStateForeignItem(state)); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: %w", ErrAntennaPodRowIteration, err)
	}
	return pager.finish()
}

type opmlStreamMode uint8

const (
	opmlSubscriptions opmlStreamMode = iota
	opmlItemStates
)

func streamOPMLFile(
	ctx context.Context,
	path string,
	mode opmlStreamMode,
	pager *podcastPageEmitter,
) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > maxOPMLBytes {
		return ErrOPMLTooLarge
	}
	decoder := xml.NewDecoder(io.LimitReader(f, maxOPMLBytes+1))
	depth, bodyDepth, outlineCount := 0, 0, 0
	rootSeen := false
	feedStack := []string{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			if !rootSeen {
				return io.EOF
			}
			return nil
		}
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			depth++
			if depth > maxOPMLDepth {
				return ErrOPMLTooDeep
			}
			if !rootSeen {
				if token.Name.Local != "opml" {
					return fmt.Errorf("podcasts: XML root is not OPML")
				}
				rootSeen = true
			}
			if token.Name.Local == "body" && bodyDepth == 0 {
				bodyDepth = depth
			}
			if token.Name.Local != "outline" || bodyDepth == 0 {
				continue
			}
			outlineCount++
			if outlineCount > maxOPMLOutlines {
				return ErrOPMLTooManyOutlines
			}
			attrs := opmlStreamAttributes(token)
			feedURL := ""
			if len(feedStack) > 0 {
				feedURL = feedStack[len(feedStack)-1]
			}
			if strings.TrimSpace(attrs.xmlURL) != "" {
				feedURL, err = readSafeFeedURL(strings.TrimSpace(attrs.xmlURL))
				if err != nil {
					return err
				}
			}
			feedStack = append(feedStack, feedURL)
			item, ok, err := attrs.foreignItem(mode, feedURL)
			if err != nil {
				return err
			}
			if ok {
				if err := pager.add(item); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if token.Name.Local == "outline" && bodyDepth > 0 && len(feedStack) > 0 {
				feedStack = feedStack[:len(feedStack)-1]
			}
			if token.Name.Local == "body" && bodyDepth == depth {
				bodyDepth = 0
			}
			depth--
		}
	}
}

type opmlStreamAttrs struct {
	text, title, xmlURL, guid, played, progress, starred string
}

func opmlStreamAttributes(element xml.StartElement) opmlStreamAttrs {
	attrs := opmlStreamAttrs{}
	for _, attr := range element.Attr {
		switch attr.Name.Local {
		case "text":
			attrs.text = attr.Value
		case "title":
			attrs.title = attr.Value
		case "xmlUrl":
			attrs.xmlURL = attr.Value
		case "guid":
			attrs.guid = attr.Value
		case "played":
			attrs.played = attr.Value
		case "progress":
			attrs.progress = attr.Value
		case "starred":
			attrs.starred = attr.Value
		}
	}
	return attrs
}

func (a opmlStreamAttrs) foreignItem(
	mode opmlStreamMode,
	feedURL string,
) (sdk.ForeignItem, bool, error) {
	if mode == opmlSubscriptions {
		if strings.TrimSpace(a.xmlURL) == "" {
			return sdk.ForeignItem{}, false, nil
		}
		title := strings.TrimSpace(a.title)
		if title == "" {
			title = strings.TrimSpace(a.text)
		}
		return sdk.ForeignItem{Subscription: sdk.Subscription{
			Present: true,
			FeedURL: feedURL,
			Title:   title,
		}}, true, nil
	}
	guid := strings.TrimSpace(a.guid)
	if guid == "" {
		return sdk.ForeignItem{}, false, nil
	}
	if feedURL == "" {
		return sdk.ForeignItem{}, false, ErrOvercastMissingFeedURL
	}
	state := ItemState{FeedURL: feedURL, GUID: guid}
	if played, ok := parseBoolAttr(a.played); ok {
		state.Played = played
	} else if strings.TrimSpace(a.played) != "" {
		return sdk.ForeignItem{}, false, ErrOvercastInvalidPlayed
	}
	if progress, ok := parseFloatAttr(a.progress); ok {
		state.Progress = progress
		state.HasProgress = true
	} else if raw := strings.TrimSpace(a.progress); raw != "" {
		if _, err := strconv.ParseFloat(raw, 64); err != nil {
			return sdk.ForeignItem{}, false, fmt.Errorf("%w: %w", ErrOvercastInvalidProgress, err)
		}
	}
	if starred, ok := parseBoolAttr(a.starred); ok {
		state.Starred = starred
	} else if strings.TrimSpace(a.starred) != "" {
		return sdk.ForeignItem{}, false, ErrOvercastInvalidStarred
	}
	return itemStateForeignItem(state), true, nil
}

func antennaPodItemState(
	feedURL string,
	guid string,
	read int64,
	position int64,
	duration int64,
) (ItemState, error) {
	parsedFeedURL, err := readSafeFeedURL(feedURL)
	if err != nil {
		return ItemState{}, fmt.Errorf("%w: %w", ErrAntennaPodInvalidRow, err)
	}
	if parsedFeedURL == "" {
		return ItemState{}, fmt.Errorf("%w: empty feed URL", ErrAntennaPodInvalidRow)
	}
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return ItemState{}, fmt.Errorf("%w: empty guid", ErrAntennaPodInvalidRow)
	}
	state := ItemState{FeedURL: parsedFeedURL, GUID: guid, Played: read == 1}
	if duration > 0 {
		progress := float64(position) / float64(duration)
		if progress >= 0 && progress <= 1 {
			state.Progress = progress
			state.HasProgress = true
		}
	}
	return state, nil
}

func itemStateForeignItem(state ItemState) sdk.ForeignItem {
	return sdk.ForeignItem{ItemState: sdk.ItemState{
		Present:     true,
		FeedURL:     state.FeedURL,
		GUID:        state.GUID,
		Played:      state.Played,
		HasProgress: state.HasProgress,
		Progress:    state.Progress,
		Starred:     state.Starred,
	}}
}

type podcastPageEmitter struct {
	ctx    context.Context
	emit   func(context.Context, []sdk.ForeignItem, string) error
	resume int
	seen   int
	page   []sdk.ForeignItem
}

func newPodcastPageEmitter(
	ctx context.Context,
	checkpoint string,
	emit func(context.Context, []sdk.ForeignItem, string) error,
) (*podcastPageEmitter, error) {
	resume := 0
	var err error
	if checkpoint != "" {
		resume, err = strconv.Atoi(checkpoint)
		if err != nil || resume < 0 {
			return nil, fmt.Errorf("invalid podcast import checkpoint %q", checkpoint)
		}
	}
	return &podcastPageEmitter{
		ctx:    ctx,
		emit:   emit,
		resume: resume,
		page:   make([]sdk.ForeignItem, 0, podcastImportPageItems),
	}, nil
}

func (p *podcastPageEmitter) add(item sdk.ForeignItem) error {
	if err := p.ctx.Err(); err != nil {
		return err
	}
	p.seen++
	if p.seen <= p.resume {
		return nil
	}
	p.page = append(p.page, item)
	if len(p.page) < podcastImportPageItems {
		return nil
	}
	return p.flush()
}

func (p *podcastPageEmitter) finish() error {
	if err := p.ctx.Err(); err != nil {
		return err
	}
	if p.seen < p.resume {
		return fmt.Errorf("invalid podcast import checkpoint %d: beyond source", p.resume)
	}
	return p.flush()
}

func (p *podcastPageEmitter) flush() error {
	if len(p.page) == 0 {
		return nil
	}
	if err := p.emit(p.ctx, p.page, strconv.Itoa(p.seen)); err != nil {
		return err
	}
	if err := p.ctx.Err(); err != nil {
		return err
	}
	p.page = make([]sdk.ForeignItem, 0, podcastImportPageItems)
	return nil
}

func appendSubscriptions(items *[]sdk.ForeignItem, outlines []Outline) {
	for _, outline := range outlines {
		if strings.TrimSpace(outline.XMLURL) != "" {
			title := strings.TrimSpace(outline.Title)
			if title == "" {
				title = strings.TrimSpace(outline.Text)
			}
			*items = append(*items, sdk.ForeignItem{Subscription: sdk.Subscription{
				Present: true,
				FeedURL: outline.XMLURL,
				Title:   title,
			}})
		}
		appendSubscriptions(items, outline.Children)
	}
}

func appendItemStates(items *[]sdk.ForeignItem, states []ItemState) {
	for _, state := range states {
		*items = append(*items, sdk.ForeignItem{ItemState: sdk.ItemState{
			Present:     true,
			FeedURL:     state.FeedURL,
			GUID:        state.GUID,
			Played:      state.Played,
			HasProgress: state.HasProgress,
			Progress:    state.Progress,
			Starred:     state.Starred,
		}})
	}
}

func selectedCategories(requested, supported []string) []string {
	if len(requested) == 0 {
		return append([]string(nil), supported...)
	}
	var selected []string
	for _, category := range supported {
		if containsString(requested, category) {
			selected = append(selected, category)
		}
	}
	return selected
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
