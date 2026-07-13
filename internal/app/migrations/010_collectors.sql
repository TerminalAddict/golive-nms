ALTER TABLE sites ADD COLUMN IF NOT EXISTS collector_identity_id uuid REFERENCES enrolled_identities(id) ON DELETE SET NULL;
