package api

import (
	"github.com/golive-nms/golive-nms/internal/store"
	"net/http"
)

func (a *API) sites(w http.ResponseWriter, r *http.Request) {
	v, e := a.s.Sites(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	if scope.restricted {
		out := v[:0]
		for _, s := range v {
			if scope.can(s.ID) {
				out = append(out, s)
			}
		}
		v = out
	}
	jsonOut(w, 200, v)
}
func (a *API) createSite(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role != "administrator" && u.Role != "manager" {
		problem(w, 403, errText("manager role required"))
		return
	}
	var v store.Site
	if !decode(w, r, &v) {
		return
	}
	v, e := a.s.CreateSite(r.Context(), v)
	if e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "create", "site", v.ID)
	jsonOut(w, 201, v)
}
func (a *API) deleteSite(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role != "administrator" && u.Role != "manager" {
		problem(w, 403, errText("manager role required"))
		return
	}
	id := r.PathValue("id")
	if e := a.s.DeleteSite(r.Context(), id); e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "delete", "site", id)
	w.WriteHeader(204)
}
func (a *API) userSites(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	v, e := a.s.UserSiteIDs(r.Context(), r.PathValue("id"))
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, map[string]any{"siteIds": v})
}
func (a *API) setUserSites(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireAdminUser(w, r)
	if !ok {
		return
	}
	var body struct {
		SiteIDs []string `json:"siteIds"`
	}
	if !decode(w, r, &body) {
		return
	}
	id := r.PathValue("id")
	if e := a.s.SetUserSites(r.Context(), id, body.SiteIDs); e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), actor.ID, "set_sites", "user", id)
	w.WriteHeader(204)
}
