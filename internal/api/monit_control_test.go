package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendMonitAction(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/_doaction" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "golive" || password != "secret" {
			t.Errorf("unexpected basic auth %q %q", username, password)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("service") != "postfix smtp" || r.Form.Get("action") != "restart" || r.Form.Get("format") != "text" {
			t.Errorf("unexpected form: %#v", r.Form)
		}
		cookie, err := r.Cookie("securitytoken")
		if err != nil || cookie.Value == "" || cookie.Value != r.Form.Get("securitytoken") {
			t.Errorf("security token cookie and form do not match")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := sendMonitAction(context.Background(), server.Client(), server.URL, "golive", "secret", "postfix smtp", "restart"); err != nil {
		t.Fatal(err)
	}
}

func TestSendMonitActionReportsHTTPError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "service not found", http.StatusBadRequest)
	}))
	defer server.Close()
	if err := sendMonitAction(context.Background(), server.Client(), server.URL, "u", "p", "missing", "start"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestSendMonitProbe(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/_status" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "golive" || password != "secret" {
			t.Errorf("unexpected basic auth %q %q", username, password)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("format") != "text" || r.Form.Get("securitytoken") == "" {
			t.Errorf("unexpected form: %#v", r.Form)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := sendMonitProbe(context.Background(), server.Client(), server.URL, "golive", "secret"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateMonitURL(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"ftp://host:2812", "http://user:pass@host:2812", "http://host:2812/path", "host:2812"} {
		if _, err := validateMonitURL(raw); err == nil {
			t.Errorf("expected %q to be rejected", raw)
		}
	}
	if got, err := validateMonitURL("https://monit.example:2812/"); err != nil || got != "https://monit.example:2812" {
		t.Fatalf("got %q, %v", got, err)
	}
}
