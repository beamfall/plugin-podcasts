// Package podcasts is the first-party Podcast Domain Plugin bundle: a pure
// declarative contribution — Entity Kinds, a home Layout, and declarative
// scrapers. The plugin's full scope is a podcast archive (downloaded-only
// Media, Subscription vs Follow, the Play Queue); this bundle is the
// declarative slice the plugin contract supports today, following the same
// shape as the audio plugin.
//
// Shape: a show is a container with ordered children (publication order);
// episodes are Media ("Episode" is a display label, never a schema name). People (hosts, guests) are one person-archetype kind, with
// hosting/guesting expressed as relations-with-attributes, and topics are a
// flat category kind.
package podcasts

import sdk "github.com/beamfall/plugin-sdk"

// OPMLImporterID is the podcasts plugin's OPML Migration Wizard importer
// slug — the (Bundle().ID, OPMLImporterID) pair core's migrationSourceResolver
// dispatches on.
const OPMLImporterID = "opml"

// OvercastImporterID is the podcasts plugin's Overcast extended-OPML
// Migration Wizard importer slug.
const OvercastImporterID = "overcast"

// AntennaPodImporterID is the podcasts plugin's AntennaPod DB-export
// Migration Wizard importer slug.
const AntennaPodImporterID = "antennapod"

// Bundle returns the Podcast plugin's declarative contribution.
func Bundle() sdk.Bundle {
	return sdk.Bundle{
		ID:           "podcasts",
		Version:      "0.1.4",
		SDKFloor:     "0.2.0",
		SDKCeiling:   "0.3.2",
		DisplayName:  sdk.LocaleString{"en": "Podcasts", "fr": "Balados"},
		MediaLabel:   sdk.LocaleString{"en": "Episode", "fr": "Épisode"},
		MediaClasses: []string{"audio"},
		// Episode (Media) fields the feed scraper routes through the
		// "media." namespace (episodes carry pubDate, duration, and the
		// episode GUID). A bare FieldMap key no-ops at identification;
		// "media.<name>" persists onto the episode Media row.
		MediaFields: []sdk.FieldSpec{
			{Name: "date", Type: "date",
				Label: sdk.LocaleString{"en": "Published", "fr": "Publié le"}},
			{Name: "duration", Type: "number",
				Label: sdk.LocaleString{"en": "Duration", "fr": "Durée"}},
			{Name: "guid", Type: "text",
				Label: sdk.LocaleString{"en": "Episode GUID", "fr": "GUID de l'épisode"}},
			{Name: "transcript", Type: "text",
				Label: sdk.LocaleString{"en": "Transcript", "fr": "Transcription"}},
		},
		// Podcast episodes resume mid-play — the one place the audio
		// no-resume default is wrong (resume eligibility is declarative,
		// not a media-class convention).
		Engagement: sdk.Engagement{ResumeMedia: boolPtr(true)},
		EntityKinds: []sdk.EntityKind{
			{
				// The feed/program. Children are episodes (Media) in
				// publication order — OrderedChildren carries the
				// play-next semantics (oldest-first for serial shows
				// is a Source concern, not structure).
				Name:            "show",
				DisplayName:     sdk.LocaleString{"en": "Show", "fr": "Émission"},
				Archetype:       "container",
				OrderedChildren: true,
				Fields: []sdk.FieldSpec{
					{Name: "feed-url", Type: "url",
						Label: sdk.LocaleString{"en": "Feed URL", "fr": "URL du flux"}},
					{Name: "website", Type: "url",
						Label: sdk.LocaleString{"en": "Website", "fr": "Site web"}},
					{Name: "publisher", Type: "text",
						Label: sdk.LocaleString{"en": "Publisher", "fr": "Éditeur"}},
					{Name: "summary", Type: "text",
						Label: sdk.LocaleString{"en": "Summary", "fr": "Résumé"}},
					// itunes:type — serial shows want oldest-first.
					{Name: "serial", Type: "flag",
						Label: sdk.LocaleString{"en": "Serial", "fr": "Sériel"}},
					{Name: "explicit", Type: "flag",
						Label: sdk.LocaleString{"en": "Explicit", "fr": "Explicite"}},
				},
				Relations: []sdk.RelationSpec{
					// Hosting: a standing role on the show.
					{Name: "hosted-by", ToKind: "person", Attributes: []string{"role", "since"}},
					// Guesting: per-appearance. Relations target
					// declared kinds, so the episode is carried as
					// attributes (episode-guid).
					{Name: "guested-by", ToKind: "person", Attributes: []string{"role", "episode-guid"}},
					{Name: "about", ToKind: "topic"},
				},
			},
			{
				// Hosts and guests share one kind; the relation
				// (hosted-by vs guested-by) is what distinguishes them.
				Name:        "person",
				DisplayName: sdk.LocaleString{"en": "Person", "fr": "Personne"},
				Archetype:   "person",
				Fields: []sdk.FieldSpec{
					{Name: "bio", Type: "text",
						Label: sdk.LocaleString{"en": "Biography", "fr": "Biographie"}},
					{Name: "website", Type: "url",
						Label: sdk.LocaleString{"en": "Website", "fr": "Site web"}},
				},
			},
			{
				// Flat topic taxonomy (Apple genre vocabulary seeds it).
				Name:        "topic",
				DisplayName: sdk.LocaleString{"en": "Topic", "fr": "Sujet"},
				Archetype:   "category",
			},
		},
		// Section titles use the full locale-map form: en + fr at launch
		// (further locales can be added without a schema change).
		Layouts: []sdk.LayoutSpec{{ID: "home", Role: "browse", Sections: []sdk.SectionSpec{
			{
				ID:          "latest-episodes",
				Kind:        "shelf",
				Title:       sdk.LocaleString{"en": "Latest Episodes", "fr": "Derniers épisodes"},
				ArtworkSlot: "poster",
				Source:      sdk.SourceSpec{Kind: "recently-added"},
			},
			{
				// Backed by the Engagement.ResumeMedia override above:
				// podcast audio stores resume positions.
				ID:          "continue-listening",
				Kind:        "shelf",
				Title:       sdk.LocaleString{"en": "Continue Listening", "fr": "Reprendre l'écoute"},
				ArtworkSlot: "thumb",
				Source:      sdk.SourceSpec{Kind: "continue-watching"},
			},
			{
				ID:          "all-episodes",
				Kind:        "grid",
				Title:       sdk.LocaleString{"en": "All Episodes", "fr": "Tous les épisodes"},
				ArtworkSlot: "poster",
				Source:      sdk.SourceSpec{Kind: "all-items"},
			},
			{
				ID:          "archive-search",
				Kind:        "list",
				Title:       sdk.LocaleString{"en": "Search Archive", "fr": "Rechercher dans l'archive"},
				ArtworkSlot: "thumb",
				Source:      sdk.SourceSpec{Kind: "all-items"},
			},
		}}},
		Scrapers: []sdk.ScraperSpec{
			{
				// The feed is the scraper: the subscription's own RSS
				// is the metadata source for subscribed shows. {id} is
				// the absolute feed URL, so LookupTemplate "{id}" makes
				// the runtime use the feed URL directly — Endpoint is
				// unused for rss-kind scrapers and is set to a sentinel
				// value. Directory search uses the itunes-directory
				// scraper below.
				// Both scrapers are retained: rss for episode metadata,
				// itunes-directory for catalog search.
				ID:             "feed",
				DisplayName:    sdk.L("Podcast Feed (RSS)"),
				Kind:           "rss",
				Endpoint:       "https://feed.invalid",
				LookupTemplate: "{id}",
				FieldMap: map[string]string{
					// Episode (Media) fields. title/summary are core built-ins;
					// date/duration/guid route through the "media." namespace
					// to the declared MediaFields above.
					"title":   "item.title",
					"summary": "item.description",
					// pubDate is RFC-822; itunes:duration is "HH:MM:SS",
					// "MM:SS", or seconds — both normalized by transforms.
					"media.date":     "item.pubDate",
					"media.duration": "item.itunes:duration",
					"media.guid":     "item.guid",
					// Show (container entity) fields (channel.title,
					// channel.description, channel.itunes:author) belong on the
					// `show` Entity, which FieldMap cannot target — entity
					// hydration is EntityMap's job and entity-field population
					// is a later layer. They are intentionally
					// not mapped here rather than silently no-op'd.
				},
				Transforms: map[string]string{
					"media.date":     "date-from-rfc822",
					"media.duration": "seconds-from-clock",
				},
				ArtworkMap: map[string]string{
					// Show-level square art fills poster; per-episode
					// art (rare) fills thumb.
					"poster": "channel.itunes:image.href",
					"thumb":  "item.itunes:image.href",
				},
			},
			{
				// Directory search: iTunes Search API — keyless,
				// rate-limited, the de-facto podcast catalog. A real
				// search→candidates declaration: each `results` element is
				// one show, mapped element-relative, and the candidate "id"
				// is its feed URL — the value the rss feed scraper's
				// {id}-is-the-URL lookup consumes. Search-only by nature.
				ID:              "itunes-directory",
				DisplayName:     sdk.L("Apple Podcasts Directory"),
				Kind:            "http-json",
				Endpoint:        "https://itunes.apple.com",
				SearchTemplate:  "/search?media=podcast&term={query}",
				CandidatesPath:  "results",
				CandidateIDPath: "feedUrl",
				FieldMap: map[string]string{
					// The core "title" field so identification candidates
					// render with a name (each candidate IS a program). This
					// is a search-only directory: candidates carry only the
					// built-in candidate fields, so artistName/feedUrl/
					// primaryGenreName (publisher/feed-url/topic) have no
					// FieldMap route — they describe the `show` Entity, not the
					// episode Media, and entity-field population is a later
					// layer. Left unmapped rather than no-op'd.
					"title": "collectionName",
				},
				ArtworkMap: map[string]string{
					"poster": "artworkUrl600",
				},
			},
		},
		// Migration Wizard importers. OPML and Overcast's extended OPML both
		// carry feed URLs that can create Library subscriptions and follow them
		// for the importing Profile. The parsers (opml.go, overcast.go,
		// antennapod.go) have no store dependencies; the sdk.Source
		// implementations in source.go adapt them into migration items.
		//
		// Overcast extended OPML and AntennaPod DB exports also carry feed-scoped
		// item-state rows. The Core handler records those rows in the generic
		// subscription/GUID replay ledger; OPML remains subscription-only.
		Importers: []sdk.ImporterSpec{
			{
				ID:          OPMLImporterID,
				DisplayName: sdk.LocaleString{"en": "OPML", "fr": "OPML"},
				Categories:  []string{"subscriptions"},
				Input:       "export-file",
				Conflict:    "additive",
			},
			{
				ID:          OvercastImporterID,
				DisplayName: sdk.LocaleString{"en": "Overcast", "fr": "Overcast"},
				Categories:  []string{"subscriptions", "item-state"},
				Input:       "export-file",
				Conflict:    "additive",
			},
			{
				ID:          AntennaPodImporterID,
				DisplayName: sdk.LocaleString{"en": "AntennaPod", "fr": "AntennaPod"},
				Categories:  []string{"item-state"},
				Input:       "export-file",
				Conflict:    "additive",
			},
		},
	}
}

func boolPtr(value bool) *bool {
	return &value
}
