// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"context"
	"reflect"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

// timelineRepoFeaturedTags checks the featured hashtags: replaced in the account's order, read in
// and out of a transaction, emptied, kept by the rebuild of the projections, and counted over the
// account's public and unlisted posts of each tag alone.
func timelineRepoFeaturedTags(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	alice, bob := r.local(), r.remote()
	cats, dogs, birds := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	at := r.clock.Now()
	replace := func(account timelineAuthor, tags ...string) []apptimeline.FeaturedTagRow {
		rows := make([]apptimeline.FeaturedTagRow, len(tags))
		for i, tag := range tags {
			rows[i] = apptimeline.FeaturedTagRow{AccountID: account.id, TagID: tag, Ordinal: i + 1, FeaturedAt: at}
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.repo.ReplaceFeaturedTags(ctx, tx, account.id, rows)
		})
		return rows
	}
	replace(alice, cats, dogs)
	want := replace(alice, dogs, birds, cats) // a new order, and a tag more
	wantBob := replace(bob, cats)
	if got, err := r.repo.FeaturedTagsOf(ctx, alice.id); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("FeaturedTagsOf = %+v, %v; want %+v", got, err, want)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.repo.FeaturedTagsTx(ctx, tx, alice.id); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("FeaturedTagsTx = %+v, %v; want %+v", got, err, want)
		}
		return nil
	})

	// The usage counts alice's public and unlisted posts of each featured tag.
	r.post(t, alice, appcontent.VisibilityPublic, entryOpts{tags: []string{cats, dogs}})
	last := r.post(t, alice, appcontent.VisibilityUnlisted, entryOpts{tags: []string{cats}})
	lastAt := r.clock.Now()
	for _, v := range []appcontent.Visibility{appcontent.VisibilityLocal, appcontent.VisibilityPrivate, appcontent.VisibilityDirect} {
		r.post(t, alice, v, entryOpts{tags: []string{cats, birds}})
	}
	r.post(t, bob, appcontent.VisibilityPublic, entryOpts{tags: []string{cats, birds}})
	r.post(t, alice, appcontent.VisibilityPublic, entryOpts{tags: []string{r.ids.NewID()}})
	usage, err := r.repo.FeaturedTagUsageOf(ctx, alice.id)
	wantUsage := map[string]apptimeline.FeaturedTagUsage{
		cats: {TagID: cats, StatusesCount: 2, LastStatusAt: lastAt},
		dogs: {TagID: dogs, StatusesCount: 1, LastStatusAt: lastAt.Add(-time.Second)},
	}
	if err != nil || !reflect.DeepEqual(usage, wantUsage) {
		t.Errorf("FeaturedTagUsageOf(alice) = %+v, %v; want %+v (last post %s)", usage, err, wantUsage, last.ID)
	}
	if usage, err := r.repo.FeaturedTagUsageOf(ctx, bob.id); err != nil || len(usage) != 1 || usage[cats].StatusesCount != 1 {
		t.Errorf("FeaturedTagUsageOf(bob) = %+v, %v; want cats once", usage, err)
	}

	// A rebuild keeps them: they record what the accounts chose.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.DeleteProjection(ctx, tx) })
	all, err := r.repo.AllFeaturedTags(ctx)
	wantAll := append(append([]apptimeline.FeaturedTagRow{}, want...), wantBob...)
	if alice.id > bob.id {
		wantAll = append(append([]apptimeline.FeaturedTagRow{}, wantBob...), want...)
	}
	if err != nil || !reflect.DeepEqual(all, wantAll) {
		t.Errorf("AllFeaturedTags = %+v, %v; want %+v", all, err, wantAll)
	}

	replace(alice)
	if got, err := r.repo.FeaturedTagsOf(ctx, alice.id); err != nil || len(got) != 0 {
		t.Errorf("FeaturedTagsOf(emptied) = %+v, %v; want none", got, err)
	}
	if usage, err := r.repo.FeaturedTagUsageOf(ctx, alice.id); err != nil || len(usage) != 0 {
		t.Errorf("FeaturedTagUsageOf(emptied) = %+v, %v; want none", usage, err)
	}
}
