package api

import (
	"github.com/TerminalAddict/golive-nms/internal/store"
	"net/http"
)

type siteScope struct {
	restricted bool
	allowed    map[string]bool
}

func (a *API) scope(r *http.Request) (siteScope, error) {
	u, _ := CurrentUser(r.Context())
	ids, err := a.s.UserSiteIDs(r.Context(), u.ID)
	if err != nil {
		return siteScope{}, err
	}
	allowed := map[string]bool{}
	for _, id := range ids {
		allowed[id] = true
	}
	return siteScope{restricted: u.Role == "site_manager" || (u.Role == "viewer" && len(ids) > 0), allowed: allowed}, nil
}
func (s siteScope) can(id string) bool { return !s.restricted || s.allowed[id] }
func filterDevices(v []store.Device, s siteScope) []store.Device {
	out := v[:0]
	for _, x := range v {
		if s.can(x.SiteID) {
			out = append(out, x)
		}
	}
	return out
}
func filterChecks(v []store.Check, s siteScope) []store.Check {
	out := v[:0]
	for _, x := range v {
		if s.can(x.SiteID) {
			out = append(out, x)
		}
	}
	return out
}
func filterIncidents(v []store.Incident, s siteScope) []store.Incident {
	out := v[:0]
	for _, x := range v {
		if s.can(x.SiteID) {
			out = append(out, x)
		}
	}
	return out
}
