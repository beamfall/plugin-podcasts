package podcasts

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestParseOvercastState(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Overcast state</title></head>
  <body>
    <outline text="Podcast" xmlUrl="https://feeds.example.com/a.xml">
      <outline text="Episode A" guid="guid-a" played="true" progress="0.75" starred="true"></outline>
      <outline text="Episode B" guid="guid-b" played="false" progress="0.10"></outline>
      <outline text="Ignored folder">
        <outline text="Episode C" guid="guid-c" progress="1"></outline>
      </outline>
    </outline>
  </body>
</opml>`)

	got, err := ParseOvercastState(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOvercastEpisodeState returned error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("state count = %d, want 3", len(got))
	}
	byGUID := map[string]ItemState{}
	for _, state := range got {
		byGUID[state.GUID] = state
	}
	if state := byGUID["guid-a"]; state.FeedURL != "https://feeds.example.com/a.xml" || !state.Played || !state.HasProgress || state.Progress != 0.75 || !state.Starred {
		t.Fatalf("guid-a = %+v, want feed/played/progress=0.75/starred", state)
	}
	if state := byGUID["guid-b"]; state.FeedURL != "https://feeds.example.com/a.xml" || state.Played || !state.HasProgress || state.Progress != 0.10 {
		t.Fatalf("guid-b = %+v, want unplayed/progress=0.10", state)
	}
	if state := byGUID["guid-c"]; state.FeedURL != "https://feeds.example.com/a.xml" || state.Played || !state.HasProgress || state.Progress != 1 {
		t.Fatalf("guid-c = %+v, want progress=1", state)
	}
}

func TestParseOvercastStateDoesNotEchoSecretFeedURLs(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Overcast state</title></head>
  <body>
    <outline text="Podcast" xmlUrl="https://user:token@example.test:badport/private.xml" played="true" progress="not-a-number"></outline>
  </body>
</opml>`)

	_, err := ParseOvercastState(bytes.NewReader(input))
	if err == nil {
		t.Fatal("ParseOvercastEpisodeState succeeded, want error")
	}
	if strings.Contains(err.Error(), "user:token@example.test") {
		t.Fatalf("ParseOvercastEpisodeState error echoed secret URL: %v", err)
	}
}

func TestParseOvercastStateCapsOversizedInput(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	buf.WriteString(`<opml version="2.0"><head><title>Oversized</title></head><body>` + "\n")
	buf.WriteString(`<outline text="Podcast" xmlUrl="https://feeds.example.com/a.xml">` + "\n")
	// Pad well past maxOPMLBytes with a single unterminated outline so the
	// decoder is still mid-document when the limit reader cuts it off.
	padding := strings.Repeat("x", 1024)
	for buf.Len() < maxOPMLBytes+1024 {
		buf.WriteString(`<outline text="` + padding + `" guid="g"></outline>` + "\n")
	}

	_, err := ParseOvercastState(&buf)
	if err == nil {
		t.Fatal("ParseOvercastState succeeded on oversized input, want error from truncated decode")
	}
}

func TestParseOvercastStateDistinctInvalidStateSentinels(t *testing.T) {
	cases := []struct {
		name string
		xml  string
		want error
	}{
		{
			name: "missing feed URL",
			xml: `<opml version="2.0"><head><title>t</title></head><body>
				<outline text="orphan" guid="guid-a"></outline>
			</body></opml>`,
			want: ErrOvercastMissingFeedURL,
		},
		{
			name: "invalid played",
			xml: `<opml version="2.0"><head><title>t</title></head><body>
				<outline text="feed" xmlUrl="https://feeds.example.com/a.xml">
					<outline text="ep" guid="guid-a" played="maybe"></outline>
				</outline>
			</body></opml>`,
			want: ErrOvercastInvalidPlayed,
		},
		{
			name: "invalid progress",
			xml: `<opml version="2.0"><head><title>t</title></head><body>
				<outline text="feed" xmlUrl="https://feeds.example.com/a.xml">
					<outline text="ep" guid="guid-a" progress="not-a-number"></outline>
				</outline>
			</body></opml>`,
			want: ErrOvercastInvalidProgress,
		},
		{
			name: "invalid starred",
			xml: `<opml version="2.0"><head><title>t</title></head><body>
				<outline text="feed" xmlUrl="https://feeds.example.com/a.xml">
					<outline text="ep" guid="guid-a" starred="maybe"></outline>
				</outline>
			</body></opml>`,
			want: ErrOvercastInvalidStarred,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseOvercastState(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>` + tc.xml))
			if !errors.Is(err, tc.want) {
				t.Fatalf("ParseOvercastState error = %v, want errors.Is match for %v", err, tc.want)
			}
		})
	}

	// Sanity: the four sentinels must be distinct from one another.
	sentinels := []error{ErrOvercastMissingFeedURL, ErrOvercastInvalidPlayed, ErrOvercastInvalidProgress, ErrOvercastInvalidStarred}
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && errors.Is(a, b) {
				t.Fatalf("sentinel %v unexpectedly matches %v", a, b)
			}
		}
	}
}

// TestParseOvercastStateOutOfRangeProgressIsAbsent verifies a progress value
// outside [0, 1] — including NaN, which compares false against both range
// bounds and would otherwise slip through as present progress — is treated as
// absent (HasProgress=false), not clamped and not an error: it is
// syntactically a valid float, just not a valid progress fraction.
func TestParseOvercastStateOutOfRangeProgressIsAbsent(t *testing.T) {
	cases := []struct {
		name     string
		progress string
	}{
		{"negative", "-1"},
		{"above one", "5"},
		{"nan", "NaN"},
		{"positive infinity", "+Inf"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Overcast state</title></head>
  <body>
    <outline text="Podcast" xmlUrl="https://feeds.example.com/a.xml">
      <outline text="Episode A" guid="guid-a" progress="` + tc.progress + `"></outline>
    </outline>
  </body>
</opml>`)
			got, err := ParseOvercastState(bytes.NewReader(input))
			if err != nil {
				t.Fatalf("ParseOvercastState returned error: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("state count = %d, want 1", len(got))
			}
			if got[0].HasProgress {
				t.Fatalf("HasProgress = true for out-of-range progress %q, want false", tc.progress)
			}
			if got[0].Progress != 0 {
				t.Fatalf("Progress = %v for out-of-range progress %q, want 0 (unset)", got[0].Progress, tc.progress)
			}
		})
	}
}

func TestParseOvercastStateRejectsEmbeddedCredentials(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Overcast state</title></head>
  <body>
    <outline text="Podcast" xmlUrl="https://user:token@example.test/private.xml" played="true" progress="0.5"></outline>
  </body>
</opml>`)

	_, err := ParseOvercastState(bytes.NewReader(input))
	if err == nil {
		t.Fatal("ParseOvercastEpisodeState succeeded, want error for embedded credentials")
	}
	if strings.Contains(err.Error(), "user:token@example.test") {
		t.Fatalf("ParseOvercastEpisodeState error echoed secret URL: %v", err)
	}
}
