package api

import (
	"net/http"
	"time"
)

func (a *API) createEnrollment(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role != "administrator" && u.Role != "manager" {
		problem(w, 403, errText("manager role required"))
		return
	}
	var body struct {
		Kind       string `json:"kind"`
		SiteID     string `json:"siteId"`
		TTLMinutes int    `json:"ttlMinutes"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.TTLMinutes <= 0 || body.TTLMinutes > 1440 {
		body.TTLMinutes = 15
	}
	token, expires, e := a.s.CreateEnrollment(r.Context(), body.Kind, body.SiteID, u.ID, time.Duration(body.TTLMinutes)*time.Minute)
	if e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "create", "enrollment_token", body.Kind)
	jsonOut(w, 201, map[string]any{"token": token, "expiresAt": expires})
}
func (a *API) enroll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
		Name  string `json:"name"`
		CSR   string `json:"csr"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Token == "" || body.Name == "" || body.CSR == "" {
		problem(w, 400, errText("token, name and csr are required"))
		return
	}
	enrollment, e := a.s.ConsumeEnrollment(r.Context(), body.Token, body.Name)
	if e != nil {
		problem(w, 401, e)
		return
	}
	identity, cert, e := a.authority.IssueClient(r.Context(), enrollment, []byte(body.CSR))
	if e != nil {
		problem(w, 400, e)
		return
	}
	jsonOut(w, 201, map[string]any{"identity": identity, "certificate": string(cert), "caCertificate": string(a.authority.CACertificate())})
}
func (a *API) identities(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role != "administrator" && u.Role != "manager" {
		problem(w, 403, errText("manager role required"))
		return
	}
	v, e := a.s.Identities(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, v)
}
func (a *API) revokeIdentity(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role != "administrator" && u.Role != "manager" {
		problem(w, 403, errText("manager role required"))
		return
	}
	id := r.PathValue("id")
	if e := a.s.RevokeIdentity(r.Context(), id); e != nil {
		problem(w, 404, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "revoke", "identity", id)
	w.WriteHeader(204)
}
