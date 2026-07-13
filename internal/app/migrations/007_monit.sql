ALTER TABLE incidents ALTER COLUMN check_id DROP NOT NULL;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'check';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS source_key text;
CREATE UNIQUE INDEX IF NOT EXISTS incidents_one_active_source ON incidents(source,source_key) WHERE source_key IS NOT NULL AND state IN ('open','acknowledged');
CREATE TABLE IF NOT EXISTS monit_hosts (id uuid PRIMARY KEY DEFAULT gen_random_uuid(),monit_id text NOT NULL UNIQUE,device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,version text NOT NULL,incarnation bigint NOT NULL DEFAULT 0,last_report_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS monit_services (host_id uuid NOT NULL REFERENCES monit_hosts(id) ON DELETE CASCADE,name text NOT NULL,type integer NOT NULL,status bigint NOT NULL,monitor integer NOT NULL,collected_at timestamptz,updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(host_id,name));
CREATE TABLE IF NOT EXISTS monit_events (id bigserial PRIMARY KEY,host_id uuid NOT NULL REFERENCES monit_hosts(id) ON DELETE CASCADE,service text NOT NULL,event_id bigint NOT NULL,state integer NOT NULL,action integer NOT NULL,message text NOT NULL,collected_at timestamptz NOT NULL,received_at timestamptz NOT NULL DEFAULT now());
