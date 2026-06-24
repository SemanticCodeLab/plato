CREATE TABLE IF NOT EXISTS schema_version (
  version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS api_tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  scopes TEXT NOT NULL,
  created_at TEXT NOT NULL,
  last_used_at TEXT,
  revoked_at TEXT
);

CREATE TABLE IF NOT EXISTS wikis (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  slug TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  root_path TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  wiki_id INTEGER NOT NULL,
  slug TEXT NOT NULL,
  title TEXT NOT NULL,
  rel_path TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT,
  UNIQUE(wiki_id, slug),
  UNIQUE(wiki_id, rel_path),
  FOREIGN KEY(wiki_id) REFERENCES wikis(id)
);

CREATE TABLE IF NOT EXISTS links (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  wiki_id INTEGER NOT NULL,
  from_page_id INTEGER NOT NULL,
  to_page_id INTEGER,
  target TEXT NOT NULL,
  raw TEXT NOT NULL,
  label TEXT,
  kind TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(wiki_id) REFERENCES wikis(id),
  FOREIGN KEY(from_page_id) REFERENCES pages(id),
  FOREIGN KEY(to_page_id) REFERENCES pages(id)
);

CREATE TABLE IF NOT EXISTS audit_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor TEXT NOT NULL,
  action TEXT NOT NULL,
  wiki_id INTEGER,
  page_id INTEGER,
  metadata_json TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pages_wiki_slug ON pages(wiki_id, slug);
CREATE INDEX IF NOT EXISTS idx_pages_wiki_rel_path ON pages(wiki_id, rel_path);
CREATE INDEX IF NOT EXISTS idx_links_from_page ON links(wiki_id, from_page_id);
CREATE INDEX IF NOT EXISTS idx_links_to_page ON links(wiki_id, to_page_id);
CREATE INDEX IF NOT EXISTS idx_links_status ON links(wiki_id, status);
