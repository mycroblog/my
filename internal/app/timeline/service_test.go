// SPDX-License-Identifier: AGPL-3.0-only

package timeline_test

import (
	"context"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// leakyRepository returns whatever rows the test gives it, as a repository whose SQL forgot
// part of the visibility policy would, so that the tests see what CanView removes.
type leakyRepository struct {
	apptimeline.Repository
	rows  []apptimeline.Entry
	facts apptimeline.Facts
	homes []apptimeline.HomeQuery
	// mutedDomains answers MutedDomains.
	mutedDomains []string
}

func (r *leakyRepository) ListLocal(context.Context, apptimeline.LocalQuery) ([]apptimeline.Entry, error) {
	return r.rows, nil
}

func (r *leakyRepository) ListHome(_ context.Context, q apptimeline.HomeQuery) ([]apptimeline.Entry, error) {
	r.homes = append(r.homes, q)
	return r.rows, nil
}

func (r *leakyRepository) Facts(context.Context, apptimeline.FactsQuery) (apptimeline.Facts, error) {
	return r.facts, nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func post(id, author string, v appcontent.Visibility) apptimeline.Entry {
	return apptimeline.Entry{
		ID: id, Kind: timeline.KindPost, StatusID: id, AuthorAccountID: author, OriginalAuthorAccountID: author,
		Visibility: v, IsLocal: true, Language: "en", RevisionSeq: 1,
	}
}

func ids(p apptimeline.Page) []string {
	var out []string
	for _, e := range p.Entries {
		out = append(out, e.ID)
	}
	return out
}

func TestServiceReappliesCanView(t *testing.T) {
	repo := &leakyRepository{
		rows: []apptimeline.Entry{
			post(testID(6), blockerID, appcontent.VisibilityPublic),
			post(testID(5), authorID, appcontent.VisibilityDirect),
			post(testID(4), authorID, appcontent.VisibilityPrivate),
			post(testID(3), authorID, appcontent.VisibilityUnlisted),
			post(testID(2), authorID, appcontent.VisibilityPublic),
			post(testID(1), mutedID, appcontent.VisibilityPublic),
		},
		facts: apptimeline.Facts{Exclusions: map[string][]timeline.ExclusionReason{
			blockerID: {timeline.ExcludedBlockedBy}, mutedID: {timeline.ExcludedMute},
		}},
	}
	svc := apptimeline.NewService(apptimeline.Deps{Repository: repo, Clock: fixedClock{time.Now()}})
	viewer := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: viewerID}
	p, err := svc.Local(t.Context(), viewer, apptimeline.Query{Limit: 10})
	if err != nil {
		t.Fatalf("Local: %v", err)
	}
	if got, want := ids(p), []string{testID(2)}; !slices.Equal(got, want) {
		t.Errorf("Local = %v, want %v", got, want)
	}
	// The page is cut before CanView, so the cursor follows the rows read, not the rows kept.
	p, err = svc.Local(t.Context(), viewer, apptimeline.Query{Limit: 3})
	if err != nil {
		t.Fatalf("Local: %v", err)
	}
	if !slices.Equal(ids(p), nil) || p.Next != testID(4) {
		t.Errorf("Local(limit 3) = %v next %q, want no entries and next 04", ids(p), p.Next)
	}
}

func TestServiceChecksBoostedStatuses(t *testing.T) {
	boost := apptimeline.Entry{
		ID: testID(2), Kind: timeline.KindBoost, StatusID: testID(1), AuthorAccountID: followeeID, OriginalAuthorAccountID: blockerID,
		Visibility: appcontent.VisibilityPublic, IsLocal: true, Language: "en", RevisionSeq: 1,
	}
	repo := &leakyRepository{
		rows: []apptimeline.Entry{boost},
		facts: apptimeline.Facts{
			Followed:   map[string]bool{followeeID: true},
			Exclusions: map[string][]timeline.ExclusionReason{blockerID: {timeline.ExcludedBlockedBy}},
		},
	}
	svc := apptimeline.NewService(apptimeline.Deps{Repository: repo, Clock: fixedClock{time.Now()}})
	viewer := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: viewerID}
	p, err := svc.Home(t.Context(), viewer, apptimeline.Query{})
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if len(p.Entries) != 0 {
		t.Errorf("Home = %v, want the boost of a blocker's status left out", ids(p))
	}
	// An unfilled page widens the window through every bound, each older than the last, and
	// then drops the bound.
	var floors []string
	for _, q := range repo.homes {
		floors = append(floors, q.Floor)
	}
	ok := len(floors) == len(timeline.HomeWindows())+1 && floors[len(floors)-1] == ""
	for i := 1; ok && i < len(floors)-1; i++ {
		ok = floors[i] < floors[i-1]
	}
	if !ok {
		t.Errorf("Home floors = %v, want %d widening bounds and then none", floors, len(timeline.HomeWindows()))
	}
}

func TestHomeEntry(t *testing.T) {
	repo := &leakyRepository{
		rows: []apptimeline.Entry{
			post(testID(3), followeeID, appcontent.VisibilityPublic),
			post(testID(2), followeeID, appcontent.VisibilityDirect),
			post(testID(1), followeeID, appcontent.VisibilityPublic),
		},
		facts: apptimeline.Facts{Followed: map[string]bool{followeeID: true}},
	}
	svc := apptimeline.NewService(apptimeline.Deps{Repository: repo, Clock: fixedClock{time.Now()}})
	viewer := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: viewerID}
	q := apptimeline.Query{Languages: []string{"en"}}

	e, ok, err := svc.HomeEntry(t.Context(), viewer, testID(1), q)
	if err != nil || !ok || e.ID != testID(1) {
		t.Errorf("HomeEntry(01) = %+v, %v, %v; want the entry", e, ok, err)
	}
	if hq := repo.homes[len(repo.homes)-1]; hq.Floor != testID(1) || hq.ViewerAccountID != viewerID || !slices.Equal(hq.Languages, q.Languages) {
		t.Errorf("HomeEntry read %+v, want the viewer's entries from 01 up in its languages", hq)
	}
	// CanView still applies: a direct post not addressed to the viewer is not listed.
	if _, ok, err := svc.HomeEntry(t.Context(), viewer, testID(2), q); err != nil || ok {
		t.Errorf("HomeEntry(02) = %v, %v; want not listed", ok, err)
	}
	if _, ok, err := svc.HomeEntry(t.Context(), viewer, testID(4), q); err != nil || ok {
		t.Errorf("HomeEntry(04) = %v, %v; want not listed", ok, err)
	}
	if _, _, err := svc.HomeEntry(t.Context(), appcontent.Viewer{}, testID(1), q); err == nil {
		t.Error("HomeEntry for a visitor succeeded")
	}
}

func (r *leakyRepository) MutedDomains(_ context.Context, _ string, domains []string) ([]string, error) {
	var out []string
	for _, d := range domains {
		if slices.Contains(r.mutedDomains, d) {
			out = append(out, d)
		}
	}
	return out, nil
}

// TestSearchVisible checks the policy of a search's results: CanView in ContextSearch, the mutes, and
// the domain mutes of a domain or its parent (ADR-0149 §8).
func TestSearchVisible(t *testing.T) {
	viewerID, authorID, mutedID, blockerID, farID, silencedID := testID(10), testID(11), testID(12), testID(13), testID(14), testID(15)
	repo := &leakyRepository{
		facts: apptimeline.Facts{
			Exclusions: map[string][]timeline.ExclusionReason{
				blockerID: {timeline.ExcludedBlockedBy}, mutedID: {timeline.ExcludedMute},
			},
			AccountRestrictions: map[string][]timeline.Restriction{silencedID: {timeline.RestrictPublicDiscovery}},
		},
		mutedDomains: []string{"muted.example"},
	}
	svc := apptimeline.NewService(apptimeline.Deps{Repository: repo, Clock: fixedClock{time.Now()}})
	candidate := func(n int, author, domain string, v appcontent.Visibility) apptimeline.SearchCandidate {
		return apptimeline.SearchCandidate{StatusID: testID(n), AuthorAccountID: author, AuthorDomain: domain, Visibility: v}
	}
	candidates := []apptimeline.SearchCandidate{
		candidate(1, authorID, "", appcontent.VisibilityPublic),
		candidate(2, authorID, "", appcontent.VisibilityLocal),
		candidate(3, authorID, "", appcontent.VisibilityUnlisted),
		candidate(4, authorID, "", appcontent.VisibilityPrivate),
		candidate(5, authorID, "", appcontent.VisibilityDirect),
		candidate(6, mutedID, "", appcontent.VisibilityPublic),
		candidate(7, blockerID, "", appcontent.VisibilityPublic),
		candidate(8, silencedID, "", appcontent.VisibilityPublic),
		candidate(9, farID, "muted.example", appcontent.VisibilityPublic),
		candidate(16, farID, "sub.muted.example", appcontent.VisibilityPublic),
		candidate(17, farID, "fine.example", appcontent.VisibilityPublic),
		candidate(18, farID, "fine.example", appcontent.VisibilityLocal),
	}
	viewer := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: viewerID}
	got, err := svc.SearchVisible(t.Context(), viewer, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{testID(1), testID(2), testID(17)}; !slices.Equal(got, want) {
		t.Errorf("SearchVisible = %v, want %v", got, want)
	}
	// The author and a moderator get nothing wider.
	for _, v := range []appcontent.Viewer{
		{Kind: appcontent.ViewerLocal, AccountID: authorID},
		{Kind: appcontent.ViewerLocal, AccountID: viewerID, Moderator: true},
	} {
		got, err := svc.SearchVisible(t.Context(), v, candidates[:5])
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{testID(1), testID(2)}; !slices.Equal(got, want) {
			t.Errorf("SearchVisible for %+v = %v, want %v", v, got, want)
		}
	}
	// No one signed out sees anything, without reading a thing.
	got, err = svc.SearchVisible(t.Context(), appcontent.Viewer{}, candidates)
	if err != nil || len(got) != 0 {
		t.Errorf("SearchVisible signed out = %v, %v", got, err)
	}
}
