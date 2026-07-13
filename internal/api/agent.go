package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/TerminalAddict/golive-nms/internal/store"
)

func (a *API) agentReport(w http.ResponseWriter, r *http.Request) {
	authenticated := false
	serial := ""
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		cert := r.TLS.PeerCertificates[0]
		if a.s.IdentityActive(r.Context(), cert.SerialNumber.String()) {
			authenticated = true
			serial = cert.SerialNumber.String()
			a.s.TouchIdentity(r.Context(), cert.SerialNumber.String())
		}
	}
	want, got := []byte("Bearer "+a.agentToken), []byte(r.Header.Get("Authorization"))
	if !authenticated && (a.agentToken == "" || len(want) != len(got) || subtle.ConstantTimeCompare(want, got) != 1) {
		problem(w, http.StatusUnauthorized, errText("invalid agent token"))
		return
	}
	var report store.AgentReport
	if !decode(w, r, &report) {
		return
	}
	if report.AgentID == "" || report.Hostname == "" {
		problem(w, 400, errText("agentId and hostname are required"))
		return
	}
	deviceID, err := a.s.RecordAgentReport(r.Context(), report, serial)
	if err != nil {
		problem(w, 500, err)
		return
	}
	a.events.Publish("agent.report", map[string]string{"deviceId": deviceID, "agentId": report.AgentID})
	go a.writeAgentMetrics(context.Background(), deviceID, report.Metrics)
	jsonOut(w, 202, map[string]string{"deviceId": deviceID})
}

func (a *API) agentInventory(w http.ResponseWriter, r *http.Request) {
	v, err := a.s.AgentInventory(r.Context())
	if err != nil {
		problem(w, 500, err)
		return
	}
	scope, err := a.scope(r)
	if err != nil {
		problem(w, 500, err)
		return
	}
	out := v[:0]
	for _, item := range v {
		if scope.can(item.SiteID) {
			out = append(out, item)
		}
	}
	jsonOut(w, 200, out)
}

func (a *API) writeAgentMetrics(ctx context.Context, deviceID string, metrics map[string]any) {
	if a.metricsURL == "" {
		return
	}
	var body bytes.Buffer
	stamp := time.Now().UnixMilli()
	for key, value := range metrics {
		var n float64
		switch v := value.(type) {
		case float64:
			n = v
		case int:
			n = float64(v)
		case int64:
			n = float64(v)
		case uint64:
			n = float64(v)
		default:
			continue
		}
		fmt.Fprintf(&body, "golive_agent_%s{device_id=\"%s\"} %g %d\n", metricName(key), deviceID, n, stamp)
	}
	if body.Len() == 0 {
		return
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.metricsURL, "/")+"/api/v1/import/prometheus", &body)
	if e != nil {
		return
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, e := http.DefaultClient.Do(req)
	if e == nil {
		resp.Body.Close()
	}
}
func metricName(v string) string {
	var b strings.Builder
	for i, r := range v {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
