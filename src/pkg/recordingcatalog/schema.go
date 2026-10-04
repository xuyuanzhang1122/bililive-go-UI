package recordingcatalog

const schema = `
CREATE TABLE IF NOT EXISTS catalog_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS recordings (
 id TEXT PRIMARY KEY, source_version TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS recording_aliases (
 path TEXT PRIMARY KEY, recording_id TEXT NOT NULL REFERENCES recordings(id),
 file_id TEXT NOT NULL, size INTEGER NOT NULL CHECK(size>=0), mtime_ns INTEGER NOT NULL,
 present INTEGER NOT NULL, room TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS aliases_identity ON recording_aliases(file_id);
CREATE TABLE IF NOT EXISTS media_assets (
 path TEXT PRIMARY KEY, recording_id TEXT NOT NULL REFERENCES recordings(id),
 source_version TEXT NOT NULL, asset_version TEXT NOT NULL, kind TEXT NOT NULL,
 status TEXT NOT NULL, size INTEGER NOT NULL DEFAULT 0, mtime_ns INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS room_statistics (
 folder TEXT PRIMARY KEY, video_count INTEGER NOT NULL, total_size INTEGER NOT NULL,
 derived_size INTEGER NOT NULL DEFAULT 0, latest_video TEXT NOT NULL,
 latest_at INTEGER NOT NULL, checked_at INTEGER NOT NULL
);
PRAGMA user_version=1;
`
