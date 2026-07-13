CREATE TABLE IF NOT EXISTS pki_authority (
 id boolean PRIMARY KEY DEFAULT true CHECK(id), certificate_pem bytea NOT NULL,
 encrypted_private_key bytea NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS enrollment_tokens (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), token_hash bytea NOT NULL UNIQUE,
 kind text NOT NULL CHECK(kind IN ('agent','collector')), site_id uuid REFERENCES sites(id) ON DELETE CASCADE,
 created_by uuid REFERENCES users(id) ON DELETE SET NULL, expires_at timestamptz NOT NULL,
 used_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS enrolled_identities (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), kind text NOT NULL CHECK(kind IN ('agent','collector')),
 site_id uuid REFERENCES sites(id) ON DELETE SET NULL, name text NOT NULL, serial text NOT NULL UNIQUE,
 certificate_pem bytea NOT NULL, expires_at timestamptz NOT NULL, revoked_at timestamptz,
 last_seen_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS identities_serial_idx ON enrolled_identities(serial) WHERE revoked_at IS NULL;
