package scheduler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/csvimport"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

const testFeedURL = "https://hc-ping.com/12345678-1234-1234-1234-123456789abc"

func TestFeedFreshnessClockOutcomes(t *testing.T) {
	now := time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, key, value                   string
		interval                           time.Duration
		err                                error
		disabled, unavailable, input, fail bool
	}{
		{"fresh", "feed_dh_orders_clean", now.Format(time.RFC3339), 0, nil, false, false, false, false},
		{"orders boundary", "feed_dh_orders_clean", now.Add(-90 * time.Minute).Format(time.RFC3339), 0, nil, false, false, false, false},
		{"orders stale", "feed_dh_orders_clean", now.Add(-90*time.Minute - time.Second).Format(time.RFC3339), 0, nil, false, false, false, true},
		{"custom interval", "feed_dh_orders_clean", now.Add(-31 * time.Minute).Format(time.RFC3339), 10 * time.Minute, nil, false, false, false, true},
		{"inventory boundary", "feed_dh_inventory_clean", now.Add(-6 * time.Hour).Format(time.RFC3339), 0, nil, false, false, false, false},
		{"inventory stale", "feed_dh_inventory_clean", now.Add(-6*time.Hour - time.Second).Format(time.RFC3339), 0, nil, false, false, false, true},
		{"CL boundary", "feed_cardladder_clean", now.Add(-30 * time.Hour).Format(time.RFC3339), 0, nil, false, false, false, false},
		{"CL stale", "feed_cardladder_clean", now.Add(-30*time.Hour - time.Second).Format(time.RFC3339), 0, nil, false, false, false, true},
		{"CL config missing", "feed_cardladder_clean", now.Format(time.RFC3339), 0, nil, false, false, false, true},
		{"CL config unreadable", "feed_cardladder_clean", now.Format(time.RFC3339), 0, nil, false, false, false, true},
		{"PSA import stale", "feed_psa_clean", now.Add(-30*time.Hour - time.Second).Format(time.RFC3339), 0, nil, false, false, false, true},
		{"future", "feed_dh_orders_clean", now.Add(time.Second).Format(time.RFC3339), 0, nil, false, false, false, true},
		{"missing", "feed_dh_orders_clean", "", 0, nil, false, false, false, true},
		{"invalid", "feed_dh_orders_clean", "secret-invalid", 0, nil, false, false, false, true},
		{"unreadable", "feed_dh_orders_clean", "", 0, errors.New("secret"), false, false, false, true},
		{"disabled", "feed_dh_orders_clean", "", 0, nil, true, false, false, false},
		{"unconfigured", "feed_dh_orders_clean", "", 0, nil, false, true, false, true},
		{"input boundary", "feed_psa_clean", now.Add(-26 * time.Hour).Format(time.RFC3339), 0, nil, false, false, true, false},
		{"input stale", "feed_psa_clean", now.Add(-26*time.Hour - time.Second).Format(time.RFC3339), 0, nil, false, false, true, true},
		{"input unreadable", "feed_psa_clean", now.Format(time.RFC3339), 0, errors.New("secret"), false, false, true, true},
		{"concurrent completion", "feed_dh_orders_clean", now.Add(time.Second).Format(time.RFC3339), 0, nil, false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.ShowPrepRefresh.Enabled = false
			cfg.DH.Enabled = strings.HasPrefix(tc.key, "feed_dh_") && !tc.disabled
			cfg.DH.OrdersPollInterval = tc.interval
			cfg.DH.InventoryPollInterval = tc.interval
			cfg.CardLadder.Enabled = tc.key == "feed_cardladder_clean"
			cfg.PSASync.Enabled = tc.key == "feed_psa_clean"
			deps := fullyGatedDeps()
			deps.DHOrdersClient = &mocks.MockDHOrdersClient{}
			deps.OrdersImporter = &mocks.MockImportService{}
			repo := &mocks.PurchaseRepositoryMock{}
			deps.CardLadderStore = &postgres.CardLadderStore{}
			deps.CardLadderPurchaseLister = repo
			deps.CardLadderValueUpdater = repo
			deps.ShowPrepCredentials = cardladder.NewConfiguredClient(&mocks.CardLadderStoreMock{GetConfigFn: func(context.Context) (*postgres.CardLadderConfig, error) {
				if tc.name == "CL config missing" {
					return nil, nil
				}
				if tc.name == "CL config unreadable" {
					return nil, errors.New("secret credentials")
				}
				return &postgres.CardLadderConfig{FirebaseAPIKey: "fixture", RefreshToken: "fixture"}, nil
			}}, nil, deps.Logger)
			if tc.unavailable {
				deps.DHOrdersClient = nil
			}
			readNow := now
			store := newMockSyncStateStore()
			deps.SyncStateStore = store
			store.GetFn = func(ctx context.Context, key string) (string, error) {
				_, ok := ctx.Deadline()
				require.True(t, ok)
				if tc.name == "concurrent completion" {
					readNow = now.Add(time.Second)
				}
				if key == tc.key && !tc.input {
					require.False(t, tc.disabled || tc.unavailable)
					return tc.value, tc.err
				}
				return now.Format(time.RFC3339), nil
			}
			deps.PSASnapshotFetchedAt = func(context.Context) (time.Time, error) {
				if tc.input {
					at, _ := time.Parse(time.RFC3339, tc.value)
					return at, tc.err
				}
				return now, nil
			}
			s := buildFeedFreshnessScheduler(&cfg, deps, nil)
			s.showPrep = func(context.Context) (ShowPrepWorkerStatus, error) {
				return ShowPrepWorkerStatus{}, errors.New("disabled evidence store unavailable")
			}
			body, failed, _ := s.report(context.Background(), func() time.Time { return readNow })
			require.Equal(t, tc.fail, failed)
			require.NotContains(t, body, "secret")
			if tc.name == "orders stale" {
				require.Contains(t, body, "last clean "+tc.value)
			}
		})
	}
}

func TestFeedFreshnessCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, state                            string
		hour                                   int
		missing, configured, readErr, wantFail bool
	}{
		{"idle", "idle", 3, false, true, false, false},
		{"hold during rollover", "auth_hold", 1, false, true, false, true},
		{"failed during rollover", "failed", 1, false, true, false, true},
		{"failed identity", "identity_failure", 1, false, true, false, true},
		{"missing", "idle", 3, true, true, false, true},
		{"stale", "stale", 3, false, true, false, true},
		{"rollover allowance", "idle", 1, true, true, false, false},
		{"rollover ends", "idle", 2, true, true, false, true},
		{"unconfigured", "idle", 3, false, false, false, true},
		{"read error", "idle", 3, false, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := buildFeedFreshnessScheduler(defaultCfg(), minimalDeps(), nil)
			s.feeds = nil
			s.psaEnabled = false
			s.showPrep = func(context.Context) (ShowPrepWorkerStatus, error) {
				status := ShowPrepWorkerStatus{Configured: tc.configured, WorkerStatus: sp.WorkerStatus{State: tc.state}}
				if tc.missing {
					status.MissingIdentities = 1
				}
				if tc.state == "stale" {
					status.StaleIdentities = 1
				}
				if tc.state == "identity_failure" {
					status.FailedIdentities = 1
				}
				if tc.readErr {
					return status, errors.New("secret")
				}
				return status, nil
			}
			body, failed, _ := s.report(context.Background(), func() time.Time { return time.Date(2026, 11, 8, tc.hour, 0, 0, 0, time.UTC) })
			require.Equal(t, tc.wantFail, failed)
			require.NotContains(t, body, "secret")
		})
	}
}

func TestFeedFreshnessTransport(t *testing.T) {
	for _, tc := range []struct {
		name, reply              string
		status                   int
		fail, redirect, wantWarn bool
	}{
		{"success", "OK", 200, false, false, false}, {"feed failure", "OK", 200, true, false, false},
		{"bad acknowledgement", "OK (not found)", 200, false, false, true},
		{"truncated acknowledgement", "OK" + strings.Repeat(" ", 30) + "not found", 200, false, false, true},
		{"cancelled", "", 0, false, false, true},
		{"server error", "OK", 500, false, false, true}, {"redirect", "OK", 302, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger := mocks.NewCapturingLogger()
			deps := minimalDeps()
			deps.Logger = logger
			s := buildFeedFreshnessScheduler(defaultCfg(), deps, nil)
			s.url = testFeedURL
			s.feeds = nil
			s.showPrepEnabled = false
			s.psaEnabled = tc.fail
			calls := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.client.Transport = mocks.HTTPTransportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if tc.name == "cancelled" {
					cancel()
					<-r.Context().Done()
					return nil, r.Context().Err()
				}
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "text/plain; charset=utf-8", r.Header.Get("Content-Type"))
				wantPath := "/12345678-1234-1234-1234-123456789abc"
				if tc.fail {
					wantPath += "/fail"
				}
				require.Equal(t, wantPath, r.URL.Path)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NotContains(t, string(body), testFeedURL)
				header := make(http.Header)
				if tc.redirect {
					header.Set("Location", "https://other.invalid/stolen")
				}
				return &http.Response{StatusCode: tc.status, Header: header, Body: io.NopCloser(strings.NewReader(tc.reply)), Request: r}, nil
			})
			s.check(ctx)
			require.Equal(t, 1, calls)
			require.Equal(t, tc.wantWarn, len(logger.Entries()) > 0)
			require.NotContains(t, fmt.Sprint(logger.Entries()), testFeedURL)
		})
	}
}

func TestFeedFreshnessStartupJudgment(t *testing.T) {
	for _, tc := range []struct {
		name string
		want []string
	}{
		{"fresh saved outcomes", []string{"healthy"}},
		{"pending only", nil},
		{"finished failure and pending", []string{"fail"}},
		{"completion during read", []string{"healthy"}},
		{"all recover", []string{"healthy"}},
		{"CL configuration missing", []string{"fail"}},
		{"CL configuration missing, unreadable clock", []string{"fail"}},
		{"show-prep unconfigured", []string{"fail"}},
		{"auth hold", []string{"fail"}},
		{"show-prep failed pending", nil},
		{"cancelled", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := buildFeedFreshnessScheduler(defaultCfg(), minimalDeps(), nil)
				s.url, s.psaEnabled, s.showPrepEnabled = testFeedURL, false, false
				s.feeds = []freshnessFeed{{"DH orders", syncStateKeyDHOrdersClean, time.Hour, true, true}, {"DH inventory", syncStateKeyDHInventoryClean, time.Hour, true, true}}
				ordersDone, inventoryDone := s.observeStartup("DH orders"), s.observeStartup("DH inventory")
				fresh := tc.name == "fresh saved outcomes"
				inventoryFresh := fresh
				reads := 0
				store := newMockSyncStateStore()
				s.state = store
				store.GetFn = func(_ context.Context, key string) (string, error) {
					reads++
					if strings.HasSuffix(tc.name, "unreadable clock") && key == syncStateKeyCLClean {
						return "", errors.New("unreadable")
					}
					value := ""
					if fresh && (key != syncStateKeyDHInventoryClean || inventoryFresh) {
						value = time.Now().Format(time.RFC3339)
					}
					if tc.name == "completion during read" && reads == 1 {
						fresh, inventoryFresh = true, true
						ordersDone()
						inventoryDone()
					}
					return value, nil
				}
				if tc.name == "finished failure and pending" {
					ordersDone()
				}
				if strings.HasPrefix(tc.name, "CL configuration missing") {
					s.feeds = append(s.feeds, freshnessFeed{"CardLadder", syncStateKeyCLClean, time.Hour, true, true})
					s.observeStartup("CardLadder")
					s.clConfigured = func(context.Context) (bool, error) { return false, nil }
				}
				if strings.HasPrefix(tc.name, "show-prep") || tc.name == "auth hold" {
					s.showPrepEnabled = true
					s.observeStartup("Show-prep")
					s.showPrep = func(context.Context) (ShowPrepWorkerStatus, error) {
						state := "failed"
						if tc.name == "auth hold" {
							state = "auth_hold"
						}
						return ShowPrepWorkerStatus{Configured: tc.name != "show-prep unconfigured", WorkerStatus: sp.WorkerStatus{State: state}}, nil
					}
				}
				var got []string
				s.client.Transport = mocks.HTTPTransportFunc(func(r *http.Request) (*http.Response, error) {
					kind := "healthy"
					if strings.HasSuffix(r.URL.Path, "/fail") {
						kind = "fail"
					}
					got = append(got, kind)
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					if tc.name == "finished failure and pending" {
						require.Contains(t, string(body), "DH inventory: startup catch-up pending; not judged")
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("OK")), Header: make(http.Header)}, nil
				})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tc.name == "cancelled" {
					cancel()
				}
				go s.Start(ctx)
				synctest.Wait()
				if tc.name == "all recover" {
					require.Empty(t, got)
					fresh = true
					ordersDone()
					synctest.Wait()
					require.Empty(t, got) // A completed fresh feed cannot clear another pending feed.
					inventoryFresh = true
					inventoryDone()
					ordersDone()
					synctest.Wait()
				}
				require.Equal(t, len(tc.want) == 0, len(got) == 0)
				for _, kind := range got {
					require.Contains(t, tc.want, kind)
				}
				if tc.name == "cancelled" {
					require.Zero(t, reads)
				}
				cancel()
				synctest.Wait()
			})
		})
	}
}

func TestFeedFreshnessStartupWiring(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := defaultCfg()
		cfg.DH.Enabled, cfg.PSASync.Enabled = enabled, enabled
		built := BuildGroup(cfg, fullyGatedDeps())
		for _, sched := range built.Group.schedulers {
			if s, ok := sched.(*FeedFreshnessScheduler); ok {
				want := 0
				if enabled {
					want = 2
				} // Only inventory and PSA have dependencies in this fixture.
				require.Len(t, s.startup, want)
				if enabled {
					require.Contains(t, s.startup, "DH inventory")
					require.Contains(t, s.startup, "PSA sync")
				}
			}
		}
	}
}

func TestPSAFirstAttemptWaitsForOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fetchDone := make(chan struct{})
		provider := &mocks.PSARowProviderMock{FetchRowsFn: func(context.Context) ([]csvimport.PSAExportRow, error) {
			<-fetchDone
			return nil, errors.New("failed fetch")
		}}
		s := NewPSASyncScheduler(provider, &mocks.MockImportService{}, nopLogger{}, config.PSASyncConfig{Enabled: true, SyncHour: -1})
		completed := 0
		s.onFirstRun = func() { completed++ }
		go func() { require.Error(t, s.RunOnce(context.Background())) }()
		synctest.Wait()
		s.config.InitialDelay = 0
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go s.Start(ctx)
		synctest.Wait()
		require.Zero(t, completed)
		require.ErrorIs(t, s.RunOnce(ctx), ErrSyncInProgress)
		close(fetchDone)
		synctest.Wait()
		require.Equal(t, 1, completed)
		cancel()
		synctest.Wait()
	})
}

func TestFeedFreshnessStartupDoesNotJudge(t *testing.T) {
	for _, raw := range []string{"", testFeedURL, "http://hc-ping.com/12345678-1234-1234-1234-123456789abc", testFeedURL + "?create=1", testFeedURL + "#fragment", "https://user:pass@hc-ping.com/12345678-1234-1234-1234-123456789abc", testFeedURL + "/fail", "https://other.invalid/12345678-1234-1234-1234-123456789abc"} {
		t.Run(raw, func(t *testing.T) {
			cfg := config.Default()
			cfg.Adapters.FeedFreshnessPingURL = raw
			logger := mocks.NewCapturingLogger()
			deps := minimalDeps()
			deps.Logger = logger
			store := newMockSyncStateStore()
			deps.SyncStateStore = store
			store.GetFn = func(context.Context, string) (string, error) { t.Fatal("startup must not read"); return "", nil }
			s := buildFeedFreshnessScheduler(&cfg, deps, nil)
			s.client.Transport = mocks.HTTPTransportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("startup must not ping"); return nil, nil })
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			s.Start(ctx)
			warn := false
			for _, entry := range logger.Entries() {
				warn = warn || entry.Level == "warn"
			}
			require.Equal(t, raw != "" && raw != testFeedURL, warn)
		})
	}
}
