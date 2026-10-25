// SPDX-License-Identifier: MIT

package mybot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/pkg/mykit"
)

// CatchUp is what a scheduled job does about the times it missed while the bot was stopped.
type CatchUp int

const (
	// Skip leaves the missed times out: the job runs from its next time after the start. It is the default.
	Skip CatchUp = iota
	// Latest runs, when the bot starts, the newest of the times after the job's last run in the Store, once; the
	// older ones are left out, so that a long stop never posts a pile at once. A job that never ran runs its
	// newest past time.
	Latest
)

// EveryOptions are the options of a scheduled job.
type EveryOptions struct {
	// Name names the job in the Store (its last run) and in the Idempotency-Key of its posts: keep it when the
	// expression changes. "" means the expression. Two jobs of one bot cannot share a name.
	Name string
	// TimeZone is the IANA time zone of the expression, such as "Asia/Tokyo". "" means UTC.
	TimeZone string
	// CatchUp is Skip (the default) or Latest.
	CatchUp CatchUp
	// Visibility is the visibility of the posts of Context.Post: unlisted (the default), public, or local, which
	// reaches the server of the bot and its followers alone (ADR-0017).
	Visibility mykit.Visibility
}

// everyJob is a job that Every registered.
type everyJob struct {
	name       string
	schedule   *Schedule
	handler    Handler
	catchUp    CatchUp
	visibility mykit.Visibility
}

// Errors of the scheduled jobs.
var (
	// ErrNotScheduled is the error of Context.Post outside a scheduled job.
	ErrNotScheduled = errors.New("mybot: Post is for the handler of a scheduled job")
	// ErrPostedAlready is the error of a second Context.Post in one run of a scheduled job, whose Idempotency-Key
	// would answer with the first post.
	ErrPostedAlready = errors.New("mybot: the run of the scheduled job posted already")
)

// EveryIdempotencyKey is the Idempotency-Key of the post of the run at of the scheduled job name (ADR-0094 §5):
// mybot:every:<name>:<the time in RFC 3339, UTC>. Two processes of one bot that run the same time post once. Use
// it when posting with mykit directly.
func EveryIdempotencyKey(name string, at time.Time) string {
	return "mybot:every:" + name + ":" + at.UTC().Format(time.RFC3339)
}

// Every registers a scheduled job: h runs at each time of the cron expression expr, of the five fields minute hour
// day-of-month month day-of-week (ParseSchedule), in opts.TimeZone (UTC by default). Its Context carries Job and
// ScheduledAt, and Post posts with the Idempotency-Key EveryIdempotencyKey. The run counts in the Store as the
// job's last run after h returns. It returns an error for an expression or a time zone it does not take, a
// visibility other than unlisted, public, and local, and a name that another job has.
func (b *Bot) Every(expr string, h Handler, opts EveryOptions) error {
	s, err := ParseSchedule(expr, opts.TimeZone)
	if err != nil {
		return err
	}
	name := opts.Name
	if name == "" {
		name = expr
	}
	for _, j := range b.every {
		if j.name == name {
			return fmt.Errorf("mybot: a scheduled job is named %q already", name)
		}
	}
	v := opts.Visibility
	switch v {
	case "":
		v = mykit.VisibilityUnlisted
	case mykit.VisibilityUnlisted, mykit.VisibilityPublic, mykit.VisibilityLocal:
	default:
		return fmt.Errorf("mybot: the scheduled job %q cannot post with the visibility %q: use unlisted, public, or local", name, v)
	}
	if opts.CatchUp != Skip && opts.CatchUp != Latest {
		return fmt.Errorf("mybot: the scheduled job %q has an unknown CatchUp %d", name, opts.CatchUp)
	}
	b.every = append(b.every, &everyJob{name: name, schedule: s, handler: h, catchUp: opts.CatchUp, visibility: v})
	return nil
}

// due is the newest time of j at or before now and after its last run, the zero time when there is none.
func (b *Bot) due(ctx context.Context, j *everyJob, now time.Time) (time.Time, error) {
	last, err := b.store.LastRun(ctx, j.name)
	if err != nil {
		return time.Time{}, fmt.Errorf("mybot: read the last run of %q from the store: %w", j.name, err)
	}
	at := j.schedule.Prev(now)
	if at.IsZero() || !at.After(last) {
		return time.Time{}, nil
	}
	return at, nil
}

// submitDue hands the jobs their due time: every job (RunOnce), or those of CatchUp Latest (Run).
func (b *Bot) submitDue(ctx context.Context, all bool) {
	now := b.now()
	for _, j := range b.every {
		if !all && j.catchUp != Latest {
			continue
		}
		at, err := b.due(ctx, j, now)
		if err != nil {
			b.logger.Error("mybot: the scheduled job did not catch up", "job", j.name, "error", err)
			continue
		}
		if !at.IsZero() {
			b.submitRun(j, at)
		}
	}
}

// submitRun hands the run at of j to the dispatcher; the runs of one job go one after the other.
func (b *Bot) submitRun(j *everyJob, at time.Time) {
	b.dispatcher.submit(job{every: j, at: at, sender: "\x00every:" + j.name})
}

// schedule runs j at its times until ctx ends. A run that the clock reaches late, such as after the machine
// slept, runs once, and the times it passed are skipped.
func (b *Bot) schedule(ctx context.Context, j *everyJob) {
	next := j.schedule.Next(b.now())
	for !next.IsZero() {
		if err := b.sleep(ctx, next.Sub(b.now())); err != nil {
			return
		}
		if now := b.now(); now.Before(next) {
			continue // woken early, as a clock of a test may
		}
		b.submitRun(j, next)
		now := b.now()
		if now.Before(next) {
			now = next
		}
		next = j.schedule.Next(now)
	}
}

// runScheduled runs one time of a job and records it as the job's last run.
func (b *Bot) runScheduled(ctx context.Context, j job) {
	e := j.every
	last, err := b.store.LastRun(ctx, e.name)
	if err != nil {
		b.logger.Error("mybot: read the store", "job", e.name, "error", err)
		return
	}
	if !j.at.After(last) {
		return // ran already
	}
	c := &Context{Context: ctx, bot: b, sender: j.sender, Job: e.name, ScheduledAt: j.at, every: e}
	if err := b.chain(e.handler)(c); err != nil {
		c.Logger().Error("mybot: the handler failed", "error", err)
	}
	b.abandonMu.RLock()
	defer b.abandonMu.RUnlock()
	if b.abandoned || ctx.Err() != nil {
		return
	}
	if err := b.store.SetLastRun(ctx, e.name, j.at); err != nil {
		b.logger.Error("mybot: record the run of the scheduled job", "job", e.name, "error", err)
	}
}

// Post posts text from the handler of a scheduled job, with the visibility of its EveryOptions and the
// Idempotency-Key EveryIdempotencyKey(Job, ScheduledAt): a run posts once, even when two processes of the bot run
// it, or a restart runs it again. It waits for the send budget. Outside a scheduled job it returns
// ErrNotScheduled, and a second Post of a run ErrPostedAlready.
func (c *Context) Post(text string) (*mykit.Status, error) {
	if c.every == nil {
		return nil, ErrNotScheduled
	}
	if c.posted {
		return nil, ErrPostedAlready
	}
	if err := c.waitBudget(sendPost); err != nil {
		return nil, err
	}
	c.posted = true
	key := EveryIdempotencyKey(c.Job, c.ScheduledAt)
	resp, err := c.bot.api.CreateStatusWithResponse(c, &mykit.CreateStatusParams{IdempotencyKey: &key}, mykit.CreateStatusRequest{
		Text:       text,
		Visibility: c.every.visibility,
	})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("mybot: createStatus answered %s", resp.Status())
	}
	return resp.JSON200, nil
}
