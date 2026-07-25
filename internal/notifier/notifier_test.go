package notifier

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestEmailRecipients(t *testing.T) {
	got, err := emailRecipients("alice@example.com, bob@example.com;ops@example.net")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alice@example.com", "bob@example.com", "ops@example.net"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recipients = %#v, want %#v", got, want)
	}
	for _, invalid := range []string{"", "not-an-address", "good@example.com\r\nBcc: bad@example.com"} {
		if _, err = emailRecipients(invalid); err == nil {
			t.Errorf("expected %q to be rejected", invalid)
		}
	}
}

func TestNotificationText(t *testing.T) {
	cases := map[string]string{
		"opened":   "[PROBLEM]",
		"resolved": "[RECOVERED]",
		"reminder": "[STILL UNACKNOWLEDGED]",
	}
	for state, prefix := range cases {
		got := notificationText(Event{State: state, Title: "SSH unavailable", DeviceName: "mail-01"})
		if !strings.HasPrefix(got, prefix) || !strings.Contains(got, "mail-01") {
			t.Errorf("%s text = %q", state, got)
		}
	}
}

func TestSlackPayload(t *testing.T) {
	var payload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request %s content-type %q", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	n := &Notifier{client: server.Client()}
	event := Event{State: "resolved", Title: "SSH unavailable", DeviceName: "mail-01"}
	if err := n.slack(context.Background(), map[string]string{"url": server.URL}, event); err != nil {
		t.Fatal(err)
	}
	if payload["text"] != notificationText(event) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestTeamsAdaptiveCardPayload(t *testing.T) {
	var payload struct {
		Type        string `json:"type"`
		Attachments []struct {
			ContentType string `json:"contentType"`
			Content     struct {
				Type string `json:"type"`
				Body []struct {
					Text string `json:"text"`
				} `json:"body"`
			} `json:"content"`
		} `json:"attachments"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	n := &Notifier{client: server.Client()}
	event := Event{State: "opened", Title: "Database unavailable", DeviceName: "db-01"}
	if err := n.teams(context.Background(), map[string]string{"url": server.URL}, event); err != nil {
		t.Fatal(err)
	}
	if payload.Type != "message" || len(payload.Attachments) != 1 || payload.Attachments[0].ContentType != "application/vnd.microsoft.card.adaptive" || len(payload.Attachments[0].Content.Body) != 1 || payload.Attachments[0].Content.Body[0].Text != notificationText(event) {
		t.Fatalf("unexpected Teams payload: %#v", payload)
	}
}

func TestWebhookReportsResponseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid webhook", http.StatusBadRequest)
	}))
	defer server.Close()
	n := &Notifier{client: server.Client()}
	err := n.slack(context.Background(), map[string]string{"url": server.URL}, Event{State: "test", Title: "Test"})
	if err == nil || !strings.Contains(err.Error(), "invalid webhook") {
		t.Fatalf("error = %v", err)
	}
}

func TestSendPlainSMTP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	message := make(chan string, 1)
	serverError := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverError <- acceptErr
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)
		write := func(value string) {
			_, _ = writer.WriteString(value)
			_ = writer.Flush()
		}
		write("220 localhost ESMTP\r\n")
		inData := false
		var body strings.Builder
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				serverError <- readErr
				return
			}
			if inData {
				if line == ".\r\n" {
					message <- body.String()
					inData = false
					write("250 queued\r\n")
				} else {
					body.WriteString(line)
				}
				continue
			}
			command := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(command, "EHLO"):
				write("250-localhost\r\n250 OK\r\n")
			case strings.HasPrefix(command, "MAIL FROM:"), strings.HasPrefix(command, "RCPT TO:"):
				write("250 OK\r\n")
			case command == "DATA":
				inData = true
				write("354 End data with <CR><LF>.<CR><LF>\r\n")
			case command == "QUIT":
				write("221 Bye\r\n")
				serverError <- nil
				return
			default:
				serverError <- fmt.Errorf("unexpected SMTP command %q", command)
				return
			}
		}
	}()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("Subject: GoLive test\r\n\r\nNotification body\r\n")
	if err = sendSMTP(host, port, "plain", nil, "golive@example.com", []string{"ops@example.com"}, body); err != nil {
		t.Fatal(err)
	}
	if got := <-message; !strings.Contains(got, "Notification body") {
		t.Fatalf("SMTP body = %q", got)
	}
	if err = <-serverError; err != nil {
		t.Fatal(err)
	}
}
