package api

import (
	"github.com/golive-nms/golive-nms/internal/store"
	"net/http"
)

func (a *API) channels(w http.ResponseWriter, r *http.Request) {
	v, e := a.s.NotificationChannels(r.Context())
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
	for _, c := range v {
		if c.SiteID == "" || scope.can(c.SiteID) {
			out = append(out, c)
		}
	}
	jsonOut(w, 200, out)
}
func (a *API) createChannel(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok || (u.Role != "administrator" && u.Role != "manager") {
		problem(w, 403, errText("manager role required"))
		return
	}
	var c store.NotificationChannel
	if !decode(w, r, &c) {
		return
	}
	if c.SiteID == "" && u.Role != "administrator" {
		problem(w, 403, errText("only administrators may create global alert channels"))
		return
	}
	if c.SiteID != "" {
		scope, e := a.scope(r)
		if e != nil || !scope.can(c.SiteID) {
			problem(w, 403, errText("site access denied"))
			return
		}
	}
	v, e := a.s.CreateNotificationChannel(r.Context(), c)
	if e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "create", "notification_channel", v.ID)
	jsonOut(w, 201, v)
}
func (a *API) deleteChannel(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok || (u.Role != "administrator" && u.Role != "manager") {
		problem(w, 403, errText("manager role required"))
		return
	}
	id := r.PathValue("id")
	if e := a.s.DeleteNotificationChannel(r.Context(), id); e != nil {
		problem(w, 404, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "delete", "notification_channel", id)
	w.WriteHeader(204)
}
