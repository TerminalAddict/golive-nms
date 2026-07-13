DO $$ BEGIN
  ALTER TABLE checks DROP CONSTRAINT IF EXISTS checks_type_check;
  ALTER TABLE checks ADD CONSTRAINT checks_type_check CHECK (type IN ('ping','http','tcp'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS check_samples (
  check_id uuid NOT NULL REFERENCES checks(id) ON DELETE CASCADE,
  observed_at timestamptz NOT NULL DEFAULT now(),
  up boolean NOT NULL,
  latency_ms double precision NOT NULL,
  message text NOT NULL DEFAULT '',
  PRIMARY KEY(check_id, observed_at)
);
CREATE INDEX IF NOT EXISTS check_samples_time_idx ON check_samples(observed_at);
