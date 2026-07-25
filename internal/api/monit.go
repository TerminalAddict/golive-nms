package api

import (
	"compress/gzip"
	"crypto/subtle"
	"encoding/xml"
	"fmt"
	"github.com/TerminalAddict/golive-nms/internal/store"
	"io"
	"net/http"
	"strings"
	"time"
)

type monitServiceXML struct {
	NameAttribute string `xml:"name,attr"`
	NameElement   string `xml:"name"`
	TypeAttribute *int   `xml:"type,attr"`
	TypeElement   *int   `xml:"type"`
	Status        int64  `xml:"status"`
	Monitor       int    `xml:"monitor"`
	CollectedSec  int64  `xml:"collected_sec"`
	CollectedUSec int64  `xml:"collected_usec"`
}

type monitXML struct {
	XMLName     xml.Name `xml:"monit"`
	ID          string   `xml:"id,attr"`
	Version     string   `xml:"version,attr"`
	Incarnation int64    `xml:"incarnation,attr"`
	Server      struct {
		ID          string `xml:"id"`
		Version     string `xml:"version"`
		Incarnation int64  `xml:"incarnation"`
		Hostname    string `xml:"localhostname"`
	} `xml:"server"`
	Services *struct {
		Items []monitServiceXML `xml:"service"`
	} `xml:"services"`
	DirectServices []monitServiceXML `xml:"service"`
	Event          *struct {
		CollectedSec  int64  `xml:"collected_sec"`
		CollectedUSec int64  `xml:"collected_usec"`
		Service       string `xml:"service"`
		Type          int    `xml:"type"`
		ID            int64  `xml:"id"`
		State         int    `xml:"state"`
		Action        int    `xml:"action"`
		Message       string `xml:"message"`
	} `xml:"event"`
}

func decodeMonitReport(reader io.Reader) (store.MonitReport, error) {
	var doc monitXML
	decoder := xml.NewDecoder(reader)
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	if err := decoder.Decode(&doc); err != nil {
		return store.MonitReport{}, err
	}
	report := store.MonitReport{
		ID:           firstNonEmpty(doc.ID, doc.Server.ID),
		Version:      firstNonEmpty(doc.Version, doc.Server.Version),
		Hostname:     doc.Server.Hostname,
		Incarnation:  doc.Incarnation,
		FullSnapshot: doc.Services != nil || len(doc.DirectServices) > 0,
	}
	if report.Incarnation == 0 {
		report.Incarnation = doc.Server.Incarnation
	}
	services := doc.DirectServices
	if doc.Services != nil {
		services = doc.Services.Items
	}
	for _, service := range services {
		name := firstNonEmpty(service.NameAttribute, service.NameElement)
		serviceType := 0
		if service.TypeElement != nil {
			serviceType = *service.TypeElement
		}
		if service.TypeAttribute != nil {
			serviceType = *service.TypeAttribute
		}
		if name == "" {
			continue
		}
		report.Services = append(report.Services, store.MonitService{
			Name:      name,
			Type:      serviceType,
			Status:    service.Status,
			Monitor:   service.Monitor,
			Collected: unixMicro(service.CollectedSec, service.CollectedUSec),
		})
	}
	if doc.Event != nil {
		event := doc.Event
		report.Event = &store.MonitEvent{Service: event.Service, Message: event.Message, ID: event.ID, State: event.State, Action: event.Action, Collected: unixMicro(event.CollectedSec, event.CollectedUSec)}
	}
	if report.ID == "" || report.Hostname == "" {
		return store.MonitReport{}, fmt.Errorf("Monit id and hostname are required")
	}
	return report, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func (a *API) monitCollector(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	if !ok || a.monitPassword == "" || subtle.ConstantTimeCompare([]byte(user), []byte(a.monitUsername)) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte(a.monitPassword)) != 1 {
		w.Header().Set("WWW-Authenticate", `Basic realm="GoLive Monit collector"`)
		problem(w, 401, errText("invalid Monit collector credentials"))
		return
	}
	reader := io.Reader(http.MaxBytesReader(w, r.Body, 8<<20))
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, e := gzip.NewReader(reader)
		if e != nil {
			problem(w, 400, e)
			return
		}
		defer gz.Close()
		reader = gz
	}
	report, e := decodeMonitReport(reader)
	if e != nil {
		problem(w, 400, e)
		return
	}
	device, e := a.s.RecordMonit(r.Context(), report)
	if e != nil {
		problem(w, 500, e)
		return
	}
	a.events.Publish("monit.report", map[string]string{"deviceId": device, "monitId": report.ID})
	w.Header().Set("Server", "mmonit/4.3")
	w.WriteHeader(http.StatusNoContent)
}
func unixMicro(sec, usec int64) time.Time {
	if sec <= 0 {
		return time.Now().UTC()
	}
	return time.Unix(sec, usec*int64(time.Microsecond)).UTC()
}
