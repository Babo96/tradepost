CREATE TABLE IF NOT EXISTS clients (
  id text PRIMARY KEY,
  token_hash bytea NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_seen timestamptz,
  status text NOT NULL DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS markets (
  id text PRIMARY KEY,
  version bigint NOT NULL DEFAULT 0,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS scans (
  id bigserial PRIMARY KEY,
  client_id text NOT NULL REFERENCES clients(id),
  market text NOT NULL REFERENCES markets(id),
  scanned_at timestamptz NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now(),
  item_count integer NOT NULL,
  checksum bytea NOT NULL,
  client_version text
);

CREATE UNIQUE INDEX IF NOT EXISTS scans_client_checksum
  ON scans (client_id, checksum);

CREATE TABLE IF NOT EXISTS prices_latest (
  market text NOT NULL REFERENCES markets(id),
  item_key text NOT NULL,
  price bigint NOT NULL,
  quantity bigint NOT NULL,
  listings integer NOT NULL,
  scanned_at timestamptz NOT NULL,
  samples integer NOT NULL DEFAULT 1,
  PRIMARY KEY (market, item_key)
);

CREATE TABLE IF NOT EXISTS prices_hourly (
  market text NOT NULL,
  item_key text NOT NULL,
  bucket timestamptz NOT NULL,
  min_price bigint NOT NULL,
  max_price bigint NOT NULL,
  quantity bigint NOT NULL,
  samples integer NOT NULL DEFAULT 1,
  PRIMARY KEY (market, item_key, bucket)
);

CREATE INDEX IF NOT EXISTS prices_hourly_bucket
  ON prices_hourly (bucket);

CREATE TABLE IF NOT EXISTS items (
  item_id bigint PRIMARY KEY,
  name text NOT NULL,
  quality smallint NOT NULL DEFAULT 1,
  class_id smallint NOT NULL DEFAULT 0,
  subclass_id smallint NOT NULL DEFAULT 0,
  icon bigint NOT NULL DEFAULT 0,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS items_name_lower
  ON items (lower(name));

CREATE INDEX IF NOT EXISTS prices_latest_item_id
  ON prices_latest ((split_part(item_key, ':', 2)::bigint));

CREATE TABLE IF NOT EXISTS login_codes (
  code_hash bytea PRIMARY KEY,
  client_id text NOT NULL REFERENCES clients(id),
  character text NOT NULL,
  expires_at timestamptz NOT NULL,
  used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS login_codes_expires
  ON login_codes (expires_at);

CREATE TABLE IF NOT EXISTS web_sessions (
  token_hash bytea PRIMARY KEY,
  client_id text NOT NULL REFERENCES clients(id),
  character text NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS web_sessions_expires
  ON web_sessions (expires_at);

CREATE TABLE IF NOT EXISTS watch_lists (
  client_id text NOT NULL REFERENCES clients(id),
  character text NOT NULL,
  items text[] NOT NULL DEFAULT '{}',
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (client_id, character)
);

CREATE TABLE IF NOT EXISTS prices_15m (
  market text NOT NULL,
  item_key text NOT NULL,
  bucket timestamptz NOT NULL,
  min_price bigint NOT NULL,
  max_price bigint NOT NULL,
  quantity bigint NOT NULL,
  samples integer NOT NULL DEFAULT 1,
  PRIMARY KEY (market, item_key, bucket)
);

CREATE INDEX IF NOT EXISTS prices_15m_bucket
  ON prices_15m (bucket);

CREATE TABLE IF NOT EXISTS prices_raw (
  id bigserial PRIMARY KEY,
  market text NOT NULL,
  item_key text NOT NULL,
  price bigint NOT NULL,
  quantity bigint NOT NULL,
  listings integer NOT NULL,
  scanned_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS prices_raw_lookup
  ON prices_raw (market, item_key, scanned_at);

CREATE INDEX IF NOT EXISTS prices_raw_scanned
  ON prices_raw (scanned_at);
