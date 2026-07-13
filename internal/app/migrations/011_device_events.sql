CREATE TABLE IF NOT EXISTS device_events (
 id bigserial PRIMARY KEY, device_id uuid REFERENCES devices(id) ON DELETE SET NULL,
 protocol text NOT NULL CHECK(protocol IN ('syslog','snmp_trap')), source text NOT NULL,
 facility integer, severity integer, message text NOT NULL, fields jsonb NOT NULL DEFAULT '{}',
 received_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS device_events_time_idx ON device_events(received_at DESC);
CREATE INDEX IF NOT EXISTS device_events_device_idx ON device_events(device_id,received_at DESC);
