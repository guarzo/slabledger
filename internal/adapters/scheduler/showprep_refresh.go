package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

// ShowPrepConfiguration synchronizes stored credentials without contacting a provider.
// Configured is read-only: status reads must never resolve a source or wake work.
type ShowPrepConfiguration interface {
	Refresh(context.Context) error
	Configured(context.Context) (bool, error)
}

type ShowPrepWorkerStatus struct {
	sp.WorkerStatus
	Enabled    bool `json:"enabled"`
	Configured bool `json:"configured"`
}

// ShowPrepRefreshScheduler owns the only local execution path into RunOnce.
// Requests record durable intent and send a bounded hint; startup/ticks recover
// missed hints. The domain worker owns distributed leasing and acquisition.
type ShowPrepRefreshScheduler struct {
	StopHandle
	worker        *sp.EvidenceWorker
	configuration ShowPrepConfiguration
	logger        observability.Logger
	enabled       bool
	interval      time.Duration
	wake          chan struct{}
	mu            sync.Mutex
	cancel        context.CancelFunc
	joined        chan struct{}
}

func NewShowPrepRefreshScheduler(worker *sp.EvidenceWorker, configuration ShowPrepConfiguration, logger observability.Logger, enabled bool) *ShowPrepRefreshScheduler {
	return &ShowPrepRefreshScheduler{StopHandle: NewStopHandle(), worker: worker, configuration: configuration, logger: logger, enabled: enabled, interval: time.Minute, wake: make(chan struct{}, 1)}
}

func (s *ShowPrepRefreshScheduler) Start(parent context.Context) {
	s.mu.Lock()
	if s.joined != nil || !s.enabled {
		s.mu.Unlock()
		return
	}
	select {
	case <-s.Done():
		s.mu.Unlock()
		return
	default:
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.joined = make(chan struct{})
	joined := s.joined
	s.mu.Unlock()
	defer close(joined)
	defer cancel()
	RunLoop(ctx, LoopConfig{Name: "show-prep-evidence", Interval: s.interval, StopChan: s.Done(), Wake: s.wake, Logger: s.logger}, func(ctx context.Context) {
		if ctx.Err() != nil {
			return
		}
		sweep, cancelSweep := context.WithTimeout(ctx, 5*time.Minute)
		defer cancelSweep()
		// Check the actual saved configuration even when a durable auth hold means
		// RunOnce will not resolve a source. This also updates legacy CL consumers.
		if s.configuration != nil {
			check, cancelCheck := context.WithTimeout(sweep, 5*time.Second)
			err := s.configuration.Refresh(check)
			cancelCheck()
			if err != nil {
				s.logger.Warn(ctx, "show evidence credentials unavailable")
			}
		}
		if err := s.worker.RunOnce(sweep); err != nil && ctx.Err() == nil {
			s.logger.Warn(ctx, "show evidence sweep incomplete")
		}
	})
}

// Stop cancels active source work and drains the loop, not just the next tick.
func (s *ShowPrepRefreshScheduler) Stop() {
	s.StopHandle.Stop()
	s.mu.Lock()
	cancel, joined := s.cancel, s.joined
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if joined != nil {
		<-joined
	}
}

func (s *ShowPrepRefreshScheduler) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *ShowPrepRefreshScheduler) RequestRun(ctx context.Context, retry bool) error {
	if err := s.worker.RequestRun(ctx, retry); err != nil {
		return err
	}
	s.Wake()
	return nil
}
func (s *ShowPrepRefreshScheduler) CredentialsChanged(ctx context.Context) error {
	if err := s.worker.CredentialsChanged(ctx); err != nil {
		return err
	}
	s.Wake()
	return nil
}
func (s *ShowPrepRefreshScheduler) Status(ctx context.Context) (ShowPrepWorkerStatus, error) {
	status, err := s.worker.Status(ctx)
	if err != nil {
		return ShowPrepWorkerStatus{}, err
	}
	result := ShowPrepWorkerStatus{WorkerStatus: status, Enabled: s.enabled}
	if s.configuration != nil {
		result.Configured, err = s.configuration.Configured(ctx)
		if err != nil {
			return ShowPrepWorkerStatus{}, err
		}
	}
	if !s.enabled {
		result.State = "disabled"
	} else if !result.Configured {
		result.State = "unconfigured"
	}
	return result, nil
}
