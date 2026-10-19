package podcasts

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOPMLRepresentativeFixtures(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		want     Document
	}{
		{
			name:     "overcast",
			filename: filepath.Join("testdata", "overcast.opml"),
			want: Document{
				Title: "Overcast subscriptions",
				Outlines: []Outline{
					{
						Text: "Daily",
						Children: []Outline{
							{Text: "Lambda Cast", XMLURL: "https://feeds.example.test/lambda.xml"},
							{Text: "Build Mode", XMLURL: "https://feeds.example.test/build.xml"},
						},
					},
				},
			},
		},
		{
			name:     "pocketcasts",
			filename: filepath.Join("testdata", "pocketcasts.opml"),
			want: Document{
				Title: "Pocket Casts subscriptions",
				Outlines: []Outline{
					{
						Text: "Tech",
						Children: []Outline{
							{Text: "Signal Path", XMLURL: "https://feeds.example.test/signal.xml"},
						},
					},
				},
			},
		},
		{
			name:     "antennapod",
			filename: filepath.Join("testdata", "antennapod.opml"),
			want: Document{
				Title: "AntennaPod subscriptions",
				Outlines: []Outline{
					{Text: "Archive", XMLURL: "https://feeds.example.test/archive.xml"},
				},
			},
		},
	}
	for _, tc := range cases {
		data, err := os.ReadFile(tc.filename)
		if err != nil {
			t.Fatalf("%s: read fixture: %v", tc.name, err)
		}
		got, err := ParseOPML(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: ParseOPML returned error: %v", tc.name, err)
		}
		if got.Title != tc.want.Title {
			t.Fatalf("%s: title = %q, want %q", tc.name, got.Title, tc.want.Title)
		}
		if len(got.Outlines) != len(tc.want.Outlines) {
			t.Fatalf("%s: outlines = %d, want %d", tc.name, len(got.Outlines), len(tc.want.Outlines))
		}
		if got.Outlines[0].Text != tc.want.Outlines[0].Text {
			t.Fatalf("%s: top outline = %+v, want %+v", tc.name, got.Outlines[0], tc.want.Outlines[0])
		}
		if len(got.Outlines[0].Children) != len(tc.want.Outlines[0].Children) {
			t.Fatalf("%s: child outlines = %d, want %d", tc.name, len(got.Outlines[0].Children), len(tc.want.Outlines[0].Children))
		}
		for i, want := range tc.want.Outlines[0].Children {
			gotChild := got.Outlines[0].Children[i]
			if gotChild.Text != want.Text || gotChild.XMLURL != want.XMLURL {
				t.Fatalf("%s: child %d = %+v, want %+v", tc.name, i, gotChild, want)
			}
		}
	}
}

func TestWriteOPMLRoundTripsNestedSubscriptions(t *testing.T) {
	doc := Document{
		Title: "Seeded subscriptions",
		Outlines: []Outline{
			{
				Text: "Fiction",
				Children: []Outline{
					{Text: "Signal Path", XMLURL: "https://feeds.example.test/signal.xml"},
				},
			},
			{
				Text: "News",
				Children: []Outline{
					{Text: "Build Mode", XMLURL: "https://feeds.example.test/build.xml"},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteOPML(&buf, doc); err != nil {
		t.Fatalf("WriteOPML returned error: %v", err)
	}
	if !strings.Contains(buf.String(), `version="2.0"`) {
		t.Fatalf("OPML output missing version: %s", buf.String())
	}

	got, err := ParseOPML(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("ParseOPML round-trip returned error: %v", err)
	}
	if got.Title != doc.Title {
		t.Fatalf("round-trip title = %q, want %q", got.Title, doc.Title)
	}
	if len(got.Outlines) != len(doc.Outlines) {
		t.Fatalf("round-trip outlines = %d, want %d", len(got.Outlines), len(doc.Outlines))
	}
}

func TestParseOPMLDoesNotEchoSecretFeedURLs(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Bad feed</title></head>
  <body>
    <outline text="Secret" xmlUrl="https://user:token@example.test:badport/private.xml"></outline>
  </body>
</opml>`)

	_, err := ParseOPML(bytes.NewReader(input))
	if err == nil {
		t.Fatal("ParseOPML succeeded, want error")
	}
	if strings.Contains(err.Error(), "user:token@example.test") {
		t.Fatalf("ParseOPML error echoed secret URL: %v", err)
	}
}

func TestParseOPMLCapsOversizedInput(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	buf.WriteString(`<opml version="2.0"><head><title>Oversized</title></head><body>` + "\n")
	// Pad well past maxOPMLBytes with a single unterminated outline so the
	// decoder is still mid-document when the limit reader cuts it off.
	padding := strings.Repeat("x", 1024)
	for buf.Len() < maxOPMLBytes+1024 {
		buf.WriteString(`<outline text="` + padding + `"></outline>` + "\n")
	}

	_, err := ParseOPML(&buf)
	if err == nil {
		t.Fatal("ParseOPML succeeded on oversized input, want error from truncated decode")
	}
}

func TestOPMLParsersRejectExcessiveDepthAndNodeCount(t *testing.T) {
	parsers := map[string]func(string) error{
		"subscriptions": func(input string) error {
			_, err := ParseOPML(strings.NewReader(input))
			return err
		},
		"overcast state": func(input string) error {
			_, err := ParseOvercastState(strings.NewReader(input))
			return err
		},
	}

	deep := `<opml><body>` + strings.Repeat(`<outline>`, maxOPMLDepth) + strings.Repeat(`</outline>`, maxOPMLDepth) + `</body></opml>`
	many := `<opml><body>` + strings.Repeat(`<outline/>`, maxOPMLOutlines+1) + `</body></opml>`
	for name, parse := range parsers {
		t.Run(name+" depth", func(t *testing.T) {
			if err := parse(deep); !errors.Is(err, ErrOPMLTooDeep) {
				t.Fatalf("depth error = %v, want ErrOPMLTooDeep", err)
			}
		})
		t.Run(name+" nodes", func(t *testing.T) {
			if err := parse(many); !errors.Is(err, ErrOPMLTooManyOutlines) {
				t.Fatalf("node error = %v, want ErrOPMLTooManyOutlines", err)
			}
		})
	}
}

// TestReadSafeFeedURLTrimsWhitespace verifies readSafeFeedURL normalizes
// surrounding whitespace before validating: the directory-search callers
// (parseFeeds in podindex.go, parseITunesResults in directory.go) pass raw
// JSON values through unmodified, so a stray leading/trailing space or
// newline must not reject an otherwise safe https feed URL.
func TestReadSafeFeedURLTrimsWhitespace(t *testing.T) {
	got, err := readSafeFeedURL("  https://feeds.example.test/signal.xml\n")
	if err != nil {
		t.Fatalf("readSafeFeedURL returned error: %v", err)
	}
	if want := "https://feeds.example.test/signal.xml"; got != want {
		t.Fatalf("readSafeFeedURL = %q, want %q", got, want)
	}
}

func TestParseOPMLRejectsEmbeddedCredentials(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Bad feed</title></head>
  <body>
    <outline text="Secret" xmlUrl="https://user:token@example.test/private.xml"></outline>
  </body>
</opml>`)

	_, err := ParseOPML(bytes.NewReader(input))
	if err == nil {
		t.Fatal("ParseOPML succeeded, want error for embedded credentials")
	}
	if strings.Contains(err.Error(), "user:token@example.test") {
		t.Fatalf("ParseOPML error echoed secret URL: %v", err)
	}
}
