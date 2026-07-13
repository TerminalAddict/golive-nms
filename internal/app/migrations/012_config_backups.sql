CREATE TABLE IF NOT EXISTS config_backup_profiles (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), device_id uuid NOT NULL UNIQUE REFERENCES devices(id) ON DELETE CASCADE,
 credential_id uuid NOT NULL REFERENCES credentials(id) ON DELETE RESTRICT,
 command text NOT NULL, interval_seconds integer NOT NULL DEFAULT 86400 CHECK(interval_seconds BETWEEN 300 AND 2592000),
 enabled boolean NOT NULL DEFAULT true, next_run_at timestamptz NOT NULL DEFAULT now(),
 last_run_at timestamptz, last_error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS config_snapshots (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
 content_hash text NOT NULL, encrypted_content bytea NOT NULL, captured_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(device_id,content_hash)
);
CREATE INDEX IF NOT EXISTS config_snapshots_device_time ON config_snapshots(device_id,captured_at DESC);
