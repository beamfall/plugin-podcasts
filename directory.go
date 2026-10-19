// directory wires the two directory-discovery sources — the keyless iTunes
// Search catalog and the signed-auth Podcast Index 2.0 catalog — behind one
// Discover entry point that queries both and merges the results, so the server
// exposes one search endpoint over both directories.
//
// iTunes is keyless and always available; Podcast Index is the optional richer
// leg gated on operator credentials (podindex.go). Discover therefore always
// runs the iTunes leg and runs Podcast Index only when credentials are present,
// deduplicating the merged hits by feed URL so a program listed in both
// directories surfaces once. The iTunes leg mirrors the same Doer-injection +
// Feed shape as the Podcast Index client so both flow through the caller's
// SSRF gate and stay unit-testable without a network.
package podcasts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// iTunesSearchPath is the keyless iTunes Search podcast endpoint — the same
// Endpoint+SearchTemplate the declarative itunes-directory scraper pins
// (bundle.go), expressed here for the programmatic discovery path.
const iTunesSearchPath = "https://itunes.apple.com/search?media=podcast&term="

// iTunesClient searches the keyless iTunes Search directory. It carries no
// credentials (the API is keyless); the injected Doer routes the GET through
// the caller's SSRF gate, matching the Podcast Index client's shape.
type iTunesClient struct {
	doer Doer
}

// Search queries the iTunes directory by term and returns the matching feeds.
// The response nests hits under "results"; each carries collectionName (title),
// feedUrl (the RSS feed the rss scraper's {id}-is-the-URL lookup consumes),
// and artworkUrl600 (square art) — the same fields the declarative
// itunes-directory scraper maps.
func (c *iTunesClient) Search(ctx context.Context, query string) ([]Feed, error) {
	reqURL := iTunesSearchPath + url.QueryEscape(query)
	data, err := c.doer.FetchDocument(ctx, reqURL, map[string]string{"User-Agent": userAgent})
	if err != nil {
		return nil, fmt.Errorf("itunes: search request failed: %w", err)
	}
	return parseITunesResults(data)
}

// parseITunesResults decodes an iTunes Search response into Feeds. Entries whose
// feedUrl is empty (some collections have none) are skipped: a directory hit
// with no feed URL is not add-able and carries no discovery value.
func parseITunesResults(data []byte) ([]Feed, error) {
	var parsed struct {
		Results []struct {
			Title      string `json:"collectionName"`
			FeedURL    string `json:"feedUrl"`
			ArtworkURL string `json:"artworkUrl600"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("itunes: decoding search response: %w", err)
	}
	feeds := make([]Feed, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		feedURL, err := readSafeFeedURL(r.FeedURL)
		if err != nil || feedURL == "" {
			continue
		}
		feeds = append(feeds, Feed{
			Title:      r.Title,
			FeedURL:    feedURL,
			ArtworkURL: r.ArtworkURL,
		})
	}
	return feeds, nil
}

// Directory queries both directory-discovery sources. The
// iTunes leg is keyless and always present; the Podcast Index leg is added only
// when operator credentials are configured (its client reports HasCredentials).
type Directory struct {
	itunes   *iTunesClient
	podindex *Client
}

// NewDirectory builds a two-source discovery directory over one Doer. Both legs
// share the SSRF-gated fetcher; key/secret are the operator's Podcast Index
// credentials (empty disables that leg, leaving the keyless iTunes leg).
func NewDirectory(key, secret string, doer Doer) *Directory {
	return &Directory{
		itunes:   &iTunesClient{doer: doer},
		podindex: NewClient(key, secret, doer),
	}
}

// Discover searches both directories for query and returns the merged feeds,
// deduplicated by feed URL (a program listed in both directories surfaces once,
// preferring the iTunes hit for a stable order). The iTunes leg always runs;
// the Podcast Index leg runs only when credentials are present, and a failure
// of the optional Podcast Index leg never fails the whole discovery — the
// keyless iTunes results still return. A failure of the iTunes leg is surfaced.
func (d *Directory) Discover(ctx context.Context, query string) ([]Feed, error) {
	itunesFeeds, err := d.itunes.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	merged := append([]Feed(nil), itunesFeeds...)
	if d.podindex.HasCredentials() {
		merged = appendPodcastIndexLeg(ctx, d.podindex, query, merged)
	}
	return dedupeByFeedURL(merged), nil
}

// appendPodcastIndexLeg runs the optional Podcast Index search and appends its
// hits. Any failure (missing credentials, upstream error) is swallowed: the leg
// is best-effort enrichment over the mandatory iTunes results (Podcast Index is
// the richer directory, not a required one).
func appendPodcastIndexLeg(ctx context.Context, client *Client, query string, into []Feed) []Feed {
	feeds, err := client.Search(ctx, query)
	if err != nil {
		return into
	}
	return append(into, feeds...)
}

// dedupeByFeedURL collapses feeds sharing a feed URL, keeping the first seen so
// the iTunes leg wins for a program present in both directories. Feeds with an
// empty feed URL are dropped (not add-able).
func dedupeByFeedURL(feeds []Feed) []Feed {
	seen := make(map[string]struct{}, len(feeds))
	deduped := make([]Feed, 0, len(feeds))
	for _, f := range feeds {
		key := strings.TrimSpace(f.FeedURL)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, f)
	}
	return deduped
}
