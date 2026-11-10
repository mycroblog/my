// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// timelineRepoKeywordFilters checks the keyword filters: stored with their settings, contexts, and
// words in order, counted, listed oldest first, replaced, read by context and expiry for many
// accounts at once, listed by expiry for the cleanup, deleted, and kept by the rebuild of the
// projections.
func timelineRepoKeywordFilters(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	alice, bob, carol := r.local(), r.local(), r.local()
	now := r.clock.Now()
	expiry := now.Add(time.Hour)
	spoilers := timeline.KeywordFilter{
		ID: r.ids.NewID(), AccountID: alice.id, Title: "Spoilers", Action: timeline.FilterWarn,
		Contexts: []timeline.FilterContext{timeline.FilterContextHome, timeline.FilterContextPublic},
		Words: []timeline.FilterWord{
			{Word: "Spoiler", Normalized: "spoiler", WholeWord: true},
			{Word: "ネタバレ", Normalized: "ねたばれ"},
		},
		CreatedAt: now,
	}
	ads := timeline.KeywordFilter{
		ID: r.ids.NewID(), AccountID: alice.id, Title: "Ads", Action: timeline.FilterHide,
		Contexts:  []timeline.FilterContext{timeline.FilterContextNotifications},
		Words:     []timeline.FilterWord{{Word: "sale", Normalized: "sale"}},
		ExpiresAt: expiry, CreatedAt: now,
	}
	bobs := timeline.KeywordFilter{
		ID: r.ids.NewID(), AccountID: bob.id, Title: "Bob", Action: timeline.FilterHide,
		Contexts:  []timeline.FilterContext{timeline.FilterContextHome},
		Words:     []timeline.FilterWord{{Word: "x", Normalized: "x"}, {Word: "a", Normalized: "a"}},
		CreatedAt: now,
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, f := range []timeline.KeywordFilter{spoilers, ads, bobs} {
			if err := r.repo.InsertKeywordFilter(ctx, tx, f); err != nil {
				return err
			}
		}
		if filters, words, err := r.repo.CountKeywordFilters(ctx, tx, alice.id); err != nil || filters != 2 || words != 3 {
			t.Errorf("CountKeywordFilters(alice) = %d, %d, %v; want 2, 3", filters, words, err)
		}
		if filters, words, err := r.repo.CountKeywordFilters(ctx, tx, carol.id); err != nil || filters != 0 || words != 0 {
			t.Errorf("CountKeywordFilters(carol) = %d, %d, %v; want 0, 0", filters, words, err)
		}
		got, err := r.repo.FindKeywordFilter(ctx, tx, ads.ID)
		if err != nil || !reflect.DeepEqual(got, ads) {
			t.Errorf("FindKeywordFilter = %+v, %v; want %+v", got, err, ads)
		}
		if _, err := r.repo.FindKeywordFilter(ctx, tx, r.ids.NewID()); !errors.Is(err, apptimeline.ErrNotFound) {
			t.Errorf("FindKeywordFilter(unknown) = %v, want ErrNotFound", err)
		}
		return nil
	})
	sorted := func(fs ...timeline.KeywordFilter) []timeline.KeywordFilter {
		return slices.SortedFunc(slices.Values(fs), func(a, b timeline.KeywordFilter) int {
			if a.ID < b.ID {
				return -1
			}
			return 1
		})
	}
	if got, err := r.repo.KeywordFiltersOf(ctx, alice.id); err != nil || !reflect.DeepEqual(got, sorted(spoilers, ads)) {
		t.Errorf("KeywordFiltersOf(alice) = %+v, %v; want %+v", got, err, sorted(spoilers, ads))
	}
	if got, err := r.repo.KeywordFiltersOf(ctx, carol.id); err != nil || len(got) != 0 {
		t.Errorf("KeywordFiltersOf(carol) = %+v, %v; want none", got, err)
	}

	// The active filters of a context, for several accounts, with only what matching needs.
	matching := func(f timeline.KeywordFilter) timeline.KeywordFilter {
		return timeline.KeywordFilter{ID: f.ID, AccountID: f.AccountID, Title: f.Title, Action: f.Action, Words: f.Words}
	}
	active, err := r.repo.ActiveKeywordFilters(ctx, []string{alice.id, bob.id, carol.id}, timeline.FilterContextHome, now)
	want := map[string][]timeline.KeywordFilter{alice.id: {matching(spoilers)}, bob.id: {matching(bobs)}}
	if err != nil || !reflect.DeepEqual(active, want) {
		t.Errorf("ActiveKeywordFilters(home) = %+v, %v; want %+v", active, err, want)
	}
	active, err = r.repo.ActiveKeywordFilters(ctx, []string{alice.id}, timeline.FilterContextNotifications, now)
	if want := map[string][]timeline.KeywordFilter{alice.id: {matching(ads)}}; err != nil || !reflect.DeepEqual(active, want) {
		t.Errorf("ActiveKeywordFilters(notifications) = %+v, %v; want %+v", active, err, want)
	}
	// At its expiry a filter stops applying.
	active, err = r.repo.ActiveKeywordFilters(ctx, []string{alice.id}, timeline.FilterContextNotifications, expiry)
	if err != nil || len(active) != 0 {
		t.Errorf("ActiveKeywordFilters(at the expiry) = %+v, %v; want none", active, err)
	}
	if active, err := r.repo.ActiveKeywordFilters(ctx, nil, timeline.FilterContextHome, now); err != nil || len(active) != 0 {
		t.Errorf("ActiveKeywordFilters(no accounts) = %+v, %v; want none", active, err)
	}

	// Replacing the settings, contexts, and words; the expiry goes.
	replaced := ads
	replaced.Title, replaced.Action, replaced.ExpiresAt = "Shops", timeline.FilterWarn, time.Time{}
	replaced.Contexts = []timeline.FilterContext{timeline.FilterContextAccount, timeline.FilterContextThread}
	replaced.Words = []timeline.FilterWord{{Word: "shop", Normalized: "shop"}, {Word: "Sale", Normalized: "sale", WholeWord: true}}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.ReplaceKeywordFilter(ctx, tx, replaced); err != nil {
			return err
		}
		got, err := r.repo.FindKeywordFilter(ctx, tx, ads.ID)
		if err != nil || !reflect.DeepEqual(got, replaced) {
			t.Errorf("FindKeywordFilter(replaced) = %+v, %v; want %+v", got, err, replaced)
		}
		return nil
	})

	// The cleanup lists the filters expired before a time, the earliest first.
	old := spoilers
	old.ExpiresAt = now.Add(-2 * time.Hour)
	older := bobs
	older.ExpiresAt = now.Add(-3 * time.Hour)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, f := range []timeline.KeywordFilter{old, older} {
			if err := r.repo.ReplaceKeywordFilter(ctx, tx, f); err != nil {
				return err
			}
		}
		ids, err := r.repo.KeywordFiltersExpiredBefore(ctx, tx, now.Add(-time.Hour), 10)
		if want := []string{older.ID, old.ID}; err != nil || !slices.Equal(ids, want) {
			t.Errorf("KeywordFiltersExpiredBefore = %v, %v; want %v", ids, err, want)
		}
		ids, err = r.repo.KeywordFiltersExpiredBefore(ctx, tx, now.Add(-time.Hour), 1)
		if want := []string{older.ID}; err != nil || !slices.Equal(ids, want) {
			t.Errorf("KeywordFiltersExpiredBefore(1) = %v, %v; want %v", ids, err, want)
		}
		return nil
	})

	// A rebuild keeps them: they record what the accounts did.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.DeleteProjection(ctx, tx) })
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.DeleteKeywordFilter(ctx, tx, old.ID); err != nil {
			return err
		}
		return r.repo.DeleteKeywordFilter(ctx, tx, r.ids.NewID())
	})
	if got, err := r.repo.KeywordFiltersOf(ctx, alice.id); err != nil || !reflect.DeepEqual(got, []timeline.KeywordFilter{replaced}) {
		t.Errorf("KeywordFiltersOf(alice) after the delete = %+v, %v; want %+v", got, err, replaced)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if filters, words, err := r.repo.CountKeywordFilters(ctx, tx, alice.id); err != nil || filters != 1 || words != 2 {
			t.Errorf("CountKeywordFilters(alice) after the delete = %d, %d, %v; want 1, 2", filters, words, err)
		}
		return nil
	})
}
