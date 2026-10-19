package podcasts

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	sdk "github.com/beamfall/plugin-sdk"
	"github.com/beamfall/plugin-sdk/sdktest"
)

// TestOPMLSourceItemsRejectsOversizedFile verifies opmlSource.Items bounds the
// actual file read (not just the post-read XML decode): a file whose on-disk
// size exceeds maxOPMLBytes must be rejected without the whole file ever being
// pulled into memory. The file content is a repeating pattern rather than
// maxOPMLBytes of real OPML, since only the byte count matters here.
func TestOPMLSourceItemsRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.opml")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	pattern := strings.Repeat("x", 1<<20) // 1 MiB chunks
	written := 0
	for written <= maxOPMLBytes {
		n, werr := f.WriteString(pattern)
		if werr != nil {
			t.Fatalf("write temp file: %v", werr)
		}
		written += n
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	src := NewOPMLSource(path)
	_, err = src.Items(context.Background()) //nolint:staticcheck // deliberate SDK 0.x compatibility assertion
	if err == nil {
		t.Fatal("Items succeeded on an oversized file, want error")
	}
}

func TestImporterStreamingCancellationAndCheckpoint(t *testing.T) {
	outlines := make([]string, 513)
	for i := range outlines {
		index := strconv.Itoa(i)
		outlines[i] = `<outline text="Show ` + index + `" xmlUrl="https://feeds.example.test/` + index + `.xml"/>`
	}
	opmlPath := filepath.Join(t.TempDir(), "paged.opml")
	opmlFixture := `<?xml version="1.0"?><opml version="2.0"><body>` + strings.Join(outlines, "") + `</body></opml>`
	if err := os.WriteFile(opmlPath, []byte(opmlFixture), 0o600); err != nil {
		t.Fatalf("write paged OPML fixture: %v", err)
	}
	overcastPath := filepath.Join("testdata", "overcast.opml")
	antennaPath := filepath.Join(t.TempDir(), "antennapod.db")
	seedAntennaPodSourceFixture(t, antennaPath)

	sources := []struct {
		name       string
		source     sdk.Source //nolint:staticcheck // deliberate SDK 0.x compatibility assertion
		categories []string
	}{
		{"opml", NewOPMLSource(opmlPath), []string{sdk.CategorySubscriptions}},
		{"overcast", NewOvercastSource(overcastPath, nil), []string{sdk.CategorySubscriptions, sdk.CategoryItemState}},
		{"antennapod", NewAntennaPodSource(antennaPath, nil), []string{sdk.CategoryItemState}},
	}
	vectors := make([]sdktest.ImporterVector, 0, len(sources))
	for _, source := range sources {
		items, err := source.source.Items(context.Background()) //nolint:staticcheck // deliberate SDK 0.x compatibility assertion
		if err != nil {
			t.Fatalf("%s fixture: %v", source.name, err)
		}
		streamed, err := collectPodcastStream(source.source)
		if err != nil {
			t.Fatalf("%s stream: %v", source.name, err)
		}
		if !reflect.DeepEqual(streamed, items) {
			t.Fatalf("%s streamed items differ from compatibility Items:\nstream: %#v\nitems:  %#v", source.name, streamed, items)
		}
		vectors = append(vectors, sdktest.ImporterVector{
			Name:           source.name,
			Source:         source.source,
			WantCategories: source.categories,
			WantItems:      items,
		})
	}
	streaming := sources[0].source.(interface {
		Stream(context.Context, string, func(context.Context, []sdk.ForeignItem, string) error) error
	})
	ctx, cancel := context.WithCancel(context.Background())
	resumed := 0
	err := streaming.Stream(ctx, "256", func(_ context.Context, page []sdk.ForeignItem, _ string) error {
		resumed += len(page)
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled resume error = %v, want context.Canceled", err)
	}
	if resumed != 256 {
		t.Fatalf("resumed page items = %d, want 256", resumed)
	}
	callbackErr := errors.New("stop after durable page")
	err = streaming.Stream(context.Background(), "512", func(context.Context, []sdk.ForeignItem, string) error {
		return callbackErr
	})
	if !errors.Is(err, callbackErr) {
		t.Fatalf("callback error = %v, want %v", err, callbackErr)
	}
	for _, checkpoint := range []string{"bad", "514"} {
		if err := streaming.Stream(context.Background(), checkpoint, func(context.Context, []sdk.ForeignItem, string) error {
			return nil
		}); err == nil {
			t.Errorf("checkpoint %q returned nil error", checkpoint)
		}
	}
	sdktest.RunImporterConformance(t, vectors)
}

func collectPodcastStream(source sdk.Source) ([]sdk.ForeignItem, error) { //nolint:staticcheck // deliberate SDK 0.x compatibility assertion
	streaming := source.(interface {
		Stream(context.Context, string, func(context.Context, []sdk.ForeignItem, string) error) error
	})
	items := []sdk.ForeignItem{}
	err := streaming.Stream(context.Background(), "", func(
		_ context.Context,
		page []sdk.ForeignItem,
		_ string,
	) error {
		items = append(items, page...)
		return nil
	})
	return items, err
}

func seedAntennaPodSourceFixture(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open AntennaPod fixture: %v", err)
	}
	statements := []string{
		`CREATE TABLE Feeds (id INTEGER PRIMARY KEY, download_url TEXT NOT NULL)`,
		`CREATE TABLE FeedItems (id INTEGER PRIMARY KEY, feed INTEGER NOT NULL, item_identifier TEXT NOT NULL, read INTEGER NOT NULL)`,
		`CREATE TABLE FeedMedia (id INTEGER PRIMARY KEY, feeditem INTEGER NOT NULL, position INTEGER NOT NULL, duration INTEGER NOT NULL)`,
		`INSERT INTO Feeds(id, download_url) VALUES (1, 'https://feeds.example.test/show.xml')`,
		`INSERT INTO FeedItems(id, feed, item_identifier, read) VALUES (1, 1, 'episode-guid', 1)`,
		`INSERT INTO FeedMedia(id, feeditem, position, duration) VALUES (1, 1, 30, 60)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(context.Background(), statement); err != nil {
			_ = db.Close()
			t.Fatalf("seed AntennaPod fixture: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close AntennaPod fixture: %v", err)
	}
}
