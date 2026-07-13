package api

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestMonitV2Document(t *testing.T) {
	payload := `<?xml version="1.0" encoding="ISO-8859-1"?><monit id="abc123" incarnation="1710000000" version="5.35.2"><server><uptime>42</uptime><localhostname>mail-01</localhostname></server><services><service name="postfix"><type>3</type><collected_sec>1710000042</collected_sec><collected_usec>10</collected_usec><status>1</status><monitor>1</monitor></service></services><event><collected_sec>1710000042</collected_sec><service>postfix</service><id>1</id><state>1</state><action>2</action><message>process is not running</message></event></monit>`
	var doc monitXML
	decoder := xml.NewDecoder(strings.NewReader(payload))
	decoder.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	if err := decoder.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc.ID != "abc123" || doc.Server.Hostname != "mail-01" || len(doc.Services) != 1 || doc.Services[0].Status != 1 || doc.Event == nil || doc.Event.Service != "postfix" {
		t.Fatalf("unexpected document: %+v", doc)
	}
}
