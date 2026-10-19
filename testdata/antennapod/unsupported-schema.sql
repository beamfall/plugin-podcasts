-- Synthetic incompatible-schema fixture — NOT a real AntennaPod release.
--
-- No real AntennaPod version (checked: release tags 1.2, 2.0.1, 3.11.4, and
-- current develop) ever shipped a FeedItems table without item_identifier
-- and read; this fixture instead represents a structurally incompatible or
-- corrupted database an operator might mistakenly point the importer at
-- (e.g. a stripped custom export, or a non-AntennaPod SQLite file with
-- coincidentally-matching table names). It documents that ReadAntennaPodState
-- fails closed with a typed, wrapped error rather than panicking or
-- returning partial/incorrect state — see
-- TestReadAntennaPodState_UnsupportedSchemaFixture in
-- antennapod_realschema_test.go. See PROVENANCE.md for the full rationale.

CREATE TABLE Feeds (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT,
	download_url TEXT
);

-- Missing item_identifier and read: any real AntennaPod export always has
-- both (see legacy-2.0.1.sql / current-3.11.4.sql), so a file shaped like
-- this cannot be a genuine AntennaPod database.
CREATE TABLE FeedItems (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT,
	feed INTEGER,
	media INTEGER
);

CREATE TABLE FeedMedia (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	feeditem INTEGER,
	duration INTEGER,
	position INTEGER
);

INSERT INTO Feeds (id, title, download_url)
VALUES (1, 'Sample Cast (incompatible)', 'https://feeds.example.test/incompatible.xml');

INSERT INTO FeedItems (id, title, feed, media)
VALUES (1, 'Incompatible Episode', 1, 11);

INSERT INTO FeedMedia (id, feeditem, duration, position)
VALUES (11, 1, 3600000, 1800000);
