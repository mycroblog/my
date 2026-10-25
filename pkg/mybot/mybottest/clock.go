// SPDX-License-Identifier: MIT

package mybottest

import (
	"context"
	"sync"
	"time"
)

// Clock is a mybot.Clock that moves only when the test moves it, for the scheduled jobs (Every), the send budget,
// and the reply limits. Server.Config sets the server's Clock as Config.Clock.
type Clock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []waiter
	changed chan struct{}
}

type waiter struct {
	until time.Time
	ch    chan struct{}
}

// NewClock returns a Clock at the time at.
func NewClock(at time.Time) *Clock {
	return &Clock{now: at, changed: make(chan struct{})}
}

// Now is the time of the clock.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Sleep waits until the clock has moved on by d, or returns the error of ctx when it ends first. It returns at
// once for d <= 0.
func (c *Clock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	c.mu.Lock()
	w := waiter{until: c.now.Add(d), ch: make(chan struct{})}
	c.waiters = append(c.waiters, w)
	c.notifyLocked()
	c.mu.Unlock()
	select {
	case <-w.ch:
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		defer c.mu.Unlock()
		for i, o := range c.waiters {
			if o.ch == w.ch {
				c.waiters = append(c.waiters[:i], c.waiters[i+1:]...)
				c.notifyLocked()
				break
			}
		}
		return ctx.Err()
	}
}

// Advance moves the clock on by d and wakes the sleepers whose time came.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setLocked(c.now.Add(d))
}

// Set moves the clock to t, which must not be before Now, and wakes the sleepers whose time came.
func (c *Clock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.Before(c.now) {
		panic("mybottest: Clock.Set to a time before Now")
	}
	c.setLocked(t)
}

func (c *Clock) setLocked(t time.Time) {
	c.now = t
	kept := c.waiters[:0]
	for _, w := range c.waiters {
		if w.until.After(t) {
			kept = append(kept, w)
		} else {
			close(w.ch)
		}
	}
	clear(c.waiters[len(kept):])
	c.waiters = kept
	c.notifyLocked()
}

// notifyLocked wakes WaitForSleepers.
func (c *Clock) notifyLocked() {
	close(c.changed)
	c.changed = make(chan struct{})
}

// Sleepers is how many goroutines sleep on the clock.
func (c *Clock) Sleepers() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}

// WaitForSleepers waits until at least n goroutines sleep on the clock, such as a bot's scheduled jobs waiting
// for their next time, so that a following Advance wakes them. It returns the error of ctx when it ends first.
func (c *Clock) WaitForSleepers(ctx context.Context, n int) error {
	for {
		c.mu.Lock()
		count, changed := len(c.waiters), c.changed
		c.mu.Unlock()
		if count >= n {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
