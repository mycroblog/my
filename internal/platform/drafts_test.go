// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/content"
	domainnotification "github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestScheduledDraftBeyondTheQueueDelayLimit checks ADR-0156 §6 through the app: a draft
// scheduled further ahead than the delay limit of a queue service (inline with MaxDelay, as Cloud
// Tasks allows 30 days) is not published at the limit, where the job comes back early, but at its
// time, through the re-enqueue chain; the database queue, which has no limit, publishes it at its
// time too. Running the job again then publishes nothing more.
func TestScheduledDraftBeyondTheQueueDelayLimit(t *testing.T) {
	t.Parallel()
	const maxDelay = 30 * 24 * time.Hour
	for _, q := range []string{"inline", "db"} {
		t.Run(q, func(t *testing.T) {
			t.Parallel()
			start := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
			clock := testkit.NewClock(start)
			cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": q})
			app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{
				Logger: slog.New(slog.DiscardHandler), Clock: clock, InlineQueueMaxDelay: maxDelay,
			})
			if err != nil {
				t.Fatalf("NewApp: %v", err)
			}
			t.Cleanup(func() { _ = app.Close() })
			ctx := t.Context()
			reg, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
				Username: "alice", Password: "correct horse battery staple", DisplayName: "alice",
			})
			if err != nil {
				t.Fatal(err)
			}
			alice := reg.AccountID
			draft, err := app.Drafts.Create(ctx, alice, appcontent.DraftInput{Text: "forty days later", Visibility: content.VisibilityPublic})
			if err != nil {
				t.Fatalf("Create = %v", err)
			}
			publishAt := start.Add(40 * 24 * time.Hour)
			if _, err := app.Drafts.Schedule(ctx, alice, draft.Draft.ID, publishAt); err != nil {
				t.Fatalf("Schedule = %v", err)
			}
			drain := func() {
				t.Helper()
				if _, err := app.Runner.Drain(ctx); err != nil {
					t.Fatalf("Drain = %v", err)
				}
			}
			published := func() bool {
				t.Helper()
				_, err := app.Content.Replay(ctx, alice, content.DraftIdempotencyKey(draft.Draft.ID))
				if err != nil && !errors.Is(err, appcontent.ErrNotFound) {
					t.Fatalf("Replay = %v", err)
				}
				return err == nil
			}

			drain()
			clock.Advance(maxDelay + time.Minute)
			drain()
			if published() {
				t.Fatal("the draft was published at the queue's delay limit, before its time")
			}
			if v, err := app.Drafts.Get(ctx, alice, draft.Draft.ID); err != nil || v.Scheduled == nil {
				t.Fatalf("Get before the time = %+v, %v; want the draft still scheduled", v, err)
			}

			clock.Set(publishAt.Add(time.Minute))
			drain()
			if !published() {
				t.Fatal("the draft was not published at its time")
			}
			if _, err := app.Drafts.Get(ctx, alice, draft.Draft.ID); !errors.Is(err, appcontent.ErrNotFound) {
				t.Errorf("Get after publication = %v, want ErrNotFound: the draft goes with its publication", err)
			}
			post, err := app.Content.Replay(ctx, alice, content.DraftIdempotencyKey(draft.Draft.ID))
			if err != nil {
				t.Fatal(err)
			}
			if !post.Status.PublishedAt.After(publishAt) && !post.Status.PublishedAt.Equal(publishAt) {
				t.Errorf("published_at = %v, want the time of publication, not before %v", post.Status.PublishedAt, publishAt)
			}
		})
	}
}

// TestScheduledDraftFailureNotifies checks ADR-0156 §5 and §8 through the app: a scheduled reply
// whose target is deleted before its time is not published, stays a draft whose last schedule
// failed, and gives its author one scheduled_status_failed, however often the job runs.
func TestScheduledDraftFailureNotifies(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	clock := testkit.NewClock(start)
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": "db"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Clock: clock})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ctx := t.Context()
	register := func(name string) string {
		t.Helper()
		reg, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: name, Password: "correct horse battery staple", DisplayName: name})
		if err != nil {
			t.Fatal(err)
		}
		return reg.AccountID
	}
	alice, bob := register("alice"), register("bob")
	parent, err := app.Content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: bob, Visibility: content.VisibilityPublic, TextInput: appcontent.TextInput{Text: "a question"},
	})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := app.Drafts.Create(ctx, alice, appcontent.DraftInput{Text: "an answer", Visibility: content.VisibilityPublic, InReplyToID: parent.Status.ID})
	if err != nil {
		t.Fatal(err)
	}
	at := start.Add(time.Hour)
	scheduled, err := app.Drafts.Schedule(ctx, alice, draft.Draft.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Content.Delete(ctx, bob, parent.Status.ID); err != nil {
		t.Fatal(err)
	}
	clock.Set(at.Add(time.Minute))
	if _, err := app.Runner.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Drafts.PublishScheduled(ctx, scheduled.Scheduled.ScheduleID); err != nil {
		t.Fatal(err)
	}
	v, err := app.Drafts.Get(ctx, alice, draft.Draft.ID)
	if err != nil {
		t.Fatalf("Get = %v, want the draft kept", err)
	}
	if v.Scheduled != nil || v.LastOutcome == nil || v.LastOutcome.Outcome.FailureReason != content.DraftFailureReplyUnavailable {
		t.Errorf("draft = %+v, want no schedule and the last one failed with reply_unavailable", v)
	}
	page, err := app.Notifications.List(ctx, alice, notification.Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var failed int
	for _, n := range page {
		if n.Type == domainnotification.TypeScheduledStatusFailed && n.DraftScheduleID == scheduled.Scheduled.ScheduleID {
			failed++
		}
	}
	if failed != 1 {
		t.Errorf("alice's notifications = %+v, want one scheduled_status_failed of the schedule", page)
	}
}
