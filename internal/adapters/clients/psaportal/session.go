package psaportal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// FetchRequest is one authenticated HTTP request to run inside the browser page.
// URL may be absolute-path (same-origin in the page) or a full URL.
type FetchRequest struct {
	URL    string
	Method string
	Body   string
}

// FetchResponse is the page fetch() outcome.
type FetchResponse struct {
	Status int
	Body   string
}

// Fetcher performs a single authenticated request inside the browser context.
// The browser page already carries cf_clearance + the accessToken cookie, so it
// clears the Cloudflare gate that blocks a plain HTTP client on datacenter IPs.
type Fetcher interface {
	Do(ctx context.Context, req FetchRequest) (FetchResponse, error)
}

// wireRequest / wireReply are the NDJSON frames exchanged with the .mjs script.
type wireRequest struct {
	ID     int    `json:"id"`
	URL    string `json:"url"`
	Method string `json:"method"`
	Body   string `json:"body,omitempty"`
}

type wireReply struct {
	ID     int    `json:"id"`
	Status int    `json:"status"`
	Body   string `json:"body"`
	Error  string `json:"error,omitempty"`
}

// browserSession serializes fetch requests over the script's stdin and reads
// id-correlated replies off its stdout. One request is in flight at a time,
// which is all the sequential drain needs.
type browserSession struct {
	mu     sync.Mutex
	enc    *json.Encoder
	dec    *bufio.Scanner
	nextID int
	closer func() error // tears down the underlying process (set by OpenBrowserSession)
}

// newSession builds the framing core over arbitrary streams (browser-free, for
// tests and for OpenBrowserSession).
func newSession(stdin io.Writer, stdout io.Reader) *browserSession {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // portal payloads can be large
	return &browserSession{
		enc: json.NewEncoder(stdin),
		dec: sc,
	}
}

// Do sends one request and blocks for its reply.
func (s *browserSession) Do(ctx context.Context, req FetchRequest) (FetchResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return FetchResponse{}, err
	}

	s.nextID++
	id := s.nextID
	if err := s.enc.Encode(wireRequest{ID: id, URL: req.URL, Method: req.Method, Body: req.Body}); err != nil {
		return FetchResponse{}, fmt.Errorf("psaportal: session write: %w", err)
	}

	if !s.dec.Scan() {
		if err := s.dec.Err(); err != nil {
			return FetchResponse{}, fmt.Errorf("psaportal: session read: %w", err)
		}
		return FetchResponse{}, fmt.Errorf("psaportal: session closed before reply")
	}
	var reply wireReply
	if err := json.Unmarshal(s.dec.Bytes(), &reply); err != nil {
		return FetchResponse{}, fmt.Errorf("psaportal: session decode reply: %w", err)
	}
	if reply.ID != id {
		return FetchResponse{}, fmt.Errorf("psaportal: session reply id %d, want %d", reply.ID, id)
	}
	if reply.Error != "" {
		return FetchResponse{}, fmt.Errorf("psaportal: browser fetch error: %s", reply.Error)
	}
	return FetchResponse{Status: reply.Status, Body: reply.Body}, nil
}

// Close tears down the browser process, if one is attached.
func (s *browserSession) Close() error {
	if s.closer != nil {
		return s.closer()
	}
	return nil
}

// readHandshake reads the script's first line and parses the ready frame.
func readHandshake(sc *bufio.Scanner) (token string, expiresAt time.Time, err error) {
	if !sc.Scan() {
		if e := sc.Err(); e != nil {
			return "", time.Time{}, fmt.Errorf("psaportal: read handshake: %w", e)
		}
		return "", time.Time{}, fmt.Errorf("psaportal: script exited before handshake")
	}
	var hs struct {
		Type        string `json:"type"`
		AccessToken string `json:"accessToken"`
		ExpiresAt   string `json:"expiresAt"`
	}
	if err := json.Unmarshal(sc.Bytes(), &hs); err != nil {
		return "", time.Time{}, fmt.Errorf("psaportal: decode handshake: %w", err)
	}
	if hs.Type != "ready" || hs.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("psaportal: bad handshake (type=%q, token empty=%t)", hs.Type, hs.AccessToken == "")
	}
	exp, err := time.Parse(time.RFC3339, hs.ExpiresAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("psaportal: handshake expiresAt: %w", err)
	}
	return hs.AccessToken, exp, nil
}

// OpenBrowserSession launches the harvest script, waits for its login handshake,
// and returns a live session plus the freshly minted token. A still-valid
// storedToken is passed via PSA_PORTAL_ACCESS_TOKEN so the script can skip SSO.
func OpenBrowserSession(ctx context.Context, workDir, email, password, storedToken string, logger observability.Logger) (*browserSession, string, time.Time, error) {
	cmd := exec.CommandContext(ctx, "node", "web/scripts/harvest-psa-token.mjs")
	cmd.Dir = workDir
	cmd.Env = append(cmd.Environ(),
		"PSA_PORTAL_EMAIL="+email,
		"PSA_PORTAL_PASSWORD="+password,
	)
	if storedToken != "" {
		cmd.Env = append(cmd.Env, "PSA_PORTAL_ACCESS_TOKEN="+storedToken)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("psaportal: session stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("psaportal: session stdout: %w", err)
	}
	cmd.Stderr = newLogWriter(ctx, logger) // surface script stderr into logs
	if err := cmd.Start(); err != nil {
		return nil, "", time.Time{}, fmt.Errorf("psaportal: session start: %w", err)
	}

	s := newSession(stdin, stdout)
	s.closer = func() error {
		_ = s.enc.Encode(map[string]string{"type": "close"}) // best-effort graceful stop
		_ = stdin.Close()
		return cmd.Wait()
	}

	token, exp, err := readHandshake(s.dec)
	if err != nil {
		_ = s.Close()
		return nil, "", time.Time{}, err
	}
	return s, token, exp, nil
}

// logWriter forwards script stderr lines to the logger so login/selector
// failures are diagnosable.
type logWriter struct {
	ctx    context.Context
	logger observability.Logger
}

func newLogWriter(ctx context.Context, l observability.Logger) io.Writer { return &logWriter{ctx, l} }
func (w *logWriter) Write(p []byte) (int, error) {
	w.logger.Error(w.ctx, "psa harvest script", observability.String("stderr", strings.TrimSpace(string(p))))
	return len(p), nil
}
