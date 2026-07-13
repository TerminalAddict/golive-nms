ALTER TABLE checks DROP CONSTRAINT IF EXISTS checks_type_check;
ALTER TABLE checks ADD CONSTRAINT checks_type_check CHECK(type IN ('ping','http','tcp','snmp','dns','tls','ssh','smtp','mysql','postgres'));
