package store

// migrations são aplicadas em ordem; o índice+1 vira o PRAGMA user_version.
// Nunca edite uma migração já publicada: adicione uma nova.
var migrations = []string{
	`
CREATE TABLE jobs (
	id            INTEGER PRIMARY KEY,
	source        TEXT    NOT NULL,
	source_job_id TEXT    NOT NULL,
	company       TEXT    NOT NULL,
	title         TEXT    NOT NULL,
	description   TEXT    NOT NULL DEFAULT '',
	location      TEXT    NOT NULL DEFAULT '',
	country       TEXT    NOT NULL DEFAULT '',
	remote        TEXT    NOT NULL DEFAULT 'desconhecido',
	level         TEXT    NOT NULL DEFAULT 'desconhecido',
	area          TEXT    NOT NULL DEFAULT 'outros',
	url           TEXT    NOT NULL,
	posted_at     INTEGER,
	first_seen    INTEGER NOT NULL,
	last_seen     INTEGER NOT NULL,
	closed_at     INTEGER,
	fingerprint   TEXT    NOT NULL,
	aggregator    INTEGER NOT NULL DEFAULT 0,
	UNIQUE (source, company, source_job_id)
);
CREATE INDEX jobs_open        ON jobs (closed_at, posted_at DESC);
CREATE INDEX jobs_fingerprint ON jobs (fingerprint);
CREATE INDEX jobs_company     ON jobs (company);
CREATE INDEX jobs_first_seen  ON jobs (first_seen);
CREATE INDEX jobs_src_company ON jobs (source, company, last_seen);

CREATE VIRTUAL TABLE jobs_fts USING fts5 (
	title, description,
	content = 'jobs', content_rowid = 'id',
	tokenize = 'unicode61 remove_diacritics 2'
);
CREATE TRIGGER jobs_ai AFTER INSERT ON jobs BEGIN
	INSERT INTO jobs_fts (rowid, title, description) VALUES (new.id, new.title, new.description);
END;
CREATE TRIGGER jobs_ad AFTER DELETE ON jobs BEGIN
	INSERT INTO jobs_fts (jobs_fts, rowid, title, description) VALUES ('delete', old.id, old.title, old.description);
END;
CREATE TRIGGER jobs_au AFTER UPDATE OF title, description ON jobs
WHEN old.title IS NOT new.title OR old.description IS NOT new.description BEGIN
	INSERT INTO jobs_fts (jobs_fts, rowid, title, description) VALUES ('delete', old.id, old.title, old.description);
	INSERT INTO jobs_fts (rowid, title, description) VALUES (new.id, new.title, new.description);
END;

CREATE TABLE job_state (
	job_id     INTEGER PRIMARY KEY REFERENCES jobs (id) ON DELETE CASCADE,
	status     TEXT    NOT NULL DEFAULT 'nova',
	notes      TEXT    NOT NULL DEFAULT '',
	updated_at INTEGER NOT NULL
);
CREATE INDEX job_state_status ON job_state (status);

CREATE TABLE app_state (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE collection_runs (
	id          INTEGER PRIMARY KEY,
	source      TEXT    NOT NULL,
	company     TEXT    NOT NULL,
	started_at  INTEGER NOT NULL,
	finished_at INTEGER NOT NULL,
	ok          INTEGER NOT NULL,
	error       TEXT    NOT NULL DEFAULT '',
	jobs        INTEGER NOT NULL DEFAULT 0,
	closed      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX collection_runs_latest ON collection_runs (source, company, finished_at DESC);
`,
}
