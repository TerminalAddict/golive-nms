package api

import (
	"testing"

	"github.com/TerminalAddict/golive-nms/internal/store"
)

func TestFilterMonitServicesBySite(t *testing.T) {
	services := []store.MonitServiceStatus{
		{DeviceID: "one", SiteID: "site-a", Name: "sshd"},
		{DeviceID: "two", SiteID: "site-b", Name: "postfix"},
	}
	got := filterMonitServices(services, siteScope{restricted: true, allowed: map[string]bool{"site-a": true}})
	if len(got) != 1 || got[0].DeviceID != "one" {
		t.Fatalf("unexpected scoped services: %+v", got)
	}
}
