package podcasts

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeDoer captures the request and returns a canned body/error.
type fakeDoer struct {
	gotURL     string
	gotHeaders map[string]string
	body       []byte
	err        error
	calls      int
}

func (f *fakeDoer) FetchDocument(_ context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	f.calls++
	f.gotURL = rawURL
	f.gotHeaders = headers
	return f.body, f.err
}

// TestSignedHeadersDeterministic pins the Podcast Index signing scheme against a
// fixed key+secret+clock fixture: Authorization is sha1(key+secret+unix) hex and
// the secret never leaks into the headers.
func TestSignedHeadersDeterministic(t *testing.T) {
	const (
		key    = "TESTKEY1234"
		secret = "s3cr3t-do-not-log"
	)
	// Fixed clock → deterministic signature.
	now := time.Unix(1700000000, 0).UTC()

	headers := signedHeaders(key, secret, now)

	if got := headers["X-Auth-Key"]; got != key {
		t.Errorf("X-Auth-Key = %q, want %q", got, key)
	}
	if got := headers["X-Auth-Date"]; got != "1700000000" {
		t.Errorf("X-Auth-Date = %q, want %q", got, "1700000000")
	}
	// Independently recompute the expected hash the way the API verifies it.
	want := sha1.Sum([]byte(key + secret + "1700000000"))
	if got := headers["Authorization"]; got != hex.EncodeToString(want[:]) {
		t.Errorf("Authorization = %q, want %q", got, hex.EncodeToString(want[:]))
	}
	// The secret must never travel — only the hash derived from it.
	for name, value := range headers {
		if value == secret {
			t.Fatalf("header %q leaked the raw api-secret", name)
		}
	}
	// Calling again with the same fixture is byte-identical.
	again := signedHeaders(key, secret, now)
	if again["Authorization"] != headers["Authorization"] {
		t.Errorf("signing not deterministic: %q vs %q", again["Authorization"], headers["Authorization"])
	}
}

// TestSearchParsesResults checks the byterm response is decoded into Feeds and
// that the request carried the signed-auth headers.
func TestSearchParsesResults(t *testing.T) {
	doer := &fakeDoer{body: []byte(`{
		"status": "true",
		"feeds": [
			{"title": "Show One", "url": "https://one.example/feed.xml", "image": "https://one.example/art.jpg", "description": "first"},
			{"title": "Show Two", "url": "https://two.example/feed.xml", "image": "", "description": "second"}
		],
		"count": 2
	}`)}
	const key, secret = "k", "s3cr3t-search-path"
	c := NewClient(key, secret, doer)

	feeds, err := c.Search(context.Background(), "go time")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(feeds) != 2 {
		t.Fatalf("got %d feeds, want 2", len(feeds))
	}
	if feeds[0].Title != "Show One" || feeds[0].FeedURL != "https://one.example/feed.xml" {
		t.Errorf("feed[0] = %+v", feeds[0])
	}
	if feeds[1].Description != "second" {
		t.Errorf("feed[1].Description = %q, want %q", feeds[1].Description, "second")
	}
	// Query term must be URL-escaped into the request.
	if want := "https://api.podcastindex.org/api/1.0/search/byterm?q=go+time"; doer.gotURL != want {
		t.Errorf("request URL = %q, want %q", doer.gotURL, want)
	}
	// The signed-auth headers must be present on the wire request, and the
	// Authorization value must independently verify against the wire-carried
	// X-Auth-Key/X-Auth-Date the way the API itself would recompute it — not
	// just be non-empty.
	for _, h := range []string{"X-Auth-Key", "X-Auth-Date", "Authorization"} {
		if doer.gotHeaders[h] == "" {
			t.Errorf("request missing signed-auth header %q", h)
		}
	}
	wantSum := sha1.Sum([]byte(doer.gotHeaders["X-Auth-Key"] + secret + doer.gotHeaders["X-Auth-Date"]))
	if got, want := doer.gotHeaders["Authorization"], hex.EncodeToString(wantSum[:]); got != want {
		t.Errorf("Authorization = %q, want %q (recomputed from wire X-Auth-Key/X-Auth-Date)", got, want)
	}
	// The secret must never travel on the wire — in any header value or the URL.
	for name, value := range doer.gotHeaders {
		if value == secret {
			t.Fatalf("header %q leaked the raw api-secret on the search path", name)
		}
	}
	if strings.Contains(doer.gotURL, secret) {
		t.Fatalf("request URL leaked the raw api-secret: %q", doer.gotURL)
	}
}

// TestSearchMissingCredentials verifies the missing-credentials path: Search
// returns ErrMissingCredentials and makes no request when either credential is
// absent.
func TestSearchMissingCredentials(t *testing.T) {
	cases := []struct {
		name        string
		key, secret string
	}{
		{"both empty", "", ""},
		{"no key", "", "s"},
		{"no secret", "k", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doer := &fakeDoer{body: []byte(`{"feeds":[]}`)}
			c := NewClient(tc.key, tc.secret, doer)
			if c.HasCredentials() {
				t.Fatalf("HasCredentials() = true for %s", tc.name)
			}
			_, err := c.Search(context.Background(), "anything")
			if !errors.Is(err, ErrMissingCredentials) {
				t.Fatalf("err = %v, want ErrMissingCredentials", err)
			}
			if doer.calls != 0 {
				t.Errorf("made %d fetch calls; want 0 without credentials", doer.calls)
			}
		})
	}
}

// TestSearchStatusFalse ensures a status:false response (either JSON bool or
// string form) is reported as ErrAPIStatusFalse instead of silently returning
// an empty result, so callers can distinguish "API failed" from "no matches".
func TestSearchStatusFalse(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"bool false", `{"status": false, "feeds": []}`},
		{"string false", `{"status": "false", "feeds": []}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doer := &fakeDoer{body: []byte(tc.body)}
			c := NewClient("k", "s", doer)
			feeds, err := c.Search(context.Background(), "anything")
			if !errors.Is(err, ErrAPIStatusFalse) {
				t.Fatalf("err = %v, want ErrAPIStatusFalse", err)
			}
			if feeds != nil {
				t.Errorf("feeds = %+v, want nil", feeds)
			}
		})
	}
}

// TestSearchPropagatesFetchError ensures a transport error from the SSRF-gated
// fetcher surfaces wrapped, not swallowed.
func TestSearchPropagatesFetchError(t *testing.T) {
	wantErr := errors.New("ssrf: private range blocked")
	c := NewClient("k", "s", &fakeDoer{err: wantErr})
	_, err := c.Search(context.Background(), "q")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want it to wrap %v", err, wantErr)
	}
}

// TestParseFeedsSkipsUnsafeFeedURLs verifies parseFeeds routes each hit's url
// through readSafeFeedURL: a javascript: scheme and a URL carrying embedded
// credentials are both dropped, rather than passed through untrusted
// (readSafeFeedURL semantics, opml.go).
func TestParseFeedsSkipsUnsafeFeedURLs(t *testing.T) {
	body := `{"status":"true","feeds":[
		{"title":"JS Scheme","url":"javascript:alert(1)"},
		{"title":"Embedded Creds","url":"https://user:pass@host/feed.xml"},
		{"title":"Has Feed","url":"https://ok.example/feed.xml"}
	]}`
	feeds, err := parseFeeds([]byte(body))
	if err != nil {
		t.Fatalf("parseFeeds: %v", err)
	}
	if len(feeds) != 1 || feeds[0].FeedURL != "https://ok.example/feed.xml" {
		t.Fatalf("feeds = %+v, want only the single safe feed-bearing hit", feeds)
	}
}
