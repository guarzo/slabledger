package scheduler

import (
	"context"
	"sync"
)

// Scheduler is a common interface for background schedulers that can be started and stopped.
type Scheduler interface {
	Start(ctx context.Context)
	Stop()
}

// Group manages a collection of schedulers, starting and stopping them together.
type Group struct {
	schedulers []Scheduler
	wg         sync.WaitGroup
}

// NewGroup creates a new Group from the given schedulers.
func NewGroup(schedulers ...Scheduler) *Group {
	return &Group{schedulers: schedulers}
}

// Add registers an additional scheduler with the group. Must be called before
// StartAll so the new scheduler participates in Start/Stop/Wait.
func (g *Group) Add(s Scheduler) {
	g.schedulers = append(g.schedulers, s)
}

// StartAll starts each scheduler in its own goroutine.
func (g *Group) StartAll(ctx context.Context) {
	for _, s := range g.schedulers {
		g.wg.Add(1)
		go func(sched Scheduler) {
			defer g.wg.Done()
			sched.Start(ctx)
		}(s)
	}
}

// StopAll stops all schedulers in the group.
func (g *Group) StopAll() {
	for _, s := range g.schedulers {
		s.Stop()
	}
}

// Wait blocks until all scheduler goroutines have returned.
// Call after StopAll to ensure clean shutdown.
func (g *Group) Wait() {
	g.wg.Wait()
}
