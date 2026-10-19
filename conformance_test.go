// Package podcasts_test pins the podcasts bundle against the SDK contract
// vectors. Any regression in the
// declarative Bundle will cause this test to fail before it reaches CI.
package podcasts_test

import (
	"reflect"
	"testing"

	podcasts "github.com/beamfall/plugin-podcasts"
	sdktest "github.com/beamfall/plugin-sdk/sdktest"
)

func TestBundleConformance(t *testing.T) {
	sdktest.RunBundleConformance(t, podcasts.Bundle())
}

// TestBundleKindVocab pins the shipping bundle's EntityKind vocabulary (name +
// archetype, in order) to the SDK's published podcast vocabulary via its
// cross-repo conformance helper. Unlike the plugin-sdk's own vocab test, which
// can only assert against a synthetic vector because it cannot import a
// concrete plugin, this test constructs the real shipping bundle — so a rename
// or archetype change in bundle.go reds this repo's own gate.
func TestBundleKindVocab(t *testing.T) {
	sdktest.RequireEntityKindVocab(t, podcasts.Bundle(), sdktest.PodcastADR0030KindVocab)
}

func TestBundleKindNames(t *testing.T) {
	b := podcasts.Bundle()

	got := make([]string, 0, len(b.EntityKinds))
	for _, kind := range b.EntityKinds {
		got = append(got, kind.Name)
	}

	want := []string{"show", "person", "topic"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Bundle().EntityKinds names = %v, want %v", got, want)
	}
}

// TestBundleStructuralFields is a defense-in-depth structural check on this bundle's
// own ScraperSpec/ImporterSpec declarations, independent of how deep the
// shared sdktest.RunBundleConformance harness happens to be. It fails if a
// future edit to bundle.go silently drops, blanks, or mismaps a field this
// plugin's scrapers/importers rely on to function (an empty Endpoint, a
// missing LookupTemplate, a FieldMap entry pointed at the wrong response
// path, an Importer with no Categories, …), and passes on the bundle as
// declared today. Map fields (FieldMap/ArtworkMap/Transforms) are compared
// key-for-key via reflect.DeepEqual, not just by size, so a malformed value
// at an unchanged map length still fails.
func TestBundleStructuralFields(t *testing.T) {
	b := podcasts.Bundle()

	// scraperFields is the comparable projection of ScraperSpec's
	// contractual structural fields this bundle relies on (ID, DisplayName,
	// Kind, Endpoint, Lookup/Search/CandidateID templates+paths, and the
	// FieldMap/ArtworkMap/Transforms contents), scoped to this repo so the
	// test needs no SDK change.
	type scraperFields struct {
		id, displayName, kind                    string
		endpoint, lookupTemplate, searchTemplate string
		candidatesPath, candidateIDPath          string
		fieldMap, artworkMap, transforms         map[string]string
	}
	scrapers := map[string]scraperFields{}
	for _, s := range b.Scrapers {
		scrapers[s.ID] = scraperFields{
			id: s.ID, displayName: s.DisplayName["en"], kind: s.Kind,
			endpoint: s.Endpoint, lookupTemplate: s.LookupTemplate, searchTemplate: s.SearchTemplate,
			candidatesPath: s.CandidatesPath, candidateIDPath: s.CandidateIDPath,
			fieldMap: s.FieldMap, artworkMap: s.ArtworkMap, transforms: s.Transforms,
		}
	}

	wantScrapers := map[string]scraperFields{
		"feed": {
			id: "feed", displayName: "Podcast Feed (RSS)", kind: "rss",
			endpoint: "https://feed.invalid", lookupTemplate: "{id}",
			fieldMap: map[string]string{
				"title":          "item.title",
				"summary":        "item.description",
				"media.date":     "item.pubDate",
				"media.duration": "item.itunes:duration",
				"media.guid":     "item.guid",
			},
			artworkMap: map[string]string{
				"poster": "channel.itunes:image.href",
				"thumb":  "item.itunes:image.href",
			},
			transforms: map[string]string{
				"media.date":     "date-from-rfc822",
				"media.duration": "seconds-from-clock",
			},
		},
		"itunes-directory": {
			id: "itunes-directory", displayName: "Apple Podcasts Directory", kind: "http-json",
			endpoint:        "https://itunes.apple.com",
			searchTemplate:  "/search?media=podcast&term={query}",
			candidatesPath:  "results",
			candidateIDPath: "feedUrl",
			fieldMap:        map[string]string{"title": "collectionName"},
			artworkMap:      map[string]string{"poster": "artworkUrl600"},
		},
	}

	for id, want := range wantScrapers {
		got, ok := scrapers[id]
		if !ok {
			t.Errorf("Bundle().Scrapers is missing scraper %q", id)
			continue
		}
		if got.id != want.id || got.displayName != want.displayName || got.kind != want.kind ||
			got.endpoint != want.endpoint || got.lookupTemplate != want.lookupTemplate ||
			got.searchTemplate != want.searchTemplate || got.candidatesPath != want.candidatesPath ||
			got.candidateIDPath != want.candidateIDPath {
			t.Errorf("Scrapers[%s] scalar fields = %+v, want %+v", id, got, want)
		}
		if !reflect.DeepEqual(got.fieldMap, want.fieldMap) {
			t.Errorf("Scrapers[%s].FieldMap = %v, want %v", id, got.fieldMap, want.fieldMap)
		}
		if !reflect.DeepEqual(got.artworkMap, want.artworkMap) {
			t.Errorf("Scrapers[%s].ArtworkMap = %v, want %v", id, got.artworkMap, want.artworkMap)
		}
		if !reflect.DeepEqual(got.transforms, want.transforms) {
			t.Errorf("Scrapers[%s].Transforms = %v, want %v", id, got.transforms, want.transforms)
		}
	}
	if len(scrapers) != len(wantScrapers) {
		t.Errorf("Bundle().Scrapers has %d entries, want %d", len(scrapers), len(wantScrapers))
	}

	// importerFields is the comparable projection of ImporterSpec's
	// contractual structural fields: ID, English DisplayName, Categories
	// (full slice, order-sensitive per the declared vocabulary), Input form,
	// and Conflict strategy.
	type importerFields struct {
		id, displayNameEN, input, conflict string
		categories                         []string
	}
	importers := map[string]importerFields{}
	for _, imp := range b.Importers {
		importers[imp.ID] = importerFields{
			id: imp.ID, displayNameEN: imp.DisplayName["en"],
			categories: imp.Categories,
			input:      imp.Input, conflict: imp.Conflict,
		}
	}

	wantImporters := map[string]importerFields{
		"opml":       {id: "opml", displayNameEN: "OPML", categories: []string{"subscriptions"}, input: "export-file", conflict: "additive"},
		"overcast":   {id: "overcast", displayNameEN: "Overcast", categories: []string{"subscriptions", "item-state"}, input: "export-file", conflict: "additive"},
		"antennapod": {id: "antennapod", displayNameEN: "AntennaPod", categories: []string{"item-state"}, input: "export-file", conflict: "additive"},
	}

	for id, want := range wantImporters {
		got, ok := importers[id]
		if !ok {
			t.Errorf("Bundle().Importers is missing importer %q", id)
			continue
		}
		if got.id != want.id || got.displayNameEN != want.displayNameEN ||
			got.input != want.input || got.conflict != want.conflict {
			t.Errorf("Importers[%s] scalar fields = %+v, want %+v", id, got, want)
		}
		if !reflect.DeepEqual(got.categories, want.categories) {
			t.Errorf("Importers[%s].Categories = %v, want %v", id, got.categories, want.categories)
		}
	}
	if len(importers) != len(wantImporters) {
		t.Errorf("Bundle().Importers has %d entries, want %d", len(importers), len(wantImporters))
	}
}
