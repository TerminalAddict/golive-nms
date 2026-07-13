CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS sites (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  latitude double precision,
  longitude double precision,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS devices (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  site_id uuid REFERENCES sites(id) ON DELETE SET NULL,
  parent_id uuid REFERENCES devices(id) ON DELETE SET NULL,
  name text NOT NULL,
  address text NOT NULL,
  kind text NOT NULL DEFAULT 'server' CHECK (kind IN ('server','router','switch','other')),
  status text NOT NULL DEFAULT 'unknown' CHECK (status IN ('up','down','degraded','unknown','dependency')),
  tags text[] NOT NULL DEFAULT '{}',
  last_seen_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(site_id, name)
);

CREATE TABLE IF NOT EXISTS checks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  name text NOT NULL,
  type text NOT NULL CHECK (type IN ('http','tcp')),
  target text NOT NULL,
  interval_seconds integer NOT NULL DEFAULT 30 CHECK (interval_seconds BETWEEN 5 AND 86400),
  timeout_seconds integer NOT NULL DEFAULT 5 CHECK (timeout_seconds BETWEEN 1 AND 60),
  enabled boolean NOT NULL DEFAULT true,
  status text NOT NULL DEFAULT 'unknown' CHECK (status IN ('up','down','unknown')),
  last_error text NOT NULL DEFAULT '',
  last_run_at timestamptz,
  next_run_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS checks_due_idx ON checks(next_run_at) WHERE enabled;

CREATE TABLE IF NOT EXISTS incidents (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  check_id uuid NOT NULL REFERENCES checks(id) ON DELETE CASCADE,
  device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  title text NOT NULL,
  severity text NOT NULL DEFAULT 'critical',
  state text NOT NULL DEFAULT 'open' CHECK (state IN ('open','acknowledged','resolved')),
  opened_at timestamptz NOT NULL DEFAULT now(),
  acknowledged_at timestamptz,
  resolved_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS incidents_one_active_check ON incidents(check_id) WHERE state IN ('open','acknowledged');

INSERT INTO sites(name) VALUES ('Default site') ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS agent_reports (
  device_id uuid PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
  agent_id text NOT NULL UNIQUE,
  version text NOT NULL,
  metrics jsonb NOT NULL DEFAULT '{}',
  reported_at timestamptz NOT NULL DEFAULT now()
);
