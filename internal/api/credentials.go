package api

import (
	"github.com/golive-nms/golive-nms/internal/store"
	"net/http"
)

func (a *API) credentials(w http.ResponseWriter, r *http.Request) {
	v, e := a.s.Credentials(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, v)
}
func (a *API) createCredential(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok || (u.Role != "administrator" && u.Role != "manager") {
		problem(w, 403, errText("manager role required"))
		return
	}
	var c store.Credential
	if !decode(w, r, &c) {
		return
	}
	if c.Name == "" || len(c.Secret) == 0 {
		problem(w, 400, errText("name and secret are required"))
		return
	}
	v, e := a.s.CreateCredential(r.Context(), c)
	if e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "create", "credential", v.ID)
	jsonOut(w, 201, v)
}
func (a *API) deleteCredential(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok || (u.Role != "administrator" && u.Role != "manager") {
		problem(w, 403, errText("manager role required"))
		return
	}
	id := r.PathValue("id")
	if e := a.s.DeleteCredential(r.Context(), id); e != nil {
		problem(w, 404, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "delete", "credential", id)
	w.WriteHeader(204)
}
