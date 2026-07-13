package monitor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/TerminalAddict/golive-nms/internal/store"
	"github.com/go-routeros/routeros/v3"
	"github.com/gosnmp/gosnmp"
	"github.com/jackc/pgx/v5"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type Engine struct {
	store      *store.Store
	publish    func(string, any)
	logger     *slog.Logger
	notify     func(context.Context, string, string, string, string)
	metricsURL string
	httpClient *http.Client
}

func New(s *store.Store, p func(string, any), l *slog.Logger, n func(context.Context, string, string, string, string), metricsURL string) *Engine {
	return &Engine{store: s, publish: p, logger: l, notify: n, metricsURL: metricsURL, httpClient: &http.Client{Timeout: 5 * time.Second}}
}

func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < 20; i++ {
				c, err := e.store.ClaimDue(ctx)
				if errors.Is(err, pgx.ErrNoRows) {
					break
				}
				if err != nil {
					e.logger.Error("claim check", "error", err)
					break
				}
				go e.execute(ctx, *c)
			}
		}
	}
}

func (e *Engine) execute(parent context.Context, c store.DueCheck) {
	ctx, cancel := context.WithTimeout(parent, time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()
	start := time.Now()
	var err error
	switch c.Type {
	case "ping":
		err = ping(ctx, c.Target)
	case "snmp":
		err = e.snmp(ctx, c)
	case "routeros":
		err = e.routerOS(ctx, c)
	case "dns":
		_, err = net.DefaultResolver.LookupHost(ctx, c.Target)
	case "tls":
		err = tlsCheck(ctx, c.Target, c.Config)
	case "ssh":
		err = bannerCheck(ctx, c.Target, "SSH-")
	case "smtp":
		err = bannerCheck(ctx, c.Target, "220")
	case "mysql", "postgres":
		var conn net.Conn
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", c.Target)
		if conn != nil {
			conn.Close()
		}
	case "tcp":
		var conn net.Conn
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", c.Target)
		if conn != nil {
			conn.Close()
		}
	case "http":
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, c.Target, nil)
		if err == nil {
			var resp *http.Response
			resp, err = http.DefaultClient.Do(req)
			if resp != nil {
				resp.Body.Close()
				if resp.StatusCode >= 400 {
					err = fmt.Errorf("HTTP %d", resp.StatusCode)
				}
			}
		}
	default:
		err = fmt.Errorf("unsupported check type %q", c.Type)
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	latency := float64(time.Since(start).Microseconds()) / 1000
	if saveErr := e.store.RecordResult(parent, c, err == nil, latency, message); saveErr != nil {
		e.logger.Error("record check", "error", saveErr)
		return
	}
	go e.writeMetrics(context.Background(), c, err == nil, latency)
	newStatus := "down"
	if err == nil {
		newStatus = "up"
	}
	if e.notify != nil && !c.Maintenance && ((newStatus == "down" && c.Status != "down" && !c.ParentDown) || (newStatus == "up" && c.Status == "down")) {
		event := "opened"
		if newStatus == "up" {
			event = "resolved"
		}
		e.notify(parent, c.DeviceName+" is unavailable", c.DeviceID, c.DeviceName, event)
	}
	if newStatus == "down" && c.Status != "down" && !c.ParentDown && !c.Maintenance {
		go e.store.QueueAutomaticRemediation(context.Background(), c.DeviceID, c.Type)
	}
	e.publish("check.result", map[string]any{"checkId": c.CheckID, "deviceId": c.DeviceID, "up": err == nil, "latencyMs": time.Since(start).Milliseconds(), "error": message})
}
func (e *Engine) routerOS(ctx context.Context, c store.DueCheck) error {
	cred, err := e.store.CredentialSecret(ctx, c.CredentialID)
	if err != nil {
		return err
	}
	target := c.Target
	if _, _, e := net.SplitHostPort(target); e != nil {
		port := "8728"
		if cred.Secret["tls"] == "true" {
			port = "8729"
		}
		target = net.JoinHostPort(target, port)
	}
	var client *routeros.Client
	if cred.Secret["tls"] == "true" {
		pool := x509.NewCertPool()
		if ca := cred.Secret["caCertificate"]; ca != "" {
			pool.AppendCertsFromPEM([]byte(ca))
		}
		serverName := cred.Secret["serverName"]
		if serverName == "" {
			serverName = strings.Split(target, ":")[0]
		}
		client, err = routeros.DialTLSContext(ctx, target, cred.Secret["username"], cred.Secret["password"], &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool, ServerName: serverName})
	} else {
		client, err = routeros.DialContext(ctx, target, cred.Secret["username"], cred.Secret["password"])
	}
	if err != nil {
		return err
	}
	defer client.Close()
	reply, err := client.RunContext(ctx, "/system/resource/print")
	if err != nil {
		return err
	}
	if len(reply.Re) == 0 {
		return fmt.Errorf("RouterOS returned no resource data")
	}
	return nil
}
func bannerCheck(ctx context.Context, target, prefix string) error {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", target)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(buf[:n]), prefix) {
		return fmt.Errorf("unexpected service banner")
	}
	return nil
}
func tlsCheck(ctx context.Context, target string, raw json.RawMessage) error {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		host = target
		target = net.JoinHostPort(target, "443")
	}
	var cfg struct {
		MinimumDays int `json:"minimumDays"`
	}
	_ = json.Unmarshal(raw, &cfg)
	if cfg.MinimumDays <= 0 {
		cfg.MinimumDays = 30
	}
	dialer := tls.Dialer{NetDialer: &net.Dialer{}, Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return err
	}
	defer conn.Close()
	state := conn.(*tls.Conn).ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("server returned no certificate")
	}
	remaining := time.Until(state.PeerCertificates[0].NotAfter)
	if remaining < time.Duration(cfg.MinimumDays)*24*time.Hour {
		return fmt.Errorf("certificate expires in %s", remaining.Round(time.Hour))
	}
	return nil
}

func (e *Engine) snmp(ctx context.Context, c store.DueCheck) error {
	cred, err := e.store.CredentialSecret(ctx, c.CredentialID)
	if err != nil {
		return fmt.Errorf("credential: %w", err)
	}
	host, portText, splitErr := net.SplitHostPort(c.Target)
	port := uint16(161)
	if splitErr != nil {
		host = c.Target
	} else if p, e := strconv.ParseUint(portText, 10, 16); e == nil {
		port = uint16(p)
	}
	client := &gosnmp.GoSNMP{Target: host, Port: port, Timeout: time.Duration(c.TimeoutSeconds) * time.Second, Retries: 0}
	version := strings.ToLower(cred.Secret["version"])
	if version == "3" || version == "v3" {
		client.Version = gosnmp.Version3
		client.SecurityModel = gosnmp.UserSecurityModel
		client.MsgFlags = gosnmp.AuthNoPriv
		params := &gosnmp.UsmSecurityParameters{UserName: cred.Secret["username"], AuthenticationProtocol: gosnmp.SHA, AuthenticationPassphrase: cred.Secret["authPassword"], PrivacyProtocol: gosnmp.NoPriv}
		if cred.Secret["privPassword"] != "" {
			client.MsgFlags = gosnmp.AuthPriv
			params.PrivacyProtocol = gosnmp.AES
			params.PrivacyPassphrase = cred.Secret["privPassword"]
		}
		client.SecurityParameters = params
	} else {
		client.Version = gosnmp.Version2c
		client.Community = cred.Secret["community"]
	}
	if err = client.Connect(); err != nil {
		return err
	}
	defer client.Conn.Close()
	var cfg struct {
		OID string `json:"oid"`
	}
	_ = json.Unmarshal(c.Config, &cfg)
	if cfg.OID == "" {
		cfg.OID = ".1.3.6.1.2.1.1.3.0"
	}
	packet, err := client.Get([]string{cfg.OID})
	if err != nil {
		return err
	}
	if len(packet.Variables) == 0 {
		return fmt.Errorf("SNMP returned no variables")
	}
	if packet.Variables[0].Type == gosnmp.NoSuchObject || packet.Variables[0].Type == gosnmp.NoSuchInstance {
		return fmt.Errorf("OID %s is unavailable", cfg.OID)
	}
	return nil
}

func ping(ctx context.Context, target string) error {
	ip, err := net.DefaultResolver.LookupIP(ctx, "ip4", target)
	if err != nil || len(ip) == 0 {
		return fmt.Errorf("resolve %s: %w", target, err)
	}
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	msg := icmp.Message{Type: ipv4.ICMPTypeEcho, Code: 0, Body: &icmp.Echo{ID: os.Getpid() & 0xffff, Seq: 1, Data: []byte("golive-nms")}}
	data, _ := msg.Marshal(nil)
	if _, err = conn.WriteTo(data, &net.UDPAddr{IP: ip[0]}); err != nil {
		return err
	}
	buf := make([]byte, 1500)
	_, _, err = conn.ReadFrom(buf)
	return err
}
