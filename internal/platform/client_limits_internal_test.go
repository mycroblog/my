// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

type limitsClock struct{ now time.Time }

func (c *limitsClock) Now() time.Time { return c.now }

// TestClientLimitsFollowTheSetting checks that a changed cap applies without a restart, that the
// setting is read at most once a minute however many requests come, and that a failed read keeps
// the caps it has.
func TestClientLimitsFollowTheSetting(t *testing.T) {
	clock := &limitsClock{now: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)}
	caps, reads := map[string]int64{"a": 5}, 0
	var failure error
	limit := clientLimits(func(context.Context) (map[string]int64, error) {
		reads++
		return caps, failure
	}, clock, slog.New(slog.DiscardHandler))
	ctx := t.Context()

	if lim, ok := limit(ctx, "a"); !ok || lim.Max != 5 || lim.Window != 5*time.Minute {
		t.Errorf("limit(a) = %+v, %v; want 5 in 5 minutes", lim, ok)
	}
	if _, ok := limit(ctx, "b"); ok || reads != 1 {
		t.Errorf("limit(b) capped = %v after %d reads; want no cap after 1 read", ok, reads)
	}

	caps = map[string]int64{"a": 7}
	clock.now = clock.now.Add(clientLimitsTTL)
	if lim, ok := limit(ctx, "a"); !ok || lim.Max != 7 || reads != 2 {
		t.Errorf("limit(a) after the change = %+v, %v after %d reads; want 7 after 2 reads", lim, ok, reads)
	}

	caps, failure = nil, errors.New("database down")
	clock.now = clock.now.Add(clientLimitsTTL)
	if lim, ok := limit(ctx, "a"); !ok || lim.Max != 7 {
		t.Errorf("limit(a) after a failed read = %+v, %v; want the 7 it had", lim, ok)
	}
}
