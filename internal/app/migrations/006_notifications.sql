CREATE TABLE IF NOT EXISTS notification_channels (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL UNIQUE,
 kind text NOT NULL CHECK(kind IN ('email','slack','teams')),
 credential_id uuid NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,
 enabled boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS notification_deliveries (
 id bigserial PRIMARY KEY, channel_id uuid REFERENCES notification_channels(id) ON DELETE SET NULL,
 incident_id uuid REFERENCES incidents(id) ON DELETE SET NULL, event text NOT NULL,
 success boolean NOT NULL, error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
