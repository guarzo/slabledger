package scheduler

import "sync"

// StopHandle encapsulates the stop channel, once guard, and optional WaitGroup
// that every scheduler needs. Embed it to get Stop() and Wait() for free.
type StopHandle struct {
	c    chan struct{}
	once sync.Once
	wg   sync.WaitGroup
}

// NewStopHandle creates a StopHandle with an initialized channel.
func NewStopHandle() StopHandle {
	return StopHandle{c: make(chan struct{})}
}

// Done returns the receive-only stop channel.
func (h *StopHandle) Done() <-chan struct{} {
	return h.c
}

// Stop closes the stop channel. Safe to call multiple times.
func (h *StopHandle) Stop() {
	h.once.Do(func() { close(h.c) })
}

// Wait blocks until all WG-tracked goroutines have finished.
func (h *StopHandle) Wait() {
	h.wg.Wait()
}

// WG returns a pointer to the embedded WaitGroup for use with RunLoop.
func (h *StopHandle) WG() *sync.WaitGroup {
	return &h.wg
}
