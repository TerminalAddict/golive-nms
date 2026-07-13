ALTER TABLE notification_channels ADD COLUMN IF NOT EXISTS site_id uuid REFERENCES sites(id) ON DELETE CASCADE;
ALTER TABLE notification_channels ADD COLUMN IF NOT EXISTS notify_opened boolean NOT NULL DEFAULT true;
ALTER TABLE notification_channels ADD COLUMN IF NOT EXISTS notify_resolved boolean NOT NULL DEFAULT true;
ALTER TABLE notification_channels ADD COLUMN IF NOT EXISTS repeat_minutes integer NOT NULL DEFAULT 0 CHECK(repeat_minutes BETWEEN 0 AND 1440);
CREATE INDEX IF NOT EXISTS notification_channels_site_idx ON notification_channels(site_id) WHERE enabled;
ALTER TABLE notification_deliveries ADD COLUMN IF NOT EXISTS device_id uuid REFERENCES devices(id) ON DELETE SET NULL;
