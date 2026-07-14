package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/TerminalAddict/golive-nms/internal/pki"
	"github.com/TerminalAddict/golive-nms/internal/sso"
	"github.com/TerminalAddict/golive-nms/internal/store"
)

type API struct {
	s                            *store.Store
	events                       *EventBus
	logger                       *slog.Logger
	agentToken                   string
	monitUsername, monitPassword string
	metricsURL                   string
	authority                    *pki.Authority
	notify                       func(string, string, string, string)
	oidc                         *sso.OIDC
}

func New(s *store.Store, e *EventBus, l *slog.Logger, token, user, password, metricsURL string, authority *pki.Authority, notify func(string, string, string, string), oidc *sso.OIDC) *API {
	return &API{s: s, events: e, logger: l, agentToken: token, monitUsername: user, monitPassword: password, metricsURL: metricsURL, authority: authority, notify: notify, oidc: oidc}
}
func (a *API) Routes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/auth/login", a.login)
	m.HandleFunc("POST /api/v1/auth/logout", a.logout)
	m.HandleFunc("GET /api/v1/auth/me", a.me)
	m.HandleFunc("GET /api/v1/auth/config", a.authConfig)
	m.HandleFunc("GET /api/v1/auth/oidc/start", a.oidcStart)
	m.HandleFunc("GET /api/v1/auth/oidc/callback", a.oidcCallback)
	m.HandleFunc("GET /api/v1/users", a.users)
	m.HandleFunc("POST /api/v1/users", a.createUser)
	m.HandleFunc("DELETE /api/v1/users/{id}", a.deleteUser)
	m.HandleFunc("PATCH /api/v1/users/{id}", a.updateUser)
	m.HandleFunc("GET /api/v1/api-tokens", a.tokens)
	m.HandleFunc("POST /api/v1/api-tokens", a.createToken)
	m.HandleFunc("DELETE /api/v1/api-tokens/{id}", a.deleteToken)
	m.HandleFunc("GET /api/v1/credentials", a.credentials)
	m.HandleFunc("POST /api/v1/credentials", a.createCredential)
	m.HandleFunc("DELETE /api/v1/credentials/{id}", a.deleteCredential)
	m.HandleFunc("GET /api/v1/notification-channels", a.channels)
	m.HandleFunc("POST /api/v1/notification-channels", a.createChannel)
	m.HandleFunc("DELETE /api/v1/notification-channels/{id}", a.deleteChannel)
	m.HandleFunc("GET /api/v1/sites", a.sites)
	m.HandleFunc("POST /api/v1/sites", a.createSite)
	m.HandleFunc("DELETE /api/v1/sites/{id}", a.deleteSite)
	m.HandleFunc("GET /api/v1/users/{id}/sites", a.userSites)
	m.HandleFunc("PUT /api/v1/users/{id}/sites", a.setUserSites)
	m.HandleFunc("POST /api/v1/enrollment-tokens", a.createEnrollment)
	m.HandleFunc("POST /api/v1/enroll", a.enroll)
	m.HandleFunc("GET /api/v1/identities", a.identities)
	m.HandleFunc("DELETE /api/v1/identities/{id}", a.revokeIdentity)
	m.HandleFunc("GET /api/v1/collector/assignments", a.collectorAssignments)
	m.HandleFunc("POST /api/v1/collector/results", a.collectorResult)
	m.HandleFunc("GET /api/v1/device-events", a.deviceEvents)
	m.HandleFunc("GET /api/v1/config-profiles", a.configProfiles)
	m.HandleFunc("POST /api/v1/config-profiles", a.createConfigProfile)
	m.HandleFunc("DELETE /api/v1/config-profiles/{id}", a.deleteConfigProfile)
	m.HandleFunc("POST /api/v1/config-profiles/{id}/trigger", a.triggerConfigBackup)
	m.HandleFunc("GET /api/v1/devices/{deviceId}/config-snapshots", a.configSnapshots)
	m.HandleFunc("GET /api/v1/config-diff", a.configDiff)
	m.HandleFunc("GET /api/v1/action-templates", a.actionTemplates)
	m.HandleFunc("POST /api/v1/action-templates", a.createActionTemplate)
	m.HandleFunc("DELETE /api/v1/action-templates/{id}", a.deleteActionTemplate)
	m.HandleFunc("GET /api/v1/remediation-jobs", a.remediationJobs)
	m.HandleFunc("POST /api/v1/remediation-jobs", a.queueRemediation)
	m.HandleFunc("GET /api/v1/remediation-settings", a.remediationSetting)
	m.HandleFunc("PUT /api/v1/remediation-settings", a.setRemediationSetting)
	m.HandleFunc("GET /api/v1/maintenance-windows", a.maintenanceWindows)
	m.HandleFunc("POST /api/v1/maintenance-windows", a.createMaintenanceWindow)
	m.HandleFunc("DELETE /api/v1/maintenance-windows/{id}", a.deleteMaintenanceWindow)
	m.HandleFunc("GET /api/v1/agent/actions", a.agentAction)
	m.HandleFunc("POST /api/v1/agent/actions/results", a.agentActionResult)
	m.HandleFunc("GET /healthz", a.health)
	m.HandleFunc("GET /api/v1/summary", a.summary)
	m.HandleFunc("GET /api/v1/devices", a.devices)
	m.HandleFunc("POST /api/v1/devices", a.createDevice)
	m.HandleFunc("PATCH /api/v1/devices/{id}", a.updateDevice)
	m.HandleFunc("DELETE /api/v1/devices/{id}", a.deleteDevice)
	m.HandleFunc("GET /api/v1/monit-services", a.monitServices)
	m.HandleFunc("GET /api/v1/checks", a.checks)
	m.HandleFunc("POST /api/v1/checks", a.createCheck)
	m.HandleFunc("GET /api/v1/checks/{id}/history", a.checkHistory)
	m.HandleFunc("GET /api/v1/incidents", a.incidents)
	m.HandleFunc("POST /api/v1/incidents/{id}/acknowledge", a.ack)
	m.HandleFunc("POST /api/v1/incidents/{id}/assign", a.assignIncident)
	m.HandleFunc("POST /api/v1/incidents/{id}/notes", a.noteIncident)
	m.HandleFunc("GET /api/v1/events", a.stream)
	m.HandleFunc("POST /api/v1/agent/report", a.agentReport)
	m.HandleFunc("GET /api/v1/agent-inventory", a.agentInventory)
	m.HandleFunc("POST /collector", a.monitCollector)
}
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	u, token, e := a.s.Login(r.Context(), body.Email, body.Password)
	if e != nil {
		problem(w, http.StatusUnauthorized, e)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "golive_session", Value: token, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	jsonOut(w, 200, u)
}
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("golive_session"); e == nil {
		_ = a.s.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "golive_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(204)
}
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok {
		problem(w, 401, errText("authentication required"))
		return
	}
	jsonOut(w, 200, u)
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, err error) {
	jsonOut(w, status, map[string]any{"error": map[string]any{"status": status, "message": err.Error()}})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		problem(w, 400, err)
		return false
	}
	return true
}
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextTimeout(r, 2*time.Second)
	defer cancel()
	if err := a.s.Pool.Ping(ctx); err != nil {
		problem(w, 503, err)
		return
	}
	jsonOut(w, 200, map[string]string{"status": "ok"})
}
func (a *API) summary(w http.ResponseWriter, r *http.Request) {
	d, e := a.s.Devices(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	inc, e := a.s.Incidents(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	d = filterDevices(d, scope)
	inc = filterIncidents(inc, scope)
	var v store.Summary
	v.Total = len(d)
	for _, x := range d {
		switch x.Status {
		case "up":
			v.Up++
		case "down":
			v.Down++
		case "degraded":
			v.Degraded++
		default:
			v.Unknown++
		}
	}
	for _, x := range inc {
		if x.State == "open" || x.State == "acknowledged" {
			v.OpenIncidents++
		}
	}
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, v)
}
func (a *API) devices(w http.ResponseWriter, r *http.Request) {
	v, e := a.s.Devices(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, filterDevices(v, scope))
}
func (a *API) createDevice(w http.ResponseWriter, r *http.Request) {
	var v store.Device
	if !decode(w, r, &v) {
		return
	}
	v.Name = strings.TrimSpace(v.Name)
	v.Address = strings.TrimSpace(v.Address)
	if v.Name == "" || v.Address == "" {
		problem(w, 400, errText("name and address are required"))
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	if scope.restricted {
		if v.SiteID == "" && len(scope.allowed) == 1 {
			for id := range scope.allowed {
				v.SiteID = id
			}
		}
		if !scope.can(v.SiteID) {
			problem(w, 403, errText("site access denied"))
			return
		}
	}
	v, e = a.s.CreateDevice(r.Context(), v)
	if e != nil {
		problem(w, 400, e)
		return
	}
	a.events.Publish("device.created", v)
	a.audit(r, "create", "device", v.ID)
	jsonOut(w, 201, v)
}
func (a *API) updateDevice(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role != "administrator" && u.Role != "manager" && u.Role != "site_manager" {
		problem(w, 403, errText("manager role required"))
		return
	}
	id := r.PathValue("id")
	currentSite, e := a.s.DeviceSite(r.Context(), id)
	if e != nil {
		problem(w, 404, errText("device not found"))
		return
	}
	var v store.Device
	if !decode(w, r, &v) {
		return
	}
	v.ID = id
	v.Name = strings.TrimSpace(v.Name)
	v.Address = strings.TrimSpace(v.Address)
	if v.SiteID == "" {
		v.SiteID = currentSite
	}
	if v.Name == "" || v.Address == "" {
		problem(w, 400, errText("name and address are required"))
		return
	}
	validKinds := map[string]bool{"server": true, "router": true, "switch": true, "other": true}
	if !validKinds[v.Kind] {
		problem(w, 400, errText("unsupported device type"))
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	if !scope.can(currentSite) || !scope.can(v.SiteID) {
		problem(w, 403, errText("site access denied"))
		return
	}
	v, e = a.s.UpdateDevice(r.Context(), v)
	if e != nil {
		problem(w, 400, e)
		return
	}
	a.events.Publish("device.updated", v)
	a.audit(r, "update", "device", v.ID)
	jsonOut(w, 200, v)
}
func (a *API) monitServices(w http.ResponseWriter, r *http.Request) {
	v, e := a.s.MonitServices(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, filterMonitServices(v, scope))
}
func (a *API) deleteDevice(w http.ResponseWriter, r *http.Request) {
	site, e := a.s.DeviceSite(r.Context(), r.PathValue("id"))
	if e != nil {
		problem(w, 404, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	if !scope.can(site) {
		problem(w, 403, errText("site access denied"))
		return
	}
	if e := a.s.DeleteDevice(r.Context(), r.PathValue("id")); e != nil {
		problem(w, 404, e)
		return
	}
	a.audit(r, "delete", "device", r.PathValue("id"))
	w.WriteHeader(204)
}
func (a *API) checks(w http.ResponseWriter, r *http.Request) {
	v, e := a.s.Checks(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, filterChecks(v, scope))
}
func (a *API) createCheck(w http.ResponseWriter, r *http.Request) {
	var v store.Check
	if !decode(w, r, &v) {
		return
	}
	if v.DeviceID == "" || v.Name == "" || v.Target == "" {
		problem(w, 400, errText("deviceId, name and target are required"))
		return
	}
	site, e := a.s.DeviceSite(r.Context(), v.DeviceID)
	if e != nil {
		problem(w, 404, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	if !scope.can(site) {
		problem(w, 403, errText("site access denied"))
		return
	}
	validTypes := map[string]bool{"ping": true, "http": true, "tcp": true, "snmp": true, "dns": true, "tls": true, "ssh": true, "smtp": true, "mysql": true, "postgres": true, "routeros": true}
	if !validTypes[v.Type] {
		problem(w, 400, errText("unsupported check type"))
		return
	}
	if (v.Type == "snmp" || v.Type == "routeros") && v.CredentialID == "" {
		problem(w, 400, errText("SNMP checks require a credential"))
		return
	}
	v, e = a.s.CreateCheck(r.Context(), v)
	if e != nil {
		problem(w, 400, e)
		return
	}
	a.events.Publish("check.created", v)
	a.audit(r, "create", "check", v.ID)
	jsonOut(w, 201, v)
}
func (a *API) checkHistory(w http.ResponseWriter, r *http.Request) {
	site, e := a.s.CheckSite(r.Context(), r.PathValue("id"))
	if e != nil {
		problem(w, 404, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	if !scope.can(site) {
		problem(w, 403, errText("site access denied"))
		return
	}
	since := time.Now().Add(-24 * time.Hour)
	if raw := r.URL.Query().Get("since"); raw != "" {
		if parsed, e := time.Parse(time.RFC3339, raw); e == nil {
			since = parsed
		} else {
			problem(w, 400, e)
			return
		}
	}
	v, e := a.s.CheckHistory(r.Context(), r.PathValue("id"), since)
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, v)
}
func (a *API) incidents(w http.ResponseWriter, r *http.Request) {
	v, e := a.s.Incidents(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, filterIncidents(v, scope))
}
func (a *API) ack(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role == "viewer" {
		problem(w, 403, errText("manager role required"))
		return
	}
	all, e := a.s.Incidents(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	allowed := false
	for _, i := range all {
		if i.ID == r.PathValue("id") {
			allowed = scope.can(i.SiteID)
			break
		}
	}
	if !allowed {
		problem(w, 403, errText("site access denied"))
		return
	}
	if e := a.s.Acknowledge(r.Context(), r.PathValue("id")); e != nil {
		problem(w, 404, e)
		return
	}
	a.events.Publish("incident.acknowledged", map[string]string{"id": r.PathValue("id")})
	a.audit(r, "acknowledge", "incident", r.PathValue("id"))
	w.WriteHeader(204)
}
func (a *API) assignIncident(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role == "viewer" {
		problem(w, 403, errText("manager role required"))
		return
	}
	if !a.incidentAllowed(w, r) {
		return
	}
	var body struct {
		Assigned bool `json:"assigned"`
	}
	if !decode(w, r, &body) {
		return
	}
	userID := ""
	if body.Assigned {
		userID = u.ID
	}
	if e := a.s.AssignIncident(r.Context(), r.PathValue("id"), userID); e != nil {
		problem(w, 404, e)
		return
	}
	a.audit(r, "assign", "incident", r.PathValue("id"))
	w.WriteHeader(204)
}
func (a *API) noteIncident(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role == "viewer" {
		problem(w, 403, errText("manager role required"))
		return
	}
	if !a.incidentAllowed(w, r) {
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	if e := a.s.NoteIncident(r.Context(), r.PathValue("id"), body.Note); e != nil {
		problem(w, 400, e)
		return
	}
	a.audit(r, "note", "incident", r.PathValue("id"))
	w.WriteHeader(204)
}
func (a *API) incidentAllowed(w http.ResponseWriter, r *http.Request) bool {
	all, e := a.s.Incidents(r.Context())
	if e != nil {
		problem(w, 500, e)
		return false
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return false
	}
	for _, i := range all {
		if i.ID == r.PathValue("id") && scope.can(i.SiteID) {
			return true
		}
	}
	problem(w, 403, errText("site access denied"))
	return false
}
func (a *API) audit(r *http.Request, action, kind, id string) {
	if u, ok := CurrentUser(r.Context()); ok {
		if e := a.s.Audit(r.Context(), u.ID, action, kind, id); e != nil {
			a.logger.Error("audit write failed", "error", e)
		}
	}
}
func (a *API) stream(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		problem(w, 500, errText("stream unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch, done := a.events.Subscribe()
	scope, _ := a.scope(r)
	defer done()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			if scope.restricted {
				e.Data = nil
			}
			w.Write([]byte("data: "))
			w.Write(eventJSON(e))
			w.Write([]byte("\n\n"))
			f.Flush()
		}
	}
}

type errText string

func (e errText) Error() string { return string(e) }
