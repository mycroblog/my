// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
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

// AutoDeleteFlows verifies the automatic deletion of local accounts' own old posts (FR-118) on a
// database adapter, with the content module's projectors and use cases, the flows of ADR-0153 §1,
// which internal/flowtest/content runs once on SQLite; AutoDeleteContract checks the repository's
// methods on every adapter. The policies are
// saved and read back (the default one before any is saved) and listed while enabled; a preview
// counts what an unsaved policy would delete without deleting it; a run deletes the account's
// local posts published before the policy's age with origin auto_delete and a
// content.status_deleted event, and keeps the newer ones, other accounts' posts, and the
// posts that an exception keeps (a pin, the author's bookmark or favourite, media, a poll, a
// kept visibility, or the boosts or favourites thresholds); it deletes nothing of a disabled
// policy or of an account that is not active; and it deletes a bounded number of posts per
// account and run, the rest in the following runs.
func AutoDeleteFlows(t *testing.T, newHarness AutoDeleteFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *autoDeleteEnv)
	}{
		{"SavesPolicies", autoDeleteSavesPolicies},
		{"DeletesOldPostsButTheExceptions", autoDeleteDeletesOldPosts},
		{"SkipsDisabledPoliciesAndInactiveAccounts", autoDeleteSkips},
		{"BoundsTheDeletionsOfARun", autoDeleteBounds},
		{"HoldsAccountsToTheServerMaximum", autoDeleteServerMaximum},
		{"RefusesPoliciesOverTheMaximum", autoDeleteRefusesOverMaximum},
		{"CapsTheKeptPins", autoDeleteCapsPins},
		{"NoticesALowerMaximum", autoDeleteLowerMaximum},
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
	noticed  []content.AutoDeleteLimitNotice
	// limit is the server maximum and exempt the users with keep_posts_longer.
	limit  content.AutoDeleteLimit
	exempt map[string]bool
}

// autoDeleteActors gives the local accounts of the stub a user, as the account module does,
// and pages them.
type autoDeleteActors struct{ *stubActors }

func (a autoDeleteActors) FindByID(ctx context.Context, id string) (appaccount.Actor, error) {
	actor, err := a.stubActors.FindByID(ctx, id)
	if err == nil && actor.Account.IsLocal() {
		actor.Local = &account.LocalAccount{AccountID: id, UserID: "user-" + id}
	}
	return actor, err
}

func (a autoDeleteActors) LocalAccountIDs(_ context.Context, after string, limit int) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var ids []string
	for id, actor := range a.actors {
		if actor.Account.IsLocal() && id > after {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids[:min(len(ids), limit)], nil
}

// AutoDeleteDefault implements appcontent.AutoDeleteServer: the built-in default.
func (e *autoDeleteEnv) AutoDeleteDefault(context.Context, uow.Tx) (content.AutoDeletePolicy, error) {
	return content.DefaultAutoDeletePolicy(""), nil
}

// AutoDeleteLimit implements appcontent.AutoDeleteServer.
func (e *autoDeleteEnv) AutoDeleteLimit(context.Context, uow.Tx) (content.AutoDeleteLimit, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.limit, nil
}

// KeepsPostsLonger implements appcontent.AutoDeleteExemptions.
func (e *autoDeleteEnv) KeepsPostsLonger(_ context.Context, userID string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.exempt[userID], nil
}

func (e *autoDeleteEnv) setLimit(l content.AutoDeleteLimit) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.limit = l
}

func (e *autoDeleteEnv) setExempt(accountID string, exempt bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.exempt["user-"+accountID] = exempt
}

func (e *autoDeleteEnv) takeNotices() []content.AutoDeleteLimitNotice {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.noticed
	e.noticed = nil
	return out
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
	e := &autoDeleteEnv{interactionEnv: ie, repo: h.AutoDeletes, exempt: map[string]bool{}}
	captured.Projectors.Register(appcontent.EventAutoDeleteLimitNoticed, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.noticed = append(e.noticed, ev.(appcontent.AutoDeleteLimitNoticed).Notice)
		return nil
	})
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
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Statuses, AutoDeletes: e.repo, Accounts: autoDeleteActors{e.actors},
		Server: e, Exemptions: e, Clock: e.clock, IDs: e.ids, Batch: batch, PerAccount: perAccount,
	})
}

// save saves the policy of p.AccountID.
func (e *autoDeleteEnv) save(t *testing.T, p content.AutoDeletePolicy) content.AutoDeletePolicy {
	t.Helper()
	saved, err := e.service(0, 0).UpdateAutoDeletePolicy(t.Context(), p.AccountID, p)
	if err != nil {
		t.Fatalf("UpdateAutoDeletePolicy(%+v): %v", p, err)
	}
	return saved.Policy
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
	if err != nil || !reflect.DeepEqual(got, appcontent.AutoDeleteView{Policy: content.DefaultAutoDeletePolicy(alice)}) {
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
	if got, err := s.AutoDeletePolicy(ctx, alice); err != nil || !reflect.DeepEqual(got.Policy, want) || got.Limit != nil {
		t.Errorf("AutoDeletePolicy = %+v, %v; want %+v", got, err, want)
	}
	// Saving again replaces the whole policy, the kept visibilities too.
	e.clock.Advance(time.Minute)
	replaced := e.save(t, content.AutoDeletePolicy{AccountID: alice, MinAgeDays: 30, KeepVisibilities: nil})
	if got, err := s.AutoDeletePolicy(ctx, alice); err != nil || !reflect.DeepEqual(got.Policy, replaced) || got.Policy.Enabled || len(got.Policy.KeepVisibilities) != 0 {
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

// autoDeleteDefaultMaximum is the server maximum of 30 days with a grace period of 10 days and the
// default exceptions: every pin, nothing else.
func autoDeleteDefaultMaximum() content.AutoDeleteLimit {
	return content.AutoDeleteLimit{MaxAgeDays: 30, AllowPinned: true, KeptPins: content.MaxPins, GraceDays: 10}
}

// autoDeleteServerMaximum checks ADR-0151 §4 and §5: under the maximum, an account that it binds
// (a disabled policy, a longer one, or exceptions it does not allow) is told once and keeps its
// saved policy until the grace period ends, then goes by its effective policy; an account within
// the maximum goes by its own at once; one with keep_posts_longer is left alone; and an account
// that is no longer bound loses its notice in effect, which goes when the maximum is turned off.
func autoDeleteServerMaximum(t *testing.T, e *autoDeleteEnv) {
	ctx := t.Context()
	disabled, longer, within, exempt := e.localAccount(), e.localAccount(), e.localAccount(), e.localAccount()
	e.setExempt(exempt, true)
	post := func(author string) content.Status { return e.post(t, author, content.VisibilityPublic) }
	oldOf := map[string]content.Status{disabled: post(disabled), longer: post(longer), within: post(within), exempt: post(exempt)}
	bookmarked := post(longer)
	if err := e.svc.Bookmark(ctx, longer, bookmarked.ID); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(40 * 24 * time.Hour)
	e.save(t, content.AutoDeletePolicy{AccountID: longer, Enabled: true, MinAgeDays: 90, KeepBookmarked: true})
	e.save(t, content.AutoDeletePolicy{AccountID: within, Enabled: true, MinAgeDays: 7, KeepPinned: true})
	e.setLimit(autoDeleteDefaultMaximum())
	s := e.service(0, 0)

	// The view shows the maximum to the accounts it holds, and whether it binds them.
	view, err := s.AutoDeletePolicy(ctx, longer)
	if err != nil || view.Limit == nil || !view.Limit.Bound || view.Limit.Effective.MinAgeDays != 30 || view.Limit.Effective.KeepBookmarked || !view.Limit.EnforceFrom.IsZero() {
		t.Errorf("view of the longer policy before the run = %+v, %v", view.Limit, err)
	}
	if view, err := s.AutoDeletePolicy(ctx, exempt); err != nil || view.Limit != nil {
		t.Errorf("view of the exempt account = %+v, %v; want no maximum", view.Limit, err)
	}
	if view, err := s.AutoDeletePolicy(ctx, within); err != nil || view.Limit == nil || view.Limit.Bound {
		t.Errorf("view of the policy within the maximum = %+v, %v; want unbound", view.Limit, err)
	}

	// The first run notices the bound accounts and deletes only by the policy within the maximum.
	now := e.clock.Now()
	if n := e.runAll(t, s); n != 1 || !e.deleted(t, oldOf[within].ID) {
		t.Errorf("the first run deleted %d posts, want the old post of the policy within the maximum", n)
	}
	notices := e.takeNotices()
	slices.SortFunc(notices, func(a, b content.AutoDeleteLimitNotice) int { return strings.Compare(a.AccountID, b.AccountID) })
	if len(notices) != 2 || notices[0].AccountID != min(disabled, longer) || notices[1].AccountID != max(disabled, longer) {
		t.Fatalf("notices = %+v, want one to each bound account", notices)
	}
	for _, n := range notices {
		if n.MaxAgeDays != 30 || !n.NoticedAt.Equal(now) || !n.EnforceFrom.Equal(now.Add(10*24*time.Hour)) {
			t.Errorf("notice = %+v, want 30 days enforced 10 days from now", n)
		}
		if got, err := e.repo.FindAutoDeleteLimit(ctx, n.AccountID); err != nil || got.ID != n.ID || !got.EnforceFrom.Equal(n.EnforceFrom) {
			t.Errorf("notice in effect of %s = %+v, %v; want %+v", n.AccountID, got, err, n)
		}
	}
	if found, err := s.LimitNotices(ctx, longer, []string{notices[0].ID, notices[1].ID}); err != nil || len(found) != 1 {
		t.Errorf("LimitNotices of one account = %+v, %v; want its own only", found, err)
	}
	if view, err := s.AutoDeletePolicy(ctx, longer); err != nil || view.Limit == nil || !view.Limit.EnforceFrom.Equal(now.Add(10*24*time.Hour)) {
		t.Errorf("view of the bound account after the notice = %+v, %v", view.Limit, err)
	}
	for _, id := range []string{disabled, longer, exempt} {
		if e.deleted(t, oldOf[id].ID) {
			t.Errorf("the post of %s went during the grace period or while exempt", id)
		}
	}
	// A run during the grace period tells nobody again.
	e.clock.Advance(9 * 24 * time.Hour)
	if n := e.runAll(t, s); n != 0 || len(e.takeNotices()) != 0 {
		t.Errorf("a run in the grace period deleted %d posts or noticed again", n)
	}
	// Past the grace period the effective policies apply: the bookmark keeps nothing any more.
	e.clock.Advance(24 * time.Hour)
	if n := e.runAll(t, s); n != 3 || len(e.takeNotices()) != 0 {
		t.Errorf("the run after the grace period deleted %d posts, want 3", n)
	}
	for _, st := range []content.Status{oldOf[disabled], oldOf[longer], bookmarked} {
		if !e.deleted(t, st.ID) {
			t.Errorf("post %s is kept past the grace period", st.ID)
		}
	}
	if e.deleted(t, oldOf[exempt].ID) {
		t.Error("the post of the account with keep_posts_longer went")
	}

	// A policy within the maximum ends the binding: the next run removes the notice in effect.
	e.save(t, content.AutoDeletePolicy{AccountID: longer, Enabled: true, MinAgeDays: 20})
	e.runAll(t, s)
	if _, err := e.repo.FindAutoDeleteLimit(ctx, longer); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("notice in effect after the policy came within the maximum: %v, want ErrNotFound", err)
	}
	// Turning the maximum off removes every notice in effect.
	e.setLimit(content.AutoDeleteLimit{})
	e.runAll(t, s)
	if _, err := e.repo.FindAutoDeleteLimit(ctx, disabled); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("notice in effect after the maximum went: %v, want ErrNotFound", err)
	}
	if view, err := s.AutoDeletePolicy(ctx, disabled); err != nil || view.Limit != nil {
		t.Errorf("view without a maximum = %+v, %v", view.Limit, err)
	}
}

// autoDeleteRefusesOverMaximum checks ADR-0151 §3: under the maximum, a policy that is disabled,
// longer, or uses an exception that the maximum does not allow is refused, but not for an account
// with keep_posts_longer; a preview counts what the effective policy deletes.
func autoDeleteRefusesOverMaximum(t *testing.T, e *autoDeleteEnv) {
	ctx := t.Context()
	alice, exempt := e.localAccount(), e.localAccount()
	e.setExempt(exempt, true)
	e.setLimit(autoDeleteDefaultMaximum())
	s := e.service(0, 0)
	for _, p := range []content.AutoDeletePolicy{
		{MinAgeDays: 7},
		{Enabled: true, MinAgeDays: 31},
		{Enabled: true, MinAgeDays: 7, KeepMedia: true},
		{Enabled: true, MinAgeDays: 7, KeepVisibilities: []content.Visibility{content.VisibilityDirect}},
	} {
		if _, err := s.UpdateAutoDeletePolicy(ctx, alice, p); !errors.Is(err, content.ErrAutoDeleteOverLimit) {
			t.Errorf("UpdateAutoDeletePolicy(%+v) = %v, want ErrAutoDeleteOverLimit", p, err)
		}
		if _, err := s.UpdateAutoDeletePolicy(ctx, exempt, p); err != nil {
			t.Errorf("UpdateAutoDeletePolicy(%+v) with keep_posts_longer = %v", p, err)
		}
	}
	if v, err := s.UpdateAutoDeletePolicy(ctx, alice, content.AutoDeletePolicy{Enabled: true, MinAgeDays: 30, KeepPinned: true}); err != nil || v.Limit == nil || v.Limit.Bound {
		t.Errorf("UpdateAutoDeletePolicy within the maximum = %+v, %v", v, err)
	}

	old := e.post(t, alice, content.VisibilityDirect)
	e.clock.Advance(40 * 24 * time.Hour)
	// The draft keeps direct posts and posts for 90 days, which the maximum does not allow.
	draft := content.AutoDeletePolicy{MinAgeDays: 90, KeepVisibilities: []content.Visibility{content.VisibilityDirect}}
	if p, err := s.PreviewAutoDeletePolicy(ctx, alice, draft); err != nil || p.Count != 1 || p.Examples[0].ID != old.ID {
		t.Errorf("preview under the maximum = %+v, %v; want the old direct post", p, err)
	}
	if p, err := s.PreviewAutoDeletePolicy(ctx, exempt, draft); err != nil || p.Count != 0 {
		t.Errorf("preview with keep_posts_longer = %+v, %v; want nothing", p, err)
	}
}

// autoDeleteCapsPins checks the cap on the kept pins (ADR-0151 §3, §4): under a maximum that keeps
// one pin, an account with two pins of old posts is bound, and past the grace period only the
// post of its newest pin stays.
func autoDeleteCapsPins(t *testing.T, e *autoDeleteEnv) {
	ctx := t.Context()
	author := e.localAccount()
	first, second := e.post(t, author, content.VisibilityPublic), e.post(t, author, content.VisibilityPublic)
	for _, st := range []content.Status{second, first} { // first is the newest pin
		e.clock.Advance(time.Minute)
		if err := e.svc.Pin(ctx, author, st.ID); err != nil {
			t.Fatal(err)
		}
	}
	e.clock.Advance(40 * 24 * time.Hour)
	e.save(t, content.AutoDeletePolicy{AccountID: author, Enabled: true, MinAgeDays: 30, KeepPinned: true})
	limit := autoDeleteDefaultMaximum()
	limit.KeptPins = 1
	e.setLimit(limit)
	s := e.service(0, 0)
	if n := e.runAll(t, s); n != 0 || len(e.takeNotices()) != 1 {
		t.Errorf("the first run deleted %d posts; want none and a notice", n)
	}
	e.clock.Advance(10 * 24 * time.Hour)
	if n := e.runAll(t, s); n != 1 || !e.deleted(t, second.ID) || e.deleted(t, first.ID) {
		t.Errorf("after the grace period deleted %d posts; want the post of the older pin only", n)
	}
	// With one pin left, the cap keeps every pin of the account: nothing binds it any more.
	e.runAll(t, s)
	if _, err := e.repo.FindAutoDeleteLimit(ctx, author); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("notice in effect with one pin: %v, want ErrNotFound", err)
	}
}

// autoDeleteLowerMaximum checks that lowering the maximum, or losing keep_posts_longer, tells a
// bound account again with a new grace period, and that raising it does not (ADR-0151 §5).
func autoDeleteLowerMaximum(t *testing.T, e *autoDeleteEnv) {
	ctx := t.Context()
	alice, bob := e.localAccount(), e.localAccount()
	e.setExempt(bob, true)
	e.setLimit(autoDeleteDefaultMaximum())
	s := e.service(0, 0)
	e.runAll(t, s)
	if got := e.takeNotices(); len(got) != 1 || got[0].AccountID != alice {
		t.Fatalf("notices = %+v, want one to alice", got)
	}
	raised := autoDeleteDefaultMaximum()
	raised.MaxAgeDays = 60
	e.setLimit(raised)
	e.runAll(t, s)
	if got := e.takeNotices(); len(got) != 0 {
		t.Errorf("raising the maximum noticed %+v", got)
	}
	lowered := autoDeleteDefaultMaximum()
	lowered.MaxAgeDays = 14
	e.setLimit(lowered)
	e.clock.Advance(time.Hour)
	e.runAll(t, s)
	got := e.takeNotices()
	if len(got) != 1 || got[0].AccountID != alice || got[0].MaxAgeDays != 14 || !got[0].EnforceFrom.Equal(e.clock.Now().Add(10*24*time.Hour)) {
		t.Errorf("lowering the maximum noticed %+v, want a new notice of 14 days to alice", got)
	}
	if n, err := e.repo.FindAutoDeleteLimit(ctx, alice); err != nil || n.MaxAgeDays != 14 {
		t.Errorf("notice in effect = %+v, %v; want the new one", n, err)
	}
	// bob loses keep_posts_longer: bound from now, after a grace period of his own.
	e.setExempt(bob, false)
	e.runAll(t, s)
	if got := e.takeNotices(); len(got) != 1 || got[0].AccountID != bob {
		t.Errorf("notices after bob lost the permission = %+v, want one to bob", got)
	}
}
