// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// instanceAnnouncementCases are the cases of InstanceRepositoryContract about the server-wide
// announcements (FR-726): drafts, publication, revisions, and withdrawal, the window in which
// the users see one, the list with the past ones, the times at which their notifications are
// scheduled, each user's reads and their erasure, invalid input that writes nothing, and the
// announcement_heads projection that equals a rebuild.
var instanceAnnouncementCases = []struct {
	name string
	run  func(t *testing.T, e *instanceEnv)
}{
	{"AnnouncementLifecycle", instanceAnnouncementLifecycle},
	{"AnnouncementWindow", instanceAnnouncementWindow},
	{"PastAnnouncements", instancePastAnnouncements},
	{"SchedulesAnnouncements", instanceSchedulesAnnouncements},
	{"RejectsInvalidAnnouncements", instanceRejectsInvalidAnnouncements},
	{"ErasesAnnouncementReads", instanceErasesAnnouncementReads},
	{"AnnouncementProjectionEqualsRebuild", instanceAnnouncementProjectionEqualsRebuild},
}

const (
	announcementAdmin  = "0199f0e0-0000-7000-8000-0000000000ab"
	announcementReader = "0199f0e0-0000-7000-8000-0000000000ac"
)

func (e *instanceEnv) createAnnouncement(t *testing.T, body string, publish bool, startsAt, endsAt time.Time) instance.Announcement {
	t.Helper()
	e.clock.Advance(time.Second)
	a, err := e.service.CreateAnnouncement(t.Context(), appinstance.CreateAnnouncementInput{
		AnnouncementInput: appinstance.AnnouncementInput{Title: "Title: " + instance.AnnouncementLabel("", body), Body: body, StartsAt: startsAt, EndsAt: endsAt},
		Publish:           publish, CreatedByUserID: announcementAdmin,
	})
	if err != nil {
		t.Fatalf("CreateAnnouncement(%q) = %v", body, err)
	}
	return a
}

// shown returns the bodies of the announcements that the reader sees, with "*" before the read ones.
func (e *instanceEnv) shown(t *testing.T, userID string) []string {
	t.Helper()
	list, err := e.service.ShownAnnouncements(t.Context(), userID)
	if err != nil {
		t.Fatalf("ShownAnnouncements = %v", err)
	}
	out := []string{}
	for _, a := range list {
		mark := ""
		if a.Read {
			mark = "*"
		}
		out = append(out, mark+a.Revision.Body)
	}
	return out
}

func instanceAnnouncementLifecycle(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	draft := e.createAnnouncement(t, "# Maintenance\n\nOn **Sunday**.", false, time.Time{}, time.Time{})
	if draft.State != instance.AnnouncementDraft || !draft.PublishedAt.IsZero() || draft.Revision.RevisedByUserID != announcementAdmin ||
		!draft.CreatedAt.Equal(e.clock.Now()) {
		t.Errorf("draft = %+v, want a draft by the administrator, created now", draft)
	}
	if got := e.shown(t, announcementReader); len(got) != 0 {
		t.Errorf("shown with a draft = %q, want none", got)
	}
	if _, err := e.service.MarkAnnouncementRead(ctx, announcementReader, draft.ID); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("MarkAnnouncementRead(draft) = %v, want ErrNotFound", err)
	}

	e.clock.Advance(time.Second)
	published, err := e.service.PublishAnnouncement(ctx, draft.ID, announcementAdmin)
	if err != nil {
		t.Fatalf("PublishAnnouncement = %v", err)
	}
	if published.State != instance.AnnouncementPublished || !published.PublishedAt.Equal(e.clock.Now()) {
		t.Errorf("published = %+v, want published now", published)
	}
	e.clock.Advance(time.Second)
	again, err := e.service.PublishAnnouncement(ctx, draft.ID, "")
	if err != nil || !reflect.DeepEqual(again, published) {
		t.Errorf("PublishAnnouncement again = %+v, %v; want no change %+v", again, err, published)
	}

	e.clock.Advance(time.Second)
	updated, err := e.service.UpdateAnnouncement(ctx, appinstance.UpdateAnnouncementInput{
		AnnouncementID: draft.ID, AnnouncementInput: appinstance.AnnouncementInput{Title: "Maintenance on Monday", Body: "Maintenance on Monday."},
	})
	if err != nil {
		t.Fatalf("UpdateAnnouncement = %v", err)
	}
	if updated.State != instance.AnnouncementPublished || updated.Revision.ID == draft.Revision.ID ||
		updated.Revision.Body != "Maintenance on Monday." || updated.Revision.RevisedByUserID != "" || !updated.PublishedAt.Equal(published.PublishedAt) {
		t.Errorf("updated = %+v, want a new revision that stays published", updated)
	}
	if got, err := e.service.Announcement(ctx, draft.ID); err != nil || !reflect.DeepEqual(got, updated) {
		t.Errorf("Announcement = %+v, %v\nwant %+v", got, err, updated)
	}

	if got := e.shown(t, announcementReader); !reflect.DeepEqual(got, []string{"Maintenance on Monday."}) {
		t.Errorf("shown = %q, want the published announcement, unread", got)
	}
	for range 2 {
		e.clock.Advance(time.Second)
		if r, err := e.service.MarkAnnouncementRead(ctx, announcementReader, draft.ID); err != nil || !r.Read {
			t.Fatalf("MarkAnnouncementRead = %+v, %v", r, err)
		}
	}
	if got := e.shown(t, announcementReader); !reflect.DeepEqual(got, []string{"*Maintenance on Monday."}) {
		t.Errorf("shown after reading = %q, want it read", got)
	}
	if got := e.shown(t, announcementAdmin); !reflect.DeepEqual(got, []string{"Maintenance on Monday."}) {
		t.Errorf("shown to another user = %q, want it unread", got)
	}
	if r, ok, err := e.service.ShownAnnouncement(ctx, announcementReader, draft.ID); err != nil || !ok || !r.Read {
		t.Errorf("ShownAnnouncement = %+v, %v, %v; want it shown and read", r, ok, err)
	}

	e.clock.Advance(time.Second)
	withdrawn, err := e.service.WithdrawAnnouncement(ctx, draft.ID, announcementAdmin)
	if err != nil || withdrawn.State != instance.AnnouncementWithdrawn {
		t.Fatalf("WithdrawAnnouncement = %+v, %v", withdrawn, err)
	}
	if got := e.shown(t, announcementReader); len(got) != 0 {
		t.Errorf("shown after the withdrawal = %q, want none", got)
	}
	if _, ok, err := e.service.ShownAnnouncement(ctx, announcementReader, draft.ID); ok || err != nil {
		t.Errorf("ShownAnnouncement(withdrawn) = %v, %v; want not shown", ok, err)
	}
	if _, err := e.service.PublishAnnouncement(ctx, draft.ID, ""); !errors.Is(err, instance.ErrAnnouncementWithdrawn) {
		t.Errorf("PublishAnnouncement(withdrawn) = %v, want ErrAnnouncementWithdrawn", err)
	}
	if _, err := e.service.UpdateAnnouncement(ctx, appinstance.UpdateAnnouncementInput{
		AnnouncementID: draft.ID, AnnouncementInput: appinstance.AnnouncementInput{Title: "again", Body: "again"},
	}); !errors.Is(err, instance.ErrAnnouncementWithdrawn) {
		t.Errorf("UpdateAnnouncement(withdrawn) = %v, want ErrAnnouncementWithdrawn", err)
	}
	if again, err := e.service.WithdrawAnnouncement(ctx, draft.ID, ""); err != nil || !reflect.DeepEqual(again, withdrawn) {
		t.Errorf("WithdrawAnnouncement again = %+v, %v; want no change", again, err)
	}

	unknown := "0199f0e0-0000-7000-8000-0000000000ff"
	if _, err := e.service.Announcement(ctx, unknown); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("Announcement(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := e.service.PublishAnnouncement(ctx, unknown, ""); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("PublishAnnouncement(unknown) = %v, want ErrNotFound", err)
	}
	if all, err := e.service.Announcements(ctx); err != nil || len(all) != 1 || all[0].ID != draft.ID {
		t.Errorf("Announcements = %+v, %v; want the withdrawn one", all, err)
	}
}

func instanceAnnouncementWindow(t *testing.T, e *instanceEnv) {
	start := e.clock.Now().Add(time.Hour)
	e.createAnnouncement(t, "first", true, time.Time{}, time.Time{})
	later := e.createAnnouncement(t, "later", true, start, start.Add(time.Hour))
	e.createAnnouncement(t, "second", true, time.Time{}, e.clock.Now().Add(90*time.Minute))
	if !later.Revision.StartsAt.Equal(start) || !later.Revision.EndsAt.Equal(start.Add(time.Hour)) {
		t.Errorf("window = %v..%v, want %v..%v", later.Revision.StartsAt, later.Revision.EndsAt, start, start.Add(time.Hour))
	}
	if got := e.shown(t, announcementReader); !reflect.DeepEqual(got, []string{"second", "first"}) {
		t.Errorf("shown before the window = %q, want the others, newest first", got)
	}
	e.clock.Advance(time.Hour)
	if got := e.shown(t, announcementReader); !reflect.DeepEqual(got, []string{"later", "second", "first"}) {
		t.Errorf("shown in the window = %q, want the one whose window started first", got)
	}
	e.clock.Advance(40 * time.Minute)
	if got := e.shown(t, announcementReader); !reflect.DeepEqual(got, []string{"later", "first"}) {
		t.Errorf("shown after the end of second = %q", got)
	}
	e.clock.Advance(time.Hour)
	if got := e.shown(t, announcementReader); !reflect.DeepEqual(got, []string{"first"}) {
		t.Errorf("shown after every window = %q, want first", got)
	}
}

// instancePastAnnouncements checks the list with the past announcements (ADR-0125 §3): those
// that have started to show, the ended ones included, newest ShowsFrom first, page by page,
// without the drafts, the withdrawn ones, and those whose window has not started; and that the
// title is trimmed and kept with each revision.
func instancePastAnnouncements(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	now := e.clock.Now()
	ended := e.createAnnouncement(t, "ended", true, time.Time{}, now.Add(30*time.Minute))
	e.createAnnouncement(t, "draft", false, time.Time{}, time.Time{})
	future := e.createAnnouncement(t, "future", true, now.Add(48*time.Hour), time.Time{})
	withdrawn := e.createAnnouncement(t, "withdrawn", true, time.Time{}, time.Time{})
	if _, err := e.service.WithdrawAnnouncement(ctx, withdrawn.ID, announcementAdmin); err != nil {
		t.Fatal(err)
	}
	current := e.createAnnouncement(t, "current", true, time.Time{}, time.Time{})
	e.clock.Advance(time.Hour)
	var got []string
	after := appinstance.AnnouncementCursor{}
	for range 5 {
		page, more, err := e.service.PastAnnouncements(ctx, announcementReader, after, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range page {
			label := a.Revision.Body
			if a.Ended {
				label += " (ended)"
			}
			got = append(got, label)
		}
		if !more {
			break
		}
		last := page[len(page)-1]
		after = appinstance.AnnouncementCursor{ShowsFrom: last.ShowsFrom(), ID: last.ID}
	}
	if want := []string{"current", "ended (ended)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("past announcements = %q, want %q", got, want)
	}
	if _, ok, err := e.service.ShownAnnouncement(ctx, announcementReader, ended.ID); err != nil || ok {
		t.Errorf("ShownAnnouncement(ended) = %v, %v; want not shown", ok, err)
	}
	if ids, err := e.service.ShownAnnouncementIDs(ctx); err != nil || !reflect.DeepEqual(ids, []string{current.ID}) {
		t.Errorf("ShownAnnouncementIDs = %v, %v; want only the current one", ids, err)
	}
	updated, err := e.service.UpdateAnnouncement(ctx, appinstance.UpdateAnnouncementInput{
		AnnouncementID: future.ID, AnnouncementInput: appinstance.AnnouncementInput{Title: "  Moved  ", Body: "future", StartsAt: future.Revision.StartsAt},
	})
	if err != nil || updated.Revision.Title != "Moved" {
		t.Errorf("UpdateAnnouncement(title) = %+v, %v; want the title trimmed", updated.Revision, err)
	}
}

// instanceSchedulesAnnouncements checks when an announcement is scheduled for its notifications
// (ADR-0125 §2): at its publication, at ShowsFrom, and again when a revision moves the start of
// the window before it has shown; not for a draft, nor for a revision after it has shown.
func instanceSchedulesAnnouncements(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	var scheduled []appinstance.AnnouncementScheduled
	e.projectors.Register(appinstance.EventAnnouncementScheduled, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		scheduled = append(scheduled, ev.(appinstance.AnnouncementScheduled))
		return nil
	})
	take := func() []appinstance.AnnouncementScheduled {
		out := scheduled
		scheduled = nil
		return out
	}
	start := e.clock.Now().Add(2 * time.Hour)
	draft := e.createAnnouncement(t, "draft", false, start, time.Time{})
	if got := take(); len(got) != 0 {
		t.Errorf("scheduled for a draft = %+v, want none", got)
	}
	published, err := e.service.PublishAnnouncement(ctx, draft.ID, announcementAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if got := take(); len(got) != 1 || got[0].AnnouncementID != draft.ID || !got[0].ShowsFrom.Equal(start) || !published.ShowsFrom().Equal(start) {
		t.Errorf("scheduled at the publication = %+v, want at the start of the window %v", got, start)
	}
	revise := func(title string, startsAt time.Time) {
		t.Helper()
		if _, err := e.service.UpdateAnnouncement(ctx, appinstance.UpdateAnnouncementInput{
			AnnouncementID: draft.ID, AnnouncementInput: appinstance.AnnouncementInput{Title: title, Body: "draft", StartsAt: startsAt},
		}); err != nil {
			t.Fatal(err)
		}
	}
	revise("same window", start)
	if got := take(); len(got) != 0 {
		t.Errorf("scheduled for a revision of the title = %+v, want none", got)
	}
	sooner := start.Add(-time.Hour)
	revise("sooner", sooner)
	if got := take(); len(got) != 1 || !got[0].ShowsFrom.Equal(sooner) {
		t.Errorf("scheduled for a sooner start = %+v, want at %v", got, sooner)
	}
	e.clock.Advance(90 * time.Minute)
	revise("after it showed", start)
	if got := take(); len(got) != 0 {
		t.Errorf("scheduled for a revision after it showed = %+v, want none", got)
	}
	now := e.clock.Now()
	e.createAnnouncement(t, "at once", true, time.Time{}, time.Time{})
	if got := take(); len(got) != 1 || !got[0].ShowsFrom.Equal(now.Add(time.Second)) {
		t.Errorf("scheduled for an announcement published at once = %+v, want now", got)
	}
}

func instanceRejectsInvalidAnnouncements(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	now := e.clock.Now()
	for name, in := range map[string]appinstance.AnnouncementInput{
		"blank":          {Title: "t", Body: " \n\t"},
		"too long":       {Title: "t", Body: strings.Repeat("あ", instance.MaxAnnouncementBodyLength+1)},
		"NUL":            {Title: "t", Body: "a\x00b"},
		"invalid":        {Title: "t", Body: "\xff"},
		"ends first":     {Title: "t", Body: "ok", StartsAt: now.Add(time.Hour), EndsAt: now},
		"empty window":   {Title: "t", Body: "ok", StartsAt: now, EndsAt: now},
		"no title":       {Body: "ok"},
		"blank title":    {Title: " \t", Body: "ok"},
		"long title":     {Title: strings.Repeat("あ", instance.MaxAnnouncementTitleGraphemes+1), Body: "ok"},
		"two-line title": {Title: "a\nb", Body: "ok"},
		"title with NUL": {Title: "a\x00b", Body: "ok"},
		"invalid title":  {Title: "\xff", Body: "ok"},
	} {
		if _, err := e.service.CreateAnnouncement(ctx, appinstance.CreateAnnouncementInput{AnnouncementInput: in, Publish: true}); !errors.Is(err, instance.ErrInvalidValue) {
			t.Errorf("CreateAnnouncement(%s) = %v, want ErrInvalidValue", name, err)
		}
	}
	if all, err := e.service.Announcements(ctx); err != nil || len(all) != 0 {
		t.Errorf("Announcements after invalid input = %+v, %v; want none", all, err)
	}
	a := e.createAnnouncement(t, "ok", false, time.Time{}, time.Time{})
	if _, err := e.service.UpdateAnnouncement(ctx, appinstance.UpdateAnnouncementInput{
		AnnouncementID: a.ID, AnnouncementInput: appinstance.AnnouncementInput{Title: "ok", Body: ""},
	}); !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("UpdateAnnouncement(blank) = %v, want ErrInvalidValue", err)
	}
	if got, err := e.service.Announcement(ctx, a.ID); err != nil || got.Revision.ID != a.Revision.ID {
		t.Errorf("Announcement after an invalid update = %+v, %v; want unchanged", got, err)
	}
}

func instanceErasesAnnouncementReads(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	a := e.createAnnouncement(t, "hello", true, time.Time{}, time.Time{})
	for _, user := range []string{announcementReader, announcementAdmin} {
		if _, err := e.service.MarkAnnouncementRead(ctx, user, a.ID); err != nil {
			t.Fatalf("MarkAnnouncementRead(%s) = %v", user, err)
		}
	}
	step, err := e.service.EraseAccountData(ctx, appaccount.ErasureTarget{AccountID: NamedID("account"), UserID: announcementReader}, "", 100)
	if err != nil || !step.Done {
		t.Fatalf("EraseAccountData = %+v, %v", step, err)
	}
	if got := e.shown(t, announcementReader); !reflect.DeepEqual(got, []string{"hello"}) {
		t.Errorf("shown to the erased user = %q, want it unread", got)
	}
	if got := e.shown(t, announcementAdmin); !reflect.DeepEqual(got, []string{"*hello"}) {
		t.Errorf("shown to another user = %q, want it still read", got)
	}
}

func instanceAnnouncementProjectionEqualsRebuild(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(665, 20261021)) //nolint:gosec // deterministic test input, not a secret
	var ids []string
	for round := range 3 {
		for step := range 30 {
			e.clock.Advance(time.Second)
			in := appinstance.AnnouncementInput{Title: fmt.Sprintf("title %d.%d", round, step), Body: fmt.Sprintf("announcement %d.%d", round, step)}
			var err error
			switch op := rng.IntN(5); {
			case op == 0 || len(ids) == 0:
				var a instance.Announcement
				a, err = e.service.CreateAnnouncement(ctx, appinstance.CreateAnnouncementInput{AnnouncementInput: in, Publish: rng.IntN(2) == 0})
				ids = append(ids, a.ID)
			case op == 1:
				_, err = e.service.UpdateAnnouncement(ctx, appinstance.UpdateAnnouncementInput{AnnouncementID: ids[rng.IntN(len(ids))], AnnouncementInput: in})
			case op == 2:
				_, err = e.service.PublishAnnouncement(ctx, ids[rng.IntN(len(ids))], "")
			case op == 3 && rng.IntN(3) == 0:
				_, err = e.service.WithdrawAnnouncement(ctx, ids[rng.IntN(len(ids))], "")
			}
			if err != nil && !errors.Is(err, instance.ErrAnnouncementWithdrawn) {
				t.Fatalf("round %d step %d: %v", round, step, err)
			}
		}
		incremental, err := e.service.Announcements(ctx)
		if err != nil || len(incremental) == 0 {
			t.Fatalf("round %d: Announcements = %d, %v", round, len(incremental), err)
		}
		if err := appinstance.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Instance); err != nil {
			t.Fatalf("RebuildProjections = %v", err)
		}
		if rebuilt, err := e.service.Announcements(ctx); err != nil || !reflect.DeepEqual(incremental, rebuilt) {
			t.Fatalf("round %d: incremental announcements differ from a rebuild (%v):\nincremental %+v\nrebuilt     %+v", round, err, incremental, rebuilt)
		}
	}
}
