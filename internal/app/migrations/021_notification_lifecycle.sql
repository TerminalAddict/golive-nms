ALTER TABLE notification_channels ALTER COLUMN repeat_minutes SET DEFAULT 1440;
UPDATE notification_channels SET repeat_minutes=1440 WHERE repeat_minutes=0;

CREATE TABLE IF NOT EXISTS notification_events (
 id bigserial PRIMARY KEY,
 incident_id uuid NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
 event text NOT NULL CHECK(event IN ('opened','resolved')),
 created_at timestamptz NOT NULL DEFAULT now(),
 processing_at timestamptz,
 processed_at timestamptz,
 attempts integer NOT NULL DEFAULT 0,
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 last_error text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS notification_events_pending_idx
 ON notification_events(created_at)
 WHERE processed_at IS NULL;

ALTER TABLE notification_deliveries
 ADD COLUMN IF NOT EXISTS notification_event_id bigint REFERENCES notification_events(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS notification_deliveries_event_success_idx
 ON notification_deliveries(channel_id,notification_event_id)
 WHERE success AND notification_event_id IS NOT NULL;

CREATE OR REPLACE FUNCTION golive_queue_incident_notification() RETURNS trigger AS $$
BEGIN
 IF TG_OP='INSERT' AND NEW.state='open' THEN
  INSERT INTO notification_events(incident_id,event) VALUES(NEW.id,'opened');
 ELSIF TG_OP='UPDATE' AND OLD.state<>NEW.state AND NEW.state='resolved' THEN
  INSERT INTO notification_events(incident_id,event) VALUES(NEW.id,'resolved');
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS incidents_notification_lifecycle ON incidents;
CREATE TRIGGER incidents_notification_lifecycle
 AFTER INSERT OR UPDATE OF state ON incidents
 FOR EACH ROW EXECUTE FUNCTION golive_queue_incident_notification();
