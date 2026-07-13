package configbackup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/golive-nms/golive-nms/internal/store"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/ssh"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"
)

type Engine struct {
	store  *store.Store
	logger *slog.Logger
	notify func(string, string, string, string)
}

func New(s *store.Store, l *slog.Logger, n func(string, string, string, string)) *Engine {
	return &Engine{store: s, logger: l, notify: n}
}
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < 5; i++ {
				profile, err := e.store.ClaimConfigBackup(ctx)
				if errors.Is(err, pgx.ErrNoRows) {
					break
				}
				if err != nil {
					e.logger.Warn("claim config backup", "error", err)
					break
				}
				go e.capture(ctx, profile)
			}
		}
	}
}
func (e *Engine) capture(parent context.Context, p store.ConfigProfile) {
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	content, err := e.fetch(ctx, p)
	if err != nil {
		e.store.ConfigBackupFailed(parent, p.ID, err.Error())
		e.logger.Warn("configuration backup failed", "device", p.DeviceName, "error", err)
		return
	}
	_, changed, err := e.store.SaveConfigSnapshot(parent, p, content)
	if err != nil {
		e.store.ConfigBackupFailed(parent, p.ID, err.Error())
		return
	}
	if changed && e.notify != nil {
		e.notify("Configuration changed on "+p.DeviceName, p.DeviceID, p.DeviceName, "opened")
	}
}
func (e *Engine) fetch(ctx context.Context, p store.ConfigProfile) ([]byte, error) {
	cred, err := e.store.CredentialSecret(ctx, p.CredentialID)
	if err != nil {
		return nil, err
	}
	address := p.Address
	if _, _, err = net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(address, "22")
	}
	var methods []ssh.AuthMethod
	if password := cred.Secret["password"]; password != "" {
		methods = append(methods, ssh.Password(password))
	}
	if key := cred.Secret["privateKey"]; key != "" {
		signer, e := ssh.ParsePrivateKey([]byte(key))
		if e != nil {
			return nil, e
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if len(methods) == 0 {
		return nil, errors.New("SSH password or private key is required")
	}
	expected := cred.Secret["hostKeySHA256"]
	if expected == "" {
		return nil, errors.New("SSH hostKeySHA256 fingerprint is required")
	}
	cfg := &ssh.ClientConfig{User: cred.Secret["username"], Auth: methods, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		actual := ssh.FingerprintSHA256(key)
		matched := false
		for _, candidate := range strings.Split(expected, ",") {
			if strings.TrimSpace(candidate) == actual {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("SSH host key mismatch: got %s", actual)
		}
		return nil
	}, Timeout: 15 * time.Second}
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	cc, channels, requests, err := ssh.NewClientConn(conn, address, cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}
	client := ssh.NewClient(cc, channels, requests)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	output := &limitBuffer{limit: 5 << 20}
	session.Stdout = output
	session.Stderr = output
	if err = session.Run(p.Command); err != nil {
		return nil, fmt.Errorf("command failed: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return bytes.TrimSpace(output.Bytes()), nil
}

type limitBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		return 0, errors.New("configuration output exceeds 5 MiB")
	}
	original := len(p)
	if original > remaining {
		p = p[:remaining]
	}
	n, _ := b.Buffer.Write(p)
	if original > remaining || n < len(p) {
		return n, io.ErrShortWrite
	}
	return n, nil
}
