// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
)

// AutoDeleteHarness is one database under test for the automatic deletion of old posts: the
// harness of the interaction contract and the automatic deletion repository on the same
// database.
type AutoDeleteHarness struct {
	InteractionHarness
	AutoDeletes appcontent.AutoDeleteRepository
}

// AutoDeleteFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type AutoDeleteFactory func(t *testing.T, opts uow.Options) AutoDeleteHarness

// AutoDeleteContract verifies the automatic deletion of local accounts' own old posts (FR-118)
// on a database adapter, with the content module's projectors and use cases: the policies are
// saved and read back (the default one before any is saved) and listed while enabled; a preview
// counts what an unsaved policy would delete without deleting it; a run deletes the account's
// local posts published before the policy's age with origin auto_delete and a
// content.status_deleted event, and keeps the newer ones, other accounts' posts, and the
// posts that an exception keeps (a pin, the author's bookmark or favourite, media, a poll, a
// kept visibility, or the boosts or favourites thresholds); it deletes nothing of a disabled
// policy or of an account that is not active; and it deletes a bounded number of posts per
// account and run, the rest in the following runs.
func AutoDeleteContract(t *testing.T, newHarness AutoDeleteFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *autoDeleteEnv)
	}{
		{"SavesPolicies", autoDeleteSavesPolicies},
		{"DeletesOldPostsButTheExceptions", autoDeleteDeletesOldPosts},
		{"SkipsDisabledPoliciesAndInactiveAccounts", autoDeleteSkips},
		{"BoundsTheDeletionsOfARun", autoDeleteBounds},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newAutoDeleteEnv(t, newHarness)) })
	}
}

type autoDeleteEnv struct {
	*interactionEnv
	repo appcontent.AutoDeleteRepository

	mu       sync.Mutex
	deletion []content.Deletion
}

func newAutoDeleteEnv(t *testing.T, newHarness AutoDeleteFactory) *autoDeleteEnv {
	t.Helper()
	var (
		h        AutoDeleteHarness
		captured uow.Options
	)
	ie := newInteractionEnv(t, func(t *testing.T, opts uow.Options) InteractionHarness {
		captured = opts
		h = newHarness(t, opts)
		return h.InteractionHarness
	})
	e := &autoDeleteEnv{interactionEnv: ie, repo: h.AutoDeletes}
	captured.Projectors.Register(appcontent.EventStatusDeleted, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.deletion = append(e.deletion, ev.(appcontent.StatusDeleted).Deletion)
		return nil
	})
	return e
}

// service returns the automatic deletion with batch posts per transaction and perAccount posts
// per account and run (zero for the defaults).
func (e *autoDeleteEnv) service(batch, perAccount int) *appcontent.AutoDelete {
	return appcontent.NewAutoDelete(appcontent.AutoDeleteDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Statuses, AutoDeletes: e.repo, Accounts: e.actors, Clock: e.clock,
		Batch: batch, PerAccount: perAccount,
	})
}

// save saves the policy of p.AccountID.
func (e *autoDeleteEnv) save(t *testing.T, p content.AutoDeletePolicy) content.AutoDeletePolicy {
	t.Helper()
	saved, err := e.service(0, 0).UpdateAutoDeletePolicy(t.Context(), p.AccountID, p)
	if err != nil {
		t.Fatalf("UpdateAutoDeletePolicy(%+v): %v", p, err)
	}
	return saved
}

// runAll runs the job from the first policy to the last and returns how many posts it deleted.
func (e *autoDeleteEnv) runAll(t *testing.T, s *appcontent.AutoDelete) int {
	t.Helper()
	total, after := 0, ""
	for {
		next, n, err := s.Run(t.Context(), after)
		if err != nil {
			t.Fatalf("Run(%q): %v", after, err)
		}
		total += n
		if next == "" {
			return total
		}
		after = next
	}
}

// deleted reports whether the status has a deletion with origin auto_delete.
func (e *autoDeleteEnv) deleted(t *testing.T, statusID string) bool {
	t.Helper()
	d, err := e.h.Statuses.FindDeletion(t.Context(), statusID)
	if errors.Is(err, appcontent.ErrNotFound) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	if d.Origin != content.DeletedByAutoDelete {
		t.Errorf("deletion of %s has origin %q, want auto_delete", statusID, d.Origin)
	}
	return true
}

// postWith creates a local status of author whose first revision edit changes.
func (e *autoDeleteEnv) postWith(t *testing.T, author string, v content.Visibility, edit func(*content.Revision)) content.Status {
	t.Helper()
	s := e.newStatus(author, v)
	rev := e.firstRevision(s)
	edit(&rev)
	if err := e.create(t, s, nil, rev); err != nil {
		t.Fatalf("create status: %v", err)
	}
	return s
}

func (e *autoDeleteEnv) takeDeletions() []content.Deletion {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.deletion
	e.deletion = nil
	return out
}

func autoDeleteSavesPolicies(t *testing.T, e *autoDeleteEnv) {
	ctx := t.Context()
	alice, bob := e.localAccount(), e.localAccount()
	s := e.service(0, 0)
	got, err := s.AutoDeletePolicy(ctx, alice)
	if err != nil || !reflect.DeepEqual(got, content.DefaultAutoDeletePolicy(alice)) {
		t.Errorf("AutoDeletePolicy before a save = %+v, %v; want the default", got, err)
	}
	want := content.AutoDeletePolicy{
		AccountID: alice, Enabled: true, MinAgeDays: 7, KeepPinned: true, KeepPolls: true,
		KeepVisibilities: []content.Visibility{content.VisibilityDirect, content.VisibilityLocal},
		KeepMinBoosts:    3, KeepMinFavourites: 10,
	}
	saved := e.save(t, want)
	want.KeepVisibilities = []content.Visibility{content.VisibilityLocal, content.VisibilityDirect}
	want.UpdatedAt = saved.UpdatedAt
	if !reflect.DeepEqual(saved, want) {
		t.Errorf("saved = %+v, want %+v", saved, want)
	}
	if got, err := s.AutoDeletePolicy(ctx, alice); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("AutoDeletePolicy = %+v, %v; want %+v", got, err, want)
	}
	// Saving again replaces the whole policy, the kept visibilities too.
	e.clock.Advance(time.Minute)
	replaced := e.save(t, content.AutoDeletePolicy{AccountID: alice, MinAgeDays: 30, KeepVisibilities: nil})
	if got, err := s.AutoDeletePolicy(ctx, alice); err != nil || !reflect.DeepEqual(got, replaced) || got.Enabled || len(got.KeepVisibilities) != 0 {
		t.Errorf("AutoDeletePolicy after the replacement = %+v, %v; want %+v", got, err, replaced)
	}
	if _, err := s.UpdateAutoDeletePolicy(ctx, alice, content.AutoDeletePolicy{MinAgeDays: 0}); err == nil {
		t.Error("UpdateAutoDeletePolicy accepted a policy of 0 days")
	}

	// Only the enabled policies are listed, in account order after the cursor.
	e.save(t, content.AutoDeletePolicy{AccountID: bob, Enabled: true, MinAgeDays: 1})
	e.save(t, content.AutoDeletePolicy{AccountID: alice, Enabled: true, MinAgeDays: 1})
	all, err := e.repo.ListEnabledAutoDeletePolicies(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(all))
	for _, p := range all {
		ids = append(ids, p.AccountID)
	}
	if want := []string{min(alice, bob), max(alice, bob)}; !slices.Equal(ids, want) {
		t.Errorf("enabled policies = %v, want %v", ids, want)
	}
	if rest, err := e.repo.ListEnabledAutoDeletePolicies(ctx, min(alice, bob), 10); err != nil || len(rest) != 1 || rest[0].AccountID != max(alice, bob) {
		t.Errorf("enabled policies after the first = %+v, %v", rest, err)
	}
	e.save(t, content.AutoDeletePolicy{AccountID: bob, MinAgeDays: 1})
	if rest, err := e.repo.ListEnabledAutoDeletePolicies(ctx, "", 10); err != nil || len(rest) != 1 || rest[0].AccountID != alice {
		t.Errorf("enabled policies after disabling bob's = %+v, %v; want alice's", rest, err)
	}
}

func autoDeleteDeletesOldPosts(t *testing.T, e *autoDeleteEnv) {
	ctx := t.Context()
	author, other, fan, fan2 := e.localAccount(), e.localAccount(), e.localAccount(), e.localAccount()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	plain := e.post(t, author, content.VisibilityPublic)
	followersOnly := e.post(t, author, content.VisibilityPrivate)
	pinned := e.post(t, author, content.VisibilityPublic)
	must(e.svc.Pin(ctx, author, pinned.ID))
	bookmarked := e.post(t, author, content.VisibilityPublic)
	must(e.svc.Bookmark(ctx, author, bookmarked.ID))
	bookmarkedByOther := e.post(t, author, content.VisibilityPublic)
	must(e.svc.Bookmark(ctx, fan, bookmarkedByOther.ID))
	favourited := e.post(t, author, content.VisibilityPublic)
	_, err := e.svc.Favourite(ctx, author, favourited.ID)
	must(err)
	withMedia := e.postWith(t, author, content.VisibilityPublic, func(r *content.Revision) {
		r.Attachments = []content.Attachment{{Position: 0, MediaID: e.ids.NewID()}}
	})
	withPoll := e.postWith(t, author, content.VisibilityPublic, func(r *content.Revision) {
		r.Poll = &content.Poll{ID: e.ids.NewID(), Options: []content.PollOption{{Position: 0, Title: "a"}, {Position: 1, Title: "b"}}}
	})
	direct := e.post(t, author, content.VisibilityDirect)
	boosted := e.post(t, author, content.VisibilityPublic)
	for _, booster := range []string{fan, fan2} {
		_, err := e.svc.Boost(ctx, booster, boosted.ID)
		must(err)
	}
	onceFavourited := e.post(t, author, content.VisibilityPublic)
	_, err = e.svc.Favourite(ctx, fan, onceFavourited.ID)
	must(err)
	othersOld := e.post(t, other, content.VisibilityPublic)
	e.clock.Advance(31 * 24 * time.Hour)
	fresh := e.post(t, author, content.VisibilityPublic)

	policy := content.AutoDeletePolicy{
		AccountID: author, Enabled: true, MinAgeDays: 30,
		KeepPinned: true, KeepBookmarked: true, KeepFavourited: true, KeepMedia: true, KeepPolls: true,
		KeepVisibilities: []content.Visibility{content.VisibilityDirect}, KeepMinBoosts: 2, KeepMinFavourites: 2,
	}
	// The preview of the unsaved, disabled policy counts what it would delete, and deletes nothing.
	draft := policy
	draft.Enabled = false
	preview, err := e.service(0, 0).PreviewAutoDeletePolicy(ctx, author, draft)
	must(err)
	if preview.Count != 4 || preview.More {
		t.Errorf("preview = %d posts (more %v), want 4", preview.Count, preview.More)
	}
	var examples []string
	for _, st := range preview.Examples {
		examples = append(examples, st.ID)
	}
	if want := []string{plain.ID, followersOnly.ID, bookmarkedByOther.ID}; !slices.Equal(examples, want) {
		t.Errorf("preview examples = %v, want the oldest %v", examples, want)
	}
	if all, err := e.service(0, 0).PreviewAutoDeletePolicy(ctx, author, content.AutoDeletePolicy{MinAgeDays: 30}); err != nil || all.Count != 11 {
		t.Errorf("preview without exceptions = %+v, %v; want 11 posts", all, err)
	}
	if _, err := e.service(0, 0).PreviewAutoDeletePolicy(ctx, author, content.AutoDeletePolicy{MinAgeDays: 0}); err == nil {
		t.Error("PreviewAutoDeletePolicy accepted a policy of 0 days")
	}
	if e.deleted(t, plain.ID) {
		t.Error("the preview deleted a post")
	}

	e.save(t, policy)
	e.takeDeletions()
	s := e.service(2, 0)
	if n := e.runAll(t, s); n != 4 {
		t.Errorf("deleted %d posts, want 4", n)
	}
	for _, st := range []content.Status{plain, followersOnly, bookmarkedByOther, onceFavourited} {
		if !e.deleted(t, st.ID) {
			t.Errorf("old post %s without an exception is not deleted", st.ID)
		}
	}
	kept := []content.Status{pinned, bookmarked, favourited, withMedia, withPoll, direct, boosted, othersOld, fresh}
	for _, st := range kept {
		if e.deleted(t, st.ID) {
			t.Errorf("post %s is deleted, want it kept", st.ID)
		}
	}
	got := e.takeDeletions()
	if len(got) != 4 {
		t.Fatalf("status_deleted events = %+v, want 4", got)
	}
	for _, d := range got {
		if d.Origin != content.DeletedByAutoDelete || d.AuthorAccountID != author || !d.DeletedAt.Equal(e.clock.Now()) {
			t.Errorf("deletion = %+v, want one by auto_delete now", d)
		}
	}
	// A run again deletes nothing more.
	if n := e.runAll(t, s); n != 0 || len(e.takeDeletions()) != 0 {
		t.Errorf("a second run deleted %d posts", n)
	}

	// Without the exceptions, every old post of the author goes.
	e.save(t, content.AutoDeletePolicy{AccountID: author, Enabled: true, MinAgeDays: 30})
	if n := e.runAll(t, s); n != 7 {
		t.Errorf("without exceptions deleted %d posts, want 7", n)
	}
	for _, st := range kept[:7] {
		if !e.deleted(t, st.ID) {
			t.Errorf("old post %s is kept without exceptions", st.ID)
		}
	}
	if e.deleted(t, othersOld.ID) || e.deleted(t, fresh.ID) {
		t.Error("another account's post or a fresh post is deleted")
	}
}

func autoDeleteSkips(t *testing.T, e *autoDeleteEnv) {
	disabled := e.localAccount()
	deactivated := e.account(true, account.StateDeactivated)
	a := e.post(t, disabled, content.VisibilityPublic)
	b := e.post(t, deactivated, content.VisibilityPublic)
	e.clock.Advance(2 * 24 * time.Hour)
	e.save(t, content.AutoDeletePolicy{AccountID: disabled, MinAgeDays: 1})
	e.save(t, content.AutoDeletePolicy{AccountID: deactivated, Enabled: true, MinAgeDays: 1})
	if n := e.runAll(t, e.service(0, 0)); n != 0 || e.deleted(t, a.ID) || e.deleted(t, b.ID) {
		t.Errorf("deleted %d posts of a disabled policy or an inactive account", n)
	}
}

func autoDeleteBounds(t *testing.T, e *autoDeleteEnv) {
	author := e.localAccount()
	posts := make([]content.Status, 0, 5)
	for range 5 {
		posts = append(posts, e.post(t, author, content.VisibilityPublic))
	}
	e.clock.Advance(2 * 24 * time.Hour)
	e.save(t, content.AutoDeletePolicy{AccountID: author, Enabled: true, MinAgeDays: 1})
	s := e.service(2, 3)
	if n := e.runAll(t, s); n != 3 {
		t.Errorf("the first run deleted %d posts, want 3", n)
	}
	// The oldest go first.
	for i, st := range posts {
		if got := e.deleted(t, st.ID); got != (i < 3) {
			t.Errorf("post %d deleted = %v after the first run", i, got)
		}
	}
	if n := e.runAll(t, s); n != 2 {
		t.Errorf("the second run deleted %d posts, want 2", n)
	}
}
