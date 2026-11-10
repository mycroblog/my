// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// DraftRepositoryHarness is one database under test: its UnitOfWork, the drafts, and the media
// files they attach.
type DraftRepositoryHarness struct {
	UnitOfWork uow.UnitOfWork
	Drafts     appcontent.DraftRepository
	Media      appcontent.MediaRepository
}

// DraftRepositoryFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
type DraftRepositoryFactory func(t *testing.T, opts uow.Options) DraftRepositoryHarness

// DraftRepositoryContract verifies an adapter's DraftRepository (data-model §4.4, ADR-0156): a
// save replaces the whole content; the schedule in effect is one per draft and one of two
// concurrent deletions of it wins; the counts of the limits; the outcomes; deleting a draft deletes
// every row of it; the projection rebuilds; and a file that a draft attaches counts as attached
// for the cleanup of unattached uploads until the draft goes.
func DraftRepositoryContract(t *testing.T, newHarness DraftRepositoryFactory) {
	t.Helper()
	at := time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)
	ids := testkit.NewIDGenerator(testkit.NewClock(at))
	setup := func(t *testing.T) DraftRepositoryHarness {
		return newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}})
	}
	tx := func(t *testing.T, h DraftRepositoryHarness, fn func(tx uow.Tx) error) {
		t.Helper()
		if err := h.UnitOfWork.Tx(t.Context(), fn); err != nil {
			t.Fatal(err)
		}
	}
	newDraft := func(t *testing.T, h DraftRepositoryHarness, account string, c content.DraftContent) content.Draft {
		t.Helper()
		d := content.Draft{ID: ids.NewID(), AccountID: account, CreatedAt: at}
		c.DraftID = d.ID
		if c.Visibility == "" {
			c.Visibility = content.VisibilityPublic
		}
		if c.UpdatedAt.IsZero() {
			c.UpdatedAt = at
		}
		tx(t, h, func(tx uow.Tx) error {
			if err := h.Drafts.InsertDraft(t.Context(), tx, d); err != nil {
				return err
			}
			return h.Drafts.SaveDraftContent(t.Context(), tx, c)
		})
		return d
	}
	schedule := func(t *testing.T, h DraftRepositoryHarness, d content.Draft, publishAt time.Time) content.DraftSchedule {
		t.Helper()
		s := content.DraftSchedule{ID: ids.NewID(), DraftID: d.ID, PublishAt: publishAt, CreatedAt: at}
		tx(t, h, func(tx uow.Tx) error {
			if err := h.Drafts.InsertDraftSchedule(t.Context(), tx, s); err != nil {
				return err
			}
			return h.Drafts.InsertScheduledDraft(t.Context(), tx, content.ScheduledDraft{
				DraftID: d.ID, ScheduleID: s.ID, AccountID: d.AccountID, PublishAt: publishAt,
			})
		})
		return s
	}

	t.Run("SavesAndReplacesTheContent", func(t *testing.T) {
		t.Parallel()
		h := setup(t)
		ctx := t.Context()
		account := ids.NewID()
		full := content.DraftContent{
			Text: "hello", SpoilerText: "cw", Sensitive: true, Language: "ja", Visibility: content.VisibilityPrivate,
			InReplyToID: ids.NewID(), QuotedStatusID: ids.NewID(),
			InteractionPolicy: map[content.InteractionKind]content.InteractionSetting{
				content.InteractionReply: {Automatic: content.AudienceFollowers, Manual: content.AudienceEveryone},
				content.InteractionQuote: {Automatic: content.AudienceNobody},
			},
			Attachments: []content.DraftAttachment{{Position: 0, MediaID: ids.NewID(), Description: "a cat"}, {Position: 1, MediaID: ids.NewID()}},
			UpdatedAt:   at,
		}
		d := newDraft(t, h, account, full)
		full.DraftID = d.ID
		got, err := h.Drafts.FindDraftContent(ctx, d.ID)
		if err != nil || !reflect.DeepEqual(got, full) {
			t.Errorf("FindDraftContent = %+v, %v; want %+v", got, err, full)
		}
		if got, err := h.Drafts.FindDraft(ctx, d.ID); err != nil || got != d {
			t.Errorf("FindDraft = %+v, %v; want %+v", got, err, d)
		}
		poll := content.DraftContent{
			DraftID: d.ID, Visibility: content.VisibilityPublic, UpdatedAt: at.Add(time.Second),
			Poll: &content.DraftPoll{Options: []string{"yes", "no"}, Duration: time.Hour, Multiple: true},
		}
		tx(t, h, func(tx uow.Tx) error {
			if _, err := h.Drafts.LockDraft(ctx, tx, d.ID); err != nil {
				return err
			}
			return h.Drafts.SaveDraftContent(ctx, tx, poll)
		})
		if got, err := h.Drafts.FindDraftContent(ctx, d.ID); err != nil || !reflect.DeepEqual(got, poll) {
			t.Errorf("FindDraftContent after a save = %+v, %v; want %+v alone", got, err, poll)
		}
		if _, err := h.Drafts.FindDraft(ctx, ids.NewID()); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("FindDraft of an unknown draft = %v, want ErrNotFound", err)
		}
	})

	t.Run("KeepsOneScheduleAndCountsTheLimits", func(t *testing.T) {
		t.Parallel()
		h := setup(t)
		ctx := t.Context()
		account, other := ids.NewID(), ids.NewID()
		a := newDraft(t, h, account, content.DraftContent{Text: "a"})
		b := newDraft(t, h, account, content.DraftContent{Text: "b"})
		newDraft(t, h, account, content.DraftContent{Text: "c"})
		newDraft(t, h, other, content.DraftContent{Text: "other"})
		day := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
		sa := schedule(t, h, a, day.Add(23*time.Hour))
		schedule(t, h, b, day.Add(25*time.Hour))
		tx(t, h, func(tx uow.Tx) error {
			if n, err := h.Drafts.CountUnscheduledDrafts(ctx, tx, account); err != nil || n != 1 {
				t.Errorf("CountUnscheduledDrafts = %d, %v; want 1", n, err)
			}
			if n, err := h.Drafts.CountScheduledDrafts(ctx, tx, account, time.Time{}, time.Time{}); err != nil || n != 2 {
				t.Errorf("CountScheduledDrafts = %d, %v; want 2", n, err)
			}
			if n, err := h.Drafts.CountScheduledDrafts(ctx, tx, account, day, day.AddDate(0, 0, 1)); err != nil || n != 1 {
				t.Errorf("CountScheduledDrafts of a day = %d, %v; want 1", n, err)
			}
			return nil
		})
		// A second schedule in effect for one draft is refused.
		err := h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return h.Drafts.InsertScheduledDraft(ctx, tx, content.ScheduledDraft{DraftID: a.ID, ScheduleID: ids.NewID(), AccountID: account, PublishAt: day})
		})
		if err == nil {
			t.Error("a second schedule in effect of one draft was stored")
		}
		yes, no := true, false
		for name, tc := range map[string]struct {
			scheduled *bool
			want      int
		}{"all": {nil, 3}, "scheduled": {&yes, 2}, "unscheduled": {&no, 1}} {
			if got, err := h.Drafts.ListDrafts(ctx, account, "", tc.scheduled, 10); err != nil || len(got) != tc.want {
				t.Errorf("ListDrafts(%s) = %d drafts, %v; want %d", name, len(got), err, tc.want)
			}
		}
		page, err := h.Drafts.ListDrafts(ctx, account, "", nil, 2)
		if err != nil || len(page) != 2 || page[0].ID < page[1].ID {
			t.Fatalf("ListDrafts first page = %+v, %v; want two, newest first", page, err)
		}
		if rest, err := h.Drafts.ListDrafts(ctx, account, page[1].ID, nil, 2); err != nil || len(rest) != 1 || rest[0].ID != a.ID {
			t.Errorf("ListDrafts after %s = %+v, %v; want the oldest", page[1].ID, rest, err)
		}

		// Of two concurrent deletions of the schedule in effect, one alone wins.
		var wins int
		var mu sync.Mutex
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
					ok, err := h.Drafts.DeleteScheduledDraft(ctx, tx, sa.ID)
					if ok {
						mu.Lock()
						wins++
						mu.Unlock()
					}
					return err
				})
				if err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if wins != 1 {
			t.Errorf("concurrent DeleteScheduledDraft won %d times, want 1", wins)
		}
		if _, err := h.Drafts.FindScheduledDraftBySchedule(ctx, sa.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("FindScheduledDraftBySchedule after the deletion = %v, want ErrNotFound", err)
		}
	})

	t.Run("RecordsOutcomesAndRebuilds", func(t *testing.T) {
		t.Parallel()
		h := setup(t)
		ctx := t.Context()
		account := ids.NewID()
		d := newDraft(t, h, account, content.DraftContent{Text: "x"})
		first := schedule(t, h, d, at.Add(time.Hour))
		failed := content.DraftScheduleOutcome{ScheduleID: first.ID, Outcome: content.DraftFailed, FailureReason: content.DraftFailureRejected, OccurredAt: at.Add(time.Hour)}
		tx(t, h, func(tx uow.Tx) error {
			if _, err := h.Drafts.DeleteScheduledDraft(ctx, tx, first.ID); err != nil {
				return err
			}
			return h.Drafts.InsertDraftScheduleOutcome(ctx, tx, failed)
		})
		last, err := h.Drafts.LatestDraftScheduleOutcome(ctx, d.ID)
		if err != nil || last.Schedule != first || last.Outcome != failed {
			t.Errorf("LatestDraftScheduleOutcome = %+v, %v; want %+v with %+v", last, err, first, failed)
		}
		second := schedule(t, h, d, at.Add(2*time.Hour))
		recs, err := h.Drafts.DraftSchedules(ctx, []string{first.ID, second.ID})
		if err != nil || recs[first.ID].Outcome != failed || recs[second.ID].Outcome.Outcome != "" || len(recs) != 2 {
			t.Errorf("DraftSchedules = %+v, %v; want the failed one and the one in effect", recs, err)
		}
		tx(t, h, func(tx uow.Tx) error { return h.Drafts.RebuildScheduledDrafts(ctx, tx) })
		if cur, err := h.Drafts.FindScheduledDraft(ctx, d.ID); err != nil || cur.ScheduleID != second.ID || !cur.PublishAt.Equal(second.PublishAt) || cur.AccountID != account {
			t.Errorf("FindScheduledDraft after the rebuild = %+v, %v; want %+v", cur, err, second)
		}
		tx(t, h, func(tx uow.Tx) error {
			ok, err := h.Drafts.DeleteDraft(ctx, tx, d.ID)
			if !ok && err == nil {
				t.Error("DeleteDraft reported no draft")
			}
			return err
		})
		if _, err := h.Drafts.LatestDraftScheduleOutcome(ctx, d.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("LatestDraftScheduleOutcome after DeleteDraft = %v, want ErrNotFound", err)
		}
		if recs, err := h.Drafts.DraftSchedules(ctx, []string{first.ID, second.ID}); err != nil || len(recs) != 0 {
			t.Errorf("DraftSchedules after DeleteDraft = %+v, %v; want none", recs, err)
		}
		if _, err := h.Drafts.FindScheduledDraft(ctx, d.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("FindScheduledDraft after DeleteDraft = %v, want ErrNotFound", err)
		}
		if _, err := h.Drafts.FindDraftContent(ctx, d.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("FindDraftContent after DeleteDraft = %v, want ErrNotFound", err)
		}
	})

	t.Run("CountsDraftMediaAsAttached", func(t *testing.T) {
		t.Parallel()
		h := setup(t)
		ctx := t.Context()
		account := ids.NewID()
		media := ids.NewID()
		tx(t, h, func(tx uow.Tx) error {
			return h.Media.InsertMediaFile(ctx, tx, content.MediaFile{ID: media, OwnerAccountID: account, Origin: content.MediaOriginUpload, CreatedAt: at})
		})
		d := newDraft(t, h, account, content.DraftContent{Text: "x", Attachments: []content.DraftAttachment{{MediaID: media}}})
		unattached := func() []string {
			t.Helper()
			got, err := h.Media.ListUnattachedUploads(ctx, "", at.Add(48*time.Hour), 10)
			if err != nil {
				t.Fatal(err)
			}
			return got
		}
		if got := unattached(); len(got) != 0 {
			t.Errorf("ListUnattachedUploads = %v, want none: a draft attaches the file", got)
		}
		owned, err := h.Media.ListOwnerUploads(ctx, account, "", 10)
		if err != nil || len(owned) != 1 || !owned[0].Attached {
			t.Errorf("ListOwnerUploads = %+v, %v; want the file attached", owned, err)
		}
		tx(t, h, func(tx uow.Tx) error {
			if deleted, err := h.Media.DeleteUnattachedMedia(ctx, tx, media); err != nil || deleted {
				t.Errorf("DeleteUnattachedMedia of a draft's file = %v, %v; want it kept", deleted, err)
			}
			return nil
		})
		tx(t, h, func(tx uow.Tx) error { _, err := h.Drafts.DeleteDraft(ctx, tx, d.ID); return err })
		if got := unattached(); !reflect.DeepEqual(got, []string{media}) {
			t.Errorf("ListUnattachedUploads after the draft went = %v, want [%s]", got, media)
		}
	})
}
