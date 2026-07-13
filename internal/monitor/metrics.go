package monitor

import (
	"context"
	"fmt"
	"github.com/TerminalAddict/golive-nms/internal/store"
	"io"
	"net/http"
	"strings"
	"time"
)

func (e *Engine) writeMetrics(ctx context.Context, c store.DueCheck, up bool, latency float64) {
	if e.metricsURL == "" {
		return
	}
	value := 0
	if up {
		value = 1
	}
	labels := fmt.Sprintf(`check_id="%s",device_id="%s",type="%s"`, metricEscape(c.CheckID), metricEscape(c.DeviceID), metricEscape(c.Type))
	stamp := time.Now().UnixMilli()
	body := fmt.Sprintf("golive_check_up{%s} %d %d\ngolive_check_latency_milliseconds{%s} %.3f %d\n", labels, value, stamp, labels, latency, stamp)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(e.metricsURL, "/")+"/api/v1/import/prometheus", strings.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		e.logger.Warn("write metrics", "error", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		e.logger.Warn("write metrics", "status", resp.StatusCode)
	}
}
func metricEscape(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return strings.ReplaceAll(v, "\n", `\n`)
}
