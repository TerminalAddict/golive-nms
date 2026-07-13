ALTER TABLE credentials DROP CONSTRAINT IF EXISTS credentials_kind_check;
ALTER TABLE credentials ADD CONSTRAINT credentials_kind_check CHECK(kind IN ('snmp','ssh','smtp','webhook','routeros'));
ALTER TABLE checks DROP CONSTRAINT IF EXISTS checks_type_check;
ALTER TABLE checks ADD CONSTRAINT checks_type_check CHECK(type IN ('ping','http','tcp','snmp','dns','tls','ssh','smtp','mysql','postgres','routeros'));
