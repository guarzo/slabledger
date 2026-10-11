package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Explicit owned disposable DB only; production runtime/router/auth/stores and
// built frontend. All control routes are compiled exclusively into the test.
func TestShowScanRealBrowser(t *testing.T) {
	raw := os.Getenv("SHOW_READINESS_E2E_URL")
	if raw == "" {
		t.Skip("set SHOW_READINESS_E2E_URL to the owned disposable e2e database explicitly")
	}
	require.Equal(t, readinessDBURL, raw, "refusing unspecified/developer/production database")
	t.Chdir("../..")
	_, err := os.Stat("web/dist/index.html")
	require.NoError(t, err, "run cd web && npm run build first")
	started := time.Now().UTC()
	f := &readinessSourceFixture{now: started, started: started}
	f.setMode("blocked")
	source := startReadinessProvider(t, f)
	t.Setenv("CL_SEARCH_URL", source.URL+"/search")
	owner := newScanBrowserFixture(t, raw, f, source.URL)
	defer owner.close(t)
	app := httptest.NewServer(http.HandlerFunc(owner.serve))
	defer app.Close()
	control := httptest.NewServer(readinessHandler(t.Errorf, func(w http.ResponseWriter, r *http.Request) error { return owner.control(t, w, r) }))
	defer control.Close()
	artifacts := os.Getenv("SHOW_READINESS_ARTIFACTS")
	if artifacts == "" {
		artifacts = t.TempDir()
	}
	artifacts, err = filepath.Abs(artifacts)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(artifacts, 0750))
	cmd := exec.CommandContext(t.Context(), "node", "web/tests/show-scan-real.cjs")
	cmd.Env = append(os.Environ(), "SHOW_READINESS_APP="+app.URL, "SHOW_READINESS_CONTROL="+control.URL,
		"SHOW_READINESS_TOKEN="+readinessToken, "SHOW_READINESS_ARTIFACTS="+artifacts)
	output, err := cmd.CombinedOutput()
	t.Logf("browser output:\n%s\nartifacts=%s", output, artifacts)
	require.NoError(t, err)
	require.GreaterOrEqual(t, owner.restarts, 2, "actual SQL pool/auth/router/services rebuilt")
	archiveReadiness(t, artifacts, "scan-final", readinessProof(t, owner.db, f))
}
