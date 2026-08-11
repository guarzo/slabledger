package mocks

import "sync"

// PricingEnqueuerMock is a mock inventory.PricingEnqueuer following the
// Fn-field pattern: override EnqueueFn to change behaviour, otherwise calls
// are recorded and can be read back with Certs.
//
// Deliberately not asserted against inventory.PricingEnqueuer with a
// `var _` line here: this package is imported by domain tests, and the
// interface is declared in internal/domain/inventory, so the compile-time
// check lands at each use site instead (the field it is assigned to is
// already typed inventory.PricingEnqueuer).
type PricingEnqueuerMock struct {
	EnqueueFn func(certNumber string)

	mu    sync.Mutex
	certs []string
}

func (m *PricingEnqueuerMock) Enqueue(certNumber string) {
	if m.EnqueueFn != nil {
		m.EnqueueFn(certNumber)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.certs = append(m.certs, certNumber)
}

// Certs returns a copy of the certs recorded so far, in call order.
func (m *PricingEnqueuerMock) Certs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.certs))
	copy(out, m.certs)
	return out
}
