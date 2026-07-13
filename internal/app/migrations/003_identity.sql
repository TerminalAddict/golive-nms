CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), email text NOT NULL UNIQUE,
  display_name text NOT NULL, password_hash text NOT NULL,
  role text NOT NULL CHECK(role IN ('administrator','manager','site_manager','viewer')),
  active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS user_site_grants (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  site_id uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE, PRIMARY KEY(user_id,site_id)
);
CREATE TABLE IF NOT EXISTS sessions (
  token_hash bytea PRIMARY KEY, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions(expires_at);
CREATE TABLE IF NOT EXISTS audit_log (
  id bigserial PRIMARY KEY, user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  action text NOT NULL, resource_type text NOT NULL, resource_id text NOT NULL DEFAULT '',
  details jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now()
);
