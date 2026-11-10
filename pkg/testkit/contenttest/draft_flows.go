// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
)

// DraftHarness is a PostingHarness with the persistence of drafts.
type DraftHarness struct {
	PostingHarness
	Drafts appcontent.DraftRepository
}

// DraftFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
type DraftFactory func(t *testing.T, opts uow.Options) DraftHarness

// DraftFlows verifies drafts and scheduled posts (ADR-0156) on a database adapter, the flows that
// internal/flowtest/content runs once on SQLite: saving, reading, and replacing a draft with its
// If-Match; the limits of the instance settings; scheduling with the checks of a new post and the
// job of the schedule; publishing once, even when the job runs twice or at once; the failures that
// leave the draft and tell the author; and the cancellation of a schedule.
func DraftFlows(t *testing.T, newHarness DraftFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *draftEnv)
	}{
		{"SavesAndReplacesADraft", draftSaves},
		{"EnforcesTheLimits", draftLimits},
		{"PublishesAScheduledDraftOnce", draftPublishesOnce},
		{"PublishesAtOnce", draftPublishesAtOnce},
		{"FailsAndNotifiesWhenTheAuthorIsSanctioned", draftFailsSanctioned},
		{"FailsWhenTheReplyTargetIsGone", draftFailsReplyGone},
		{"CancelsAndMovesASchedule", draftCancels},
		{"ErasesTheDraftsOfAnAccount", draftErases},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newDraftEnv(t, newHarness)) })
	}
}

type draftEnv struct {
	*postingEnv
	drafts   *appcontent.DraftService
	repo     appcontent.DraftRepository
	outbox   *recordingOutbox
	mu       sync.Mutex
	failures []appcontent.DraftScheduleFailed
}

// recordingOutbox is a queue.Outbox that keeps the jobs enqueued, for the test to run.
type recordingOutbox struct {
	mu   sync.Mutex
	jobs []queue.NewJob
}

func (o *recordingOutbox) EnqueueTx(_ context.Context, _ uow.Tx, jobs ...queue.NewJob) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.jobs = append(o.jobs, jobs...)
	return nil
}

func newDraftEnv(t *testing.T, newHarness DraftFactory) *draftEnv {
	t.Helper()
	e := &draftEnv{outbox: &recordingOutbox{}}
	e.postingEnv = newPostingEnv(t, func(t *testing.T, opts uow.Options) PostingHarness {
		h := newHarness(t, opts)
		e.repo = h.Drafts
		opts.Projectors.Register(appcontent.EventDraftScheduleFailed, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.failures = append(e.failures, ev.(appcontent.DraftScheduleFailed))
			return nil
		})
		return h.PostingHarness
	})
	e.drafts = appcontent.NewDraftService(appcontent.DraftDeps{
		UnitOfWork: e.H.UnitOfWork, Drafts: e.repo, Media: e.media, Statuses: e.content, Outbox: e.outbox,
		Limits: func(ctx context.Context) (appcontent.DraftLimits, error) {
			s, err := e.instance.Settings(ctx)
			if err != nil {
				return appcontent.DraftLimits{}, err
			}
			return appcontent.DraftLimits{
				MaxDrafts: s.MaxDrafts(), MaxScheduledPosts: s.MaxScheduledPosts(), MaxScheduledPerDay: s.MaxScheduledPostsPerDay(),
				MinLeadMinutes: s.ScheduledPostMinLeadMinutes(), MaxLeadDays: s.ScheduledPostMaxLeadDays(),
			}, nil
		},
		Clock: e.Clock, IDs: e.IDs,
	})
	return e
}

func (e *draftEnv) create(t *testing.T, author string, in appcontent.DraftInput) appcontent.DraftView {
	t.Helper()
	if in.Visibility == "" {
		in.Visibility = content.VisibilityPublic
	}
	v, err := e.drafts.Create(t.Context(), author, in)
	if err != nil {
		t.Fatalf("Create(%+v) = %v", in, err)
	}
	return v
}

func (e *draftEnv) schedule(t *testing.T, author, id string, at time.Time) appcontent.DraftView {
	t.Helper()
	v, err := e.drafts.Schedule(t.Context(), author, id, at)
	if err != nil {
		t.Fatalf("Schedule(%s, %v) = %v", id, at, err)
	}
	if v.Scheduled == nil {
		t.Fatalf("Schedule(%s) = %+v, want a schedule in effect", id, v)
	}
	return v
}

func (e *draftEnv) setting(t *testing.T, key instance.SettingKey, value string) {
	t.Helper()
	if _, err := e.instance.SetSetting(t.Context(), appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
		t.Fatalf("SetSetting(%s, %s) = %v", key, value, err)
	}
}

// published returns the post that publishing the draft id made, or false.
func (e *draftEnv) published(t *testing.T, author, id string) (appcontent.Post, bool) {
	t.Helper()
	p, err := e.content.Replay(t.Context(), author, content.DraftIdempotencyKey(id))
	if errors.Is(err, appcontent.ErrNotFound) {
		return appcontent.Post{}, false
	}
	if err != nil {
		t.Fatalf("Replay = %v", err)
	}
	return p, true
}

// authorPosts counts the statuses of author.
func (e *draftEnv) authorPosts(t *testing.T, author string) int {
	t.Helper()
	e.events.Mu.Lock()
	defer e.events.Mu.Unlock()
	n := 0
	for _, ev := range e.events.Events {
		if c, ok := ev.(appcontent.StatusCreated); ok && c.Status.AuthorAccountID == author {
			n++
		}
	}
	return n
}

func draftSaves(t *testing.T, e *draftEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	processing := e.upload(t, alice, "")
	poll := &content.DraftPoll{Options: []string{"yes", ""}, Duration: time.Hour}
	v := e.create(t, alice, appcontent.DraftInput{
		Text: "", SpoilerText: "cw", Language: "ja", Visibility: content.VisibilityUnlisted,
		InteractionPolicy: appcontent.InteractionPolicyChange{content.InteractionQuote: {Automatic: content.AudienceFollowers, Manual: content.AudienceEveryone}},
		Poll:              poll,
	})
	got, err := e.drafts.Get(ctx, alice, v.Draft.ID)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	c := got.Content
	if c.Text != "" || c.SpoilerText != "cw" || c.Language != "ja" || c.Visibility != content.VisibilityUnlisted || c.Poll == nil ||
		len(c.Poll.Options) != 2 || c.Poll.Duration != time.Hour || c.InteractionPolicy[content.InteractionQuote].Manual != content.AudienceEveryone ||
		!c.UpdatedAt.Equal(v.Content.UpdatedAt) {
		t.Errorf("Get = %+v, want what was saved (%+v)", c, v.Content)
	}
	if _, err := e.drafts.Get(ctx, bob, v.Draft.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Get by another account = %v, want ErrNotFound", err)
	}

	// A save replaces the whole content; a file in processing may be attached.
	updated, err := e.drafts.Update(ctx, alice, v.Draft.ID, v.Content.UpdatedAt, appcontent.DraftInput{
		Text: "now with a picture", Visibility: content.VisibilityPublic, Media: []appcontent.MediaInput{{MediaID: processing, Description: "a cat"}},
	})
	if err != nil {
		t.Fatalf("Update = %v", err)
	}
	if c := updated.Content; c.Poll != nil || len(c.InteractionPolicy) != 0 || len(c.Attachments) != 1 || c.Attachments[0].Description != "a cat" ||
		!c.UpdatedAt.After(v.Content.UpdatedAt) {
		t.Errorf("Update = %+v, want the new content alone, saved later", c)
	}
	// A save on another device since the client read the draft refuses the stale one.
	if _, err := e.drafts.Update(ctx, alice, v.Draft.ID, v.Content.UpdatedAt, appcontent.DraftInput{Text: "stale", Visibility: content.VisibilityPublic}); !errors.Is(err, appcontent.ErrDraftChanged) {
		t.Errorf("Update with a stale If-Match = %v, want ErrDraftChanged", err)
	}
	if _, err := e.drafts.Update(ctx, bob, v.Draft.ID, updated.Content.UpdatedAt, appcontent.DraftInput{Text: "x", Visibility: content.VisibilityPublic}); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Update by another account = %v, want ErrNotFound", err)
	}
	for name, in := range map[string]appcontent.DraftInput{
		"another account's file": {Visibility: content.VisibilityPublic, Media: []appcontent.MediaInput{{MediaID: e.upload(t, bob, content.MediaReady)}}},
		"a poll with media":      {Visibility: content.VisibilityPublic, Media: []appcontent.MediaInput{{MediaID: processing}}, Poll: poll},
		"no visibility":          {Text: "x"},
	} {
		if _, err := e.drafts.Create(ctx, alice, in); err == nil {
			t.Errorf("Create with %s succeeded, want an error", name)
		}
	}

	list, err := e.drafts.List(ctx, alice, "", nil, 10)
	if err != nil || len(list) != 1 || list[0].Draft.ID != v.Draft.ID {
		t.Errorf("List = %+v, %v; want the one draft", list, err)
	}
	if err := e.drafts.Delete(ctx, alice, v.Draft.ID); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if _, err := e.drafts.Get(ctx, alice, v.Draft.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Get after Delete = %v, want ErrNotFound", err)
	}
}

func draftLimits(t *testing.T, e *draftEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	e.setting(t, instance.SettingMaxDrafts, "2")
	e.setting(t, instance.SettingMaxScheduledPostsPerDay, "1")
	e.setting(t, instance.SettingMaxScheduledPosts, "2")
	first := e.create(t, alice, appcontent.DraftInput{Text: "one"})
	e.create(t, alice, appcontent.DraftInput{Text: "two"})
	var limit *appcontent.DraftLimitError
	if _, err := e.drafts.Create(ctx, alice, appcontent.DraftInput{Text: "three", Visibility: content.VisibilityPublic}); !errors.As(err, &limit) ||
		limit.Limit != appcontent.LimitDrafts || limit.Max != 2 {
		t.Errorf("Create past max_drafts = %v, want the limit max_drafts of 2", err)
	}

	now := e.Clock.Now()
	for name, tc := range map[string]struct {
		at    time.Time
		limit appcontent.DraftLimit
		max   int
	}{
		"too soon": {now.Add(4 * time.Minute), appcontent.LimitScheduleMinLead, 5},
		"too far":  {now.AddDate(0, 0, 367), appcontent.LimitScheduleMaxLead, 366},
	} {
		if _, err := e.drafts.Schedule(ctx, alice, first.Draft.ID, tc.at); !errors.As(err, &limit) || limit.Limit != tc.limit || limit.Max != tc.max {
			t.Errorf("Schedule %s = %v, want the limit %s of %d", name, err, tc.limit, tc.max)
		}
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 3)
	e.schedule(t, alice, first.Draft.ID, day.Add(10*time.Hour))
	// Scheduling one draft leaves room for another draft without a schedule.
	third := e.create(t, alice, appcontent.DraftInput{Text: "three"})
	if _, err := e.drafts.Schedule(ctx, alice, third.Draft.ID, day.Add(20*time.Hour)); !errors.As(err, &limit) || limit.Limit != appcontent.LimitScheduledPostsPerDay {
		t.Errorf("Schedule a second post on one UTC day = %v, want max_scheduled_posts_per_day", err)
	}
	// Moving a schedule within the same day does not count it twice.
	e.schedule(t, alice, first.Draft.ID, day.Add(11*time.Hour))
	kept := e.schedule(t, alice, third.Draft.ID, day.Add(36*time.Hour))
	// A lowered limit keeps the schedules there are, but a new time is a new schedule.
	e.setting(t, instance.SettingMaxScheduledPosts, "1")
	if _, err := e.drafts.Schedule(ctx, alice, third.Draft.ID, day.Add(60*time.Hour)); !errors.As(err, &limit) || limit.Limit != appcontent.LimitScheduledPosts {
		t.Errorf("moving a schedule past a lowered max_scheduled_posts = %v, want max_scheduled_posts", err)
	}
	if cur, err := e.repo.FindScheduledDraft(ctx, third.Draft.ID); err != nil || cur.ScheduleID != kept.Scheduled.ScheduleID {
		t.Errorf("schedule after the refused move = %+v, %v; want the one before kept", cur, err)
	}
	fourth := e.create(t, alice, appcontent.DraftInput{Text: "four"})
	if _, err := e.drafts.Schedule(ctx, alice, fourth.Draft.ID, day.Add(90*time.Hour)); !errors.As(err, &limit) || limit.Limit != appcontent.LimitScheduledPosts {
		t.Errorf("Schedule past max_scheduled_posts = %v, want max_scheduled_posts", err)
	}
	// Scheduling checks the draft as a new post.
	empty := fourth.Draft.ID
	if _, err := e.drafts.Update(ctx, alice, empty, fourth.Content.UpdatedAt, appcontent.DraftInput{Visibility: content.VisibilityPublic}); err != nil {
		t.Fatal(err)
	}
	e.setting(t, instance.SettingMaxScheduledPosts, "10")
	if _, err := e.drafts.Schedule(ctx, alice, empty, day.Add(100*time.Hour)); !errors.Is(err, appcontent.ErrEmptyText) {
		t.Errorf("Schedule an empty draft = %v, want ErrEmptyText", err)
	}
}

// runJobs runs every job the outbox holds through the handler of JobPublishScheduledDraft, each
// times times, at once.
func (e *draftEnv) runJobs(t *testing.T, times int) {
	t.Helper()
	reg := &queue.JobHandlerRegistry{}
	e.drafts.RegisterJobs(reg)
	e.outbox.mu.Lock()
	jobs := append([]queue.NewJob(nil), e.outbox.jobs...)
	e.outbox.mu.Unlock()
	var wg sync.WaitGroup
	errs := make(chan error, len(jobs)*times)
	for _, nj := range jobs {
		if nj.Type != appcontent.JobPublishScheduledDraft {
			continue
		}
		for range times {
			wg.Add(1)
			go func() {
				defer wg.Done()
				h, _, ok := reg.Lookup(nj.Type)
				if !ok {
					errs <- errors.New("no handler")
					return
				}
				if r := h(t.Context(), queue.Job{Type: nj.Type, Payload: nj.Payload}); !r.IsDone() {
					errs <- errors.New("the job did not finish: " + nj.IdempotencyKey)
				}
			}()
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func draftPublishesOnce(t *testing.T, e *draftEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	v := e.create(t, alice, appcontent.DraftInput{Text: "hello later", Visibility: content.VisibilityPublic})
	at := e.Clock.Now().Add(time.Hour)
	s := e.schedule(t, alice, v.Draft.ID, at)
	if len(e.outbox.jobs) != 1 || !e.outbox.jobs[0].RunAt.Equal(at) || e.outbox.jobs[0].IdempotencyKey != "publish_scheduled_draft:"+s.Scheduled.ScheduleID {
		t.Fatalf("enqueued %+v, want one job at %v keyed by the schedule", e.outbox.jobs, at)
	}

	// A run before the time (a queue that brings the job early) publishes nothing.
	if wait, err := e.drafts.PublishScheduled(ctx, s.Scheduled.ScheduleID); err != nil || wait <= 0 {
		t.Errorf("PublishScheduled before the time = %v, %v; want a wait", wait, err)
	}
	if _, ok := e.published(t, alice, v.Draft.ID); ok {
		t.Fatal("published before its time")
	}

	// The author edits the scheduled draft; the post has the content it has at its time.
	if _, err := e.drafts.Update(ctx, alice, v.Draft.ID, v.Content.UpdatedAt, appcontent.DraftInput{Text: "hello, edited", Visibility: content.VisibilityPublic}); err != nil {
		t.Fatal(err)
	}
	e.Clock.Set(at.Add(time.Second))
	before := e.authorPosts(t, alice)
	e.runJobs(t, 3)
	e.runJobs(t, 1)
	if got := e.authorPosts(t, alice) - before; got != 1 {
		t.Errorf("the job running four times posted %d statuses, want 1", got)
	}
	p, ok := e.published(t, alice, v.Draft.ID)
	if !ok {
		t.Fatal("not published at its time")
	}
	rev, err := e.content.Find(ctx, p.Status.ID)
	if err != nil {
		t.Fatal(err)
	}
	if src := rev.Revision.Source; src == nil || src.Text != "hello, edited" {
		t.Errorf("published source = %+v, want the content at publication", src)
	}
	if !p.Status.PublishedAt.After(at) {
		t.Errorf("published_at = %v, want the time it was made, after %v", p.Status.PublishedAt, at)
	}
	if _, err := e.drafts.Get(ctx, alice, v.Draft.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Get after publication = %v, want ErrNotFound", err)
	}
	if len(e.failures) != 0 {
		t.Errorf("failures = %+v, want none", e.failures)
	}
}

func draftPublishesAtOnce(t *testing.T, e *draftEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	v := e.create(t, alice, appcontent.DraftInput{Text: "now", Visibility: content.VisibilityPublic, Poll: &content.DraftPoll{Options: []string{"a", "b"}, Duration: time.Hour}})
	e.schedule(t, alice, v.Draft.ID, e.Clock.Now().Add(time.Hour))
	p, err := e.drafts.Publish(ctx, alice, v.Draft.ID, "")
	if err != nil {
		t.Fatalf("Publish = %v", err)
	}
	if p.Revision.Poll == nil || p.Revision.Poll.ClosesAt == nil || !p.Revision.Poll.ClosesAt.Equal(p.Status.PublishedAt.Add(time.Hour).Truncate(time.Second)) &&
		p.Revision.Poll.ClosesAt.Sub(p.Status.PublishedAt) > time.Hour+time.Second {
		t.Errorf("poll = %+v, want one that ends an hour after publication", p.Revision.Poll)
	}
	if _, err := e.drafts.Get(ctx, alice, v.Draft.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Get after Publish = %v, want ErrNotFound", err)
	}
	// The job of the schedule finds nothing to do.
	e.Clock.Advance(2 * time.Hour)
	e.runJobs(t, 1)
	if len(e.failures) != 0 {
		t.Errorf("failures = %+v, want none", e.failures)
	}
	empty := e.create(t, alice, appcontent.DraftInput{Visibility: content.VisibilityPublic})
	if _, err := e.drafts.Publish(ctx, alice, empty.Draft.ID, ""); !errors.Is(err, appcontent.ErrEmptyText) {
		t.Errorf("Publish an empty draft = %v, want ErrEmptyText", err)
	}
	if _, err := e.drafts.Get(ctx, alice, empty.Draft.ID); err != nil {
		t.Errorf("Get after a failed Publish = %v, want the draft kept", err)
	}
}

// failure returns the one failure the env recorded.
func (e *draftEnv) failure(t *testing.T) appcontent.DraftScheduleFailed {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.failures) != 1 {
		t.Fatalf("failures = %+v, want one", e.failures)
	}
	return e.failures[0]
}

func draftFailsSanctioned(t *testing.T, e *draftEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	v := e.create(t, alice, appcontent.DraftInput{Text: "later", Visibility: content.VisibilityPublic})
	at := e.Clock.Now().Add(time.Hour)
	s := e.schedule(t, alice, v.Draft.ID, at)
	// The author is barred from posting after scheduling.
	e.restrictions.set(alice, appcontent.Restrictions{Post: true})
	e.Clock.Set(at)
	e.runJobs(t, 2)
	f := e.failure(t)
	if f.AccountID != alice || f.Schedule.ID != s.Scheduled.ScheduleID || f.Schedule.DraftID != v.Draft.ID ||
		f.Outcome.FailureReason != content.DraftFailureRestricted {
		t.Errorf("failure = %+v, want the schedule of alice's draft, restricted", f)
	}
	got, err := e.drafts.Get(ctx, alice, v.Draft.ID)
	if err != nil {
		t.Fatalf("Get after the failure = %v, want the draft kept", err)
	}
	if got.Scheduled != nil || got.LastOutcome == nil || got.LastOutcome.Outcome.Outcome != content.DraftFailed ||
		got.LastOutcome.Outcome.FailureReason != content.DraftFailureRestricted {
		t.Errorf("draft after the failure = %+v, want no schedule and the last one failed", got)
	}
	if _, ok := e.published(t, alice, v.Draft.ID); ok {
		t.Error("a sanctioned author's draft was published")
	}
	recs, err := e.repo.DraftSchedules(ctx, []string{s.Scheduled.ScheduleID, e.IDs.NewID()})
	if err != nil || len(recs) != 1 || recs[s.Scheduled.ScheduleID].Outcome.FailureReason != content.DraftFailureRestricted {
		t.Errorf("DraftSchedules = %+v, %v; want the failed schedule alone", recs, err)
	}
	// Once the restriction is lifted the author schedules it again.
	e.restrictions.set(alice, appcontent.Restrictions{})
	e.schedule(t, alice, v.Draft.ID, e.Clock.Now().Add(time.Hour))
}

func draftFailsReplyGone(t *testing.T, e *draftEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	parent := e.postText(t, bob, "a question", content.VisibilityPublic)
	v := e.create(t, alice, appcontent.DraftInput{Text: "an answer", Visibility: content.VisibilityPublic, InReplyToID: parent.Status.ID})
	at := e.Clock.Now().Add(time.Hour)
	e.schedule(t, alice, v.Draft.ID, at)
	if _, err := e.content.Delete(ctx, bob, parent.Status.ID); err != nil {
		t.Fatal(err)
	}
	e.Clock.Set(at)
	e.runJobs(t, 1)
	if f := e.failure(t); f.Outcome.FailureReason != content.DraftFailureReplyUnavailable {
		t.Errorf("failure = %+v, want reply_unavailable", f)
	}
	if _, err := e.drafts.Get(ctx, alice, v.Draft.ID); err != nil {
		t.Errorf("Get = %v, want the draft kept", err)
	}
	// Scheduling a reply to a status that is gone is refused at once.
	if _, err := e.drafts.Schedule(ctx, alice, v.Draft.ID, e.Clock.Now().Add(time.Hour)); !errors.Is(err, appcontent.ErrReplyTargetNotFound) {
		t.Errorf("Schedule a reply to a deleted status = %v, want ErrReplyTargetNotFound", err)
	}
}

func draftCancels(t *testing.T, e *draftEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	v := e.create(t, alice, appcontent.DraftInput{Text: "maybe", Visibility: content.VisibilityPublic})
	first := e.schedule(t, alice, v.Draft.ID, e.Clock.Now().Add(time.Hour))
	second := e.schedule(t, alice, v.Draft.ID, e.Clock.Now().Add(2*time.Hour))
	if second.Scheduled.ScheduleID == first.Scheduled.ScheduleID || second.LastOutcome == nil ||
		second.LastOutcome.Schedule.ID != first.Scheduled.ScheduleID || second.LastOutcome.Outcome.Outcome != content.DraftCancelled {
		t.Errorf("moved schedule = %+v, want a new schedule after the first one cancelled", second)
	}
	// The job of the first schedule does nothing at its time.
	e.Clock.Advance(time.Hour + time.Second)
	if _, err := e.drafts.PublishScheduled(ctx, first.Scheduled.ScheduleID); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.published(t, alice, v.Draft.ID); ok {
		t.Error("the job of a moved schedule published the draft")
	}
	got, err := e.drafts.Unschedule(ctx, alice, v.Draft.ID)
	if err != nil || got.Scheduled != nil {
		t.Fatalf("Unschedule = %+v, %v; want the draft without a schedule", got, err)
	}
	if _, err := e.drafts.Unschedule(ctx, alice, v.Draft.ID); !errors.Is(err, appcontent.ErrDraftNotScheduled) {
		t.Errorf("Unschedule again = %v, want ErrDraftNotScheduled", err)
	}
	e.Clock.Advance(2 * time.Hour)
	e.runJobs(t, 1)
	if _, ok := e.published(t, alice, v.Draft.ID); ok {
		t.Error("a cancelled schedule published the draft")
	}
	scheduled := true
	if list, err := e.drafts.List(ctx, alice, "", &scheduled, 10); err != nil || len(list) != 0 {
		t.Errorf("List(scheduled) = %+v, %v; want none", list, err)
	}
	// The projection rebuilds from the schedules without an outcome.
	again := e.schedule(t, alice, v.Draft.ID, e.Clock.Now().Add(time.Hour))
	if err := appcontent.RebuildScheduledDrafts(ctx, e.H.UnitOfWork, e.repo); err != nil {
		t.Fatal(err)
	}
	if cur, err := e.repo.FindScheduledDraft(ctx, v.Draft.ID); err != nil || cur.ScheduleID != again.Scheduled.ScheduleID {
		t.Errorf("after the rebuild, FindScheduledDraft = %+v, %v; want %s", cur, err, again.Scheduled.ScheduleID)
	}
}

func draftErases(t *testing.T, e *draftEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	a := e.create(t, alice, appcontent.DraftInput{Text: "one", Visibility: content.VisibilityPublic})
	e.schedule(t, alice, a.Draft.ID, e.Clock.Now().Add(time.Hour))
	b := e.create(t, bob, appcontent.DraftInput{Text: "two", Visibility: content.VisibilityPublic})
	err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return appcontent.EraseDrafts(ctx, tx, e.repo, alice) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.repo.FindDraft(ctx, a.Draft.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("alice's draft after the erasure = %v, want ErrNotFound", err)
	}
	if _, err := e.repo.FindScheduledDraft(ctx, a.Draft.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("alice's schedule after the erasure = %v, want ErrNotFound", err)
	}
	if _, err := e.repo.FindDraft(ctx, b.Draft.ID); err != nil {
		t.Errorf("bob's draft after alice's erasure = %v, want it kept", err)
	}
}
