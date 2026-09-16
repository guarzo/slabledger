package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	cl "github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	cldomain "github.com/guarzo/slabledger/internal/domain/cardladder"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

const diagnosticSecret = "postgres://user:SECRET@remote.example/db?token=SECRET SQL PARAM SECRET"

// Exercise the actual worker -> runtime log boundary, not a formatter in isolation.
func TestShowPrepPersistenceDiagnostics(t *testing.T) {
	for _, stage := range []string{"acquire", "candidates", "begin", "finish", "end"} {
		for _, failure := range []struct {
			name               string
			err                error
			state              string
			deadline, canceled bool
		}{
			{"sql", &pgconn.PgError{Code: "40001", Message: diagnosticSecret, Detail: diagnosticSecret, InternalQuery: diagnosticSecret}, "40001", false, false},
			{"deadline", fmt.Errorf("%s: %w", diagnosticSecret, context.DeadlineExceeded), "", true, false},
			{"cancel", fmt.Errorf("%s: %w", diagnosticSecret, context.Canceled), "", false, true},
			{"untrusted", apperrors.StorageError(diagnosticSecret, &pgconn.PgError{Code: diagnosticSecret, Message: diagnosticSecret}).WithContext("operation", diagnosticSecret).WithContext("category", diagnosticSecret), "", false, false},
		} {
			t.Run(stage+"/"+failure.name, func(t *testing.T) {
				id := sp.Identity{ProfileID: "fixture", Grader: "PSA", Grade: 10}
				store := &mocks.ShowPrepWorkerStoreMock{CandidatesFn: func(context.Context) ([]sp.WorkerCandidate, error) {
					return []sp.WorkerCandidate{{Identity: id, Cards: 1}}, nil
				}}
				switch stage {
				case "acquire":
					store.AcquireFn = func(context.Context, string) (sp.WorkerLease, bool, error) {
						return sp.WorkerLease{}, false, failure.err
					}
				case "candidates":
					store.CandidatesFn = func(context.Context) ([]sp.WorkerCandidate, error) { return nil, failure.err }
				case "begin":
					store.BeginFn = func(context.Context, sp.WorkerLease, sp.Identity, time.Time) (int64, error) { return 0, failure.err }
				case "finish":
					store.FinishFn = func(context.Context, sp.WorkerLease, sp.Identity, int64, sp.Snapshot, time.Time, bool) error {
						return failure.err
					}
				case "end":
					store.EndFn = func(context.Context, sp.WorkerLease, sp.WorkerRunResult, time.Time) error { return failure.err }
				}
				source := &mocks.ShowPrepSourceMock{FetchFn: func(_ context.Context, id sp.Identity, now time.Time) (sp.Snapshot, error) {
					start, end := sp.Window(now)
					return sp.Snapshot{Identity: id, Source: "cardladder", Complete: true, WindowStart: start, WindowEnd: end, RefreshedAt: now}, nil
				}}
				worker := sp.NewEvidenceWorker(store, func(context.Context) (sp.Source, error) { return source, nil }, nil, nil)
				err := worker.RunOnce(context.Background())
				require.ErrorIs(t, err, failure.err)
				var diagnostic *apperrors.AppError
				require.ErrorAs(t, err, &diagnostic)
				require.Equal(t, "showprep."+stage, diagnostic.Context["operation"])
				var pg *pgconn.PgError
				if errors.As(failure.err, &pg) {
					var got *pgconn.PgError
					require.ErrorAs(t, err, &got)
					require.Same(t, pg, got)
				}
				logger := mocks.NewCapturingLogger()
				s := NewShowPrepRefreshScheduler(worker, nil, logger, true)
				entry := runDiagnosticSweep(t, s, logger, "show evidence sweep incomplete")
				assertSafeDiagnostic(t, entry, "showprep."+stage, "storage", failure.state, failure.deadline, failure.canceled)
			})
		}
	}
}

func TestShowPrepConfigurationDiagnostics(t *testing.T) {
	for _, failure := range []struct {
		name               string
		err                error
		deadline, canceled bool
	}{
		{"database", &pgconn.PgError{Code: "42P01", Message: diagnosticSecret}, false, false},
		{"deadline", fmt.Errorf("%s: %w", diagnosticSecret, context.DeadlineExceeded), true, false},
		{"cancel", fmt.Errorf("%s: %w", diagnosticSecret, context.Canceled), false, true},
	} {
		t.Run(failure.name, func(t *testing.T) {
			logger := mocks.NewCapturingLogger()
			config := cl.NewConfiguredClient(&mocks.CardLadderStoreMock{GetConfigFn: func(context.Context) (*cldomain.Config, error) { return nil, failure.err }}, nil, logger)
			s := NewShowPrepRefreshScheduler(sp.NewEvidenceWorker(&mocks.ShowPrepWorkerStoreMock{}, nil, nil, nil), config, logger, true)
			entry := runDiagnosticSweep(t, s, logger, "show evidence credentials unavailable")
			state := ""
			if failure.name == "database" {
				state = "42P01"
			}
			assertSafeDiagnostic(t, entry, "showprep.configuration.refresh", "configuration", state, failure.deadline, failure.canceled)
		})
	}
}

func TestShowPrepSourceDiagnosticsNeverLogProviderText(t *testing.T) {
	for _, stage := range []string{"resolve", "fetch"} {
		for _, cause := range []error{errors.New(diagnosticSecret), context.DeadlineExceeded, context.Canceled} {
			t.Run(stage+"/"+fmt.Sprint(errors.Is(cause, context.DeadlineExceeded))+"/"+fmt.Sprint(errors.Is(cause, context.Canceled)), func(t *testing.T) {
				logger := mocks.NewCapturingLogger()
				// Even plausible forged stage metadata must not turn a provider fault
				// into a database diagnosis. Never copy provider Context to the log.
				failure := apperrors.ProviderUnavailable(diagnosticSecret, cause).WithContext("operation", "showprep.begin")
				store := &mocks.ShowPrepWorkerStoreMock{CandidatesFn: func(context.Context) ([]sp.WorkerCandidate, error) {
					return []sp.WorkerCandidate{{Identity: sp.Identity{ProfileID: "fixture", Grader: "PSA", Grade: 10}}}, nil
				}}
				worker := sp.NewEvidenceWorker(store, func(context.Context) (sp.Source, error) {
					if stage == "resolve" {
						return nil, failure
					}
					return &mocks.ShowPrepSourceMock{FetchFn: func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) { return sp.Snapshot{}, failure }}, nil
				}, nil, nil)
				err := worker.RunOnce(context.Background())
				require.ErrorIs(t, err, sp.ErrWorkerFailed)
				require.ErrorIs(t, err, cause)
				s := NewShowPrepRefreshScheduler(worker, nil, logger, true)
				entry := runDiagnosticSweep(t, s, logger, "show evidence sweep incomplete")
				assertSafeDiagnostic(t, entry, "showprep.source", "source", "", errors.Is(cause, context.DeadlineExceeded), errors.Is(cause, context.Canceled))
			})
		}
	}
}

func runDiagnosticSweep(t *testing.T, s *ShowPrepRefreshScheduler, logger *mocks.CapturingLogger, message string) mocks.LogEntry {
	t.Helper()
	group := NewGroup(s)
	group.StartAll(context.Background())
	t.Cleanup(func() { group.StopAll(); group.Wait() })
	var found mocks.LogEntry
	require.Eventually(t, func() bool {
		for _, entry := range logger.Entries() {
			if entry.Message == message {
				found = entry
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond)
	group.StopAll()
	group.Wait()
	encoded, err := json.Marshal(logger.Entries())
	require.NoError(t, err)
	for _, secret := range []string{"SECRET", "remote.example", "postgres://", "SQL PARAM"} {
		require.NotContains(t, string(encoded), secret)
	}
	return found
}

func assertSafeDiagnostic(t *testing.T, entry mocks.LogEntry, operation, category, state string, deadline, canceled bool) {
	t.Helper()
	fields := map[string]any{}
	for _, f := range entry.Fields {
		fields[f.Key] = f.Value
	}
	want := map[string]any{"operation": operation, "category": category, "deadline": deadline, "canceled": canceled}
	if state != "" {
		want["sqlstate"] = state
	}
	require.Equal(t, want, fields)
	t.Logf("safe diagnostic: %v", fields)
}
