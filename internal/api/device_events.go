package api

import (
	"net/http"
	"strconv"
)

func (a *API) deviceEvents(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	v, e := a.s.DeviceEvents(r.Context(), r.URL.Query().Get("protocol"), r.URL.Query().Get("q"), limit)
	if e != nil {
		problem(w, 500, e)
		return
	}
	scope, e := a.scope(r)
	if e != nil {
		problem(w, 500, e)
		return
	}
	if scope.restricted {
		out := v[:0]
		for _, event := range v {
			if event.SiteID != "" && scope.can(event.SiteID) {
				out = append(out, event)
			}
		}
		v = out
	}
	jsonOut(w, 200, v)
}
