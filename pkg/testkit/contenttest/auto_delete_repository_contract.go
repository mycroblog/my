// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// AutoDeleteContract verifies the automatic deletion part of the status repository of a database
// adapter by its methods alone, the port contract of ADR-0153 §1, on statuses and interactions
// that the status and interaction repositories store: the policies saved, replaced, read back with
// their kept visibilities in the order of the visibilities, and listed while enabled in account
// order; the statuses to delete, oldest first, local, published before the cutoff, not deleted,
// and kept by none of the exceptions (visibilities, the author's pins, the newest MaxKeptPins of
// them, the author's bookmarks and favourites, media and polls of the current revision, and the
// thresholds of boosts and favourites); the pins of statuses that are not deleted; and the notices
// of the server maximum, of which one per account is in effect. The automatic deletion use cases
// on top of it are AutoDeleteFlows, which internal/flowtest/content runs once on SQLite.
func AutoDeleteContract(t *testing.T, newHarness AutoDeleteFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *autoDeleteRepo)
	}{
		{"SavesAndListsPolicies", autoDeleteRepoPolicies},
		{"ListsTheStatusesToDelete", autoDeleteRepoStatuses},
		{"KeepsOnlyTheNewestPins", autoDeleteRepoPins},
		{"KeepsTheNoticeInEffect", autoDeleteRepoNotices},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var h AutoDeleteHarness
			se := newStatusEnv(t, func(t *testing.T, opts uow.Options) StatusHarness {
				h = newHarness(t, opts)
				return StatusHarness{UnitOfWork: h.UnitOfWork, Statuses: h.Statuses}
			})
			tc.run(t, &autoDeleteRepo{se: se, h: h, repo: h.AutoDeletes})
		})
	}
}

type autoDeleteRepo struct {
	se   *StatusEnv
	h    AutoDeleteHarness
	repo appcontent.AutoDeleteRepository
}

func (r *autoDeleteRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

func (r *autoDeleteRepo) id() string { return r.se.IDs.NewID() }

func (r *autoDeleteRepo) save(t *testing.T, p content.AutoDeletePolicy) content.AutoDeletePolicy {
	t.Helper()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.SaveAutoDeletePolicy(ctx, tx, p) })
	n, err := p.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// status stores a status of author a second after the previous one, with rev changed by edit.
func (r *autoDeleteRepo) status(t *testing.T, author string, v content.Visibility, local bool, edit func(*content.Revision)) content.Status {
	t.Helper()
	r.se.Clock.Advance(time.Second)
	s := r.se.NewStatus(author, v)
	s.IsLocal = local
	rev := r.se.FirstRevision(s)
	if edit != nil {
		edit(&rev)
	}
	if err := r.se.Create(t, s, nil, rev); err != nil {
		t.Fatalf("create status: %v", err)
	}
	return s
}

func autoDeleteRepoPolicies(t *testing.T, r *autoDeleteRepo) {
	ctx := t.Context()
	ids := []string{r.id(), r.id(), r.id(), r.id()}
	if _, err := r.repo.FindAutoDeletePolicy(ctx, ids[0]); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindAutoDeletePolicy(never saved) = %v, want ErrNotFound", err)
	}
	now := r.se.Clock.Now()
	first := r.save(t, content.AutoDeletePolicy{
		AccountID: ids[0], Enabled: true, MinAgeDays: 30, KeepPinned: true, KeepBookmarked: true, KeepFavourited: true,
		KeepMedia: true, KeepPolls: true, KeepMinBoosts: 5, KeepMinFavourites: 7,
		KeepVisibilities: []content.Visibility{content.VisibilityDirect, content.VisibilityPrivate, content.VisibilityPublic}, UpdatedAt: now,
	})
	if got, err := r.repo.FindAutoDeletePolicy(ctx, ids[0]); err != nil || !reflect.DeepEqual(got, first) {
		t.Errorf("FindAutoDeletePolicy = %+v, %v; want %+v", got, err, first)
	}
	// Saving again replaces the policy and its kept visibilities.
	r.se.Clock.Advance(time.Second)
	replaced := r.save(t, content.AutoDeletePolicy{
		AccountID: ids[0], Enabled: true, MinAgeDays: 7, KeepVisibilities: []content.Visibility{content.VisibilityUnlisted}, UpdatedAt: r.se.Clock.Now(),
	})
	if got, err := r.repo.FindAutoDeletePolicy(ctx, ids[0]); err != nil || !reflect.DeepEqual(got, replaced) {
		t.Errorf("FindAutoDeletePolicy after a second save = %+v, %v; want %+v", got, err, replaced)
	}
	disabled := r.save(t, content.AutoDeletePolicy{AccountID: ids[1], MinAgeDays: 7, KeepVisibilities: []content.Visibility{}, UpdatedAt: now})
	if got, err := r.repo.FindAutoDeletePolicy(ctx, ids[1]); err != nil || got.Enabled || len(got.KeepVisibilities) != 0 {
		t.Errorf("FindAutoDeletePolicy(disabled) = %+v, %v; want %+v", got, err, disabled)
	}
	third := r.save(t, content.AutoDeletePolicy{AccountID: ids[2], Enabled: true, MinAgeDays: 1, KeepVisibilities: []content.Visibility{}, UpdatedAt: now})
	fourth := r.save(t, content.AutoDeletePolicy{AccountID: ids[3], Enabled: true, MinAgeDays: 2, KeepVisibilities: []content.Visibility{}, UpdatedAt: now})

	var got []content.AutoDeletePolicy
	after := ""
	for range 3 {
		page, err := r.repo.ListEnabledAutoDeletePolicies(ctx, after, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) > 2 {
			t.Fatalf("ListEnabledAutoDeletePolicies page = %d, want at most 2", len(page))
		}
		for _, p := range page {
			got, after = append(got, p), p.AccountID
		}
	}
	if want := []content.AutoDeletePolicy{replaced, third, fourth}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListEnabledAutoDeletePolicies pages = %+v, want the enabled ones in account order %+v", got, want)
	}
}

func autoDeleteRepoStatuses(t *testing.T, r *autoDeleteRepo) {
	author, other := r.id(), r.id()
	public := func() content.Status { return r.status(t, author, content.VisibilityPublic, true, nil) }
	plain := public()
	r.status(t, author, content.VisibilityPublic, false, nil) // not local
	r.status(t, other, content.VisibilityPublic, true, nil)   // another author
	private := r.status(t, author, content.VisibilityPrivate, true, nil)
	pinned, bookmarked, favourited := public(), public(), public()
	byOther := public() // bookmarked and favourited by another account only
	withMedia := r.status(t, author, content.VisibilityPublic, true, func(rev *content.Revision) {
		rev.Attachments = []content.Attachment{{Position: 0, MediaID: r.id()}}
	})
	withPoll := r.status(t, author, content.VisibilityPublic, true, func(rev *content.Revision) {
		rev.Poll = &content.Poll{ID: r.id(), Options: []content.PollOption{{Position: 0, Title: "yes"}, {Position: 1, Title: "no"}}}
	})
	boosted, liked := public(), public()
	deleted := public()
	r.se.MustRemove(t, deleted)
	cutoff := r.se.Clock.Now().Add(time.Second)
	r.se.Clock.Advance(time.Hour)
	public() // too new
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		now := r.se.Clock.Now()
		ix := r.h.Interactions
		for _, step := range []func() error{
			func() error {
				_, err := ix.InsertPin(ctx, tx, content.Pin{ID: r.id(), AccountID: author, StatusID: pinned.ID, CreatedAt: now})
				return err
			},
			func() error {
				_, err := ix.InsertBookmark(ctx, tx, content.Bookmark{ID: r.id(), AccountID: author, StatusID: bookmarked.ID, CreatedAt: now})
				return err
			},
			func() error {
				_, err := ix.InsertFavourite(ctx, tx, content.Favourite{ID: r.id(), AccountID: author, StatusID: favourited.ID, URI: "https://" + testkit.ContractHost + "/likes/1", CreatedAt: now})
				return err
			},
			func() error {
				_, err := ix.InsertBookmark(ctx, tx, content.Bookmark{ID: r.id(), AccountID: other, StatusID: byOther.ID, CreatedAt: now})
				return err
			},
			func() error {
				_, err := ix.InsertFavourite(ctx, tx, content.Favourite{ID: r.id(), AccountID: other, StatusID: byOther.ID, URI: "https://" + testkit.ContractHost + "/likes/2", CreatedAt: now})
				return err
			},
			func() error { return ix.AddBoosts(ctx, tx, boosted.ID, 3) },
			func() error { return ix.AddFavourites(ctx, tx, liked.ID, 3) },
		} {
			if err := step(); err != nil {
				return err
			}
		}
		return nil
	})

	list := func(p content.AutoDeletePolicy, limit int) []content.Status {
		t.Helper()
		p.AccountID = author
		var out []content.Status
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			out, err = r.repo.ListStatusesToAutoDelete(ctx, tx, p, cutoff, limit)
			return err
		})
		return out
	}
	ids := func(ss []content.Status) []string {
		out := make([]string, len(ss))
		for i, s := range ss {
			out[i] = s.ID
		}
		return out
	}
	everything := []content.Status{plain, private, pinned, bookmarked, favourited, byOther, withMedia, withPoll, boosted, liked}
	if got := list(content.AutoDeletePolicy{}, 100); !slices.Equal(ids(got), ids(everything)) {
		t.Errorf("ListStatusesToAutoDelete(no exceptions) = %v, want the old local live posts of the author oldest first %v", ids(got), ids(everything))
	} else if !reflect.DeepEqual(got[0], plain) {
		t.Errorf("ListStatusesToAutoDelete()[0] = %+v, want %+v", got[0], plain)
	}
	if got := list(content.AutoDeletePolicy{}, 3); !slices.Equal(ids(got), ids(everything[:3])) {
		t.Errorf("ListStatusesToAutoDelete(limit 3) = %v, want %v", ids(got), ids(everything[:3]))
	}
	for _, tc := range []struct {
		name   string
		policy content.AutoDeletePolicy
		kept   content.Status
	}{
		{"visibilities", content.AutoDeletePolicy{KeepVisibilities: []content.Visibility{content.VisibilityPrivate}}, private},
		{"pinned", content.AutoDeletePolicy{KeepPinned: true}, pinned},
		{"bookmarked", content.AutoDeletePolicy{KeepBookmarked: true}, bookmarked},
		{"favourited", content.AutoDeletePolicy{KeepFavourited: true}, favourited},
		{"media", content.AutoDeletePolicy{KeepMedia: true}, withMedia},
		{"polls", content.AutoDeletePolicy{KeepPolls: true}, withPoll},
		{"boosts", content.AutoDeletePolicy{KeepMinBoosts: 3}, boosted},
		{"favourites", content.AutoDeletePolicy{KeepMinFavourites: 3}, liked},
	} {
		want := slices.DeleteFunc(slices.Clone(everything), func(s content.Status) bool { return s.ID == tc.kept.ID })
		if got := list(tc.policy, 100); !slices.Equal(ids(got), ids(want)) {
			t.Errorf("ListStatusesToAutoDelete(keep %s) = %v, want %v", tc.name, ids(got), ids(want))
		}
	}
	// A threshold above the count keeps nothing.
	if got := list(content.AutoDeletePolicy{KeepMinBoosts: 4, KeepMinFavourites: 4}, 100); len(got) != len(everything) {
		t.Errorf("ListStatusesToAutoDelete(thresholds above the counts) = %v, want every post", ids(got))
	}
}

func autoDeleteRepoPins(t *testing.T, r *autoDeleteRepo) {
	author := r.id()
	var statuses []content.Status
	for range 4 {
		statuses = append(statuses, r.status(t, author, content.VisibilityPublic, true, nil))
	}
	// Pin the first three, the third newest; the pin of a deleted status does not count.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, s := range statuses[:3] {
			r.se.Clock.Advance(time.Second)
			if _, err := r.h.Interactions.InsertPin(ctx, tx, content.Pin{ID: r.id(), AccountID: author, StatusID: s.ID, CreatedAt: r.se.Clock.Now()}); err != nil {
				return err
			}
		}
		return nil
	})
	cutoff := r.se.Clock.Now().Add(time.Second)
	count := func() int {
		t.Helper()
		var n int
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			n, err = r.repo.CountAutoDeletePins(ctx, tx, author)
			return err
		})
		return n
	}
	if n := count(); n != 3 {
		t.Errorf("CountAutoDeletePins = %d, want 3", n)
	}
	list := func(maxKept int) []string {
		t.Helper()
		var out []string
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ss, err := r.repo.ListStatusesToAutoDelete(ctx, tx, content.AutoDeletePolicy{AccountID: author, KeepPinned: true, MaxKeptPins: maxKept}, cutoff, 100)
			for _, s := range ss {
				out = append(out, s.ID)
			}
			return err
		})
		return out
	}
	if got, want := list(0), []string{statuses[3].ID}; !slices.Equal(got, want) {
		t.Errorf("ListStatusesToAutoDelete(every pin kept) = %v, want %v", got, want)
	}
	if got, want := list(2), []string{statuses[0].ID, statuses[3].ID}; !slices.Equal(got, want) {
		t.Errorf("ListStatusesToAutoDelete(2 pins kept) = %v, want the oldest pin's post too %v", got, want)
	}
	// Once the newest pin's status is deleted, the two pins left are the newest.
	r.se.MustRemove(t, statuses[2])
	if n := count(); n != 2 {
		t.Errorf("CountAutoDeletePins after a deletion = %d, want 2", n)
	}
	if got, want := list(2), []string{statuses[3].ID}; !slices.Equal(got, want) {
		t.Errorf("ListStatusesToAutoDelete(2 pins kept, one deleted) = %v, want %v", got, want)
	}
}

func autoDeleteRepoNotices(t *testing.T, r *autoDeleteRepo) {
	ctx := t.Context()
	alice, bob := r.id(), r.id()
	if _, err := r.repo.FindAutoDeleteLimit(ctx, alice); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindAutoDeleteLimit(none) = %v, want ErrNotFound", err)
	}
	notice := func(account string, maxAge int) content.AutoDeleteLimitNotice {
		t.Helper()
		r.se.Clock.Advance(time.Second)
		n := content.NewAutoDeleteLimitNotice(r.id(), account, content.AutoDeleteLimit{MaxAgeDays: maxAge, GraceDays: 7}, r.se.Clock.Now())
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.RecordAutoDeleteLimitNotice(ctx, tx, n) })
		return n
	}
	first := notice(alice, 90)
	second := notice(alice, 30)
	forBob := notice(bob, 30)
	// The notice in effect carries what the job needs of it, NoticedAt aside.
	inEffect := func(n content.AutoDeleteLimitNotice) content.AutoDeleteLimitNotice {
		n.NoticedAt = time.Time{}
		return n
	}
	if got, err := r.repo.FindAutoDeleteLimit(ctx, alice); err != nil || inEffect(got) != inEffect(second) {
		t.Errorf("FindAutoDeleteLimit = %+v, %v; want the newer notice %+v", got, err, second)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.repo.FindAutoDeleteLimitTx(ctx, tx, bob); err != nil || inEffect(got) != inEffect(forBob) {
			t.Errorf("FindAutoDeleteLimitTx = %+v, %v; want %+v", got, err, forBob)
		}
		return r.repo.DeleteAutoDeleteLimit(ctx, tx, alice)
	})
	if _, err := r.repo.FindAutoDeleteLimit(ctx, alice); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindAutoDeleteLimit after DeleteAutoDeleteLimit = %v, want ErrNotFound", err)
	}
	if got, err := r.repo.FindAutoDeleteLimit(ctx, bob); err != nil || inEffect(got) != inEffect(forBob) {
		t.Errorf("FindAutoDeleteLimit(bob) after alice's deletion = %+v, %v; want %+v", got, err, forBob)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.DeleteAutoDeleteLimits(ctx, tx) })
	if _, err := r.repo.FindAutoDeleteLimit(ctx, bob); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindAutoDeleteLimit after DeleteAutoDeleteLimits = %v, want ErrNotFound", err)
	}
	// The notices stay.
	got, err := r.repo.FindAutoDeleteLimitNotices(ctx, []string{forBob.ID, r.id(), first.ID, second.ID})
	if want := []content.AutoDeleteLimitNotice{first, second, forBob}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("FindAutoDeleteLimitNotices = %+v, %v; want %+v", got, err, want)
	}
}
