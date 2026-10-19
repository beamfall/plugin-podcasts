-- Schema-faithful sanitized AntennaPod DB fixture — legacy release.
--
-- Source: AntennaPod, GPL-3.0-only, release tag 2.0.1
-- (PodDBAdapter.VERSION = 1090001), file
-- core/src/main/java/de/danoeh/antennapod/core/storage/PodDBAdapter.java,
-- CREATE_TABLE_FEEDS / CREATE_TABLE_FEED_ITEMS / CREATE_TABLE_FEED_MEDIA.
-- See testdata/antennapod/PROVENANCE.md for fetch details and sanitization
-- rules; this file has no real feed URLs, credentials, or identities.
--
-- Only the three tables ReadAntennaPodState reads are included (Feeds,
-- FeedItems, FeedMedia); DownloadLog/Queue/SimpleChapters/Favorites/
-- FeedImages exist in a real export but are out of scope for this importer.

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
	keep_updated INTEGER DEFAULT 1,
	is_paged INTEGER DEFAULT 0,
	next_page_link TEXT,
	hide TEXT,
	sort_order TEXT,
	last_update_failed INTEGER DEFAULT 0,
	auto_delete_action INTEGER DEFAULT 0,
	feed_playback_speed REAL DEFAULT 1,
	feed_volume_adaption INTEGER DEFAULT 0,
	feed_skip_intro INTEGER DEFAULT 0,
	feed_skip_ending INTEGER DEFAULT 0
);

CREATE TABLE FeedItems (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT,
	content_encoded TEXT,
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
	auto_download INTEGER
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
	(101, 'Sample Cast (legacy)', 'https://feeds.example.test/legacy-a.xml', 'https://example.test/legacy-a', 'en', 'Fixture Author'),
	(102, 'Second Sample (legacy)', 'https://feeds.example.test/legacy-b.xml', 'https://example.test/legacy-b', 'en', 'Fixture Author');

INSERT INTO FeedItems (id, feed, title, item_identifier, read, media)
VALUES
	(1, 101, 'Legacy Episode Played', 'legacy-guid-played', 1, 11),
	(2, 101, 'Legacy Episode Unplayed', 'legacy-guid-unplayed', 0, 12),
	(3, 102, 'Legacy Episode New', 'legacy-guid-new', -1, 13);

INSERT INTO FeedMedia (id, feeditem, duration, position, filesize, mime_type)
VALUES
	(11, 1, 3600000, 1800000, 25000000, 'audio/mpeg'),
	(12, 2, 1200000, 300000, 9000000, 'audio/mpeg'),
	(13, 3, 900000, 0, 6000000, 'audio/mpeg');
