// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/mycroblog/my/internal/app/kernel"
)

// healthCacheTTL is how long PathInstanceHealth answers with the result of one check. It is
// fixed, not a setting (ADR-0085 §1).
const healthCacheTTL = 60 * time.Second

// readyCheck runs the readiness check (App.Ready) once for the concurrent calls (ADR-0085 §2),
// within readinessTimeout. The check runs without the caller's cancellation, since it serves
// every caller that joined it. It logs and counts a failed check (ADR-0056 §3), once per check
// rather than per caller.
func (a *App) readyCheck(ctx context.Context) error {
	_, err, _ := a.readyFlight.Do("ready", func() (any, error) {
		ctx = context.WithoutCancel(ctx)
		checkCtx, cancel := context.WithTimeout(ctx, readinessTimeout)
		defer cancel()
		err := a.Ready(checkCtx)
		if err != nil {
			a.Logger.WarnContext(ctx, "not ready", slog.Any("error", err))
			a.readinessFailures.Add(ctx, 1, kernel.Labels{})
		}
		return nil, err
	})
	return err
}

// healthCache keeps the result of a check for healthCacheTTL, and runs one check for the
// concurrent calls that find no fresh result. It implements clientapi.HealthCheck.
type healthCache struct {
	check func(ctx context.Context) error
	clock kernel.Clock

	flight singleflight.Group
	mu     sync.Mutex
	// checkedAt is when the last check ran, zero before the first; healthy is its result.
	checkedAt time.Time
	healthy   bool
}

func newHealthCache(check func(ctx context.Context) error, clock kernel.Clock) *healthCache {
	return &healthCache{check: check, clock: clock}
}

// Check returns the result of the last check while it is fresh, and checks again otherwise.
func (c *healthCache) Check(ctx context.Context) (healthy bool, checkedAt time.Time) {
	if healthy, checkedAt, ok := c.fresh(); ok {
		return healthy, checkedAt
	}
	v, _, _ := c.flight.Do("health", func() (any, error) {
		// A call that found the result stale may come after another flight refreshed it.
		if healthy, checkedAt, ok := c.fresh(); ok {
			return healthResult{healthy, checkedAt}, nil
		}
		at := c.clock.Now()
		ok := c.check(ctx) == nil
		c.mu.Lock()
		c.healthy, c.checkedAt = ok, at
		c.mu.Unlock()
		return healthResult{ok, at}, nil
	})
	r := v.(healthResult)
	return r.healthy, r.checkedAt
}

type healthResult struct {
	healthy   bool
	checkedAt time.Time
}

func (c *healthCache) fresh() (healthy bool, checkedAt time.Time, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.checkedAt.IsZero() || c.clock.Now().Sub(c.checkedAt) >= healthCacheTTL {
		return false, time.Time{}, false
	}
	return c.healthy, c.checkedAt, true
}
