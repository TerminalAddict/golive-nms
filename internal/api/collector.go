package api

import (
	"context"
	"github.com/golive-nms/golive-nms/internal/store"
	"net/http"
)

func (a *API) collectorIdentity(w http.ResponseWriter, r *http.Request) (store.Identity, bool) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		problem(w, 401, errText("collector client certificate required"))
		return store.Identity{}, false
	}
	v, e := a.s.RequireCollector(r.Context(), r.TLS.PeerCertificates[0].SerialNumber.String())
	if e != nil {
		problem(w, 403, e)
		return v, false
	}
	return v, true
}
func (a *API) collectorAssignments(w http.ResponseWriter, r *http.Request) {
	identity, ok := a.collectorIdentity(w, r)
	if !ok {
		return
	}
	v, e := a.s.CollectorAssignments(r.Context(), identity.SiteID)
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, v)
}
func (a *API) collectorResult(w http.ResponseWriter, r *http.Request) {
	identity, ok := a.collectorIdentity(w, r)
	if !ok {
		return
	}
	var body struct {
		CheckID   string  `json:"checkId"`
		Up        bool    `json:"up"`
		LatencyMS float64 `json:"latencyMs"`
		Message   string  `json:"message"`
	}
	if !decode(w, r, &body) {
		return
	}
	check, e := a.s.CollectorCheck(r.Context(), body.CheckID, identity.SiteID)
	if e != nil {
		problem(w, 404, e)
		return
	}
	if e = a.s.RecordResult(r.Context(), check, body.Up, body.LatencyMS, body.Message); e != nil {
		problem(w, 500, e)
		return
	}
	newStatus := "down"
	if body.Up {
		newStatus = "up"
	}
	if a.notify != nil && !check.Maintenance && ((newStatus == "down" && check.Status != "down" && !check.ParentDown) || (newStatus == "up" && check.Status == "down")) {
		event := "opened"
		if body.Up {
			event = "resolved"
		}
		a.notify(check.DeviceName+" is unavailable", check.DeviceID, check.DeviceName, event)
	}
	if newStatus == "down" && check.Status != "down" && !check.ParentDown && !check.Maintenance {
		go a.s.QueueAutomaticRemediation(context.Background(), check.DeviceID, check.Type)
	}
	a.events.Publish("check.result", body)
	w.WriteHeader(204)
}
