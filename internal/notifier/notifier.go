package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/TerminalAddict/golive-nms/internal/store"
	"net/http"
	"net/smtp"
	"strconv"
	"time"
)

type Event struct{ IncidentID, DeviceID, Title, DeviceName, State string }
type Notifier struct {
	store  *store.Store
	client *http.Client
}

func New(s *store.Store) *Notifier {
	return &Notifier{store: s, client: &http.Client{Timeout: 10 * time.Second}}
}
func (n *Notifier) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reminders, err := n.store.NotificationReminders(ctx)
			if err != nil {
				continue
			}
			for _, r := range reminders {
				n.publishChannel(ctx, r.Channel, Event{IncidentID: r.IncidentID, DeviceID: r.DeviceID, DeviceName: r.DeviceName, Title: r.Title, State: "reminder"})
			}
		}
	}
}
func (n *Notifier) Publish(ctx context.Context, event Event) {
	channels, err := n.store.NotificationChannels(ctx)
	if err != nil {
		return
	}
	for _, ch := range channels {
		if !ch.Enabled || (ch.SiteID != "" && !n.store.DeviceBelongsToSite(ctx, event.DeviceID, ch.SiteID)) || (event.State == "opened" && !ch.NotifyOpened) || (event.State == "resolved" && !ch.NotifyResolved) {
			continue
		}
		n.publishChannel(ctx, ch, event)
	}
}
func (n *Notifier) publishChannel(ctx context.Context, ch store.NotificationChannel, event Event) {
	cred, e := n.store.CredentialSecret(ctx, ch.CredentialID)
	if e == nil {
		switch ch.Kind {
		case "email":
			e = n.email(cred.Secret, event)
		case "slack", "teams":
			e = n.webhook(ctx, cred.Secret, event)
		}
	}
	message := ""
	if e != nil {
		message = e.Error()
	}
	n.store.RecordDelivery(ctx, ch.ID, event.IncidentID, event.DeviceID, event.State, e == nil, message)
}
func (n *Notifier) webhook(ctx context.Context, secret map[string]string, e Event) error {
	payload := map[string]string{"text": fmt.Sprintf("[%s] %s (%s)", e.State, e.Title, e.DeviceName)}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, secret["url"], bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}
func (n *Notifier) email(secret map[string]string, e Event) error {
	host := secret["host"]
	port := secret["port"]
	if port == "" {
		port = "587"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return err
	}
	var auth smtp.Auth
	if secret["username"] != "" {
		auth = smtp.PlainAuth("", secret["username"], secret["password"], host)
	}
	subject := fmt.Sprintf("GoLive NMS: %s", e.Title)
	message := []byte("From: " + secret["from"] + "\r\nTo: " + secret["to"] + "\r\nSubject: " + subject + "\r\n\r\nState: " + e.State + "\r\nDevice: " + e.DeviceName + "\r\n")
	return smtp.SendMail(host+":"+port, auth, secret["from"], []string{secret["to"]}, message)
}
