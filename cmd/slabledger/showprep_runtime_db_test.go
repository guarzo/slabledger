package main

import (
	"net"
	"net/url"
	"strconv"
	"testing"
)

// Only this explicitly named disposable DB may be truncated. Reject query-based
// host/user/database overrides before pgx can interpret them, and check DB owner
// after connecting. Credentials and ports may differ between local runs and CI.
func safeShowRuntimeTestURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	port, err := strconv.Atoi(u.Port())
	return (u.Hostname() == "localhost" || ip.IsLoopback()) && err == nil && port > 0 && port <= 65535 &&
		u.Path == "/showprep_runtime_test" && u.RawQuery == "sslmode=disable" && u.Fragment == "" &&
		(u.User.Username() == "showprep" || u.User.Username() == "slabledger")
}

func TestShowPrepRuntimeDatabaseGuard(t *testing.T) {
	for _, tc := range []struct {
		name, uri string
		want      bool
	}{
		{"local", "postgres://showprep:fixture@127.0.0.1:44620/showprep_runtime_test?sslmode=disable", true},
		{"ci", "postgresql://slabledger:fixture@localhost:5432/showprep_runtime_test?sslmode=disable", true},
		{"ipv6", "postgres://showprep:fixture@[::1]:5432/showprep_runtime_test?sslmode=disable", true},
		{"storage", "postgres://showprep:fixture@127.0.0.1:44620/showprep_cached_test?sslmode=disable", false},
		{"browser", "postgres://showprep:fixture@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable", false},
		{"remote", "postgres://showprep:fixture@database.example:5432/showprep_runtime_test?sslmode=disable", false},
		{"override", "postgres://showprep:fixture@localhost:5432/showprep_runtime_test?sslmode=disable&host=database.example", false},
		{"wrong-user", "postgres://developer:fixture@localhost:5432/showprep_runtime_test?sslmode=disable", false},
		{"no-user", "postgres://localhost:5432/showprep_runtime_test?sslmode=disable", false},
		{"dsn", "host=localhost dbname=showprep_runtime_test", false},
		{"default-port", "postgres://showprep:fixture@localhost/showprep_runtime_test?sslmode=disable", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeShowRuntimeTestURL(tc.uri); got != tc.want {
				t.Errorf("safe target = %v, want %v", got, tc.want)
			}
		})
	}
}

// The storage package resets its own schema in parallel with cmd tests in CI.
// A storage opt-in must never activate the destructive runtime fixture.
func TestShowPrepRuntimeRequiresIndependentOptIn(t *testing.T) {
	t.Setenv("SHOW_PREP_RUNTIME_TEST_URL", "")
	t.Setenv("POSTGRES_TEST_URL", "postgresql://slabledger:slabledger@localhost:5432/slabledger?sslmode=disable")
	skipped := false
	t.Run("storage_only", func(t *testing.T) {
		defer func() { skipped = t.Skipped() }()
		db := showRuntimeDB(t)
		if db != nil {
			t.Fatal("runtime fixture activated by storage environment")
		}
	})
	if !skipped {
		t.Error("runtime fixture must skip without its dedicated opt-in")
	}
}
