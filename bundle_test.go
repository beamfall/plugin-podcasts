package podcasts

import (
	"testing"

	sdk "github.com/beamfall/plugin-sdk"
)

// TestBundleImporterIDs asserts the Bundle's importer dispatch keys match the
// exported ID constants and the raw slugs core's composition root keys on.
func TestBundleImporterIDs(t *testing.T) {
	b := Bundle()

	if len(b.Importers) != 3 {
		t.Fatalf("len(Bundle().Importers) = %d, want 3", len(b.Importers))
	}

	opml := b.Importers[0]
	if opml.ID != OPMLImporterID || OPMLImporterID != "opml" {
		t.Errorf("Importers[0].ID = %q, OPMLImporterID = %q, want %q", opml.ID, OPMLImporterID, "opml")
	}

	overcast := b.Importers[1]
	if overcast.ID != OvercastImporterID || OvercastImporterID != "overcast" {
		t.Errorf("Importers[1].ID = %q, OvercastImporterID = %q, want %q", overcast.ID, OvercastImporterID, "overcast")
	}

	antennapod := b.Importers[2]
	if antennapod.ID != AntennaPodImporterID || AntennaPodImporterID != "antennapod" {
		t.Errorf("Importers[2].ID = %q, AntennaPodImporterID = %q, want %q", antennapod.ID, AntennaPodImporterID, "antennapod")
	}
}

func TestBundleBrowseSections(t *testing.T) {
	b := Bundle()
	want := []string{"latest-episodes", "continue-listening", "all-episodes", "archive-search"}
	sections := b.Layouts[0].Sections
	if len(sections) != len(want) {
		t.Fatalf("browse section count = %d, want %d", len(sections), len(want))
	}
	for i, section := range sections {
		if section.ID != want[i] {
			t.Errorf("browse section %d = %q, want %q", i, section.ID, want[i])
		}
	}
}

func TestBundleLayoutsHaveNoDirectorySources(t *testing.T) {
	// Core does not resolve directory layout sources yet; lift this once it does.
	for _, layout := range Bundle().Layouts {
		for _, section := range layout.Sections {
			if section.Source.Kind == "directory" {
				t.Errorf("layout %q section %q binds a directory source", layout.ID, section.ID)
			}
		}
	}
}

func TestBundleArchiveSearchSection(t *testing.T) {
	b := Bundle()

	for _, section := range b.Layouts[0].Sections {
		if section.ID == "archive-search" {
			if section.Source.Kind != "all-items" {
				t.Fatalf("archive-search source = %#v, want all-items", section.Source)
			}
			return
		}
	}
	t.Fatal("missing archive-search section")
}

// wantBundleVersion is the release semver this head carries: the release step
// tags the head `v`+wantBundleVersion, so a bundle Version left behind by a
// tag bump fails here instead of shipping a stale identity.
const wantBundleVersion = "0.1.4"

// TestBundleVersionMatchesReleaseTag pins Bundle().Version to the semver of
// the release tag applied to this head.
func TestBundleVersionMatchesReleaseTag(t *testing.T) {
	if got := Bundle().Version; got != wantBundleVersion {
		t.Fatalf("Bundle().Version = %q, want %q", got, wantBundleVersion)
	}
}

// TestBundleCompatibleWithPinnedSDK pins the declared [SDKFloor, SDKCeiling]
// range against the SDK identity this module pins, so a bundle whose range
// excludes its own pinned SDK fails here rather than at core load time
// (sdk.CompatibleWithSDK).
func TestBundleCompatibleWithPinnedSDK(t *testing.T) {
	b := Bundle()
	if !sdk.CompatibleWithSDK(b, sdk.Version) {
		t.Fatalf("Bundle() compat range [%s, %s] excludes pinned SDK %s", b.SDKFloor, b.SDKCeiling, sdk.Version)
	}
}
