package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TerminalAddict/golive-nms/internal/store"
)

var monitActions = map[string]bool{"start": true, "stop": true, "restart": true, "monitor": true, "unmonitor": true}

func validateMonitURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("Monit URL must be an http:// or https:// URL with a host and port")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("Monit URL must not contain credentials, a path, query, or fragment")
	}
	u.Path = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func sendMonitAction(ctx context.Context, client *http.Client, endpoint, username, password, service, action string) error {
	return sendMonitRequest(ctx, client, endpoint, username, password, "/_doaction", url.Values{"action": {action}, "service": {service}})
}

func sendMonitProbe(ctx context.Context, client *http.Client, endpoint, username, password string) error {
	return sendMonitRequest(ctx, client, endpoint, username, password, "/_status", url.Values{})
}

func sendMonitRequest(ctx context.Context, client *http.Client, endpoint, username, password, path string, form url.Values) error {
	base, err := validateMonitURL(endpoint)
	if err != nil {
		return err
	}
	tokenBytes := make([]byte, 16)
	if _, err = rand.Read(tokenBytes); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes)
	form.Set("format", "text")
	form.Set("securitytoken", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", "securitytoken="+token)
	req.SetBasicAuth(username, password)
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("contact Monit: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("Monit returned %s: %s", response.Status, message)
	}
	return nil
}

func (a *API) configuredMonitClient(ctx context.Context, deviceID string) (store.MonitControl, store.Credential, *http.Client, error) {
	control, err := a.s.MonitControl(ctx, deviceID)
	if err != nil {
		return control, store.Credential{}, nil, fmt.Errorf("configure Monit remote control for this device first")
	}
	credential, err := a.s.CredentialSecret(ctx, control.CredentialID)
	if err != nil || credential.Kind != "monit" {
		return control, credential, nil, fmt.Errorf("Monit credential is unavailable")
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	return control, credential, client, nil
}

func (a *API) authorizeMonitDevice(r *http.Request, deviceID string) (store.User, bool) {
	u, ok := CurrentUser(r.Context())
	if !ok || u.Role == "viewer" {
		return u, false
	}
	site, err := a.s.DeviceSite(r.Context(), deviceID)
	if err != nil {
		return u, false
	}
	scope, err := a.scope(r)
	return u, err == nil && scope.can(site)
}

func (a *API) monitControl(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")
	if _, ok := a.authorizeMonitDevice(r, deviceID); !ok {
		problem(w, 403, errText("manager access to this device is required"))
		return
	}
	v, err := a.s.MonitControl(r.Context(), deviceID)
	if store.IsNotFound(err) {
		jsonOut(w, 200, map[string]any{"DeviceID": deviceID, "URL": "", "CredentialID": "", "Enabled": false})
		return
	}
	if err != nil {
		problem(w, 500, err)
		return
	}
	jsonOut(w, 200, v)
}

func (a *API) setMonitControl(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")
	u, ok := a.authorizeMonitDevice(r, deviceID)
	if !ok {
		problem(w, 403, errText("manager access to this device is required"))
		return
	}
	var body struct{ URL, CredentialID string }
	if !decode(w, r, &body) {
		return
	}
	cleanURL, err := validateMonitURL(body.URL)
	if err != nil {
		problem(w, 400, err)
		return
	}
	v, err := a.s.SetMonitControl(r.Context(), store.MonitControl{DeviceID: deviceID, URL: cleanURL, CredentialID: body.CredentialID})
	if err != nil {
		problem(w, 400, err)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, "configure", "monit_control", deviceID)
	jsonOut(w, 200, v)
}

func (a *API) testMonitControl(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")
	if _, ok := a.authorizeMonitDevice(r, deviceID); !ok {
		problem(w, 403, errText("manager access to this device is required"))
		return
	}
	control, credential, client, err := a.configuredMonitClient(r.Context(), deviceID)
	if err != nil {
		problem(w, 400, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err = sendMonitProbe(ctx, client, control.URL, credential.Secret["username"], credential.Secret["password"]); err != nil {
		problem(w, 502, err)
		return
	}
	jsonOut(w, 200, map[string]any{"ok": true, "message": "Connected and authenticated to Monit successfully"})
}

func (a *API) runMonitAction(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")
	u, ok := a.authorizeMonitDevice(r, deviceID)
	if !ok {
		problem(w, 403, errText("manager access to this device is required"))
		return
	}
	var body struct{ Service, Action string }
	if !decode(w, r, &body) {
		return
	}
	body.Service = strings.TrimSpace(body.Service)
	body.Action = strings.ToLower(strings.TrimSpace(body.Action))
	if body.Service == "" || !monitActions[body.Action] {
		problem(w, 400, errText("service and a valid action are required"))
		return
	}
	exists, err := a.s.MonitServiceExists(r.Context(), deviceID, body.Service)
	if err != nil {
		problem(w, 500, err)
		return
	}
	if !exists {
		problem(w, 404, errText("reported Monit service not found"))
		return
	}
	control, credential, client, err := a.configuredMonitClient(r.Context(), deviceID)
	if err != nil {
		problem(w, 400, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	err = sendMonitAction(ctx, client, control.URL, credential.Secret["username"], credential.Secret["password"], body.Service, body.Action)
	result := store.MonitAction{DeviceID: deviceID, Service: body.Service, Action: body.Action, Success: err == nil, Message: "Command accepted by Monit"}
	if err != nil {
		result.Message = err.Error()
	}
	result, recordErr := a.s.RecordMonitAction(r.Context(), result, u.ID)
	if recordErr != nil {
		problem(w, 500, recordErr)
		return
	}
	_ = a.s.Audit(r.Context(), u.ID, body.Action, "monit_service", deviceID+":"+body.Service)
	if err != nil {
		problem(w, 502, err)
		return
	}
	jsonOut(w, 200, result)
}

func (a *API) monitActionHistory(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")
	if _, ok := a.authorizeMonitDevice(r, deviceID); !ok {
		problem(w, 403, errText("manager access to this device is required"))
		return
	}
	v, err := a.s.MonitActions(r.Context(), deviceID)
	if err != nil {
		problem(w, 500, err)
		return
	}
	jsonOut(w, 200, v)
}
