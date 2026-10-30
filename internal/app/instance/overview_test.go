// SPDX-License-Identifier: AGPL-3.0-only

package instance_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/instance"
)

// fakeClock is a clock that moves only by Advance.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

// fakeOverviewSources answers fixed parts and counts the reads of each.
type fakeOverviewSources struct {
	members        int
	daily          []int
	err            error
	countReads     int
	dailyTo        time.Time
	dailyDays      int
	startedAtValue time.Time
}

func (f *fakeOverviewSources) CountMembers(context.Context) (int, error) {
	f.countReads++
	return f.members, f.err
}

func (f *fakeOverviewSources) StartedAt(context.Context) (time.Time, error) {
	return f.startedAtValue, nil
}

func (f *fakeOverviewSources) DailyPublicPosts(_ context.Context, to time.Time, days int) ([]int, error) {
	f.dailyTo, f.dailyDays = to, days
	return f.daily, nil
}

func TestOverviewKeepsWhatItReadForTheTTL(t *testing.T) {
	ctx := t.Context()
	clock := &fakeClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	started := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	src := &fakeOverviewSources{members: 3, daily: []int{1, 0, 0, 2, 0, 0, 5}, startedAtValue: started}
	o := instance.NewOverview(src, clock)

	got, err := o.Counts(ctx)
	want := instance.Counts{Members: 3, DailyPosts: []int{1, 0, 0, 2, 0, 0, 5}, StartedAt: started, CountedAt: clock.Now()}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Counts = %+v, %v; want %+v", got, err, want)
	}
	if !src.dailyTo.Equal(clock.Now()) || src.dailyDays != instance.OverviewDays {
		t.Errorf("DailyPublicPosts(%v, %d), want (%v, %d)", src.dailyTo, src.dailyDays, clock.Now(), instance.OverviewDays)
	}
	// A caller that changes the answer does not change what is kept.
	got.DailyPosts[0] = 99
	src.members = 4
	clock.Advance(instance.OverviewTTL - time.Second)
	if again, _ := o.Counts(ctx); !reflect.DeepEqual(again, want) || src.countReads != 1 {
		t.Errorf("Counts within the TTL = %+v after %d reads, want %+v from 1 read", again, src.countReads, want)
	}
	clock.Advance(time.Second)
	if again, _ := o.Counts(ctx); again.Members != 4 || !again.CountedAt.Equal(clock.Now()) || src.countReads != 2 {
		t.Errorf("Counts after the TTL = %+v after %d reads, want 4 members counted now", again, src.countReads)
	}

}

func TestOverviewReadsAgainAfterAFailure(t *testing.T) {
	ctx := t.Context()
	clock := &fakeClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	boom := errors.New("boom")
	src := &fakeOverviewSources{err: boom}
	o := instance.NewOverview(src, clock)
	if _, err := o.Counts(ctx); !errors.Is(err, boom) {
		t.Fatalf("Counts = %v, want boom", err)
	}
	src.err, src.members = nil, 2
	if got, err := o.Counts(ctx); err != nil || got.Members != 2 {
		t.Errorf("Counts after a failure = %+v, %v; want 2 members", got, err)
	}
}
