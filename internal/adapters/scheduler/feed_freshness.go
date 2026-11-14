package scheduler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	syncStateKeyDHOrdersClean    = "feed_dh_orders_clean"
	syncStateKeyDHInventoryClean = "feed_dh_inventory_clean"
	syncStateKeyCLClean          = "feed_cardladder_clean"
	syncStateKeyPSAClean         = "feed_psa_clean"
)

type freshnessFeed struct {
	name, key          string
	maxAge             time.Duration
	enabled, available bool
}

// FeedFreshnessScheduler checks outcomes, never attempts or provider event cursors.
type FeedFreshnessScheduler struct {
	StopHandle
	url                         string
	state                       SyncStateStore
	feeds                       []freshnessFeed
	startup                     map[string]<-chan struct{}
	wake                        chan struct{}
	showPrep                    func(context.Context) (ShowPrepWorkerStatus, error)
	clConfigured                func(context.Context) (bool, error)
	showPrepEnabled, psaEnabled bool
	snapshotAt                  func(context.Context) (time.Time, error)
	client                      *http.Client
	logger                      observability.Logger
}

func (s *FeedFreshnessScheduler) Start(ctx context.Context) {
	if s.url == "" {
		s.logger.Info(ctx, "feed freshness disabled: ping URL unset")
		return
	}
	// Restrict the secret destination, including path shape, before any network IO.
	if !regexp.MustCompile(`^https://hc-ping\.com/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(s.url) {
		s.logger.Warn(ctx, "feed freshness disabled: invalid ping URL")
		return
	}
	RunLoop(ctx, LoopConfig{Name: "feed-freshness", Interval: 15 * time.Minute, Wake: s.wake, StopChan: s.Done(), WG: s.WG(), Logger: s.logger}, s.check)
}

// observeStartup is wired before any scheduler starts. Completion allows judgment,
// never recovery by itself; a failed or skipped first attempt must also notify.
func (s *FeedFreshnessScheduler) observeStartup(name string) func() {
	if s.startup == nil {
		s.startup = make(map[string]<-chan struct{})
	}
	done := make(chan struct{})
	s.startup[name] = done
	return sync.OnceFunc(func() {
		close(done)
		select {
		case s.wake <- struct{}{}:
		default:
		}
	})
}

func freshnessReason(at, now time.Time, age time.Duration) string {
	if at.IsZero() || at.After(now) {
		return "success time missing or invalid"
	}
	if now.Sub(at) > age {
		return "stale; last clean " + at.UTC().Format(time.RFC3339)
	}
	return ""
}

func (s *FeedFreshnessScheduler) report(parent context.Context, now func() time.Time) (string, bool, bool) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	// Snapshot BEFORE reads: completion during a read queues a fresh evaluation,
	// rather than permitting failure based on evidence read before completion.
	waiting := make(map[string]bool, len(s.startup))
	for name, done := range s.startup {
		select {
		case <-done:
		default:
			waiting[name] = true
		}
	}
	var lines []string
	failed, pending := false, false
	add := func(name, reason string) {
		if reason != "" && waiting[name] {
			pending = true
			lines = append(lines, name+": startup catch-up pending; not judged")
		} else if reason != "" {
			failed = true
			lines = append(lines, name+": "+reason)
		} else {
			lines = append(lines, name+": fresh")
		}
	}
	for _, f := range s.feeds {
		if !f.enabled {
			lines = append(lines, f.name+": disabled by config")
			continue
		}
		if !f.available || s.state == nil {
			waiting[f.name] = false
			add(f.name, "unconfigured")
			continue
		}
		if f.name == "CardLadder" {
			configured := false
			var err error
			if s.clConfigured != nil {
				configured, err = s.clConfigured(ctx)
			}
			if !configured || err != nil {
				waiting[f.name] = false
				add(f.name, "configuration unavailable")
				continue
			}
		}
		value, err := s.state.Get(ctx, f.key)
		if err != nil {
			add(f.name, "status unreadable")
			continue
		}
		at, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			at = time.Time{}
		}
		add(f.name, freshnessReason(at, now(), f.maxAge))
	}
	if s.psaEnabled {
		at := time.Time{}
		var err error
		if s.snapshotAt != nil {
			at, err = s.snapshotAt(ctx)
		}
		reason := freshnessReason(at, now(), 26*time.Hour)
		if err != nil {
			reason = "status unreadable"
		}
		add("PSA input", reason)
	}
	if s.showPrepEnabled {
		reason := "status unavailable"
		if s.showPrep != nil {
			status, err := s.showPrep(ctx)
			if err == nil {
				if !status.Configured || status.State == "auth_hold" {
					waiting["Show-prep"] = false
				}
				reason = ""
				switch {
				case !status.Configured:
					reason = "unconfigured"
				case status.State == "auth_hold" || status.State == "failed" || status.FailedIdentities > 0:
					reason = "held or failed; last sweep " + status.LastSweepAt
				case now().UTC().Hour() >= 2 && (status.MissingIdentities > 0 || status.StaleIdentities > 0):
					reason = fmt.Sprintf("coverage missing=%d stale=%d; last sweep %s", status.MissingIdentities, status.StaleIdentities, status.LastSweepAt)
				}
			}
		}
		add("Show-prep", reason)
	} else {
		lines = append(lines, "Show-prep: disabled by config")
	}
	return strings.Join(lines, "\n"), failed, pending
}

func (s *FeedFreshnessScheduler) check(parent context.Context) {
	select {
	case <-parent.Done():
		return
	case <-s.Done():
		return
	default:
	}
	body, failed, pending := s.report(parent, time.Now)
	if pending && !failed {
		return // Silence cannot clear an alert while any feed is unjudged.
	}
	target := s.url
	if failed {
		target += "/fail"
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		s.logger.Warn(ctx, "feed freshness ping unavailable")
		return
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Warn(ctx, "feed freshness ping unavailable")
		return
	}
	defer func() { _ = resp.Body.Close() }() // The bounded acknowledgement read determines delivery success.
	ack, err := io.ReadAll(io.LimitReader(resp.Body, 32))
	if err != nil || len(ack) == 32 || resp.StatusCode != http.StatusOK || strings.TrimSpace(string(ack)) != "OK" {
		s.logger.Warn(ctx, "feed freshness ping not acknowledged")
	}
}
