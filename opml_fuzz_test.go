package podcasts

import (
	"bytes"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

const atPathOPML = `<?xml version="1.0"?><opml version="2.0"><body><outline text="At path" xmlUrl="https://feeds.example.test/a@b.xml"/></body></opml>`

func validateFuzzFeedURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("feed URL did not parse")
	}
	if parsed.User != nil {
		return errors.New("feed URL contains credentials")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("feed URL has unsafe scheme")
	}
	if parsed.Host == "" {
		return errors.New("feed URL has no host")
	}
	return nil
}

func TestFuzzFeedURLInvariant(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "at in path", raw: "https://feeds.example.test/a@b.xml"},
		{name: "credentials", raw: "https://user:token@feeds.example.test/feed.xml", wantErr: true},
		{name: "unsafe scheme", raw: "ftp://feeds.example.test/feed.xml", wantErr: true},
		{name: "missing host", raw: "https:///feed.xml", wantErr: true},
		{name: "parse failure", raw: "https://%zz", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateFuzzFeedURL(test.raw); (err != nil) != test.wantErr {
				t.Fatalf("validateFuzzFeedURL() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

// FuzzParseOPML exercises ParseOPML against malformed, oversized, and deeply
// nested OPML/feed inputs. It must never panic and must never echo a raw
// feed URL (including any embedded credentials) back in an error message.
func FuzzParseOPML(f *testing.F) {
	seedDirs := []string{
		filepath.Join("testdata"),
		filepath.Join("testdata", "adversarial"),
	}
	for _, dir := range seedDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			f.Fatalf("read seed dir %s: %v", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				f.Fatalf("read seed %s: %v", entry.Name(), err)
			}
			f.Add(data)
		}
	}
	f.Add([]byte(atPathOPML))

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ParseOPML panicked on input: %v", r)
			}
		}()
		doc, err := ParseOPML(bytes.NewReader(data))
		if err != nil {
			if bytes.Contains(data, []byte("user:token@")) && bytes.Contains([]byte(err.Error()), []byte("user:token@")) {
				t.Fatalf("ParseOPML error echoed embedded credentials: %v", err)
			}
			return
		}
		// A successful parse must not surface a feed URL with embedded
		// credentials or a non-http(s) scheme.
		var walk func(outlines []Outline)
		walk = func(outlines []Outline) {
			for _, o := range outlines {
				if o.XMLURL != "" {
					if err := validateFuzzFeedURL(o.XMLURL); err != nil {
						t.Fatalf("ParseOPML returned an unsafe feed URL: %v", err)
					}
				}
				walk(o.Children)
			}
		}
		walk(doc.Outlines)
	})
}
