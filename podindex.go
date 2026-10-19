// podindex is the Podcast Index API directory-search client: discovery over
// the Podcast Index directory, the richer Podcast 2.0 catalog alongside the
// keyless iTunes Search.
//
// The Podcast Index API authenticates with a per-request signed scheme rather
// than a static bearer token: each request carries the API key in plaintext
// (X-Auth-Key), a Unix timestamp (X-Auth-Date), and an Authorization header
// holding sha1(apiKey + apiSecret + unixTime) as lowercase hex. The secret is
// never transmitted — only the hash that proves possession of it — and the
// timestamp bounds replay. The api-secret is operator config:
// it lives in the secure config surface, never in code, and is never logged.
//
// This package owns only the signing + parse; the actual HTTPS GET is done by
// an injected Doer so the request flows through the caller's SSRF
// gate (scrape.Engine.FetchDocument satisfies Doer) and so signing is
// unit-testable against a fixed clock without a network.
package podcasts

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// searchByTermPath is the Podcast Index API v1 byterm search endpoint.
const searchByTermPath = "https://api.podcastindex.org/api/1.0/search/byterm?q="

// ErrMissingCredentials is returned when Search is called without both an API
// key and secret configured. Callers treat this as "directory unavailable, use
// the keyless iTunes path" rather than an error to surface — Podcast Index is
// the optional richer directory, not the mandatory one.
var ErrMissingCredentials = errors.New("podindex: API key and secret are required")

// Doer fetches a URL with the given request headers and returns the response
// body. scrape.Engine.FetchDocument satisfies it, routing the request through
// the SSRF gate (https-only, no literal IPs, host-pinned
// redirects). Kept as an interface so the client never imports scrape.
type Doer interface {
	FetchDocument(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error)
}

// Feed is one directory search hit: a program and its RSS feed URL (the value
// the rss feed scraper's {id}-is-the-URL lookup consumes).
type Feed struct {
	Title       string
	FeedURL     string
	ArtworkURL  string
	Description string
}

// Client searches the Podcast Index directory with signed-auth requests.
type Client struct {
	key    string
	secret string
	doer   Doer
}

// NewClient builds a Podcast Index search client. key/secret are the operator's
// API credentials (secret is secure config, never code); doer performs the
// SSRF-gated fetch.
func NewClient(key, secret string, doer Doer) *Client {
	return &Client{key: key, secret: secret, doer: doer}
}

// HasCredentials reports whether both the API key and secret are present, so a
// caller can skip the Podcast Index leg cheaply before constructing a request.
func (c *Client) HasCredentials() bool {
	return c.key != "" && c.secret != ""
}

// signedHeaders builds the Podcast Index signed-auth headers for a request made
// at now. The Authorization value is sha1(apiKey + apiSecret + unixSeconds) as
// lowercase hex; X-Auth-Key carries the key in plaintext and X-Auth-Date the
// timestamp the hash was computed over. Deterministic in (key, secret, now) so
// it can be asserted against a fixed fixture. The secret never appears in the
// returned map — only the hash derived from it.
func signedHeaders(key, secret string, now time.Time) map[string]string {
	unix := strconv.FormatInt(now.Unix(), 10)
	sum := sha1.Sum([]byte(key + secret + unix))
	return map[string]string{
		"X-Auth-Key":    key,
		"X-Auth-Date":   unix,
		"Authorization": hex.EncodeToString(sum[:]),
		"User-Agent":    userAgent,
	}
}

// userAgent identifies Beamfall to the directory; the Podcast Index API
// recommends every client send a descriptive User-Agent.
const userAgent = "Beamfall/1.0"

// Search queries the directory by term and returns the matching feeds. Returns
// ErrMissingCredentials (and no request is made) when either credential is
// absent.
func (c *Client) Search(ctx context.Context, query string) ([]Feed, error) {
	if !c.HasCredentials() {
		return nil, ErrMissingCredentials
	}
	headers := signedHeaders(c.key, c.secret, time.Now().UTC())
	reqURL := searchByTermPath + url.QueryEscape(query)
	data, err := c.doer.FetchDocument(ctx, reqURL, headers)
	if err != nil {
		// Never wrap with the URL — the query is harmless, but keep the error
		// shape uniform with the rest of the directory path.
		return nil, fmt.Errorf("podindex: search request failed: %w", err)
	}
	return parseFeeds(data)
}

// ErrAPIStatusFalse is returned when the Podcast Index API responds with
// status:false, which signals a server-side failure (e.g. bad auth, upstream
// error) rather than a genuine zero-result search. Surfacing this distinctly
// keeps callers from mistaking "the API rejected the request" for "no feeds
// matched the query".
var ErrAPIStatusFalse = errors.New("podindex: API responded with status:false")

// parseFeeds decodes a byterm search response into Feeds. The Podcast Index
// schema nests results under "feeds"; each carries title, url (the RSS feed),
// image, and description. The response also carries a "status" field that the
// API sets to false (as either a JSON bool or the string "false") on failure;
// that case is reported as ErrAPIStatusFalse rather than silently returning
// zero feeds.
func parseFeeds(data []byte) ([]Feed, error) {
	var parsed struct {
		Status json.RawMessage `json:"status"`
		Feeds  []struct {
			Title       string `json:"title"`
			FeedURL     string `json:"url"`
			ArtworkURL  string `json:"image"`
			Description string `json:"description"`
		} `json:"feeds"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("podindex: decoding search response: %w", err)
	}
	if status := strings.TrimSpace(string(parsed.Status)); status == "false" || status == `"false"` {
		return nil, ErrAPIStatusFalse
	}
	feeds := make([]Feed, 0, len(parsed.Feeds))
	for _, f := range parsed.Feeds {
		feedURL, err := readSafeFeedURL(f.FeedURL)
		if err != nil || feedURL == "" {
			continue
		}
		feeds = append(feeds, Feed{
			Title:       f.Title,
			FeedURL:     feedURL,
			ArtworkURL:  f.ArtworkURL,
			Description: f.Description,
		})
	}
	return feeds, nil
}
