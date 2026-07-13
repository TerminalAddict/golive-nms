package api

import (
	"github.com/golive-nms/golive-nms/internal/store"
	"github.com/pmezard/go-difflib/difflib"
	"net/http"
)

func (a *API) configProfiles(w http.ResponseWriter, r *http.Request) {
	v, e := a.s.ConfigProfiles(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	out := v[:0]
	for _, p := range v {
		if scope.can(p.SiteID) {
			out = append(out, p)
		}
	}
	jsonOut(w, 200, out)
}
func (a *API) createConfigProfile(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role != "administrator" && u.Role != "manager" {
		problem(w, 403, errText("manager role required"))
		return
	}
	var v store.ConfigProfile
	if !decode(w, r, &v) {
		return
	}
	site, e := a.s.DeviceSite(r.Context(), v.DeviceID)
	if e != nil {
		problem(w, 404, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil || !scope.can(site) {
		problem(w, 403, errText("site access denied"))
		return
	}
	v, e = a.s.CreateConfigProfile(r.Context(), v)
	if e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "create", "config_profile", v.ID)
	jsonOut(w, 201, v)
}
func (a *API) deleteConfigProfile(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role != "administrator" && u.Role != "manager" {
		problem(w, 403, errText("manager role required"))
		return
	}
	if e := a.s.DeleteConfigProfile(r.Context(), r.PathValue("id")); e != nil {
		problem(w, 404, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "delete", "config_profile", r.PathValue("id"))
	w.WriteHeader(204)
}
func (a *API) triggerConfigBackup(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role != "administrator" && u.Role != "manager" {
		problem(w, 403, errText("manager role required"))
		return
	}
	if e := a.s.TriggerConfigBackup(r.Context(), r.PathValue("id")); e != nil {
		problem(w, 404, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "trigger", "config_profile", r.PathValue("id"))
	w.WriteHeader(204)
}
func (a *API) configSnapshots(w http.ResponseWriter, r *http.Request) {
	device := r.PathValue("deviceId")
	site, e := a.s.DeviceSite(r.Context(), device)
	if e != nil {
		problem(w, 404, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil || !scope.can(site) {
		problem(w, 403, errText("site access denied"))
		return
	}
	v, e := a.s.ConfigSnapshots(r.Context(), device)
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, v)
}
func (a *API) configDiff(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role == "viewer" {
		problem(w, 403, errText("manager role required"))
		return
	}
	fromID, toID := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	fromDevice, e := a.s.ConfigSnapshotDevice(r.Context(), fromID)
	if e != nil {
		problem(w, 404, e)
		return
	}
	toDevice, e := a.s.ConfigSnapshotDevice(r.Context(), toID)
	if e != nil || fromDevice != toDevice {
		problem(w, 400, errText("snapshots must belong to the same device"))
		return
	}
	site, e := a.s.DeviceSite(r.Context(), fromDevice)
	scope, se := a.scope(r)
	if e != nil || se != nil || !scope.can(site) {
		problem(w, 403, errText("site access denied"))
		return
	}
	from, e := a.s.ConfigContent(r.Context(), fromID)
	if e != nil {
		problem(w, 500, e)
		return
	}
	to, e := a.s.ConfigContent(r.Context(), toID)
	if e != nil {
		problem(w, 500, e)
		return
	}
	diff, e := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: difflib.SplitLines(string(from)), B: difflib.SplitLines(string(to)), FromFile: "previous", ToFile: "current", Context: 3})
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, map[string]string{"diff": diff})
}
