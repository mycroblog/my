// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The overview of the instance that the landing page shows (GET /instance/overview): how many
// members it has, how many public posts they made lately, since when it runs, and who runs it.
// Only local, public data: the members are counted, never listed, and the operators are the
// accounts that chose to show the badge of a staff role. The parts are read through other
// modules and kept for OverviewTTL, so that the landing page, which every visitor opens, adds
// no database work per request (ADR-0003).

// OverviewDays is the number of days of 24 hours that Counts.DailyPosts covers.
const OverviewDays = 7

// OverviewTTL is how long Overview keeps what it read before reading it again.
const OverviewTTL = 5 * time.Minute

// Counts are the counts of the overview, as taken at CountedAt.
type Counts struct {
	// Members is the number of local accounts that are not deleted.
	Members int
	// DailyPosts holds the public posts of local accounts in each of the OverviewDays periods
	// of 24 hours before CountedAt, oldest first.
	DailyPosts []int
	// StartedAt is when the instance was set up.
	StartedAt time.Time
	CountedAt time.Time
}

// clone returns c with a slice of its own.
func (c Counts) clone() Counts {
	c.DailyPosts = slices.Clone(c.DailyPosts)
	return c
}

// OverviewSources reads the parts of the overview from the other modules: the account module
// counts the members and knows when the instance actor was made, the content module counts the
// posts, and the identity and account modules name the operators. internal/platform implements
// it.
type OverviewSources interface {
	// CountMembers returns the number of local accounts that are not deleted.
	CountMembers(ctx context.Context) (int, error)
	// StartedAt returns when the instance was set up: when its instance actor was made.
	StartedAt(ctx context.Context) (time.Time, error)
	// DailyPublicPosts counts the public posts of local accounts that are not deleted in each
	// of the days periods of 24 hours before to, oldest first.
	DailyPublicPosts(ctx context.Context, to time.Time, days int) ([]int, error)
	// OperatorAccountIDs returns the local accounts that show the badge of a staff role on
	// their profile, highest role first.
	OperatorAccountIDs(ctx context.Context) ([]string, error)
}

// Overview reads the parts of the overview and keeps each for OverviewTTL. It is safe for
// concurrent use; a part that fails to read is read again on the next call.
type Overview struct {
	src   OverviewSources
	clock kernel.Clock

	mu          sync.Mutex
	counts      cached[Counts]
	operatorIDs cached[[]string]
}

type cached[T any] struct {
	value T
	at    time.Time
	ok    bool
}

func (c *cached[T]) fresh(now time.Time) bool { return c.ok && now.Sub(c.at) < OverviewTTL }

// NewOverview returns an Overview that reads from src.
func NewOverview(src OverviewSources, clock kernel.Clock) *Overview {
	return &Overview{src: src, clock: clock}
}

// Counts returns the counts of the overview, read at most OverviewTTL ago.
func (o *Overview) Counts(ctx context.Context) (Counts, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	now := o.clock.Now()
	if o.counts.fresh(now) {
		return o.counts.value.clone(), nil
	}
	members, err := o.src.CountMembers(ctx)
	if err != nil {
		return Counts{}, err
	}
	started, err := o.src.StartedAt(ctx)
	if err != nil {
		return Counts{}, err
	}
	daily, err := o.src.DailyPublicPosts(ctx, now, OverviewDays)
	if err != nil {
		return Counts{}, err
	}
	o.counts = cached[Counts]{value: Counts{Members: members, DailyPosts: daily, StartedAt: started, CountedAt: now}, at: now, ok: true}
	return o.counts.value.clone(), nil
}

// OperatorAccountIDs returns the accounts of the operators, read at most OverviewTTL ago.
func (o *Overview) OperatorAccountIDs(ctx context.Context) ([]string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	now := o.clock.Now()
	if o.operatorIDs.fresh(now) {
		return slices.Clone(o.operatorIDs.value), nil
	}
	ids, err := o.src.OperatorAccountIDs(ctx)
	if err != nil {
		return nil, err
	}
	o.operatorIDs = cached[[]string]{value: ids, at: now, ok: true}
	return slices.Clone(ids), nil
}
