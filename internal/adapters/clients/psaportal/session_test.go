package psaportal

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
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
	in := `{"type":"ready","accessToken":"tok-9","expiresAt":"2099-01-01T00:00:00Z"}` + "\n"
	sc := bufio.NewScanner(strings.NewReader(in))
	sc.Buffer(make([]byte, 0, 1024), 1<<20)
	tok, exp, err := readHandshake(sc)
	if err != nil {
		t.Fatalf("readHandshake: %v", err)
	}
	if tok != "tok-9" {
		t.Fatalf("tok = %q", tok)
	}
	if exp.Year() != 2099 {
		t.Fatalf("exp = %v", exp)
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
