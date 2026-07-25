package notifier

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/golive-nms/internal/store"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type Event struct {
	IncidentID, DeviceID, Title, DeviceName, State string
	NotificationEventID                            int64
}
type Notifier struct {
	store  *store.Store
	client *http.Client
}

func New(s *store.Store) *Notifier {
	return &Notifier{store: s, client: &http.Client{Timeout: 10 * time.Second}}
}
func (n *Notifier) Run(ctx context.Context) {
	events := time.NewTicker(2 * time.Second)
	reminders := time.NewTicker(time.Minute)
	defer events.Stop()
	defer reminders.Stop()
	n.processEvents(ctx)
	n.processReminders(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-events.C:
			n.processEvents(ctx)
		case <-reminders.C:
			n.processReminders(ctx)
		}
	}
}

func (n *Notifier) processEvents(ctx context.Context) {
	events, err := n.store.ClaimNotificationEvents(ctx, 50)
	if err != nil {
		return
	}
	for _, pending := range events {
		event := Event{
			IncidentID: pending.IncidentID, DeviceID: pending.DeviceID,
			DeviceName: pending.DeviceName, Title: pending.Title, State: pending.State,
			NotificationEventID: pending.ID,
		}
		err = n.Publish(ctx, event)
		n.store.CompleteNotificationEvent(ctx, pending.ID, err)
	}
}

func (n *Notifier) processReminders(ctx context.Context) {
	reminders, err := n.store.NotificationReminders(ctx)
	if err != nil {
		return
	}
	for _, reminder := range reminders {
		_ = n.publishChannel(ctx, reminder.Channel, Event{
			IncidentID: reminder.IncidentID, DeviceID: reminder.DeviceID,
			DeviceName: reminder.DeviceName, Title: reminder.Title, State: "reminder",
		})
	}
}

func (n *Notifier) Publish(ctx context.Context, event Event) error {
	channels, err := n.store.NotificationChannels(ctx)
	if err != nil {
		return err
	}
	var deliveryErrors []error
	for _, ch := range channels {
		if !ch.Enabled || (ch.SiteID != "" && !n.store.DeviceBelongsToSite(ctx, event.DeviceID, ch.SiteID)) || (event.State == "opened" && !ch.NotifyOpened) || (event.State == "resolved" && !ch.NotifyResolved) {
			continue
		}
		if event.NotificationEventID != 0 && n.store.NotificationEventDelivered(ctx, ch.ID, event.NotificationEventID) {
			continue
		}
		if err = n.publishChannel(ctx, ch, event); err != nil {
			deliveryErrors = append(deliveryErrors, fmt.Errorf("%s: %w", ch.Name, err))
		}
	}
	return errors.Join(deliveryErrors...)
}

func (n *Notifier) Test(ctx context.Context, channelID string) error {
	channel, err := n.store.NotificationChannel(ctx, channelID)
	if err != nil {
		return err
	}
	return n.publishChannel(ctx, channel, Event{Title: "Test notification", DeviceName: "GoLive NMS", State: "test"})
}

func (n *Notifier) publishChannel(ctx context.Context, ch store.NotificationChannel, event Event) error {
	cred, e := n.store.CredentialSecret(ctx, ch.CredentialID)
	if e == nil {
		switch ch.Kind {
		case "email":
			e = n.email(cred.Secret, event)
		case "slack":
			e = n.slack(ctx, cred.Secret, event)
		case "teams":
			e = n.teams(ctx, cred.Secret, event)
		default:
			e = fmt.Errorf("unsupported notification channel kind %q", ch.Kind)
		}
	}
	message := ""
	if e != nil {
		message = e.Error()
	}
	n.store.RecordDelivery(ctx, ch.ID, event.IncidentID, event.DeviceID, event.State, event.NotificationEventID, e == nil, message)
	return e
}
func (n *Notifier) slack(ctx context.Context, secret map[string]string, event Event) error {
	return n.webhook(ctx, secret, map[string]string{"text": notificationText(event)})
}

func (n *Notifier) teams(ctx context.Context, secret map[string]string, event Event) error {
	payload := map[string]any{
		"type": "message",
		"attachments": []map[string]any{{
			"contentType": "application/vnd.microsoft.card.adaptive",
			"content": map[string]any{
				"$schema": "https://adaptivecards.io/schemas/adaptive-card.json",
				"type":    "AdaptiveCard",
				"version": "1.4",
				"body": []map[string]any{{
					"type": "TextBlock", "text": notificationText(event), "wrap": true,
				}},
			},
		}},
	}
	return n.webhook(ctx, secret, payload)
}

func (n *Notifier) webhook(ctx context.Context, secret map[string]string, payload any) error {
	webhookURL := strings.TrimSpace(secret["url"])
	if webhookURL == "" {
		return errors.New("webhook URL is required")
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
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
		response, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("webhook returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(response)))
	}
	return nil
}
func (n *Notifier) email(secret map[string]string, e Event) error {
	host := strings.TrimSpace(secret["host"])
	if host == "" {
		return errors.New("SMTP host is required")
	}
	port := secret["port"]
	security := strings.ToLower(strings.TrimSpace(secret["security"]))
	if security == "" {
		security = "starttls"
	}
	if port == "" {
		if security == "implicit_tls" {
			port = "465"
		} else {
			port = "587"
		}
	}
	if _, err := strconv.Atoi(port); err != nil {
		return err
	}
	if security != "starttls" && security != "implicit_tls" && security != "plain" {
		return errors.New("SMTP security must be STARTTLS, implicit TLS, or plain")
	}
	var auth smtp.Auth
	if secret["username"] != "" {
		auth = smtp.PlainAuth("", secret["username"], secret["password"], host)
	}
	from := strings.TrimSpace(secret["from"])
	recipients, err := emailRecipients(secret["to"])
	if from == "" || strings.ContainsAny(from, "\r\n") {
		return errors.New("valid SMTP from address is required")
	}
	if err != nil {
		return err
	}
	subjectTitle := strings.NewReplacer("\r", " ", "\n", " ").Replace(e.Title)
	subject := fmt.Sprintf("GoLive NMS [%s]: %s", strings.ToUpper(e.State), subjectTitle)
	message := []byte("From: " + from + "\r\nTo: " + strings.Join(recipients, ", ") + "\r\nSubject: " + subject + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + notificationText(e) + "\r\n")
	return sendSMTP(host, port, security, auth, from, recipients, message)
}

func sendSMTP(host, port, security string, auth smtp.Auth, from string, recipients []string, message []byte) error {
	address := net.JoinHostPort(host, port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var (
		conn net.Conn
		err  error
	)
	if security == "implicit_tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host})
	} else {
		conn, err = dialer.Dial("tcp", address)
	}
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer client.Close()
	if security == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server does not advertise STARTTLS")
		}
		if err = client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}); err != nil {
			return err
		}
	}
	if auth != nil {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err = client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = writer.Write(message); err != nil {
		writer.Close()
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func emailRecipients(value string) ([]string, error) {
	var recipients []string
	for _, candidate := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' }) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.ContainsAny(candidate, "\r\n") || !strings.Contains(candidate, "@") {
			return nil, fmt.Errorf("invalid notification recipient %q", candidate)
		}
		recipients = append(recipients, candidate)
	}
	if len(recipients) == 0 {
		return nil, errors.New("at least one notification recipient is required")
	}
	return recipients, nil
}

func notificationText(event Event) string {
	state := strings.ToUpper(event.State)
	switch event.State {
	case "opened":
		state = "PROBLEM"
	case "resolved":
		state = "RECOVERED"
	case "reminder":
		state = "STILL UNACKNOWLEDGED"
	}
	return fmt.Sprintf("[%s] %s\nDevice: %s", state, event.Title, event.DeviceName)
}
