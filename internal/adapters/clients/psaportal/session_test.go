package psaportal

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// fakePeer emulates the .mjs side: reads one request line, echoes a response
// keyed by the request id, for each scripted reply.
func TestBrowserSession_Do_FramesAndCorrelates(t *testing.T) {
	reqR, reqW := io.Pipe()   // Go -> script
	respR, respW := io.Pipe() // script -> Go

	// Scripted peer: for each incoming request line, write back a response with
	// the same id and a body derived from the URL.
	go func() {
		sc := bufio.NewScanner(reqR)
		w := bufio.NewWriter(respW)
		for sc.Scan() {
			var req map[string]any
			if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
				return
			}
			resp := map[string]any{"id": req["id"], "status": 200, "body": "ok:" + req["url"].(string)}
			b, _ := json.Marshal(resp)
			w.Write(b)
			w.WriteByte('\n')
			w.Flush()
		}
	}()

	s := newSession(reqW, respR)
	defer s.Close()

	r1, err := s.Do(context.Background(), FetchRequest{URL: "/a", Method: "GET"})
	if err != nil {
		t.Fatalf("Do(/a): %v", err)
	}
	if r1.Status != 200 || r1.Body != "ok:/a" {
		t.Fatalf("r1 = %+v, want status 200 body ok:/a", r1)
	}

	r2, err := s.Do(context.Background(), FetchRequest{URL: "/b", Method: "POST", Body: "{}"})
	if err != nil {
		t.Fatalf("Do(/b): %v", err)
	}
	if r2.Body != "ok:/b" {
		t.Fatalf("r2.Body = %q, want ok:/b", r2.Body)
	}
}

func TestReadHandshake(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr error // nil means expect success
		wantTok string
		wantYr  int
	}{
		{
			name:    "valid handshake",
			in:      `{"type":"ready","accessToken":"tok-9","expiresAt":"2099-01-01T00:00:00Z"}`,
			wantTok: "tok-9",
			wantYr:  2099,
		},
		{
			// An already-expired handshake token means the script never completed a
			// fresh SSO login — it read back the stale cookie we injected and echoed
			// it as "harvested". Accepting that is how an eight-hour outage kept
			// reporting a healthy login (2026-08-17).
			name:    "already-expired token rejected",
			in:      `{"type":"ready","accessToken":"tok-stale","expiresAt":"2020-01-01T00:00:00Z"}`,
			wantErr: ErrTokenExpired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := bufio.NewScanner(strings.NewReader(tt.in + "\n"))
			sc.Buffer(make([]byte, 0, 1024), 1<<20)
			tok, exp, err := readHandshake(sc)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("readHandshake err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("readHandshake: %v", err)
			}
			if tok != tt.wantTok {
				t.Fatalf("tok = %q, want %q", tok, tt.wantTok)
			}
			if exp.Year() != tt.wantYr {
				t.Fatalf("exp = %v, want year %d", exp, tt.wantYr)
			}
		})
	}
}

func TestBrowserSession_Do_PropagatesScriptError(t *testing.T) {
	reqR, reqW := io.Pipe()
	respR, respW := io.Pipe()
	go func() {
		sc := bufio.NewScanner(reqR)
		for sc.Scan() {
			var req map[string]any
			_ = json.Unmarshal(sc.Bytes(), &req)
			b, _ := json.Marshal(map[string]any{"id": req["id"], "error": "boom"})
			respW.Write(append(b, '\n'))
		}
	}()

	s := newSession(reqW, respR)
	defer s.Close()

	_, err := s.Do(context.Background(), FetchRequest{URL: "/x", Method: "GET"})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want to contain boom", err)
	}
}

func TestReusableToken(t *testing.T) {
	now := time.Date(2026, 8, 17, 22, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		token     string
		expiresAt time.Time
		want      string
	}{
		{"valid with room to spare", "tok", now.Add(4 * time.Hour), "tok"},
		{"already expired", "tok", now.Add(-1 * time.Minute), ""},
		{"expires inside the safety margin", "tok", now.Add(tokenReuseMargin / 2), ""},
		{"exactly at the margin is not reusable", "tok", now.Add(tokenReuseMargin), ""},
		{"no stored token", "", now.Add(4 * time.Hour), ""},
		{"zero expiry is never reusable", "tok", time.Time{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReusableToken(tt.token, tt.expiresAt, now); got != tt.want {
				t.Errorf("ReusableToken() = %q, want %q", got, tt.want)
			}
		})
	}
}
