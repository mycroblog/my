// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
)

// queryIndex answers Search with page or err, and fails the test on nothing else: the rest of Index
// stays unset, since a search writes nothing.
type queryIndex struct {
	Index
	page  Page
	err   error
	calls int
}

func (x *queryIndex) Search(context.Context, Query) (Page, error) {
	x.calls++
	return x.page, x.err
}

// queryWorld is a database of statuses by ID, with one author of each indexable value.
type queryWorld struct {
	syncWorld
	statuses map[string]appcontent.Status
	deleted  map[string]bool
	reads    map[string]int
}

func (w *queryWorld) ObjectStatus(_ context.Context, id string) (appcontent.Status, appcontent.Revision, bool, error) {
	st, ok := w.statuses[id]
	if !ok {
		return appcontent.Status{}, appcontent.Revision{}, false, appcontent.ErrNotFound
	}
	return st, appcontent.Revision{}, w.deleted[id], nil
}

func (w *queryWorld) ProfileIndexable(_ context.Context, id string) (bool, error) {
	w.reads[id]++
	return id != "unindexable", nil
}

func (w *queryWorld) AccountDomain(_ context.Context, id string) (string, error) {
	if id == "remote" {
		return "remote.example", nil
	}
	return "", nil
}

// allowAll records what reaches the policy and lets it through, which the tests of timeline judge.
type allowAll struct{ got []apptimeline.SearchCandidate }

func (v *allowAll) SearchVisible(_ context.Context, _ appcontent.Viewer, cs []apptimeline.SearchCandidate) ([]string, error) {
	v.got = cs
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.StatusID)
	}
	return out, nil
}

// TestSearchReadsTheDatabase checks that only what exists and qualifies now reaches the policy, in
// the backend's order, whatever the index holds (ADR-0149 §8).
func TestSearchReadsTheDatabase(t *testing.T) {
	status := func(id, author string, v appcontent.Visibility, local bool) appcontent.Status {
		return appcontent.Status{ID: id, AuthorAccountID: author, Visibility: v, IsLocal: local}
	}
	w := &queryWorld{
		statuses: map[string]appcontent.Status{
			"keep":     status("keep", "a", appcontent.VisibilityPublic, true),
			"keep2":    status("keep2", "a", appcontent.VisibilityPublic, true),
			"far":      status("far", "remote", appcontent.VisibilityPublic, false),
			"deleted":  status("deleted", "a", appcontent.VisibilityPublic, true),
			"unlisted": status("unlisted", "a", appcontent.VisibilityUnlisted, true),
			"private":  status("private", "a", appcontent.VisibilityPrivate, true),
			"direct":   status("direct", "a", appcontent.VisibilityDirect, true),
			"local":    status("local", "a", appcontent.VisibilityLocal, true),
			"opted":    status("opted", "unindexable", appcontent.VisibilityPublic, true),
		},
		deleted: map[string]bool{"deleted": true},
		reads:   map[string]int{},
	}
	w.covered = []string{"public"}
	ids := []string{"keep", "gone", "deleted", "unlisted", "private", "direct", "local", "opted", "far", "keep2"}
	x := &queryIndex{page: Page{StatusIDs: ids, Next: "more"}}
	v := &allowAll{}
	s := NewService(Deps{Index: x, Statuses: w, Accounts: w, Settings: w, Viewing: v})

	page, err := s.Search(t.Context(), appcontent.Viewer{}, Query{Text: "cats", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"keep", "far", "keep2"}; !slices.Equal(page.StatusIDs, want) {
		t.Errorf("found %v, want %v", page.StatusIDs, want)
	}
	if page.Next != "more" {
		t.Errorf("Next = %q, want the backend's", page.Next)
	}
	if v.got[1].AuthorDomain != "remote.example" || v.got[0].AuthorDomain != "" {
		t.Errorf("the domains of the candidates = %+v", v.got)
	}
	if w.reads["a"] != 1 {
		t.Errorf("an author read %d times in a page, want 1", w.reads["a"])
	}

	// The setting decides what qualifies when the search runs, not when the document was written.
	w.covered = []string{"public", "local"}
	page, err = s.Search(t.Context(), appcontent.Viewer{}, Query{Text: "cats", Limit: 20})
	if err != nil || !slices.Contains(page.StatusIDs, "local") {
		t.Errorf("found %v, %v, want local with the setting covering it", page.StatusIDs, err)
	}
}

// TestSearchFailures checks that a bad cursor and a failed backend stay apart, and that nothing the
// backend said, which may quote the query, leaves in the error (ADR-0149 §9).
func TestSearchFailures(t *testing.T) {
	w := &queryWorld{reads: map[string]int{}}
	x := &queryIndex{err: errors.New(`GET http://engine/_search?q=secretword: refused`)}
	s := NewService(Deps{Index: x, Statuses: w, Accounts: w, Settings: w, Viewing: &allowAll{}})
	_, err := s.Search(t.Context(), appcontent.Viewer{}, Query{Text: "secretword", Limit: 1})
	if !errors.Is(err, ErrBackendFailed) || errors.Is(err, ErrInvalidCursor) {
		t.Errorf("err = %v, want ErrBackendFailed", err)
	}
	if strings.Contains(err.Error(), "secretword") {
		t.Errorf("the error holds the query: %v", err)
	}

	x.err = context.DeadlineExceeded
	if _, err = s.Search(t.Context(), appcontent.Viewer{}, Query{Text: "cats", Limit: 1}); !errors.Is(err, ErrBackendFailed) || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want a timeout", err)
	}

	x.err = ErrInvalidCursor
	if _, err = s.Search(t.Context(), appcontent.Viewer{}, Query{Text: "cats", Cursor: "forged", Limit: 1}); !errors.Is(err, ErrInvalidCursor) || errors.Is(err, ErrBackendFailed) {
		t.Errorf("err = %v, want ErrInvalidCursor", err)
	}
}

// TestSearchNone checks that search = none reaches nothing.
func TestSearchNone(t *testing.T) {
	page, err := NewService(Deps{}).Search(t.Context(), appcontent.Viewer{}, Query{Text: "cats", Limit: 1})
	if err != nil || len(page.StatusIDs) != 0 || page.Next != "" {
		t.Errorf("Search = %+v, %v with no backend", page, err)
	}
}
