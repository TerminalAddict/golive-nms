ALTER TABLE monit_controls DROP CONSTRAINT IF EXISTS monit_controls_credential_id_fkey;
ALTER TABLE monit_controls ADD CONSTRAINT monit_controls_credential_id_fkey FOREIGN KEY(credential_id) REFERENCES credentials(id) ON DELETE CASCADE;
