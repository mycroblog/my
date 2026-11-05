// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/queue"
)

// TestEligible checks the rule of what the index holds (ADR-0149 §5): the setting chooses between
// public and local, and nothing lets unlisted, private, or direct through.
func TestEligible(t *testing.T) {
	both := []Visibility{VisibilityPublic, VisibilityLocal}
	public := []Visibility{VisibilityPublic}
	local := []Visibility{VisibilityLocal}
	ok := Candidate{Visibility: appcontent.VisibilityPublic, Indexable: true}
	with := func(f func(*Candidate)) Candidate {
		c := ok
		f(&c)
		return c
	}
	for _, tc := range []struct {
		name    string
		c       Candidate
		covered []Visibility
		want    Visibility
	}{
		{"public", ok, public, VisibilityPublic},
		{"public outside the setting", ok, local, ""},
		{"local", with(func(c *Candidate) { c.Visibility = appcontent.VisibilityLocal }), both, VisibilityLocal},
		{"local outside the setting", with(func(c *Candidate) { c.Visibility = appcontent.VisibilityLocal }), public, ""},
		{"unlisted", with(func(c *Candidate) { c.Visibility = appcontent.VisibilityUnlisted }), both, ""},
		{"private", with(func(c *Candidate) { c.Visibility = appcontent.VisibilityPrivate }), both, ""},
		{"direct", with(func(c *Candidate) { c.Visibility = appcontent.VisibilityDirect }), both, ""},
		{"deleted", with(func(c *Candidate) { c.Deleted = true }), both, ""},
		{"not indexable", with(func(c *Candidate) { c.Indexable = false }), both, ""},
		{"profile_view", with(func(c *Candidate) { c.Author.ProfileView = true }), both, ""},
		{"public_discovery", with(func(c *Candidate) { c.Author.PublicDiscovery = true }), both, ""},
		{"ceiling of private", with(func(c *Candidate) { c.Author.VisibilityCeilingPrivate = true }), both, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, qualifies := Eligible(tc.c, tc.covered)
			if got != tc.want || qualifies != (tc.want != "") {
				t.Errorf("Eligible = %q, %v, want %q", got, qualifies, tc.want)
			}
		})
	}
}

// syncWorld is the state a sync reads, which a test changes between the reads.
type syncWorld struct {
	status    appcontent.Status
	revision  appcontent.Revision
	deleted   bool
	missing   bool
	indexable bool
	author    appcontent.AuthorRestrictions
	covered   []string
	statusIDs []string
	onRead    func(reads int)
	reads     int
}

func (w *syncWorld) ObjectStatus(context.Context, string) (appcontent.Status, appcontent.Revision, bool, error) {
	w.reads++
	if w.onRead != nil {
		w.onRead(w.reads)
	}
	if w.missing {
		return appcontent.Status{}, appcontent.Revision{}, false, appcontent.ErrNotFound
	}
	return w.status, w.revision, w.deleted, nil
}

func (w *syncWorld) AuthorRestrictions(context.Context, string) (appcontent.AuthorRestrictions, error) {
	return w.author, nil
}

func (w *syncWorld) AuthorSearchableStatusIDs(_ context.Context, _, after string, limit int) ([]string, error) {
	var out []string
	for _, id := range w.statusIDs {
		if id > after && len(out) < limit {
			out = append(out, id)
		}
	}
	return out, nil
}

func (w *syncWorld) SearchableStatusIDs(ctx context.Context, after string, limit int) ([]string, error) {
	return w.AuthorSearchableStatusIDs(ctx, "", after, limit)
}

func (w *syncWorld) ProfileIndexable(context.Context, string) (bool, error) {
	return w.indexable, nil
}

func (w *syncWorld) AccountIDsOnDomain(context.Context, string, string, int) ([]string, error) {
	return nil, nil
}

func (w *syncWorld) SearchVisibilities(context.Context) ([]string, error) { return w.covered, nil }

// recordingIndex records the calls of the backend in order, and fails with err when it is set.
type recordingIndex struct {
	Index
	calls []string
	docs  []Document
	err   error
	// deadline is how far off the deadline of the last call was.
	deadline time.Duration
}

func (x *recordingIndex) record(ctx context.Context, call string) error {
	if d, ok := ctx.Deadline(); ok {
		x.deadline = time.Until(d)
	}
	x.calls = append(x.calls, call)
	return x.err
}

func (x *recordingIndex) Replace(ctx context.Context, doc Document) error {
	x.docs = append(x.docs, doc)
	return x.record(ctx, "replace")
}

func (x *recordingIndex) Remove(ctx context.Context, _ string) error {
	return x.record(ctx, "remove")
}

func (x *recordingIndex) RemoveByAuthor(ctx context.Context, _ string) error {
	return x.record(ctx, "remove_by_author")
}

func (x *recordingIndex) RemoveByVisibility(ctx context.Context, v Visibility) error {
	return x.record(ctx, "remove_by_visibility:"+string(v))
}

type upperSanitizer struct{}

func (upperSanitizer) SanitizeHTML(s string) string { return s }
func (upperSanitizer) PlainText(s string) string    { return s }
func (upperSanitizer) PolicyVersion() int           { return 1 }

func newSyncService(w *syncWorld, x *recordingIndex) *Service {
	return NewService(Deps{Index: x, Statuses: w, Accounts: w, Settings: w, Sanitizer: upperSanitizer{}})
}

func publicWorld() *syncWorld {
	return &syncWorld{
		status:    appcontent.Status{ID: "s1", AuthorAccountID: "a1", Visibility: appcontent.VisibilityPublic},
		revision:  appcontent.Revision{ID: "r1", Language: "und"},
		indexable: true,
		covered:   []string{"public"},
	}
}

func runJob(t *testing.T, s *Service, jobType string, payload any) queue.Result {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	reg := &queue.JobHandlerRegistry{}
	s.RegisterJobs(reg)
	h, _, ok := reg.Lookup(jobType)
	if !ok {
		t.Fatalf("no handler of %s", jobType)
	}
	return h(t.Context(), queue.Job{ID: "job", Type: jobType, Payload: b})
}

// TestSyncStatusReadsAgainAfterWriting checks the race of ADR-0149 §7: what changed while the
// document was written is written again or removed.
func TestSyncStatusReadsAgainAfterWriting(t *testing.T) {
	t.Run("nothing changed", func(t *testing.T) {
		w, x := publicWorld(), &recordingIndex{}
		if res := runJob(t, newSyncService(w, x), JobSyncStatus, syncStatusPayload{StatusID: "s1"}); !res.IsDone() {
			t.Fatalf("result %+v", res)
		}
		if !slices.Equal(x.calls, []string{"replace"}) || w.reads != 2 {
			t.Errorf("calls = %v after %d reads, want one replace and two reads", x.calls, w.reads)
		}
	})
	t.Run("a new revision", func(t *testing.T) {
		w, x := publicWorld(), &recordingIndex{}
		w.onRead = func(reads int) {
			if reads == 2 {
				w.revision.ID = "r2"
				w.revision.Bodies = []appcontent.Body{{ContentHTML: "newer"}}
			}
		}
		if res := runJob(t, newSyncService(w, x), JobSyncStatus, syncStatusPayload{StatusID: "s1"}); !res.IsDone() {
			t.Fatalf("result %+v", res)
		}
		if !slices.Equal(x.calls, []string{"replace", "replace"}) || x.docs[1].Text != "newer" {
			t.Errorf("calls = %v, docs = %+v, want the newer text written last", x.calls, x.docs)
		}
	})
	t.Run("deleted meanwhile", func(t *testing.T) {
		w, x := publicWorld(), &recordingIndex{}
		w.onRead = func(reads int) { w.deleted = reads >= 2 }
		if res := runJob(t, newSyncService(w, x), JobSyncStatus, syncStatusPayload{StatusID: "s1"}); !res.IsDone() {
			t.Fatalf("result %+v", res)
		}
		if !slices.Equal(x.calls, []string{"replace", "remove"}) {
			t.Errorf("calls = %v, want the document removed", x.calls)
		}
	})
	t.Run("sanctioned meanwhile", func(t *testing.T) {
		w, x := publicWorld(), &recordingIndex{}
		w.onRead = func(reads int) { w.author.VisibilityCeilingPrivate = reads >= 2 }
		if res := runJob(t, newSyncService(w, x), JobSyncStatus, syncStatusPayload{StatusID: "s1"}); !res.IsDone() {
			t.Fatalf("result %+v", res)
		}
		if !slices.Equal(x.calls, []string{"replace", "remove"}) {
			t.Errorf("calls = %v, want the document removed", x.calls)
		}
	})
	t.Run("never settles", func(t *testing.T) {
		w, x := publicWorld(), &recordingIndex{}
		w.onRead = func(reads int) { w.revision.ID = strconv.Itoa(reads) }
		res := runJob(t, newSyncService(w, x), JobSyncStatus, syncStatusPayload{StatusID: "s1"})
		if res.IsDone() || res.Err() == nil {
			t.Errorf("result %+v, want a retry", res)
		}
	})
}

// TestSyncStatusRemovesWhatDoesNotQualify checks that a status gone from the database or not
// qualifying is removed, and never written.
func TestSyncStatusRemovesWhatDoesNotQualify(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*syncWorld)
	}{
		{"missing", func(w *syncWorld) { w.missing = true }},
		{"deleted", func(w *syncWorld) { w.deleted = true }},
		{"private", func(w *syncWorld) { w.status.Visibility = appcontent.VisibilityPrivate }},
		{"direct", func(w *syncWorld) { w.status.Visibility = appcontent.VisibilityDirect }},
		{"unlisted", func(w *syncWorld) { w.status.Visibility = appcontent.VisibilityUnlisted }},
		{"outside the setting", func(w *syncWorld) { w.covered = []string{"local"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, x := publicWorld(), &recordingIndex{}
			tc.set(w)
			if res := runJob(t, newSyncService(w, x), JobSyncStatus, syncStatusPayload{StatusID: "s1"}); !res.IsDone() {
				t.Fatalf("result %+v", res)
			}
			if !slices.Equal(x.calls, []string{"remove"}) {
				t.Errorf("calls = %v, want one remove", x.calls)
			}
		})
	}
}

// TestDocumentHoldsTheCurrentText checks what the search looks in (ADR-0149 §5): a job carries no
// text, and the document takes the warning, the body, and the alternative texts of the revision.
func TestDocumentHoldsTheCurrentText(t *testing.T) {
	w, x := publicWorld(), &recordingIndex{}
	w.revision.SpoilerText = "warning"
	w.revision.Source = &appcontent.Source{Text: "typed body"}
	w.revision.Bodies = []appcontent.Body{{ContentHTML: "<p>rendered</p>"}}
	w.revision.Attachments = []appcontent.Attachment{{Description: "alt one"}, {Description: ""}, {Description: "alt two"}}
	w.revision.Poll = &appcontent.Poll{Options: []appcontent.PollOption{{Title: "option"}}}
	if res := runJob(t, newSyncService(w, x), JobSyncStatus, syncStatusPayload{StatusID: "s1"}); !res.IsDone() {
		t.Fatalf("result %+v", res)
	}
	doc := x.docs[0]
	if doc.Text != "warning typed body alt one alt two" {
		t.Errorf("Text = %q", doc.Text)
	}
	if doc.Language != "" || doc.AuthorID != "a1" || doc.Visibility != VisibilityPublic {
		t.Errorf("document = %+v", doc)
	}
}

// TestBackendCalls checks that a failed call retries and that each call has a short deadline
// (ADR-0149 §7).
func TestBackendCalls(t *testing.T) {
	w, x := publicWorld(), &recordingIndex{err: errors.New("down")}
	res := runJob(t, newSyncService(w, x), JobSyncStatus, syncStatusPayload{StatusID: "s1"})
	if res.IsDone() || res.Err() == nil {
		t.Errorf("result %+v, want a retry", res)
	}
	if x.deadline <= 0 || x.deadline > indexCallTimeout {
		t.Errorf("the deadline of the call was %s away, want at most %s", x.deadline, indexCallTimeout)
	}
}

// TestSyncAuthor checks both ways of the job: removing every document, and walking the statuses.
func TestSyncAuthor(t *testing.T) {
	t.Run("not indexable", func(t *testing.T) {
		w, x := publicWorld(), &recordingIndex{}
		w.indexable = false
		w.statusIDs = []string{"s1", "s2"}
		if res := runJob(t, newSyncService(w, x), JobSyncAuthor, syncAuthorPayload{AccountID: "a1"}); !res.IsDone() {
			t.Fatalf("result %+v", res)
		}
		if !slices.Equal(x.calls, []string{"remove_by_author"}) {
			t.Errorf("calls = %v", x.calls)
		}
	})
	t.Run("indexable", func(t *testing.T) {
		w, x := publicWorld(), &recordingIndex{}
		w.statusIDs = []string{"s1", "s2", "s3"}
		if res := runJob(t, newSyncService(w, x), JobSyncAuthor, syncAuthorPayload{AccountID: "a1"}); !res.IsDone() {
			t.Fatalf("result %+v", res)
		}
		if !slices.Equal(x.calls, []string{"replace", "replace", "replace"}) {
			t.Errorf("calls = %v", x.calls)
		}
	})
}

// TestNarrow checks that the job removes a visibility the setting left out, and leaves it when the
// setting covers it again by the time it runs.
func TestNarrow(t *testing.T) {
	w, x := publicWorld(), &recordingIndex{}
	w.covered = []string{"local"}
	if res := runJob(t, newSyncService(w, x), JobNarrow, narrowPayload{Visibility: VisibilityPublic}); !res.IsDone() {
		t.Fatalf("result %+v", res)
	}
	if !slices.Equal(x.calls, []string{"remove_by_visibility:public"}) {
		t.Errorf("calls = %v", x.calls)
	}

	w, x = publicWorld(), &recordingIndex{}
	if res := runJob(t, newSyncService(w, x), JobNarrow, narrowPayload{Visibility: VisibilityPublic}); !res.IsDone() {
		t.Fatalf("result %+v", res)
	}
	if len(x.calls) != 0 {
		t.Errorf("calls = %v, want none when the setting covers it again", x.calls)
	}

	// Nothing else can be narrowed away, least of all by a job that names unlisted.
	if res := runJob(t, newSyncService(w, x), JobNarrow, map[string]string{"visibility": "unlisted"}); res.IsDone() || res.Err() == nil {
		t.Errorf("result %+v, want a failure", res)
	}
}
