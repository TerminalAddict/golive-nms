package api

import (
	"strings"
	"testing"
)

func TestMonitV2Document(t *testing.T) {
	payload := `<?xml version="1.0" encoding="ISO-8859-1"?><monit id="abc123" incarnation="1710000000" version="5.35.2"><server><uptime>42</uptime><localhostname>mail-01</localhostname></server><services><service name="postfix"><type>3</type><collected_sec>1710000042</collected_sec><collected_usec>10</collected_usec><status>1</status><monitor>1</monitor></service></services><event><collected_sec>1710000042</collected_sec><service>postfix</service><id>1</id><state>1</state><action>2</action><message>process is not running</message></event></monit>`
	report, err := decodeMonitReport(strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if report.ID != "abc123" || report.Hostname != "mail-01" || len(report.Services) != 1 || report.Services[0].Name != "postfix" || report.Services[0].Type != 3 || report.Services[0].Status != 1 || report.Event == nil || report.Event.Service != "postfix" || !report.FullSnapshot {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestMonitHTTPStatusDocument(t *testing.T) {
	payload := `<?xml version="1.0" encoding="ISO-8859-1"?><monit><server><id>def456</id><incarnation>1710000100</incarnation><version>5.35.2</version><localhostname>media-01</localhostname></server><service type="5"><name>media-01</name><collected_sec>1710000142</collected_sec><status>0</status><monitor>1</monitor></service><service type="3"><name>plex</name><collected_sec>1710000142</collected_sec><status>0</status><monitor>0</monitor></service></monit>`
	report, err := decodeMonitReport(strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if report.ID != "def456" || report.Version != "5.35.2" || report.Incarnation != 1710000100 || report.Hostname != "media-01" || len(report.Services) != 2 || report.Services[0].Type != 5 || report.Services[1].Name != "plex" || report.Services[1].Monitor != 0 || !report.FullSnapshot {
		t.Fatalf("unexpected report: %+v", report)
	}
}
