// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"sync"
	"time"

	"github.com/mycroblog/my/internal/domain/federation"
)

// CircuitBreaker is the short-term, in-process circuit breaker of inbox hosts (ADR-0006
// §3): after federation.BreakerThreshold consecutive retryable failures of a host, it sends
// nothing there for federation.BreakerCooldown. Each process has its own; the long-term state
// that every process shares is instance_health. The zero value is not usable; use
// NewCircuitBreaker. It is safe for concurrent use.
type CircuitBreaker struct {
	mu    sync.Mutex
	hosts map[string]federation.BreakerState
}

// NewCircuitBreaker returns a breaker with every host closed.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{hosts: map[string]federation.BreakerState{}}
}

// Allows reports whether a request to host may be sent at now.
func (b *CircuitBreaker) Allows(host string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hosts[host].Allows(now)
}

// Record updates host with the outcome of a request sent at now. A retryable failure counts; any
// answer from the server (a success or a permanent failure) closes the breaker and forgets the
// host.
func (b *CircuitBreaker) Record(host string, outcome federation.DeliveryOutcome, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch outcome {
	case federation.DeliveryRetryableFailure:
		b.hosts[host] = b.hosts[host].Failed(now)
	case federation.DeliverySucceeded, federation.DeliveryPermanentFailure:
		delete(b.hosts, host)
	}
}
