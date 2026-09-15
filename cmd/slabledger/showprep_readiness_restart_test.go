package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Keep restart failures as ordinary handler errors, not FailNow in an HTTP
// goroutine. Callers own the concrete test-only DB/router reconstruction.
func restartReadiness(mu *sync.RWMutex, closeCurrent, reopen func() error) error {
	mu.Lock()
	defer mu.Unlock()
	if err := closeCurrent(); err != nil {
		return fmt.Errorf("close fixture database: %w", err)
	}
	if err := reopen(); err != nil {
		return fmt.Errorf("reopen fixture application: %w", err)
	}
	return nil
}

func TestReadinessConcurrentRestartClosesCurrentInstance(t *testing.T) {
	const restarts = 16
	var mu sync.RWMutex
	directory := t.TempDir()
	current, err := os.CreateTemp(directory, "instance-")
	require.NoError(t, err)
	instances := []*os.File{current}
	closes := map[*os.File]int{}
	t.Cleanup(func() {
		for _, instance := range instances {
			_ = instance.Close()
		}
	})
	ready := make(chan struct{}, restarts)
	results := make(chan error, restarts)
	mu.Lock()
	for range restarts {
		go func() {
			// Mirror the caller's close binding. All callbacks are constructed
			// before any restart may replace current, making early binding fail
			// deterministically rather than relying on the race detector alone.
			closeCurrent := func() error { return current.Close() }
			ready <- struct{}{}
			results <- restartReadiness(&mu, func() error {
				if err := closeCurrent(); err != nil {
					return err
				}
				closes[current]++
				return nil
			}, func() error {
				next, err := os.CreateTemp(directory, "instance-")
				if err != nil {
					return err
				}
				current = next
				instances = append(instances, next)
				return nil
			})
		}()
	}
	for range restarts {
		<-ready
	}
	mu.Unlock()
	// Drain all workers before assertions/cleanup touch shared instance state.
	errs := make([]error, restarts)
	for i := range errs {
		errs[i] = <-results
	}
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Len(t, instances, restarts+1)
	for _, prior := range instances[:restarts] {
		require.Equal(t, 1, closes[prior], "each displaced instance must close exactly once")
		_, err := prior.WriteString("closed")
		require.ErrorIs(t, err, os.ErrClosed)
	}
	require.Same(t, instances[restarts], current)
	require.Zero(t, closes[current], "the last installed instance must remain open")
	_, err = current.WriteString("still current")
	require.NoError(t, err)
}

func TestReadinessRestartFailureReleasesRequests(t *testing.T) {
	for _, stage := range []string{"close", "reopen"} {
		t.Run(stage, func(t *testing.T) {
			var mu sync.RWMutex
			failed := false
			failOnce := func(at string) error {
				if at == stage && !failed {
					failed = true
					return errors.New("controlled " + stage + " failure")
				}
				return nil
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/restart" {
					err := restartReadiness(&mu, func() error { return failOnce("close") }, func() error { return failOnce("reopen") })
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
				}
				mu.RLock()
				defer mu.RUnlock()
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			client := &http.Client{Timeout: time.Second}
			response, err := client.Post(server.URL+"/restart", "application/json", strings.NewReader("{}"))
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, response.Body.Close())
			require.NoError(t, err)

			// A RED must not leave httptest.Close hanging forever. Probe the actual
			// shared lock, then release either our probe or the broken retained lock.
			unlocked := mu.TryLock()
			mu.Unlock()
			require.True(t, unlocked, "failed restart retained the application write lock")
			require.Equal(t, http.StatusInternalServerError, response.StatusCode)
			require.Contains(t, string(body), "controlled "+stage+" failure")
			for _, route := range []string{"/read", "/restart", "/read"} {
				response, err = client.Get(server.URL + route)
				require.NoError(t, err, "later requests must settle after failed reconstruction")
				require.NoError(t, response.Body.Close())
				require.Equal(t, http.StatusOK, response.StatusCode)
			}
			done := make(chan struct{})
			go func() { server.Close(); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("shutdown hung after failed restart")
			}
		})
	}
}
