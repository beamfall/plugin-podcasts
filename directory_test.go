package podcasts

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// routingDoer routes a fetch to a canned response chosen by a substring of the
// request URL, and records which hosts it was asked to fetch. It lets one Doer
// stand in for both the iTunes and Podcast Index legs in a single Discover call.
type routingDoer struct {
	itunesBody   []byte
	itunesErr    error
	podindexBody []byte
	podindexErr  error
	hits         []string
}

func (d *routingDoer) FetchDocument(_ context.Context, rawURL string, _ map[string]string) ([]byte, error) {
	d.hits = append(d.hits, rawURL)
	switch {
	case strings.Contains(rawURL, "itunes.apple.com"):
		return d.itunesBody, d.itunesErr
	case strings.Contains(rawURL, "api.podcastindex.org"):
		return d.podindexBody, d.podindexErr
	default:
		return nil, errors.New("routingDoer: unexpected host: " + rawURL)
	}
}

const itunesTwoHits = `{"results":[
	{"collectionName":"Apple One","feedUrl":"https://a.example/feed.xml","artworkUrl600":"https://a.example/art.jpg"},
	{"collectionName":"Shared Show","feedUrl":"https://shared.example/feed.xml","artworkUrl600":"https://shared.example/art.jpg"}
]}`

const podindexTwoHits = `{"status":"true","feeds":[
	{"title":"Index One","url":"https://p.example/feed.xml","image":"https://p.example/art.jpg","description":"pi"},
	{"title":"Shared Show","url":"https://shared.example/feed.xml","image":"","description":"dup"}
]}`

// TestDiscoverQueriesBothDirectoriesAndMerges is the acceptance
// check: with credentials present, Discover queries BOTH the iTunes and Podcast
// Index directories and returns the merged, feed-URL-deduplicated results.
func TestDiscoverQueriesBothDirectoriesAndMerges(t *testing.T) {
	doer := &routingDoer{itunesBody: []byte(itunesTwoHits), podindexBody: []byte(podindexTwoHits)}
	dir := NewDirectory("k", "s3cr3t", doer)

	feeds, err := dir.Discover(context.Background(), "go time")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Both directories must have been queried.
	var hitITunes, hitPodindex bool
	for _, u := range doer.hits {
		hitITunes = hitITunes || strings.Contains(u, "itunes.apple.com")
		hitPodindex = hitPodindex || strings.Contains(u, "api.podcastindex.org")
	}
	if !hitITunes {
		t.Error("Discover did not query the iTunes directory")
	}
	if !hitPodindex {
		t.Error("Discover did not query the Podcast Index directory")
	}

	// Merged + deduped by feed URL: apple-one, shared-show (once), index-one = 3.
	byURL := map[string]Feed{}
	for _, f := range feeds {
		if _, dup := byURL[f.FeedURL]; dup {
			t.Fatalf("feed URL %q appeared twice; dedupe failed", f.FeedURL)
		}
		byURL[f.FeedURL] = f
	}
	for _, want := range []string{
		"https://a.example/feed.xml",      // iTunes-only
		"https://p.example/feed.xml",      // Podcast Index-only
		"https://shared.example/feed.xml", // present in both, merged to one
	} {
		if _, ok := byURL[want]; !ok {
			t.Errorf("merged results missing feed %q", want)
		}
	}
	if len(feeds) != 3 {
		t.Fatalf("got %d merged feeds, want 3", len(feeds))
	}
	// The shared show keeps the iTunes hit (first-seen wins): its title/art are
	// the iTunes ones, not the Podcast Index duplicate's.
	if shared := byURL["https://shared.example/feed.xml"]; shared.Description == "dup" {
		t.Error("shared feed took the Podcast Index duplicate; iTunes hit should win")
	}
}

// TestDiscoverKeylessITunesOnly verifies the mandatory keyless leg: without
// Podcast Index credentials, Discover still returns the iTunes results and never
// touches the Podcast Index host.
func TestDiscoverKeylessITunesOnly(t *testing.T) {
	doer := &routingDoer{itunesBody: []byte(itunesTwoHits)}
	dir := NewDirectory("", "", doer)

	feeds, err := dir.Discover(context.Background(), "q")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(feeds) != 2 {
		t.Fatalf("got %d feeds, want 2 (iTunes only)", len(feeds))
	}
	for _, u := range doer.hits {
		if strings.Contains(u, "api.podcastindex.org") {
			t.Fatalf("queried Podcast Index without credentials: %q", u)
		}
	}
}

// TestDiscoverITunesFailureSurfaces verifies a failure of the mandatory iTunes
// leg surfaces as an error rather than silently returning empty.
func TestDiscoverITunesFailureSurfaces(t *testing.T) {
	wantErr := errors.New("ssrf: blocked")
	doer := &routingDoer{itunesErr: wantErr}
	dir := NewDirectory("k", "s", doer)
	if _, err := dir.Discover(context.Background(), "q"); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want it to wrap %v", err, wantErr)
	}
}

// TestDiscoverPodcastIndexFailureIsBestEffort verifies a failing optional
// Podcast Index leg never fails discovery — the keyless iTunes results still
// return.
func TestDiscoverPodcastIndexFailureIsBestEffort(t *testing.T) {
	doer := &routingDoer{
		itunesBody:  []byte(itunesTwoHits),
		podindexErr: errors.New("podindex upstream 500"),
	}
	dir := NewDirectory("k", "s", doer)

	feeds, err := dir.Discover(context.Background(), "q")
	if err != nil {
		t.Fatalf("Discover surfaced an optional-leg failure: %v", err)
	}
	if len(feeds) != 2 {
		t.Fatalf("got %d feeds, want 2 (iTunes leg still returns)", len(feeds))
	}
}

// TestParseITunesResultsSkipsFeedlessHits verifies collections with no feed URL
// are dropped (not add-able).
func TestParseITunesResultsSkipsFeedlessHits(t *testing.T) {
	body := `{"results":[
		{"collectionName":"No Feed","feedUrl":"","artworkUrl600":"x"},
		{"collectionName":"Has Feed","feedUrl":"https://ok.example/feed.xml"}
	]}`
	feeds, err := parseITunesResults([]byte(body))
	if err != nil {
		t.Fatalf("parseITunesResults: %v", err)
	}
	if len(feeds) != 1 || feeds[0].FeedURL != "https://ok.example/feed.xml" {
		t.Fatalf("feeds = %+v, want the single feed-bearing hit", feeds)
	}
}

// TestParseITunesResultsSkipsUnsafeFeedURLs verifies parseITunesResults routes
// each hit's feedUrl through readSafeFeedURL: a javascript: scheme and a URL
// carrying embedded credentials are both dropped like an empty feed URL,
// rather than passed through untrusted (readSafeFeedURL semantics, opml.go).
func TestParseITunesResultsSkipsUnsafeFeedURLs(t *testing.T) {
	body := `{"results":[
		{"collectionName":"JS Scheme","feedUrl":"javascript:alert(1)"},
		{"collectionName":"Embedded Creds","feedUrl":"https://user:pass@host/feed.xml"},
		{"collectionName":"Has Feed","feedUrl":"https://ok.example/feed.xml"}
	]}`
	feeds, err := parseITunesResults([]byte(body))
	if err != nil {
		t.Fatalf("parseITunesResults: %v", err)
	}
	if len(feeds) != 1 || feeds[0].FeedURL != "https://ok.example/feed.xml" {
		t.Fatalf("feeds = %+v, want only the single safe feed-bearing hit", feeds)
	}
}
