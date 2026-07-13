package api

import (
	"github.com/TerminalAddict/golive-nms/internal/store"
	"net/http"
)

func (a *API) maintenanceWindows(w http.ResponseWriter, r *http.Request) {
	v, e := a.s.MaintenanceWindows(r.Context())
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
	for _, x := range v {
		site := x.SiteID
		if site == "" && x.DeviceID != "" {
			site, _ = a.s.DeviceSite(r.Context(), x.DeviceID)
		}
		if scope.can(site) {
			out = append(out, x)
		}
	}
	jsonOut(w, 200, out)
}
func (a *API) createMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role == "viewer" {
		problem(w, 403, errText("manager role required"))
		return
	}
	var v store.MaintenanceWindow
	if !decode(w, r, &v) {
		return
	}
	site := v.SiteID
	if site == "" && v.DeviceID != "" {
		var e error
		site, e = a.s.DeviceSite(r.Context(), v.DeviceID)
		if e != nil {
			problem(w, 404, e)
			return
		}
	}
	scope, e := a.scope(r)
	if e != nil || !scope.can(site) {
		problem(w, 403, errText("site access denied"))
		return
	}
	v, e = a.s.CreateMaintenanceWindow(r.Context(), v, u.ID)
	if e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "create", "maintenance_window", v.ID)
	jsonOut(w, 201, v)
}
func (a *API) deleteMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role == "viewer" {
		problem(w, 403, errText("manager role required"))
		return
	}
	if e := a.s.DeleteMaintenanceWindow(r.Context(), r.PathValue("id")); e != nil {
		problem(w, 404, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "delete", "maintenance_window", r.PathValue("id"))
	w.WriteHeader(204)
}
