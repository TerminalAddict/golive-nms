CREATE TABLE IF NOT EXISTS credentials (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL UNIQUE,
  kind text NOT NULL CHECK(kind IN ('snmp','ssh','smtp','webhook')),
  encrypted_data bytea NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE checks ADD COLUMN IF NOT EXISTS credential_id uuid REFERENCES credentials(id) ON DELETE SET NULL;
ALTER TABLE checks ADD COLUMN IF NOT EXISTS config jsonb NOT NULL DEFAULT '{}';
ALTER TABLE checks DROP CONSTRAINT IF EXISTS checks_type_check;
ALTER TABLE checks ADD CONSTRAINT checks_type_check CHECK(type IN ('ping','http','tcp','snmp'));
