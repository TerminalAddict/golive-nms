package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type actionPayload struct {
	JobID          string   `json:"jobId"`
	Executable     string   `json:"executable"`
	Arguments      []string `json:"arguments"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
	ExpiresAt      int64    `json:"expiresAt"`
}
type signedAction struct {
	Payload   actionPayload `json:"payload"`
	Signature string        `json:"signature"`
}

func processActions(ctx context.Context, client *http.Client, server, stateDir string) error {
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(server, "/")+"/api/v1/agent/actions", nil)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusNoContent {
			resp.Body.Close()
			return nil
		}
		if resp.StatusCode >= 300 {
			err = responseError(resp)
			resp.Body.Close()
			return err
		}
		var action signedAction
		err = json.NewDecoder(resp.Body).Decode(&action)
		resp.Body.Close()
		if err != nil {
			return err
		}
		success, output, message := executeSigned(ctx, stateDir, action)
		body, _ := json.Marshal(map[string]any{"jobId": action.Payload.JobID, "success": success, "output": output, "error": message})
		resultReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(server, "/")+"/api/v1/agent/actions/results", bytes.NewReader(body))
		resultReq.Header.Set("Content-Type", "application/json")
		result, err := client.Do(resultReq)
		if err != nil {
			return err
		}
		if result.StatusCode >= 300 {
			err = responseError(result)
			result.Body.Close()
			return err
		}
		result.Body.Close()
	}
	return nil
}
func executeSigned(parent context.Context, stateDir string, action signedAction) (bool, string, string) {
	raw, _ := json.Marshal(action.Payload)
	signature, err := base64.RawStdEncoding.DecodeString(action.Signature)
	if err != nil {
		return false, "", "invalid action signature encoding"
	}
	caPEM, err := os.ReadFile(filepath.Join(stateDir, "ca.crt"))
	if err != nil {
		return false, "", err.Error()
	}
	block, _ := pem.Decode(caPEM)
	if block == nil {
		return false, "", "invalid CA certificate"
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, "", err.Error()
	}
	public, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return false, "", "CA key is not ECDSA"
	}
	sum := sha256.Sum256(raw)
	if !ecdsa.VerifyASN1(public, sum[:], signature) {
		return false, "", "action signature verification failed"
	}
	if time.Now().Unix() > action.Payload.ExpiresAt {
		return false, "", "action expired"
	}
	if !filepath.IsAbs(action.Payload.Executable) || action.Payload.TimeoutSeconds < 1 || action.Payload.TimeoutSeconds > 300 {
		return false, "", "invalid action bounds"
	}
	blocked := map[string]bool{"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true, "fish": true, "python": true, "python3": true, "perl": true, "ruby": true, "node": true}
	if blocked[filepath.Base(action.Payload.Executable)] {
		return false, "", "shell and interpreter execution is blocked"
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(action.Payload.TimeoutSeconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, action.Payload.Executable, action.Payload.Arguments...)
	output := &actionBuffer{limit: 65536}
	cmd.Stdout = output
	cmd.Stderr = output
	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return false, output.String(), "action timed out"
	}
	if err != nil {
		return false, output.String(), err.Error()
	}
	return true, output.String(), ""
}

type actionBuffer struct {
	bytes.Buffer
	limit int
}

func (b *actionBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	original := len(p)
	if original > remaining {
		p = p[:remaining]
	}
	b.Buffer.Write(p)
	return original, nil
}
func responseError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("server returned %s: %s", resp.Status, data)
}

var _ = errors.New
