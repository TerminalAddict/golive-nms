package api

import (
	"compress/gzip"
	"crypto/subtle"
	"encoding/xml"
	"github.com/TerminalAddict/golive-nms/internal/store"
	"io"
	"net/http"
	"strings"
	"time"
)

type monitXML struct {
	XMLName     xml.Name `xml:"monit"`
	ID          string   `xml:"id,attr"`
	Version     string   `xml:"version,attr"`
	Incarnation int64    `xml:"incarnation,attr"`
	Server      struct {
		Hostname string `xml:"localhostname"`
	} `xml:"server"`
	Services []struct {
		Name          string `xml:"name,attr"`
		Type          int    `xml:"type"`
		Status        int64  `xml:"status"`
		Monitor       int    `xml:"monitor"`
		CollectedSec  int64  `xml:"collected_sec"`
		CollectedUSec int64  `xml:"collected_usec"`
	} `xml:"services>service"`
	Event *struct {
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
	var doc monitXML
	decoder := xml.NewDecoder(reader)
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	if e := decoder.Decode(&doc); e != nil {
		problem(w, 400, e)
		return
	}
	if doc.ID == "" || doc.Server.Hostname == "" {
		problem(w, 400, errText("Monit id and hostname are required"))
		return
	}
	report := store.MonitReport{ID: doc.ID, Version: doc.Version, Hostname: doc.Server.Hostname, Incarnation: doc.Incarnation}
	for _, v := range doc.Services {
		report.Services = append(report.Services, store.MonitService{Name: v.Name, Type: v.Type, Status: v.Status, Monitor: v.Monitor, Collected: unixMicro(v.CollectedSec, v.CollectedUSec)})
	}
	if doc.Event != nil {
		e := doc.Event
		report.Event = &store.MonitEvent{Service: e.Service, Message: e.Message, ID: e.ID, State: e.State, Action: e.Action, Collected: unixMicro(e.CollectedSec, e.CollectedUSec)}
	}
	device, e := a.s.RecordMonit(r.Context(), report)
	if e != nil {
		problem(w, 500, e)
		return
	}
	a.events.Publish("monit.report", map[string]string{"deviceId": device, "monitId": doc.ID})
	w.Header().Set("Server", "mmonit/4.3")
	w.WriteHeader(http.StatusNoContent)
}
func unixMicro(sec, usec int64) time.Time {
	if sec <= 0 {
		return time.Now().UTC()
	}
	return time.Unix(sec, usec*int64(time.Microsecond)).UTC()
}
