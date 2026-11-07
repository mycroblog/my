// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// PostingContract verifies the repository methods of the content module that the posting use
// cases rely on and StatusRepositoryContract does not check, on a database adapter, by those
// methods alone, the port contract of ADR-0153 §1: one Idempotency-Key per author, also when
// transactions use it concurrently (ADR-0153 §5); IsDeleted in the transaction of a deletion; the
// sensitivity overrides of moderation actions and hook decisions, each source ending only its
// own, and their rebuild; the local post counters, gone at zero; LockStatusKeys in a
// transaction; and the media files and the newest processing result that an attachment is
// checked against. The posting use cases on top of it are PostingFlows, which
// internal/flowtest/content runs once on SQLite.
func PostingContract(t *testing.T, newHarness PostingFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *postingRepo)
	}{
		{"KeepsOneIdempotencyKeyPerAuthor", postingRepoIdempotencyKeys},
		{"AgreesOnConcurrentIdempotencyKeys", postingRepoConcurrentKeys},
		{"ReportsDeletionsInATransaction", postingRepoIsDeleted},
		{"KeepsSensitivityOverridesPerSource", postingRepoSensitivity},
		{"CountsLocalPosts", postingRepoLocalPosts},
		{"LocksStatusKeys", postingRepoLockKeys},
		{"FindsMediaAndTheirLatestResult", postingRepoMedia},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectors := &uow.Projectors{}
			h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
			appcontent.RegisterProjectors(projectors, h.Statuses)
			clock := NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			s := &statusEnv{h: StatusHarness{UnitOfWork: h.UnitOfWork, Statuses: h.Statuses}, clock: clock, ids: NewIDGenerator(clock)}
			tc.run(t, &postingRepo{statusEnv: s, media: h.Media})
		})
	}
}

// postingRepo writes the rows of statuses as statusEnv does, with the media repository beside it.
type postingRepo struct {
	*statusEnv
	media appcontent.MediaRepository
}

func (r *postingRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

func postingRepoIdempotencyKeys(t *testing.T, r *postingRepo) {
	ctx := t.Context()
	alice, bob := r.ids.NewID(), r.ids.NewID()
	s := r.newStatus(alice, content.VisibilityPublic)
	local := content.LocalStatus{StatusID: s.ID, AuthorAccountID: alice, ClientID: r.ids.NewID(), IdempotencyKey: "key-1"}
	if err := r.create(t, s, &local, r.firstRevision(s)); err != nil {
		t.Fatalf("create = %v", err)
	}
	if got, err := r.h.Statuses.FindLocalStatusByIdempotencyKey(ctx, alice, "key-1"); err != nil || got != local {
		t.Errorf("FindLocalStatusByIdempotencyKey = %+v, %v; want %+v", got, err, local)
	}
	if _, err := r.h.Statuses.FindLocalStatusByIdempotencyKey(ctx, bob, "key-1"); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindLocalStatusByIdempotencyKey(another author) = %v, want ErrNotFound", err)
	}
	// Another author may use the same key; the same author may not.
	other := r.newStatus(bob, content.VisibilityPublic)
	if err := r.create(t, other, &content.LocalStatus{StatusID: other.ID, AuthorAccountID: bob, IdempotencyKey: "key-1"}, r.firstRevision(other)); err != nil {
		t.Errorf("create with another author's key = %v", err)
	}
	again := r.newStatus(alice, content.VisibilityPublic)
	err := r.create(t, again, &content.LocalStatus{StatusID: again.ID, AuthorAccountID: alice, IdempotencyKey: "key-1"}, r.firstRevision(again))
	if !errors.Is(err, appcontent.ErrIdempotencyKeyUsed) {
		t.Errorf("create with a used key = %v, want ErrIdempotencyKeyUsed", err)
	}
	// Statuses without a key never collide.
	for range 2 {
		s := r.newStatus(alice, content.VisibilityPublic)
		if err := r.create(t, s, &content.LocalStatus{StatusID: s.ID, AuthorAccountID: alice}, r.firstRevision(s)); err != nil {
			t.Errorf("create without a key = %v", err)
		}
	}
}

// postingRepoConcurrentKeys inserts statuses with one Idempotency-Key of one author from several
// transactions at once: one commits, and the others fail with ErrIdempotencyKeyUsed.
func postingRepoConcurrentKeys(t *testing.T, r *postingRepo) {
	alice := r.ids.NewID()
	var (
		mu       sync.Mutex
		created  []string
		wg       sync.WaitGroup
		statuses []content.Status
	)
	for range 4 {
		statuses = append(statuses, r.newStatus(alice, content.VisibilityPublic))
	}
	for _, s := range statuses {
		wg.Go(func() {
			err := r.create(t, s, &content.LocalStatus{StatusID: s.ID, AuthorAccountID: alice, IdempotencyKey: "same"}, r.firstRevision(s))
			switch {
			case err == nil:
				mu.Lock()
				created = append(created, s.ID)
				mu.Unlock()
			case !errors.Is(err, appcontent.ErrIdempotencyKeyUsed):
				t.Errorf("create = %v, want nil or ErrIdempotencyKeyUsed", err)
			}
		})
	}
	wg.Wait()
	got, err := r.h.Statuses.FindLocalStatusByIdempotencyKey(t.Context(), alice, "same")
	if len(created) != 1 || err != nil || got.StatusID != created[0] {
		t.Errorf("created %v; FindLocalStatusByIdempotencyKey = %+v, %v; want the one created", created, got, err)
	}
}

func postingRepoIsDeleted(t *testing.T, r *postingRepo) {
	s := r.newStatus(r.ids.NewID(), content.VisibilityPublic)
	r.mustCreate(t, s)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if deleted, err := r.h.Statuses.IsDeleted(ctx, tx, s.ID); err != nil || deleted {
			t.Errorf("IsDeleted before the deletion = %t, %v; want false", deleted, err)
		}
		return nil
	})
	d, err := r.remove(t, s)
	if err != nil {
		t.Fatalf("remove = %v", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if deleted, err := r.h.Statuses.IsDeleted(ctx, tx, s.ID); err != nil || !deleted {
			t.Errorf("IsDeleted after the deletion = %t, %v; want true", deleted, err)
		}
		if deleted, err := r.h.Statuses.IsDeleted(ctx, tx, r.ids.NewID()); err != nil || deleted {
			t.Errorf("IsDeleted(unknown) = %t, %v; want false", deleted, err)
		}
		return nil
	})
	if _, err := r.remove(t, s); !errors.Is(err, appcontent.ErrAlreadyDeleted) {
		t.Errorf("a second deletion = %v, want ErrAlreadyDeleted", err)
	}
	if got, err := r.h.Statuses.FindDeletion(t.Context(), s.ID); err != nil || got != d {
		t.Errorf("FindDeletion = %+v, %v; want %+v", got, err, d)
	}
}

func postingRepoSensitivity(t *testing.T, r *postingRepo) {
	ctx := t.Context()
	author := r.ids.NewID()
	a, b := r.newStatus(author, content.VisibilityPublic), r.newStatus(author, content.VisibilityPublic)
	r.mustCreate(t, a)
	r.mustCreate(t, b)
	action1, action2, decision := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	overridden := func(want map[string]bool) {
		t.Helper()
		for id, w := range want {
			if got, err := r.h.Statuses.SensitivityOverridden(ctx, id); err != nil || got != w {
				t.Errorf("SensitivityOverridden(%s) = %t, %v; want %t", id, got, err, w)
			}
		}
	}
	overridden(map[string]bool{a.ID: false, b.ID: false})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for range 2 { // recording again does nothing
			if err := r.h.Statuses.InsertSensitivityOverride(ctx, tx, a.ID, action1); err != nil {
				return err
			}
			if err := r.h.Statuses.InsertSensitivityOverrideOfHookDecision(ctx, tx, a.ID, decision); err != nil {
				return err
			}
		}
		if err := r.h.Statuses.InsertSensitivityOverride(ctx, tx, a.ID, action2); err != nil {
			return err
		}
		return r.h.Statuses.InsertSensitivityOverride(ctx, tx, b.ID, action2)
	})
	overridden(map[string]bool{a.ID: true, b.ID: true})
	// Each source ends only its own rows.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Statuses.DeleteSensitivityOverridesOf(ctx, tx, action2)
	})
	overridden(map[string]bool{a.ID: true, b.ID: false})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Statuses.DeleteSensitivityOverridesOf(ctx, tx, action1)
	})
	overridden(map[string]bool{a.ID: true})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Statuses.DeleteSensitivityOverridesOfHookDecision(ctx, tx, decision)
	})
	overridden(map[string]bool{a.ID: false})

	// The rebuild empties the table, and drops the overrides of statuses that have no row.
	gone := r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Statuses.InsertSensitivityOverride(ctx, tx, gone, action1); err != nil {
			return err
		}
		return r.h.Statuses.InsertSensitivityOverride(ctx, tx, b.ID, action1)
	})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Statuses.DeleteSensitivityOverridesWithoutStatus(ctx, tx)
	})
	overridden(map[string]bool{gone: false, b.ID: true})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Statuses.DeleteAllSensitivityOverrides(ctx, tx) })
	overridden(map[string]bool{b.ID: false})
}

func postingRepoLocalPosts(t *testing.T, r *postingRepo) {
	ctx := t.Context()
	alice, bob := r.ids.NewID(), r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Statuses.AddLocalPosts(ctx, tx, alice, 2); err != nil {
			return err
		}
		return r.h.Statuses.AddLocalPosts(ctx, tx, bob, 1)
	})
	if total, err := r.h.Statuses.LocalPostsTotal(ctx); err != nil || total != 3 {
		t.Errorf("LocalPostsTotal = %d, %v; want 3", total, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Statuses.AddLocalPosts(ctx, tx, bob, -1) })
	counters, err := r.h.Statuses.ListLocalPostCounters(ctx)
	if err != nil || len(counters) != 1 || counters[0].AccountID != alice {
		t.Errorf("ListLocalPostCounters = %+v, %v; want alice's alone (bob's is gone at zero)", counters, err)
	}
	if total, err := r.h.Statuses.LocalPostsTotal(ctx); err != nil || total != 2 {
		t.Errorf("LocalPostsTotal = %d, %v; want 2", total, err)
	}
}

func postingRepoLockKeys(t *testing.T, r *postingRepo) {
	keys := []string{"account:" + r.ids.NewID(), "status:" + r.ids.NewID()}
	// Two transactions in turn take the same keys; a transaction may take them twice.
	for range 2 {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			if err := r.h.Statuses.LockStatusKeys(ctx, tx, keys); err != nil {
				return err
			}
			return r.h.Statuses.LockStatusKeys(ctx, tx, keys[:1])
		})
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Statuses.LockStatusKeys(ctx, tx, nil) })
}

func postingRepoMedia(t *testing.T, r *postingRepo) {
	ctx := t.Context()
	owner := r.ids.NewID()
	m := content.MediaFile{ID: r.ids.NewID(), OwnerAccountID: owner, Origin: content.MediaOriginUpload, CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.media.InsertMediaFile(ctx, tx, m) })
	if got, err := r.media.FindMediaFile(ctx, m.ID); err != nil || got != m {
		t.Errorf("FindMediaFile = %+v, %v; want %+v", got, err, m)
	}
	if _, err := r.media.FindMediaFile(ctx, r.ids.NewID()); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindMediaFile(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := r.media.FindLatestProcessingResult(ctx, m.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindLatestProcessingResult before processing = %v, want ErrNotFound", err)
	}
	failed := content.MediaProcessingResult{ID: r.ids.NewID(), MediaID: m.ID, Purpose: content.PurposeAttachment, Outcome: content.MediaFailed, MediaType: content.MediaTypeUnknown, ProcessedAt: r.clock.Now()}
	r.clock.Advance(time.Second)
	ready := content.MediaProcessingResult{ID: r.ids.NewID(), MediaID: m.ID, Purpose: content.PurposeAttachment, Outcome: content.MediaReady, MediaType: content.MediaTypeImage, Width: 4, Height: 3, ProcessedAt: r.clock.Now()}
	for _, res := range []content.MediaProcessingResult{failed, ready} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.media.InsertProcessingResult(ctx, tx, res) })
	}
	got, err := r.media.FindLatestProcessingResult(ctx, m.ID)
	if err != nil || got.ID != ready.ID || got.Outcome != content.MediaReady || got.Width != 4 || got.Height != 3 || !got.ProcessedAt.Equal(ready.ProcessedAt) {
		t.Errorf("FindLatestProcessingResult = %+v, %v; want the ready result", got, err)
	}
	all, err := r.media.ListProcessingResults(ctx, m.ID)
	if err != nil || len(all) != 2 || all[0].ID != failed.ID || !reflect.DeepEqual(all[1].Variants, got.Variants) {
		t.Errorf("ListProcessingResults = %+v, %v; want both, oldest first", all, err)
	}
}
