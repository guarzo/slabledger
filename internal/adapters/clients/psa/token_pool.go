package psa

import (
	"sync"
	"time"
)

// tokenDayKey uniquely identifies a token + UTC day combination.
type tokenDayKey struct {
	token string
	day   string // UTC date string "2006-01-02"
}

// tokenPool holds the PSA API tokens and tracks which are currently usable.
//
// PSA distinguishes two failure modes that look alike from the outside but
// need opposite handling, and conflating them caused an outage (SLA-108):
//
//   - HTTP 403 "Access to this API is limited to approved customers" — the key
//     was issued but never approved. It will never work, so it is marked dead
//     for the lifetime of the process and skipped from then on. Retrying it
//     burns wall-clock and trips the circuit breaker for no possible gain.
//   - HTTP 429 "API calls quota exceeded! maximum admitted 100 per Day" — the
//     key is healthy but spent. It is marked spent for the current UTC day and
//     becomes usable again at rollover.
//
// Only an approved key can receive the 429; an unapproved one is rejected
// before quota is ever consulted. So a 429 is evidence a key is good.
//
// Quota is tracked by PSA's own 429 rather than a local counter. An in-memory
// counter cannot survive a restart — the process zeroes it while PSA keeps
// counting — which is exactly how a key sailed past its limit during the
// incident. One wasted call per token per day is the price of not guessing.
type tokenPool struct {
	mu     sync.Mutex
	tokens []string
	idx    int
	dead   map[string]bool      // 403 — unapproved, dead for the process lifetime
	spent  map[tokenDayKey]bool // 429 — quota exhausted for that UTC day
}

func newTokenPool(tokens []string) *tokenPool {
	return &tokenPool{
		tokens: tokens,
		dead:   make(map[string]bool),
		spent:  make(map[tokenDayKey]bool),
	}
}

func utcDay() string {
	return time.Now().UTC().Format("2006-01-02")
}

// usableLocked reports whether the token at i can be tried right now.
// Caller must hold mu.
func (p *tokenPool) usableLocked(i int, day string) bool {
	tok := p.tokens[i]
	return !p.dead[tok] && !p.spent[tokenDayKey{token: tok, day: day}]
}

// next returns the current usable token, advancing past any that are dead or
// spent. Returns ok=false when every token is unusable.
func (p *tokenPool) next() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.tokens) == 0 {
		return "", false
	}
	p.pruneSpentLocked()
	day := utcDay()
	for range p.tokens {
		i := p.idx % len(p.tokens)
		if p.usableLocked(i, day) {
			return p.tokens[i], true
		}
		p.idx = (p.idx + 1) % len(p.tokens)
	}
	return "", false
}

// advance moves past the current token so the next call to next() tries a
// different one. Returns false when no other usable token remains.
func (p *tokenPool) advance() bool {
	p.mu.Lock()
	if len(p.tokens) == 0 {
		p.mu.Unlock()
		return false
	}
	p.idx = (p.idx + 1) % len(p.tokens)
	p.mu.Unlock()
	_, ok := p.next()
	return ok
}

// markDead records that a token is permanently rejected by PSA (403).
func (p *tokenPool) markDead(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dead[token] = true
}

// markSpent records that a token has exhausted its quota for the current UTC
// day (429). It becomes usable again at the next UTC rollover.
func (p *tokenPool) markSpent(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.spent[tokenDayKey{token: token, day: utcDay()}] = true
}

// pruneSpentLocked drops spent markers from previous days so the map does not
// grow without bound. Caller must hold mu.
func (p *tokenPool) pruneSpentLocked() {
	day := utcDay()
	for k := range p.spent {
		if k.day != day {
			delete(p.spent, k)
		}
	}
}

// usableCount returns how many tokens can be tried right now. Used to bound
// the rotation loop so it visits each candidate at most once.
func (p *tokenPool) usableCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pruneSpentLocked()
	day := utcDay()
	n := 0
	for i := range p.tokens {
		if p.usableLocked(i, day) {
			n++
		}
	}
	return n
}

// stats reports pool health for logging: total tokens, how many are dead
// (unapproved) and how many are spent for today.
func (p *tokenPool) stats() (total, dead, spent int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	day := utcDay()
	for _, tok := range p.tokens {
		switch {
		case p.dead[tok]:
			dead++
		case p.spent[tokenDayKey{token: tok, day: day}]:
			spent++
		}
	}
	return len(p.tokens), dead, spent
}
