package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"github.com/go-routeros/routeros/v3"
	"github.com/gosnmp/gosnmp"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var version = "dev"

type assignment struct {
	ID              string            `json:"id"`
	DeviceID        string            `json:"deviceId"`
	DeviceName      string            `json:"deviceName"`
	Type            string            `json:"type"`
	Target          string            `json:"target"`
	IntervalSeconds int               `json:"intervalSeconds"`
	TimeoutSeconds  int               `json:"timeoutSeconds"`
	Config          json.RawMessage   `json:"config"`
	Credential      map[string]string `json:"credential"`
}

func main() {
	server := flag.String("server", os.Getenv("GOLIVE_SERVER"), "GoLive collector URL")
	enrollToken := flag.String("enroll-token", os.Getenv("GOLIVE_ENROLLMENT_TOKEN"), "one-time enrollment token")
	enrollURL := flag.String("enroll-url", os.Getenv("GOLIVE_ENROLL_URL"), "management enrollment URL")
	stateDir := flag.String("state-dir", "/var/lib/golive-collector", "certificate state directory")
	poll := flag.Duration("poll", 10*time.Second, "assignment poll interval")
	once := flag.Bool("once", false, "poll and execute once")
	flag.Parse()
	if *server == "" || (*enrollToken == "" && !hasIdentity(*stateDir)) {
		fmt.Fprintln(os.Stderr, "server and enrollment token are required for first use")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if *enrollToken != "" {
		u := *enrollURL
		if u == "" {
			u = *server
		}
		if err := enroll(ctx, u, *enrollToken, *stateDir); err != nil {
			fmt.Fprintln(os.Stderr, "enrollment:", err)
			os.Exit(1)
		}
	}
	client, err := identityClient(*stateDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "identity:", err)
		os.Exit(1)
	}
	next := map[string]time.Time{}
	for {
		if err = cycle(ctx, client, *server, next); err != nil {
			fmt.Fprintln(os.Stderr, "collector:", err)
			if *once {
				os.Exit(1)
			}
		}
		if *once {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(*poll):
		}
	}
}
func cycle(ctx context.Context, client *http.Client, server string, next map[string]time.Time) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(server, "/")+"/api/v1/collector/assignments", nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return responseError(resp)
	}
	var jobs []assignment
	if err = json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return err
	}
	now := time.Now()
	for _, job := range jobs {
		if now.Before(next[job.ID]) {
			continue
		}
		next[job.ID] = now.Add(time.Duration(job.IntervalSeconds) * time.Second)
		up, latency, message := execute(ctx, job)
		body, _ := json.Marshal(map[string]any{"checkId": job.ID, "up": up, "latencyMs": latency, "message": message})
		r, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(server, "/")+"/api/v1/collector/results", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		result, e := client.Do(r)
		if e != nil {
			return e
		}
		if result.StatusCode >= 300 {
			e = responseError(result)
			result.Body.Close()
			return e
		}
		result.Body.Close()
	}
	return nil
}
func execute(parent context.Context, a assignment) (bool, float64, string) {
	ctx, cancel := context.WithTimeout(parent, time.Duration(a.TimeoutSeconds)*time.Second)
	defer cancel()
	start := time.Now()
	var err error
	switch a.Type {
	case "ping":
		err = ping(ctx, a.Target)
	case "tcp":
		var c net.Conn
		c, err = (&net.Dialer{}).DialContext(ctx, "tcp", a.Target)
		if c != nil {
			c.Close()
		}
	case "http":
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, a.Target, nil)
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
	case "snmp":
		err = snmpCheck(a)
	case "routeros":
		err = routerOS(ctx, a)
	case "dns":
		_, err = net.DefaultResolver.LookupHost(ctx, a.Target)
	case "tls":
		err = tlsCheck(ctx, a)
	case "ssh":
		err = bannerCheck(ctx, a.Target, "SSH-")
	case "smtp":
		err = bannerCheck(ctx, a.Target, "220")
	case "mysql", "postgres":
		var c net.Conn
		c, err = (&net.Dialer{}).DialContext(ctx, "tcp", a.Target)
		if c != nil {
			c.Close()
		}
	default:
		err = fmt.Errorf("unsupported type %s", a.Type)
	}
	latency := float64(time.Since(start).Microseconds()) / 1000
	if err != nil {
		return false, latency, err.Error()
	}
	return true, latency, ""
}
func routerOS(ctx context.Context, a assignment) error {
	target := a.Target
	if _, _, e := net.SplitHostPort(target); e != nil {
		port := "8728"
		if a.Credential["tls"] == "true" {
			port = "8729"
		}
		target = net.JoinHostPort(target, port)
	}
	var client *routeros.Client
	var err error
	if a.Credential["tls"] == "true" {
		pool := x509.NewCertPool()
		if ca := a.Credential["caCertificate"]; ca != "" {
			pool.AppendCertsFromPEM([]byte(ca))
		}
		serverName := a.Credential["serverName"]
		if serverName == "" {
			serverName = strings.Split(target, ":")[0]
		}
		client, err = routeros.DialTLSContext(ctx, target, a.Credential["username"], a.Credential["password"], &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool, ServerName: serverName})
	} else {
		client, err = routeros.DialContext(ctx, target, a.Credential["username"], a.Credential["password"])
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
	c, err := (&net.Dialer{}).DialContext(ctx, "tcp", target)
	if err != nil {
		return err
	}
	defer c.Close()
	deadline, _ := ctx.Deadline()
	c.SetDeadline(deadline)
	buf := make([]byte, 512)
	n, err := c.Read(buf)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(buf[:n]), prefix) {
		return fmt.Errorf("unexpected service banner")
	}
	return nil
}
func tlsCheck(ctx context.Context, a assignment) error {
	host, _, err := net.SplitHostPort(a.Target)
	target := a.Target
	if err != nil {
		host = a.Target
		target = net.JoinHostPort(a.Target, "443")
	}
	var cfg struct {
		MinimumDays int `json:"minimumDays"`
	}
	json.Unmarshal(a.Config, &cfg)
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
func ping(ctx context.Context, target string) error {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", target)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("resolve %s: %w", target, err)
	}
	c, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return err
	}
	defer c.Close()
	deadline, _ := ctx.Deadline()
	c.SetDeadline(deadline)
	m := icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: os.Getpid() & 0xffff, Seq: 1, Data: []byte("golive")}}
	data, _ := m.Marshal(nil)
	if _, err = c.WriteTo(data, &net.UDPAddr{IP: ips[0]}); err != nil {
		return err
	}
	_, _, err = c.ReadFrom(make([]byte, 1500))
	return err
}
func snmpCheck(a assignment) error {
	host, portText, e := net.SplitHostPort(a.Target)
	port := uint16(161)
	if e != nil {
		host = a.Target
	} else if p, x := strconv.ParseUint(portText, 10, 16); x == nil {
		port = uint16(p)
	}
	c := &gosnmp.GoSNMP{Target: host, Port: port, Timeout: time.Duration(a.TimeoutSeconds) * time.Second}
	v := strings.ToLower(a.Credential["version"])
	if v == "3" || v == "v3" {
		c.Version = gosnmp.Version3
		c.SecurityModel = gosnmp.UserSecurityModel
		c.MsgFlags = gosnmp.AuthNoPriv
		p := &gosnmp.UsmSecurityParameters{UserName: a.Credential["username"], AuthenticationProtocol: gosnmp.SHA, AuthenticationPassphrase: a.Credential["authPassword"], PrivacyProtocol: gosnmp.NoPriv}
		if a.Credential["privPassword"] != "" {
			c.MsgFlags = gosnmp.AuthPriv
			p.PrivacyProtocol = gosnmp.AES
			p.PrivacyPassphrase = a.Credential["privPassword"]
		}
		c.SecurityParameters = p
	} else {
		c.Version = gosnmp.Version2c
		c.Community = a.Credential["community"]
	}
	if e = c.Connect(); e != nil {
		return e
	}
	defer c.Conn.Close()
	var cfg struct {
		OID string `json:"oid"`
	}
	json.Unmarshal(a.Config, &cfg)
	if cfg.OID == "" {
		cfg.OID = ".1.3.6.1.2.1.1.3.0"
	}
	packet, e := c.Get([]string{cfg.OID})
	if e != nil {
		return e
	}
	if len(packet.Variables) == 0 {
		return fmt.Errorf("no SNMP variables")
	}
	return nil
}

type enrollmentResponse struct {
	Certificate   string `json:"certificate"`
	CACertificate string `json:"caCertificate"`
}

func enroll(ctx context.Context, base, token, stateDir string) error {
	hostname, _ := os.Hostname()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: hostname}}, key)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"token": token, "name": hostname, "csr": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/enroll", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return responseError(resp)
	}
	var out enrollmentResponse
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	if err = os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	for name, data := range map[string][]byte{"client.crt": []byte(out.Certificate), "ca.crt": []byte(out.CACertificate), "client.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})} {
		if err = os.WriteFile(filepath.Join(stateDir, name), data, 0600); err != nil {
			return err
		}
	}
	return nil
}
func identityClient(dir string) (*http.Client, error) {
	pair, err := tls.LoadX509KeyPair(filepath.Join(dir, "client.crt"), filepath.Join(dir, "client.key"))
	if err != nil {
		return nil, err
	}
	ca, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("invalid CA")
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{pair}}}}, nil
}
func hasIdentity(dir string) bool {
	_, a := os.Stat(filepath.Join(dir, "client.crt"))
	_, b := os.Stat(filepath.Join(dir, "client.key"))
	return a == nil && b == nil
}
func responseError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("server returned %s: %s", resp.Status, data)
}

var _ = url.URL{}
