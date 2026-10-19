-- Schema-faithful sanitized AntennaPod DB fixture — current stable release.
--
-- Source: AntennaPod, GPL-3.0-only, release tag 3.11.4, verified
-- identical to develop (PodDBAdapter.VERSION = 3110000), file
-- storage/database/src/main/java/de/danoeh/antennapod/storage/database/
-- PodDBAdapter.java, CREATE_TABLE_FEEDS / CREATE_TABLE_FEED_ITEMS /
-- CREATE_TABLE_FEED_MEDIA. See testdata/antennapod/PROVENANCE.md for fetch
-- details and sanitization rules; this file has no real feed URLs,
-- credentials, or identities.
--
-- Only the three tables ReadAntennaPodState reads are included (Feeds,
-- FeedItems, FeedMedia); DownloadLog/Queue/SimpleChapters/Favorites/
-- FeedImages exist in a real export but are out of scope for this importer.
-- Columns added since the 2.0.1 legacy fixture (minimal_duration_filter,
-- feed_skip_silence, tags, episode_notification, state,
-- new_episodes_action, podcastindex_*, social_interact_url) are present
-- here to prove the importer tolerates forward schema evolution — it must
-- ignore them, not require them.

CREATE TABLE Feeds (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT,
	custom_title TEXT,
	file_url TEXT,
	download_url TEXT,
	downloaded INTEGER,
	link TEXT,
	description TEXT,
	payment_link TEXT,
	last_update TEXT,
	language TEXT,
	author TEXT,
	image_url TEXT,
	type TEXT,
	feed_identifier TEXT,
	auto_download INTEGER DEFAULT 1,
	username TEXT,
	password TEXT,
	include_filter TEXT DEFAULT '',
	exclude_filter TEXT DEFAULT '',
	minimal_duration_filter INTEGER DEFAULT -1,
	keep_updated INTEGER DEFAULT 1,
	is_paged INTEGER DEFAULT 0,
	next_page_link TEXT,
	hide TEXT,
	sort_order TEXT,
	last_update_failed INTEGER DEFAULT 0,
	auto_delete_action INTEGER DEFAULT 0,
	feed_playback_speed REAL DEFAULT 1,
	feed_skip_silence INTEGER DEFAULT 0,
	feed_volume_adaption INTEGER DEFAULT 0,
	tags TEXT,
	feed_skip_intro INTEGER DEFAULT 0,
	feed_skip_ending INTEGER DEFAULT 0,
	episode_notification INTEGER DEFAULT 0,
	state INTEGER DEFAULT 1,
	new_episodes_action INTEGER DEFAULT 0
);

CREATE TABLE FeedItems (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT,
	pubDate INTEGER,
	read INTEGER,
	link TEXT,
	description TEXT,
	payment_link TEXT,
	media INTEGER,
	feed INTEGER,
	has_simple_chapters INTEGER,
	item_identifier TEXT,
	image_url TEXT,
	auto_download INTEGER,
	podcastindex_chapter_url TEXT,
	podcastindex_transcript_type TEXT,
	podcastindex_transcript_url TEXT,
	social_interact_url TEXT
);

CREATE TABLE FeedMedia (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	duration INTEGER,
	file_url TEXT,
	download_url TEXT,
	downloaded INTEGER,
	position INTEGER,
	filesize INTEGER,
	mime_type TEXT,
	playback_completion_date INTEGER,
	feeditem INTEGER,
	played_duration INTEGER,
	has_embedded_picture INTEGER,
	last_played_time INTEGER
);

-- Sanitized synthetic data: fake example.test feeds, no real identities.
INSERT INTO Feeds (id, title, download_url, link, language, author)
VALUES
	(201, 'Sample Cast (current)', 'https://feeds.example.test/current-a.xml', 'https://example.test/current-a', 'en', 'Fixture Author'),
	(202, 'Second Sample (current)', 'https://feeds.example.test/current-b.xml', 'https://example.test/current-b', 'en', 'Fixture Author');

INSERT INTO FeedItems (id, feed, title, item_identifier, read, media)
VALUES
	(1, 201, 'Current Episode Played', 'current-guid-played', 1, 11),
	(2, 201, 'Current Episode Unplayed', 'current-guid-unplayed', 0, 12),
	(3, 202, 'Current Episode New', 'current-guid-new', -1, 13);

INSERT INTO FeedMedia (id, feeditem, duration, position, filesize, mime_type)
VALUES
	(11, 1, 3600000, 2700000, 25000000, 'audio/mpeg'),
	(12, 2, 1200000, 600000, 9000000, 'audio/mpeg'),
	(13, 3, 900000, 0, 6000000, 'audio/mpeg');
