package api

import (
	"net/http"
	"time"

	"github.com/TerminalAddict/golive-nms/internal/store"
)

func (a *API) users(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	v, e := a.s.Users(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, v)
}
func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireAdminUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
		Role        string `json:"role"`
	}
	if !decode(w, r, &body) {
		return
	}
	v, e := a.s.CreateUser(r.Context(), body.Email, body.DisplayName, body.Password, body.Role)
	if e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), actor.ID, "create", "user", v.ID)
	jsonOut(w, 201, v)
}
func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireAdminUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if e := a.s.DeleteUser(r.Context(), id, actor.ID); e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), actor.ID, "delete", "user", id)
	w.WriteHeader(204)
}
func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireAdminUser(w, r)
	if !ok {
		return
	}
	var body struct {
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
		Password    string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	v, e := a.s.UpdateUser(r.Context(), r.PathValue("id"), body.DisplayName, body.Role, body.Password)
	if e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), actor.ID, "update", "user", v.ID)
	jsonOut(w, 200, v)
}
func (a *API) tokens(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok {
		problem(w, 401, errText("authentication required"))
		return
	}
	v, e := a.s.APITokens(r.Context(), u.ID)
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, v)
}
func (a *API) createToken(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok {
		problem(w, 401, errText("authentication required"))
		return
	}
	var body struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Name == "" {
		problem(w, 400, errText("name is required"))
		return
	}
	v, e := a.s.CreateAPIToken(r.Context(), u.ID, body.Name, body.ExpiresAt)
	if e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "create", "api_token", v.ID)
	jsonOut(w, 201, v)
}
func (a *API) deleteToken(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok {
		problem(w, 401, errText("authentication required"))
		return
	}
	id := r.PathValue("id")
	if e := a.s.DeleteAPIToken(r.Context(), u.ID, id); e != nil {
		problem(w, 404, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "delete", "api_token", id)
	w.WriteHeader(204)
}
func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	_, ok := requireAdminUser(w, r)
	return ok
}
func requireAdminUser(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	u, ok := CurrentUser(r.Context())
	if !ok || u.Role != "administrator" {
		problem(w, 403, errText("administrator role required"))
		return store.User{}, false
	}
	return u, true
}
