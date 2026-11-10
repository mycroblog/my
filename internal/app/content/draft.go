// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds drafts and scheduled posts (data-model §4.4, ADR-0156). A draft is saved on the
// server, so its author goes on writing it on another device, and is published at once or at a
// chosen time. Publishing goes through Service.Create, as a new post does, so the checks, the
// decision hook local.status, federation, and notifications are those of a new post; they judge
// the author and the targets as they are at publication, and the post is stamped with the time
// it was made.

const (
	// JobPublishScheduledDraft publishes a scheduled draft at its time. Its payload is
	// publishScheduledDraftPayload, and its idempotency key that of the schedule.
	JobPublishScheduledDraft = "content.publish_scheduled_draft"
	// EventDraftScheduleFailed is emitted when a scheduled draft could not be published.
	EventDraftScheduleFailed = "content.draft_schedule_failed"
)

var (
	// ErrDraftChanged reports a save of a draft that another save changed since the client read
	// it: the If-Match of the API does not match.
	ErrDraftChanged = errors.New("content: the draft changed since it was read")
	// ErrDraftNotScheduled reports the cancellation of a schedule of a draft that has none.
	ErrDraftNotScheduled = errors.New("content: the draft is not scheduled")
	// errScheduleSuperseded reports a publication of a schedule that is no longer in effect: it
	// was cancelled, moved, or ran already.
	errScheduleSuperseded = errors.New("content: the schedule is no longer in effect")
)

// DraftLimit names a limit of drafts and scheduled posts (the instance settings of ADR-0156 §7).
type DraftLimit string

// Limits of drafts and scheduled posts.
const (
	// LimitDrafts is max_drafts: the drafts without a schedule.
	LimitDrafts DraftLimit = "max_drafts"
	// LimitScheduledPosts is max_scheduled_posts: the schedules in effect.
	LimitScheduledPosts DraftLimit = "max_scheduled_posts"
	// LimitScheduledPostsPerDay is max_scheduled_posts_per_day: the schedules in effect whose
	// time is on one UTC day.
	LimitScheduledPostsPerDay DraftLimit = "max_scheduled_posts_per_day"
	// LimitScheduleMinLead is scheduled_post_min_lead_minutes: how soon a schedule may be.
	LimitScheduleMinLead DraftLimit = "scheduled_post_min_lead_minutes"
	// LimitScheduleMaxLead is scheduled_post_max_lead_days: how far ahead a schedule may be.
	LimitScheduleMaxLead DraftLimit = "scheduled_post_max_lead_days"
)

// DraftLimitError reports a save or a schedule that a limit refuses, with the value of the limit
// (minutes for LimitScheduleMinLead, days for LimitScheduleMaxLead, a count for the others).
type DraftLimitError struct {
	Limit DraftLimit
	Max   int
}

func (e *DraftLimitError) Error() string {
	return fmt.Sprintf("content: the limit %s (%d) refuses the draft", e.Limit, e.Max)
}

// DraftLimits are the limits of drafts and scheduled posts in effect.
type DraftLimits struct {
	MaxDrafts          int
	MaxScheduledPosts  int
	MaxScheduledPerDay int
	MinLeadMinutes     int
	MaxLeadDays        int
}

// DraftScheduleFailed is emitted when a scheduled draft could not be published, in the
// transaction that records the failure: the notification module tells the author
// (scheduled_status_failed).
type DraftScheduleFailed struct {
	Schedule  content.DraftSchedule
	AccountID string
	Outcome   content.DraftScheduleOutcome
}

// EventType implements uow.Event.
func (DraftScheduleFailed) EventType() string { return EventDraftScheduleFailed }

// DraftDeps are the ports of a DraftService.
type DraftDeps struct {
	UnitOfWork uow.UnitOfWork
	Drafts     DraftRepository
	// Media reads the files that drafts attach.
	Media MediaRepository
	// Statuses publishes drafts as new posts.
	Statuses *Service
	// Outbox enqueues JobPublishScheduledDraft in the transaction that records a schedule.
	Outbox queue.Outbox
	// Limits returns the limits in effect, read per command.
	Limits func(ctx context.Context) (DraftLimits, error)
	Clock  kernel.Clock
	IDs    kernel.IDGenerator
}

// DraftService runs the use cases of drafts and scheduled posts.
type DraftService struct {
	d DraftDeps
}

// NewDraftService returns the DraftService on d.
func NewDraftService(d DraftDeps) *DraftService { return &DraftService{d: d} }

// DraftInput is the content of a draft, as a save replaces it.
type DraftInput struct {
	Text        string
	SpoilerText string
	Sensitive   bool
	// Language is a BCP 47 tag, "" for the author's default post language.
	Language       string
	Visibility     content.Visibility
	InReplyToID    string
	QuotedStatusID string
	// InteractionPolicy is who may reply to, boost, and quote the post, of the kinds chosen.
	InteractionPolicy InteractionPolicyChange
	// Media are the files to attach, in order: uploads of the author, processed or not.
	Media []MediaInput
	Poll  *content.DraftPoll
}

// DraftView is a draft with its content and schedules.
type DraftView struct {
	Draft   content.Draft
	Content content.DraftContent
	// Scheduled is the schedule in effect, nil for none.
	Scheduled *content.ScheduledDraft
	// LastOutcome is the newest schedule that ended (cancelled or failed), nil for none.
	LastOutcome *DraftScheduleRecord
}

// Create saves a new draft of accountID. It fails with ErrAccountUnavailable for an author that is
// not an active local account, content.ErrInvalidDraft (or ErrInvalidVisibility,
// ErrInvalidInteractionPolicy) for content a draft cannot keep, ErrMediaNotFound for a file that
// is not an upload of the author, and a *DraftLimitError of LimitDrafts at max_drafts.
func (s *DraftService) Create(ctx context.Context, accountID string, in DraftInput) (DraftView, error) {
	if _, err := s.d.Statuses.localActive(ctx, accountID); err != nil {
		return DraftView{}, err
	}
	now := s.now()
	d := content.Draft{ID: s.d.IDs.NewID(), AccountID: accountID, CreatedAt: now}
	c, err := s.content(ctx, accountID, d.ID, in, now)
	if err != nil {
		return DraftView{}, err
	}
	limits, err := s.d.Limits(ctx)
	if err != nil {
		return DraftView{}, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		n, err := s.d.Drafts.CountUnscheduledDrafts(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if n >= limits.MaxDrafts {
			return &DraftLimitError{Limit: LimitDrafts, Max: limits.MaxDrafts}
		}
		if err := s.d.Drafts.InsertDraft(ctx, tx, d); err != nil {
			return err
		}
		return s.d.Drafts.SaveDraftContent(ctx, tx, c)
	})
	if err != nil {
		return DraftView{}, err
	}
	return DraftView{Draft: d, Content: c}, nil
}

// Get returns a draft of accountID; another account's draft is ErrNotFound.
func (s *DraftService) Get(ctx context.Context, accountID, id string) (DraftView, error) {
	d, err := s.own(ctx, accountID, id)
	if err != nil {
		return DraftView{}, err
	}
	return s.view(ctx, d)
}

// List returns up to limit drafts of accountID, newest first, before the draft beforeID: every
// draft with a nil scheduled, or the scheduled ones (true) or the others (false).
func (s *DraftService) List(ctx context.Context, accountID, beforeID string, scheduled *bool, limit int) ([]DraftView, error) {
	ds, err := s.d.Drafts.ListDrafts(ctx, accountID, beforeID, scheduled, limit)
	if err != nil {
		return nil, err
	}
	out := make([]DraftView, 0, len(ds))
	for _, d := range ds {
		v, err := s.view(ctx, d)
		if errors.Is(err, ErrNotFound) {
			continue // deleted since the list was read
		}
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Update replaces the content of a draft of accountID, provided that its content is still the
// one saved at ifMatch (its UpdatedAt): ErrDraftChanged otherwise, so that a save on one device
// does not silently overwrite one made on another. A scheduled draft stays scheduled and is
// published with the content it has then. It fails as Create does, without the limit.
func (s *DraftService) Update(ctx context.Context, accountID, id string, ifMatch time.Time, in DraftInput) (DraftView, error) {
	if _, err := s.d.Statuses.localActive(ctx, accountID); err != nil {
		return DraftView{}, err
	}
	if _, err := s.own(ctx, accountID, id); err != nil {
		return DraftView{}, err
	}
	now := s.now()
	c, err := s.content(ctx, accountID, id, in, now)
	if err != nil {
		return DraftView{}, err
	}
	var d content.Draft
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		if d, err = s.lockOwn(ctx, tx, accountID, id); err != nil {
			return err
		}
		prev, err := s.d.Drafts.FindDraftContentTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if !prev.UpdatedAt.Equal(ifMatch) {
			return ErrDraftChanged
		}
		// Each save gets its own time, so that the ETag of a save never repeats another's.
		if !c.UpdatedAt.After(prev.UpdatedAt) {
			c.UpdatedAt = prev.UpdatedAt.Add(time.Millisecond)
		}
		return s.d.Drafts.SaveDraftContent(ctx, tx, c)
	})
	if err != nil {
		return DraftView{}, err
	}
	return s.view(ctx, d)
}

// Delete deletes a draft of accountID with every row of it; its uploads become unattached and go
// with the next cleanup. A schedule in effect goes with it, and its job does nothing.
func (s *DraftService) Delete(ctx context.Context, accountID, id string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.lockOwn(ctx, tx, accountID, id); err != nil {
			return err
		}
		_, err := s.d.Drafts.DeleteDraft(ctx, tx, id)
		return err
	})
}

// Schedule schedules a draft of accountID to be published at publishAt, replacing the schedule in
// effect, which ends as cancelled. The time must be at least scheduled_post_min_lead_minutes and
// at most scheduled_post_max_lead_days ahead, and the account within max_scheduled_posts and
// max_scheduled_posts_per_day (a *DraftLimitError). The draft must pass the checks of a new post
// now (Service.Check; an attachment still being processed passes), and is checked again when it
// is published.
func (s *DraftService) Schedule(ctx context.Context, accountID, id string, publishAt time.Time) (DraftView, error) {
	limits, err := s.d.Limits(ctx)
	if err != nil {
		return DraftView{}, err
	}
	now := s.now()
	publishAt = publishAt.UTC().Truncate(time.Millisecond)
	if publishAt.Before(now.Add(time.Duration(limits.MinLeadMinutes) * time.Minute)) {
		return DraftView{}, &DraftLimitError{Limit: LimitScheduleMinLead, Max: limits.MinLeadMinutes}
	}
	if publishAt.After(now.AddDate(0, 0, limits.MaxLeadDays)) {
		return DraftView{}, &DraftLimitError{Limit: LimitScheduleMaxLead, Max: limits.MaxLeadDays}
	}
	d, err := s.own(ctx, accountID, id)
	if err != nil {
		return DraftView{}, err
	}
	c, err := s.d.Drafts.FindDraftContent(ctx, id)
	if err != nil {
		return DraftView{}, err
	}
	if err := s.d.Statuses.Check(ctx, createInput(d, c, "")); err != nil {
		return DraftView{}, err
	}
	sch := content.DraftSchedule{ID: s.d.IDs.NewID(), DraftID: id, PublishAt: publishAt, CreatedAt: now}
	payload, err := json.Marshal(publishScheduledDraftPayload{ScheduleID: sch.ID})
	if err != nil {
		return DraftView{}, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.lockOwn(ctx, tx, accountID, id); err != nil {
			return err
		}
		if err := s.cancelTx(ctx, tx, id, now); err != nil && !errors.Is(err, ErrDraftNotScheduled) {
			return err
		}
		n, err := s.d.Drafts.CountScheduledDrafts(ctx, tx, accountID, time.Time{}, time.Time{})
		if err != nil {
			return err
		}
		if n >= limits.MaxScheduledPosts {
			return &DraftLimitError{Limit: LimitScheduledPosts, Max: limits.MaxScheduledPosts}
		}
		from, to := content.DraftDay(publishAt)
		if n, err = s.d.Drafts.CountScheduledDrafts(ctx, tx, accountID, from, to); err != nil {
			return err
		}
		if n >= limits.MaxScheduledPerDay {
			return &DraftLimitError{Limit: LimitScheduledPostsPerDay, Max: limits.MaxScheduledPerDay}
		}
		if err := s.d.Drafts.InsertDraftSchedule(ctx, tx, sch); err != nil {
			return err
		}
		err = s.d.Drafts.InsertScheduledDraft(ctx, tx, content.ScheduledDraft{
			DraftID: id, ScheduleID: sch.ID, AccountID: accountID, PublishAt: publishAt,
		})
		if err != nil {
			return err
		}
		return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
			Type: JobPublishScheduledDraft, Payload: payload, IdempotencyKey: "publish_scheduled_draft:" + sch.ID, RunAt: publishAt,
		})
	})
	if err != nil {
		return DraftView{}, err
	}
	return s.view(ctx, d)
}

// Unschedule cancels the schedule in effect of a draft of accountID, which stays a draft. A draft
// without one fails with ErrDraftNotScheduled. The job of the schedule stays in the queue and
// does nothing when it runs.
func (s *DraftService) Unschedule(ctx context.Context, accountID, id string) (DraftView, error) {
	var d content.Draft
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		if d, err = s.lockOwn(ctx, tx, accountID, id); err != nil {
			return err
		}
		return s.cancelTx(ctx, tx, id, s.now())
	})
	if err != nil {
		return DraftView{}, err
	}
	return s.view(ctx, d)
}

// cancelTx ends the schedule in effect of a draft as cancelled.
func (s *DraftService) cancelTx(ctx context.Context, tx uow.Tx, draftID string, now time.Time) error {
	cur, err := s.d.Drafts.FindScheduledDraftTx(ctx, tx, draftID)
	if errors.Is(err, ErrNotFound) {
		return ErrDraftNotScheduled
	}
	if err != nil {
		return err
	}
	if _, err := s.d.Drafts.DeleteScheduledDraft(ctx, tx, cur.ScheduleID); err != nil {
		return err
	}
	return s.d.Drafts.InsertDraftScheduleOutcome(ctx, tx, content.DraftScheduleOutcome{
		ScheduleID: cur.ScheduleID, Outcome: content.DraftCancelled, OccurredAt: now,
	})
}

// Publish publishes a draft of accountID at once as a new post by clientID ("" for none), and
// deletes the draft with every row of it in the same transaction. It fails as Service.Create
// does, and leaves the draft as it was.
func (s *DraftService) Publish(ctx context.Context, accountID, id, clientID string) (Post, error) {
	d, err := s.own(ctx, accountID, id)
	if err != nil {
		return Post{}, err
	}
	c, err := s.d.Drafts.FindDraftContent(ctx, id)
	if err != nil {
		return Post{}, err
	}
	return s.publish(ctx, d, c, "", clientID)
}

// publish posts the content c of the draft d through Service.Create, deleting the draft in the
// transaction that writes the post. With scheduleID, it does so only while that schedule is in
// effect (errScheduleSuperseded otherwise). The post's Idempotency-Key comes from the draft, so a
// second run returns the post of the first.
func (s *DraftService) publish(ctx context.Context, d content.Draft, c content.DraftContent, scheduleID, clientID string) (Post, error) {
	return s.d.Statuses.create(ctx, createInput(d, c, clientID), func(ctx context.Context, tx uow.Tx) error {
		if scheduleID != "" {
			ok, err := s.d.Drafts.DeleteScheduledDraft(ctx, tx, scheduleID)
			if err != nil {
				return err
			}
			if !ok {
				return errScheduleSuperseded
			}
		}
		ok, err := s.d.Drafts.DeleteDraft(ctx, tx, d.ID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("draft %s: %w", d.ID, ErrNotFound)
		}
		return nil
	})
}

type publishScheduledDraftPayload struct {
	ScheduleID string `json:"schedule_id"`
}

// RegisterJobs registers the JobPublishScheduledDraft handler.
func (s *DraftService) RegisterJobs(reg *queue.JobHandlerRegistry) {
	reg.Register(JobPublishScheduledDraft, func(ctx context.Context, j queue.Job) queue.Result {
		var p publishScheduledDraftPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.ScheduleID == "" {
			return queue.Fail(fmt.Errorf("content: publish scheduled draft payload %s: %w", j.Payload, err))
		}
		wait, err := s.PublishScheduled(ctx, p.ScheduleID)
		switch {
		case err != nil:
			return queue.Retry(0, err)
		case wait > 0:
			return queue.Retry(wait, errors.New("content: the schedule is not due yet"))
		}
		return queue.Done()
	})
}

// PublishScheduled publishes the draft of the schedule scheduleID once its time has come, and
// returns how long until it comes when it has not. A schedule no longer in effect (cancelled,
// moved, published, failed, or whose draft is deleted) does nothing, so the job is safe to repeat
// and to run twice at once: one run alone publishes. A draft that cannot be published (the author
// may not post, the status replied to or quoted is gone or hidden, a file is not usable, a check
// or the decision hook refuses it) ends its schedule as failed with the reason and emits
// DraftScheduleFailed; the draft stays. A failure that may pass (the database or the hook could
// not be reached) is returned for the job to retry.
func (s *DraftService) PublishScheduled(ctx context.Context, scheduleID string) (time.Duration, error) {
	cur, err := s.d.Drafts.FindScheduledDraftBySchedule(ctx, scheduleID)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if wait := cur.PublishAt.Sub(s.d.Clock.Now()); wait > 0 {
		return wait, nil
	}
	d, err := s.d.Drafts.FindDraft(ctx, cur.DraftID)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	c, err := s.d.Drafts.FindDraftContent(ctx, cur.DraftID)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	_, err = s.publish(ctx, d, c, scheduleID, "")
	switch {
	case err == nil, errors.Is(err, errScheduleSuperseded), errors.Is(err, ErrNotFound), errors.Is(err, ErrAlreadyDeleted):
		// Published, by this run or another; or no longer in effect.
		return 0, nil
	}
	reason, ok := DraftFailureReasonOf(err)
	if !ok {
		return 0, err
	}
	return 0, s.fail(ctx, cur, reason)
}

// fail ends the schedule cur as failed with reason, unless it is no longer in effect.
func (s *DraftService) fail(ctx context.Context, cur content.ScheduledDraft, reason content.DraftFailureReason) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := s.d.Drafts.DeleteScheduledDraft(ctx, tx, cur.ScheduleID)
		if err != nil || !ok {
			return err
		}
		o := content.DraftScheduleOutcome{
			ScheduleID: cur.ScheduleID, Outcome: content.DraftFailed, FailureReason: reason, OccurredAt: s.now(),
		}
		if err := s.d.Drafts.InsertDraftScheduleOutcome(ctx, tx, o); err != nil {
			return err
		}
		return tx.Emit(ctx, DraftScheduleFailed{
			Schedule:  content.DraftSchedule{ID: cur.ScheduleID, DraftID: cur.DraftID, PublishAt: cur.PublishAt},
			AccountID: cur.AccountID, Outcome: o,
		})
	})
}

// DraftFailureReasonOf returns the reason that a failure of Service.Create gives a scheduled draft,
// and false for a failure that may pass on a retry.
func DraftFailureReasonOf(err error) (content.DraftFailureReason, bool) {
	var rejected *RejectedByReviewError
	switch {
	case errors.As(err, &rejected):
		return content.DraftFailureRejected, true
	case errors.Is(err, ErrAccountUnavailable), errors.Is(err, ErrRemoteAccount), errors.Is(err, ErrPostingRestricted),
		errors.Is(err, ErrInteractionRestricted):
		return content.DraftFailureRestricted, true
	case errors.Is(err, ErrReplyTargetNotFound), errors.Is(err, ErrReplyNotAllowed):
		return content.DraftFailureReplyUnavailable, true
	case errors.Is(err, ErrQuoteTargetNotFound), errors.Is(err, ErrQuoteNotAllowed), errors.Is(err, ErrQuoteRemoteAudience):
		return content.DraftFailureQuoteUnavailable, true
	case errors.Is(err, ErrMediaNotFound), errors.Is(err, ErrMediaNotReady):
		return content.DraftFailureMediaUnavailable, true
	case errors.Is(err, ErrEmptyText), errors.Is(err, ErrTextTooLong), errors.Is(err, ErrInvalidText),
		errors.Is(err, ErrInvalidLanguage), errors.Is(err, ErrTooManyMedia), errors.Is(err, ErrInvalidMediaDescription),
		errors.Is(err, ErrMissingMediaDescription), errors.Is(err, ErrPollWithMedia), errors.Is(err, ErrLocalVisibilityUnavailable),
		errors.Is(err, content.ErrInvalidPoll), errors.Is(err, content.ErrInvalidVisibility),
		errors.Is(err, content.ErrInvalidInteractionPolicy), errors.Is(err, content.ErrInvalidIdempotencyKey):
		return content.DraftFailureInvalid, true
	}
	return "", false
}

// RebuildScheduledDrafts rebuilds the projection scheduled_drafts from the schedules without an
// outcome (ADR-0020 rule 1), for `my projection rebuild`.
func RebuildScheduledDrafts(ctx context.Context, u uow.UnitOfWork, r DraftRepository) error {
	return u.Tx(ctx, func(tx uow.Tx) error { return r.RebuildScheduledDrafts(ctx, tx) })
}

// EraseDrafts deletes every draft of an account inside tx with every row of it.
func EraseDrafts(ctx context.Context, tx uow.Tx, repo DraftRepository, accountID string) error {
	ids, err := repo.ListDraftIDsOf(ctx, tx, accountID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := repo.DeleteDraft(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

// own returns the draft id of accountID; another account's draft is ErrNotFound.
func (s *DraftService) own(ctx context.Context, accountID, id string) (content.Draft, error) {
	d, err := s.d.Drafts.FindDraft(ctx, id)
	if err != nil {
		return content.Draft{}, err
	}
	if d.AccountID != accountID {
		return content.Draft{}, fmt.Errorf("draft %s: %w", id, ErrNotFound)
	}
	return d, nil
}

// lockOwn is own inside tx, holding the draft until tx ends.
func (s *DraftService) lockOwn(ctx context.Context, tx uow.Tx, accountID, id string) (content.Draft, error) {
	d, err := s.d.Drafts.LockDraft(ctx, tx, id)
	if err != nil {
		return content.Draft{}, err
	}
	if d.AccountID != accountID {
		return content.Draft{}, fmt.Errorf("draft %s: %w", id, ErrNotFound)
	}
	return d, nil
}

func (s *DraftService) view(ctx context.Context, d content.Draft) (DraftView, error) {
	c, err := s.d.Drafts.FindDraftContent(ctx, d.ID)
	if err != nil {
		return DraftView{}, err
	}
	v := DraftView{Draft: d, Content: c}
	cur, err := s.d.Drafts.FindScheduledDraft(ctx, d.ID)
	switch {
	case err == nil:
		v.Scheduled = &cur
	case !errors.Is(err, ErrNotFound):
		return DraftView{}, err
	}
	last, err := s.d.Drafts.LatestDraftScheduleOutcome(ctx, d.ID)
	switch {
	case err == nil:
		v.LastOutcome = &last
	case !errors.Is(err, ErrNotFound):
		return DraftView{}, err
	}
	return v, nil
}

// content checks in and returns it as the content of the draft draftID of accountID, saved at now:
// the attachments must be uploads of the author, processed or not.
func (s *DraftService) content(ctx context.Context, accountID, draftID string, in DraftInput, now time.Time) (content.DraftContent, error) {
	if err := in.InteractionPolicy.Validate(); err != nil {
		return content.DraftContent{}, err
	}
	c := content.DraftContent{
		DraftID: draftID, Text: in.Text, SpoilerText: in.SpoilerText, Sensitive: in.Sensitive, Language: in.Language,
		Visibility: in.Visibility, InReplyToID: in.InReplyToID, QuotedStatusID: in.QuotedStatusID, Poll: in.Poll, UpdatedAt: now,
	}
	if len(in.InteractionPolicy) > 0 {
		c.InteractionPolicy = map[content.InteractionKind]content.InteractionSetting(in.InteractionPolicy)
	}
	for i, m := range in.Media {
		c.Attachments = append(c.Attachments, content.DraftAttachment{Position: i, MediaID: m.MediaID, Description: m.Description})
	}
	if err := c.Validate(); err != nil {
		return content.DraftContent{}, err
	}
	for _, a := range c.Attachments {
		if err := s.ownUpload(ctx, accountID, a.MediaID); err != nil {
			return content.DraftContent{}, err
		}
	}
	return c, nil
}

// ownUpload fails with ErrMediaNotFound unless mediaID is an upload of accountID.
func (s *DraftService) ownUpload(ctx context.Context, accountID, mediaID string) error {
	notFound := fmt.Errorf("media %s: %w", mediaID, ErrMediaNotFound)
	if s.d.Media == nil {
		return notFound
	}
	f, err := s.d.Media.FindMediaFile(ctx, mediaID)
	if errors.Is(err, ErrNotFound) {
		return notFound
	}
	if err != nil {
		return err
	}
	if f.OwnerAccountID != accountID || f.Origin != content.MediaOriginUpload {
		return notFound
	}
	return nil
}

// now returns the time of a command, to the millisecond that both databases keep.
func (s *DraftService) now() time.Time { return s.d.Clock.Now().UTC().Truncate(time.Millisecond) }

// createInput returns the new post that publishing the content c of the draft d creates.
func createInput(d content.Draft, c content.DraftContent, clientID string) CreateInput {
	in := CreateInput{
		TextInput: TextInput{
			Text: c.Text, SpoilerText: c.SpoilerText, Sensitive: c.Sensitive, Language: c.Language,
		},
		AuthorAccountID: d.AccountID, Visibility: c.Visibility, InReplyToID: c.InReplyToID,
		QuotedStatusID: c.QuotedStatusID, ClientID: clientID, IdempotencyKey: content.DraftIdempotencyKey(d.ID),
	}
	for _, a := range c.Attachments {
		in.Media = append(in.Media, MediaInput{MediaID: a.MediaID, Description: a.Description})
	}
	if len(c.InteractionPolicy) > 0 {
		in.InteractionPolicy = InteractionPolicyChange(c.InteractionPolicy)
	}
	if p := c.Poll; p != nil {
		in.Poll = &content.PollInput{Options: p.Options, Duration: p.Duration, Multiple: p.Multiple, HideTotals: p.HideTotals}
	}
	return in
}

// DraftSchedules returns the schedules of ids that exist, with their outcomes, by ID: what the
// notifications of failed schedules show.
func (s *DraftService) DraftSchedules(ctx context.Context, ids []string) (map[string]DraftScheduleRecord, error) {
	return s.d.Drafts.DraftSchedules(ctx, ids)
}
