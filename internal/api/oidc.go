package api

import (
	"github.com/golive-nms/golive-nms/internal/sso"
	"net/http"
)

func (a *API) authConfig(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, 200, map[string]bool{"oidcEnabled": a.oidc != nil})
}
func (a *API) oidcStart(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		problem(w, 404, errText("OIDC is not configured"))
		return
	}
	state, e := sso.State()
	if e != nil {
		problem(w, 500, e)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "golive_oidc_state", Value: state, Path: "/api/v1/auth/oidc/", MaxAge: 600, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, a.oidc.AuthURL(state), http.StatusFound)
}
func (a *API) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		problem(w, 404, errText("OIDC is not configured"))
		return
	}
	cookie, e := r.Cookie("golive_oidc_state")
	if e != nil || cookie.Value == "" || cookie.Value != r.URL.Query().Get("state") {
		problem(w, 400, errText("invalid OIDC state"))
		return
	}
	claims, e := a.oidc.Exchange(r.Context(), r.URL.Query().Get("code"))
	if e != nil {
		problem(w, 401, e)
		return
	}
	name := claims.Name
	if name == "" {
		name = claims.PreferredUsername
	}
	u, e := a.s.UpsertOIDCUser(r.Context(), claims.Email, name)
	if e != nil {
		problem(w, 500, e)
		return
	}
	token, e := a.s.CreateSession(r.Context(), u.ID)
	if e != nil {
		problem(w, 500, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "oidc_login", "session", "")
	http.SetCookie(w, &http.Cookie{Name: "golive_session", Value: token, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	http.SetCookie(w, &http.Cookie{Name: "golive_oidc_state", Path: "/api/v1/auth/oidc/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", http.StatusFound)
}
