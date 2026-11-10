// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
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

// TestScheduledArticleDraft checks the drafts of articles through the app (ADR-0160 §2, ADR-0156
// §3): the draft keeps the title, the summary, the cover, the hashtags, and the uploads its body
// shows, which it reads back as saved and which the cleanup of unattached uploads keeps; a save
// with the version read before another refuses to overwrite it; and the schedule publishes it as
// an article at its time, as it does a post.
func TestScheduledArticleDraft(t *testing.T) {
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
	reg, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "alice", Password: "correct horse battery staple", DisplayName: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	alice := reg.AccountID
	upload := func() string {
		t.Helper()
		m, err := app.Media.Upload(ctx, appcontent.UploadInput{OwnerAccountID: alice, Content: bytes.NewReader(testkit.FixturePNG(32, 32, false))})
		if err != nil {
			t.Fatalf("Upload = %v", err)
		}
		return m.File.ID
	}
	cover, inline, loose := upload(), upload(), upload()
	if _, err := app.Runner.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	in := appcontent.DraftInput{
		Text:       "Hello.\n\n![a square](media:" + inline + ")\n\n![not an upload](media:0190f000-0000-7000-8000-000000000000)",
		Visibility: content.VisibilityPublic,
		Article: &appcontent.DraftArticleInput{
			Title: "Later", Cover: &appcontent.MediaInput{MediaID: cover, Description: "a cover"}, Tags: []string{"cats"},
		},
	}
	draft, err := app.Drafts.Create(ctx, alice, in)
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
	// Another device reads the draft with its images.
	got, err := app.Drafts.Get(ctx, alice, draft.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a := got.Content.Article; a == nil || a.Title != "Later" || a.Cover == nil || a.Cover.MediaID != cover || a.Cover.Description != "a cover" ||
		len(a.Tags) != 1 || a.Tags[0] != "cats" || len(a.Images) != 1 || a.Images[0] != inline {
		t.Fatalf("Get = %+v, want the article as saved, with the one upload of the author that the body shows", got.Content.Article)
	}
	// A save on one device refuses the stale save of the other.
	in.Article.Summary = "A summary."
	saved, err := app.Drafts.Update(ctx, alice, draft.Draft.ID, got.Content.UpdatedAt, in)
	if err != nil {
		t.Fatalf("Update = %v", err)
	}
	if _, err := app.Drafts.Update(ctx, alice, draft.Draft.ID, got.Content.UpdatedAt, in); !errors.Is(err, appcontent.ErrDraftChanged) {
		t.Errorf("Update with the version read before = %v, want ErrDraftChanged", err)
	}
	if a := saved.Content.Article; a == nil || a.Summary != "A summary." || len(a.Images) != 1 {
		t.Errorf("Update = %+v, want the new summary and the image kept", a)
	}
	// The cleanup keeps the cover and the image of the draft and deletes the loose upload. It bounds
	// the uploads by their IDs, which the app makes from the wall clock.
	clock.Set(time.Now().Add(48 * time.Hour))
	if _, err := app.Media.CleanupUnattached(ctx); err != nil {
		t.Fatal(err)
	}
	for id, kept := range map[string]bool{cover: true, inline: true, loose: false} {
		if _, err := app.Media.Media(ctx, id); (err == nil) != kept {
			t.Errorf("media %s after the cleanup: %v, want kept %v", id, err, kept)
		}
	}

	at := clock.Now().Add(time.Hour)
	if _, err := app.Drafts.Schedule(ctx, alice, draft.Draft.ID, at); err != nil {
		t.Fatalf("Schedule = %v", err)
	}
	clock.Set(at.Add(time.Minute))
	if _, err := app.Runner.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	post, err := app.Content.Replay(ctx, alice, content.DraftIdempotencyKey(draft.Draft.ID))
	if err != nil {
		t.Fatalf("Replay = %v, want the article published at its time", err)
	}
	a := post.Revision.Article
	if a == nil || a.Title != "Later" || a.Summary != "A summary." || a.Cover == nil || a.Cover.MediaID != cover ||
		len(a.Images) != 1 || a.Images[0].MediaID != inline || a.Images[0].Description != "a square" {
		t.Errorf("published revision = %+v, want the article of the draft", a)
	}
	if src := post.Revision.Source; src == nil || src.Format != content.SourceTextMarkdown {
		t.Errorf("published source = %+v, want the Markdown", src)
	}
	if _, err := app.Drafts.Get(ctx, alice, draft.Draft.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Get after publication = %v, want ErrNotFound", err)
	}
}
