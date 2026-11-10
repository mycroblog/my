// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// The keyword filters (ADR-0157): the commands of their owner, their limits, the filters that
// apply in a context, and the cleanup of the long expired ones.

func filterInput(title string, action timeline.FilterAction, words ...string) apptimeline.KeywordFilterInput {
	in := apptimeline.KeywordFilterInput{Title: title, Action: action, Contexts: []timeline.FilterContext{timeline.FilterContextPublic, timeline.FilterContextHome}}
	for _, w := range words {
		in.Words = append(in.Words, apptimeline.KeywordInput{Word: w})
	}
	return in
}

func timelineKeywordFilterCommands(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	alice, bob := e.account(true), e.account(true)
	f, err := e.svc.CreateKeywordFilter(ctx, alice, filterInput("  Spoilers ", timeline.FilterWarn, " Spoiler", "ｓｐｏｉｌｅｒ", "ネタバレ"))
	if err != nil {
		t.Fatal(err)
	}
	// The title is trimmed, the contexts sorted, and a word normalized twice kept once.
	if f.Title != "Spoilers" || !slices.Equal(f.Contexts, []timeline.FilterContext{timeline.FilterContextHome, timeline.FilterContextPublic}) ||
		len(f.Words) != 2 || f.Words[0].Normalized != "spoiler" || f.Words[1].Normalized != "ねたばれ" || f.Expired {
		t.Errorf("CreateKeywordFilter = %+v", f)
	}
	for _, tc := range []struct {
		in   apptimeline.KeywordFilterInput
		want error
	}{
		{filterInput("", timeline.FilterWarn, "x"), timeline.ErrInvalidFilterTitle},
		{filterInput("x", "blur", "x"), timeline.ErrInvalidFilterAction},
		{apptimeline.KeywordFilterInput{Title: "x", Action: timeline.FilterWarn, Words: []apptimeline.KeywordInput{{Word: "x"}}}, timeline.ErrInvalidFilterContext},
		{filterInput("x", timeline.FilterWarn), timeline.ErrNoFilterWords},
		{filterInput("x", timeline.FilterWarn, "a\x00"), timeline.ErrInvalidFilterWord},
	} {
		if _, err := e.svc.CreateKeywordFilter(ctx, alice, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("CreateKeywordFilter(%+v) = %v, want %v", tc.in, err, tc.want)
		}
	}
	// Only the owner reads, changes, and deletes a filter.
	if _, err := e.svc.KeywordFilter(ctx, bob, f.ID); !errors.Is(err, apptimeline.ErrNotFound) {
		t.Errorf("KeywordFilter(bob) = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.UpdateKeywordFilter(ctx, bob, f.ID, filterInput("x", timeline.FilterWarn, "x")); !errors.Is(err, apptimeline.ErrNotFound) {
		t.Errorf("UpdateKeywordFilter(bob) = %v, want ErrNotFound", err)
	}
	if err := e.svc.DeleteKeywordFilter(ctx, bob, f.ID); !errors.Is(err, apptimeline.ErrNotFound) {
		t.Errorf("DeleteKeywordFilter(bob) = %v, want ErrNotFound", err)
	}
	// An expiry passed applies nowhere at once; the filter stays, expired.
	expiring := filterInput("Spoilers", timeline.FilterHide, "spoiler")
	expiring.ExpiresAt = e.Clock.Now().Add(time.Hour)
	if _, err := e.svc.UpdateKeywordFilter(ctx, alice, f.ID, expiring); err != nil {
		t.Fatal(err)
	}
	active, err := e.svc.StatusFiltersOf(ctx, []string{alice}, timeline.FilterContextHome)
	if err != nil || active[alice] == nil {
		t.Errorf("StatusFiltersOf before the expiry = %v, %v; want alice's", active, err)
	}
	e.Clock.Advance(time.Hour)
	if active, err := e.svc.StatusFiltersOf(ctx, []string{alice}, timeline.FilterContextHome); err != nil || len(active) != 0 {
		t.Errorf("StatusFiltersOf at the expiry = %v, %v; want none", active, err)
	}
	if got, err := e.svc.KeywordFilters(ctx, alice); err != nil || len(got) != 1 || !got[0].Expired {
		t.Errorf("KeywordFilters after the expiry = %+v, %v; want it expired", got, err)
	}
	// It is never deleted (ADR-0157 §6), and moving its expiry later applies it again.
	e.Clock.Advance(400 * 24 * time.Hour)
	if got, err := e.svc.KeywordFilters(ctx, alice); err != nil || len(got) != 1 || !got[0].Expired {
		t.Errorf("KeywordFilters long after the expiry = %+v, %v; want it kept, expired", got, err)
	}
	expiring.ExpiresAt = e.Clock.Now().Add(time.Hour)
	if got, err := e.svc.UpdateKeywordFilter(ctx, alice, f.ID, expiring); err != nil || got.Expired {
		t.Errorf("UpdateKeywordFilter(a later expiry) = %+v, %v; want it active", got, err)
	}
	if active, err := e.svc.StatusFiltersOf(ctx, []string{alice}, timeline.FilterContextHome); err != nil || active[alice] == nil {
		t.Errorf("StatusFiltersOf after the extension = %v, %v; want alice's", active, err)
	}
	if err := e.svc.DeleteKeywordFilter(ctx, alice, f.ID); err != nil {
		t.Fatal(err)
	}

	// The words: 100 per filter and 1,000 in all.
	many := func(prefix string, n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("%s%d", prefix, i)
		}
		return out
	}
	if _, err := e.svc.CreateKeywordFilter(ctx, bob, filterInput("x", timeline.FilterWarn, many("w", timeline.MaxKeywordFilterWords+1)...)); !errors.Is(err, timeline.ErrTooManyFilterWords) {
		t.Errorf("a filter of %d words = %v, want ErrTooManyFilterWords", timeline.MaxKeywordFilterWords+1, err)
	}
	var last apptimeline.OwnedKeywordFilter
	for i := range timeline.MaxKeywordFilterWordsTotal / timeline.MaxKeywordFilterWords {
		if last, err = e.svc.CreateKeywordFilter(ctx, bob, filterInput("x", timeline.FilterWarn, many(fmt.Sprintf("f%d-", i), timeline.MaxKeywordFilterWords)...)); err != nil {
			t.Fatalf("filter %d: %v", i, err)
		}
	}
	if _, err := e.svc.CreateKeywordFilter(ctx, bob, filterInput("x", timeline.FilterWarn, "one more")); !errors.Is(err, timeline.ErrTooManyFilterWords) {
		t.Errorf("a word beyond %d in all = %v, want ErrTooManyFilterWords", timeline.MaxKeywordFilterWordsTotal, err)
	}
	// Replacing a filter's words counts the others' only.
	if _, err := e.svc.UpdateKeywordFilter(ctx, bob, last.ID, filterInput("y", timeline.FilterWarn, many("z", timeline.MaxKeywordFilterWords)...)); err != nil {
		t.Errorf("replacing the words at the limit = %v", err)
	}
}
