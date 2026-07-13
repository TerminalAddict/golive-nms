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
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var version = "dev"

type report struct {
	AgentID  string         `json:"agentId"`
	Hostname string         `json:"hostname"`
	Address  string         `json:"address"`
	Version  string         `json:"version"`
	Metrics  map[string]any `json:"metrics"`
}

func main() {
	server := flag.String("server", os.Getenv("GOLIVE_SERVER"), "GoLive collector URL")
	token := flag.String("token", os.Getenv("GOLIVE_AGENT_TOKEN"), "enrollment/report token")
	enrollToken := flag.String("enroll-token", os.Getenv("GOLIVE_ENROLLMENT_TOKEN"), "one-time enrollment token")
	enrollURL := flag.String("enroll-url", os.Getenv("GOLIVE_ENROLL_URL"), "management URL used for one-time enrollment")
	stateDir := flag.String("state-dir", "/var/lib/golive-agent", "certificate state directory")
	interval := flag.Duration("interval", 60*time.Second, "report interval")
	once := flag.Bool("once", false, "send one report and exit")
	flag.Parse()
	if *server == "" || (*token == "" && *enrollToken == "" && !hasIdentity(*stateDir)) {
		fmt.Fprintln(os.Stderr, "server and either token or enroll-token are required")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if *enrollToken != "" {
		url := *enrollURL
		if url == "" {
			url = *server
		}
		if err := enroll(ctx, url, *enrollToken, *stateDir); err != nil {
			fmt.Fprintln(os.Stderr, "enrollment:", err)
			os.Exit(1)
		}
	}
	client, err := identityClient(*stateDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "identity:", err)
		os.Exit(1)
	}
	for {
		reportErr := send(ctx, client, *server, *token)
		if reportErr != nil {
			fmt.Fprintln(os.Stderr, "report:", reportErr)
			if *once {
				os.Exit(1)
			}
		}
		if reportErr == nil && hasIdentity(*stateDir) {
			if err := processActions(ctx, client, *server, *stateDir); err != nil {
				fmt.Fprintln(os.Stderr, "actions:", err)
				if *once {
					os.Exit(1)
				}
			}
		}
		if *once {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(*interval):
		}
	}
}
func hasIdentity(dir string) bool {
	_, certErr := os.Stat(filepath.Join(dir, "client.crt"))
	_, keyErr := os.Stat(filepath.Join(dir, "client.key"))
	return certErr == nil && keyErr == nil
}

func send(ctx context.Context, client *http.Client, server, token string) error {
	hostname, _ := os.Hostname()
	payload := report{AgentID: machineID(hostname), Hostname: hostname, Address: primaryAddress(), Version: version, Metrics: metrics(ctx)}
	body, _ := json.Marshal(payload)
	url := strings.TrimRight(server, "/") + "/api/v1/agent/report"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("server returned %s: %s", resp.Status, string(message))
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
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: hostname}}, key)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"token": token, "name": hostname, "csr": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/enroll", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("server returned %s: %s", resp.Status, message)
	}
	var result enrollmentResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	files := map[string][]byte{"client.crt": []byte(result.Certificate), "ca.crt": []byte(result.CACertificate), "client.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})}
	for name, data := range files {
		if err = os.WriteFile(filepath.Join(stateDir, name), data, 0600); err != nil {
			return err
		}
	}
	return nil
}
func identityClient(stateDir string) (*http.Client, error) {
	certPath, keyPath, caPath := filepath.Join(stateDir, "client.crt"), filepath.Join(stateDir, "client.key"), filepath.Join(stateDir, "ca.crt")
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return &http.Client{Timeout: 30 * time.Second}, nil
	}
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("invalid CA certificate")
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{pair}}}}, nil
}
func machineID(fallback string) string {
	if b, err := os.ReadFile("/etc/machine-id"); err == nil && strings.TrimSpace(string(b)) != "" {
		return strings.TrimSpace(string(b))
	}
	return fallback
}
func primaryAddress() string {
	cs, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, i := range cs {
		as, _ := i.Addrs()
		for _, a := range as {
			ip, _, _ := net.ParseCIDR(a.String())
			if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				return ip.String()
			}
		}
	}
	return ""
}
