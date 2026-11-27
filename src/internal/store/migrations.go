package store

// migrations is the schema history of otter.db (docs/specs/scope-v1.md,
// section 3). Append only: never edit a migration once released, add one.
var migrations = []migration{
	execAll(
		`CREATE TABLE setting (
		  key TEXT PRIMARY KEY,
		  value TEXT NOT NULL
		)`,

		`CREATE TABLE workspace (
		  id INTEGER PRIMARY KEY,
		  name TEXT NOT NULL,
		  active_environment_id INTEGER REFERENCES environment(id) ON DELETE SET NULL,
		  proxy_url TEXT NOT NULL DEFAULT '',
		  tls_insecure INTEGER NOT NULL DEFAULT 0,
		  ca_cert_path TEXT NOT NULL DEFAULT '',
		  client_cert_path TEXT NOT NULL DEFAULT '',
		  client_key_path TEXT NOT NULL DEFAULT '',
		  cookie_jar INTEGER NOT NULL DEFAULT 1
		)`,

		`CREATE TABLE environment (
		  id INTEGER PRIMARY KEY,
		  workspace_id INTEGER NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
		  name TEXT NOT NULL,
		  seq INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX idx_environment_workspace ON environment(workspace_id, seq)`,

		`CREATE TABLE env_var (
		  id INTEGER PRIMARY KEY,
		  environment_id INTEGER NOT NULL REFERENCES environment(id) ON DELETE CASCADE,
		  seq INTEGER NOT NULL DEFAULT 0,
		  key TEXT NOT NULL,
		  value TEXT NOT NULL DEFAULT '',
		  secret INTEGER NOT NULL DEFAULT 0,
		  enabled INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX idx_env_var_environment ON env_var(environment_id, seq)`,

		`CREATE TABLE folder (
		  id INTEGER PRIMARY KEY,
		  workspace_id INTEGER NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
		  parent_id INTEGER REFERENCES folder(id) ON DELETE CASCADE,
		  name TEXT NOT NULL,
		  seq INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX idx_folder_children ON folder(workspace_id, parent_id, seq)`,
		`CREATE INDEX idx_folder_parent ON folder(parent_id)`,

		`CREATE TABLE folder_var (
		  id INTEGER PRIMARY KEY,
		  folder_id INTEGER NOT NULL REFERENCES folder(id) ON DELETE CASCADE,
		  seq INTEGER NOT NULL DEFAULT 0,
		  key TEXT NOT NULL,
		  value TEXT NOT NULL DEFAULT '',
		  enabled INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX idx_folder_var_folder ON folder_var(folder_id, seq)`,

		`CREATE TABLE folder_header (
		  id INTEGER PRIMARY KEY,
		  folder_id INTEGER NOT NULL REFERENCES folder(id) ON DELETE CASCADE,
		  seq INTEGER NOT NULL DEFAULT 0,
		  key TEXT NOT NULL,
		  value TEXT NOT NULL DEFAULT '',
		  mode TEXT NOT NULL DEFAULT 'set' CHECK (mode IN ('set','ignore')),
		  enabled INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX idx_folder_header_folder ON folder_header(folder_id, seq)`,

		`CREATE TABLE request (
		  id INTEGER PRIMARY KEY,
		  folder_id INTEGER NOT NULL REFERENCES folder(id) ON DELETE CASCADE,
		  seq INTEGER NOT NULL DEFAULT 0,
		  name TEXT NOT NULL,
		  method TEXT NOT NULL DEFAULT 'GET',
		  url TEXT NOT NULL DEFAULT '',
		  body_type TEXT NOT NULL DEFAULT 'none'
		    CHECK (body_type IN ('none','json','xml','text','form-urlencoded','multipart','binary')),
		  body TEXT NOT NULL DEFAULT '',
		  body_file TEXT NOT NULL DEFAULT '',
		  post_script TEXT NOT NULL DEFAULT '',
		  encode_url INTEGER NOT NULL DEFAULT 1,
		  follow_redirects INTEGER NOT NULL DEFAULT 1,
		  max_redirects INTEGER NOT NULL DEFAULT 10,
		  timeout_ms INTEGER NOT NULL DEFAULT 30000
		)`,
		`CREATE INDEX idx_request_folder ON request(folder_id, seq)`,

		`CREATE TABLE request_var (
		  id INTEGER PRIMARY KEY,
		  request_id INTEGER NOT NULL REFERENCES request(id) ON DELETE CASCADE,
		  seq INTEGER NOT NULL DEFAULT 0,
		  key TEXT NOT NULL,
		  value TEXT NOT NULL DEFAULT '',
		  enabled INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX idx_request_var_request ON request_var(request_id, seq)`,

		`CREATE TABLE request_header (
		  id INTEGER PRIMARY KEY,
		  request_id INTEGER NOT NULL REFERENCES request(id) ON DELETE CASCADE,
		  seq INTEGER NOT NULL DEFAULT 0,
		  key TEXT NOT NULL,
		  value TEXT NOT NULL DEFAULT '',
		  mode TEXT NOT NULL DEFAULT 'set' CHECK (mode IN ('set','ignore')),
		  enabled INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX idx_request_header_request ON request_header(request_id, seq)`,

		`CREATE TABLE request_body_field (
		  id INTEGER PRIMARY KEY,
		  request_id INTEGER NOT NULL REFERENCES request(id) ON DELETE CASCADE,
		  seq INTEGER NOT NULL DEFAULT 0,
		  key TEXT NOT NULL,
		  kind TEXT NOT NULL DEFAULT 'text' CHECK (kind IN ('text','file')),
		  value TEXT NOT NULL DEFAULT '',
		  content_type TEXT NOT NULL DEFAULT '',
		  enabled INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX idx_request_body_field_request ON request_body_field(request_id, seq)`,

		`CREATE TABLE runtime_var (
		  workspace_id INTEGER NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
		  environment_id INTEGER NOT NULL DEFAULT 0,
		  key TEXT NOT NULL,
		  value TEXT NOT NULL DEFAULT '',
		  PRIMARY KEY (workspace_id, environment_id, key)
		)`,

		`CREATE TABLE import_source (
		  id INTEGER PRIMARY KEY,
		  workspace_id INTEGER NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
		  folder_id INTEGER REFERENCES folder(id) ON DELETE CASCADE,
		  kind TEXT NOT NULL CHECK (kind IN ('openapi')),
		  url TEXT NOT NULL,
		  last_sync_at INTEGER,
		  last_hash TEXT NOT NULL DEFAULT ''
		)`,

		`CREATE TABLE history (
		  id INTEGER PRIMARY KEY,
		  request_id INTEGER REFERENCES request(id) ON DELETE SET NULL,
		  workspace_id INTEGER REFERENCES workspace(id) ON DELETE CASCADE,
		  sent_at INTEGER NOT NULL,
		  method TEXT, url TEXT, status INTEGER, duration_ms INTEGER, size_bytes INTEGER,
		  req_headers TEXT, req_body TEXT, res_headers TEXT, res_body TEXT,
		  error TEXT
		)`,
		`CREATE INDEX idx_history_request ON history(request_id, sent_at DESC)`,

		// Search by words, accents ignored.
		`CREATE VIRTUAL TABLE request_fts USING fts5(
		  name, url, body,
		  content='request', content_rowid='id',
		  tokenize='unicode61 remove_diacritics 2'
		)`,
		// Search by substring (URLs, identifiers).
		`CREATE VIRTUAL TABLE request_fts_tri USING fts5(
		  name, url,
		  content='request', content_rowid='id',
		  tokenize='trigram'
		)`,
		// Both indexes follow request (external content: 'delete' rows).
		`CREATE TRIGGER request_fts_insert AFTER INSERT ON request BEGIN
		  INSERT INTO request_fts(rowid, name, url, body) VALUES (new.id, new.name, new.url, new.body);
		  INSERT INTO request_fts_tri(rowid, name, url) VALUES (new.id, new.name, new.url);
		END`,
		`CREATE TRIGGER request_fts_delete AFTER DELETE ON request BEGIN
		  INSERT INTO request_fts(request_fts, rowid, name, url, body) VALUES ('delete', old.id, old.name, old.url, old.body);
		  INSERT INTO request_fts_tri(request_fts_tri, rowid, name, url) VALUES ('delete', old.id, old.name, old.url);
		END`,
		`CREATE TRIGGER request_fts_update AFTER UPDATE OF name, url, body ON request BEGIN
		  INSERT INTO request_fts(request_fts, rowid, name, url, body) VALUES ('delete', old.id, old.name, old.url, old.body);
		  INSERT INTO request_fts_tri(request_fts_tri, rowid, name, url) VALUES ('delete', old.id, old.name, old.url);
		  INSERT INTO request_fts(rowid, name, url, body) VALUES (new.id, new.name, new.url, new.body);
		  INSERT INTO request_fts_tri(rowid, name, url) VALUES (new.id, new.name, new.url);
		END`,
	),
}
