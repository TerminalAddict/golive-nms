CREATE TABLE IF NOT EXISTS maintenance_windows (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), site_id uuid REFERENCES sites(id) ON DELETE CASCADE,
 device_id uuid REFERENCES devices(id) ON DELETE CASCADE, name text NOT NULL,
 starts_at timestamptz NOT NULL, ends_at timestamptz NOT NULL,
 created_by uuid REFERENCES users(id) ON DELETE SET NULL, created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(site_id IS NOT NULL OR device_id IS NOT NULL), CHECK(ends_at>starts_at)
);
CREATE INDEX IF NOT EXISTS maintenance_active_idx ON maintenance_windows(starts_at,ends_at);
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS assigned_to uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS notes text NOT NULL DEFAULT '';
