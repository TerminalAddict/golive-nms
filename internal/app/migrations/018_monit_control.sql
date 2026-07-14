ALTER TABLE credentials DROP CONSTRAINT IF EXISTS credentials_kind_check;
ALTER TABLE credentials ADD CONSTRAINT credentials_kind_check CHECK(kind IN ('snmp','ssh','smtp','webhook','routeros','monit'));

CREATE TABLE IF NOT EXISTS monit_controls (
 device_id uuid PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
 url text NOT NULL,
 credential_id uuid NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,
 updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS monit_actions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
 service text NOT NULL,
 action text NOT NULL CHECK(action IN ('start','stop','restart','monitor','unmonitor')),
 requested_by uuid REFERENCES users(id) ON DELETE SET NULL,
 success boolean NOT NULL,
 message text NOT NULL DEFAULT '',
 requested_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS monit_actions_device_requested_idx ON monit_actions(device_id,requested_at DESC);
