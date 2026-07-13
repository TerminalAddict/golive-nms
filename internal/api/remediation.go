package api

import (
	"encoding/base64"
	"encoding/json"
	"github.com/golive-nms/golive-nms/internal/store"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func (a *API) actionTemplates(w http.ResponseWriter, r *http.Request) {
	v, e := a.s.ActionTemplates(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, v)
}
func (a *API) createActionTemplate(w http.ResponseWriter, r *http.Request) {
	u, ok := requireAdminUser(w, r)
	if !ok {
		return
	}
	var v store.ActionTemplate
	if !decode(w, r, &v) {
		return
	}
	v, e := a.s.CreateActionTemplate(r.Context(), v, u.ID)
	if e != nil {
		problem(w, 400, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "create", "action_template", v.ID)
	jsonOut(w, 201, v)
}
func (a *API) deleteActionTemplate(w http.ResponseWriter, r *http.Request) {
	u, ok := requireAdminUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if e := a.s.DeleteActionTemplate(r.Context(), id); e != nil {
		problem(w, 404, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "disable", "action_template", id)
	w.WriteHeader(204)
}
func (a *API) remediationJobs(w http.ResponseWriter, r *http.Request) {
	v, e := a.s.RemediationJobs(r.Context())
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
	for _, j := range v {
		if scope.can(j.SiteID) {
			out = append(out, j)
		}
	}
	jsonOut(w, 200, out)
}
func (a *API) queueRemediation(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if u.Role == "viewer" {
		problem(w, 403, errText("manager role required"))
		return
	}
	var body struct {
		TemplateID string `json:"templateId"`
		DeviceID   string `json:"deviceId"`
	}
	if !decode(w, r, &body) {
		return
	}
	site, e := a.s.DeviceSite(r.Context(), body.DeviceID)
	if e != nil {
		problem(w, 404, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil || !scope.can(site) {
		problem(w, 403, errText("site access denied"))
		return
	}
	v, e := a.s.QueueRemediation(r.Context(), body.TemplateID, body.DeviceID, u.ID, false)
	if e != nil {
		problem(w, 409, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "queue", "remediation_job", v.ID)
	jsonOut(w, 201, v)
}
func (a *API) remediationSetting(w http.ResponseWriter, r *http.Request) {
	enabled, e := a.s.RemediationEnabled(r.Context())
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, map[string]bool{"enabled": enabled})
}
func (a *API) setRemediationSetting(w http.ResponseWriter, r *http.Request) {
	u, ok := requireAdminUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(w, r, &body) {
		return
	}
	if e := a.s.SetRemediationEnabled(r.Context(), body.Enabled); e != nil {
		problem(w, 500, e)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "set", "remediation_enabled", jsonBool(body.Enabled))
	w.WriteHeader(204)
}
func (a *API) agentAction(w http.ResponseWriter, r *http.Request) {
	identity, ok := a.agentIdentity(w, r)
	if !ok {
		return
	}
	payload, e := a.s.ClaimAgentAction(r.Context(), identity.ID)
	if e == pgx.ErrNoRows {
		w.WriteHeader(204)
		return
	}
	if e != nil {
		problem(w, 500, e)
		return
	}
	raw, _ := json.Marshal(payload)
	signature, e := a.authority.Sign(raw)
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, map[string]any{"payload": payload, "signature": base64.RawStdEncoding.EncodeToString(signature)})
}
func (a *API) agentActionResult(w http.ResponseWriter, r *http.Request) {
	identity, ok := a.agentIdentity(w, r)
	if !ok {
		return
	}
	var body struct {
		JobID   string `json:"jobId"`
		Success bool   `json:"success"`
		Output  string `json:"output"`
		Error   string `json:"error"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len(body.Output) > 65536 {
		body.Output = body.Output[:65536]
	}
	if len(body.Error) > 4096 {
		body.Error = body.Error[:4096]
	}
	if e := a.s.CompleteAgentAction(r.Context(), identity.ID, body.JobID, body.Success, body.Output, body.Error); e != nil {
		problem(w, 404, e)
		return
	}
	a.events.Publish("remediation.result", map[string]any{"jobId": body.JobID, "success": body.Success})
	w.WriteHeader(204)
}
func (a *API) agentIdentity(w http.ResponseWriter, r *http.Request) (store.Identity, bool) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		problem(w, 401, errText("agent client certificate required"))
		return store.Identity{}, false
	}
	v, e := a.s.RequireAgent(r.Context(), r.TLS.PeerCertificates[0].SerialNumber.String())
	if e != nil {
		problem(w, 403, e)
		return v, false
	}
	return v, true
}
func jsonBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
