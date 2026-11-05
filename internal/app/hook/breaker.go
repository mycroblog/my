// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"sync"
	"time"
)

const (
	// BreakerThreshold is how many calls in a row fail before a hook's breaker opens.
	BreakerThreshold = 5
	// BreakerOpenFor is how long an open breaker skips the plugin before it lets one call test it.
	BreakerOpenFor = 30 * time.Second
)

// breaker skips a plugin after repeated failures, so that an outage costs each subject no wait. It
// lives in the process: each process finds out about an outage by itself, which costs at most
// BreakerThreshold slow calls.
type breaker struct {
	mu       sync.Mutex
	failures int
	open     bool
	until    time.Time
	probing  bool
}

// allow reports whether a call may go to the plugin. After BreakerOpenFor, one call does.
func (b *breaker) allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.open {
		return true
	}
	if b.probing || now.Before(b.until) {
		return false
	}
	b.probing = true
	return true
}

// succeeded closes the breaker.
func (b *breaker) succeeded() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures, b.open, b.probing = 0, false, false
}

// abandoned ends a call that says nothing about the plugin: its caller went away.
func (b *breaker) abandoned() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probing = false
}

// failed counts a failure and reports whether this one opened the breaker. A failed test call
// keeps it open for another BreakerOpenFor.
func (b *breaker) failed(now time.Time) (opened bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.open {
		b.probing, b.until = false, now.Add(BreakerOpenFor)
		return false
	}
	if b.failures < BreakerThreshold {
		return false
	}
	b.open, b.until = true, now.Add(BreakerOpenFor)
	return true
}
