ALTER TABLE agent_reports ADD COLUMN IF NOT EXISTS identity_id uuid REFERENCES enrolled_identities(id) ON DELETE SET NULL;
CREATE UNIQUE INDEX IF NOT EXISTS agent_reports_identity_idx ON agent_reports(identity_id) WHERE identity_id IS NOT NULL;
CREATE TABLE IF NOT EXISTS remediation_settings (id boolean PRIMARY KEY DEFAULT true CHECK(id),enabled boolean NOT NULL DEFAULT true);
INSERT INTO remediation_settings(id,enabled) VALUES(true,true) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS action_templates (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL UNIQUE, executable text NOT NULL,
 arguments jsonb NOT NULL DEFAULT '[]', timeout_seconds integer NOT NULL DEFAULT 30 CHECK(timeout_seconds BETWEEN 1 AND 300),
 auto_check_type text, enabled boolean NOT NULL DEFAULT true, created_by uuid REFERENCES users(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS remediation_jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), template_id uuid NOT NULL REFERENCES action_templates(id) ON DELETE RESTRICT,
 device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE, requested_by uuid REFERENCES users(id) ON DELETE SET NULL,
 automatic boolean NOT NULL DEFAULT false, state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','succeeded','failed','cancelled','expired')),
 output text NOT NULL DEFAULT '', error text NOT NULL DEFAULT '', queued_at timestamptz NOT NULL DEFAULT now(),
 started_at timestamptz, finished_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS remediation_one_active ON remediation_jobs(template_id,device_id) WHERE state IN ('queued','running');
CREATE INDEX IF NOT EXISTS remediation_jobs_time_idx ON remediation_jobs(queued_at DESC);
