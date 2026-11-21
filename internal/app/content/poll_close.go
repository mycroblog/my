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

// This file holds the end of polls (FR-112). A poll ends at its closes_at without anything
// written: whether it takes votes is decided by the time (content.Poll.Closed). What happens at
// the end is a job, enqueued with the poll to run at its closes_at: it emits PollClosed, from
// which the notification module tells the author and the local voters (poll_closed) and the
// federation module sends the Update of a local poll with its final counts.

const (
	// JobClosePoll emits PollClosed for one poll once its end has passed. Its payload is
	// closePollPayload.
	JobClosePoll = "content.close_poll"
	// EventPollClosed is emitted when a poll that local accounts take part in has ended.
	EventPollClosed = "content.poll_closed"
	// pollVotersPage is the number of voters one ListPollVoterIDs call reads.
	pollVotersPage = 500
)

// PollClosed is emitted once the end of a poll has passed: a local poll, or a remote one that
// a local account voted in.
type PollClosed struct {
	Poll content.Poll
	// Status is the status whose current revision has the poll.
	Status StatusRef
	// VoterAccountIDs are the accounts that voted in the poll, local and remote.
	VoterAccountIDs []string
}

// EventType implements uow.Event.
func (PollClosed) EventType() string { return EventPollClosed }

// PollCloserDeps are the ports of a PollCloser.
type PollCloserDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	// Outbox enqueues JobClosePoll in the transaction that posts the poll or votes in it.
	Outbox queue.Outbox
	Clock  kernel.Clock
}

// PollCloser runs JobClosePoll.
type PollCloser struct {
	d PollCloserDeps
}

// NewPollCloser returns the PollCloser on d.
func NewPollCloser(d PollCloserDeps) *PollCloser {
	return &PollCloser{d: d}
}

type closePollPayload struct {
	PollID string `json:"poll_id"`
}

// RegisterProjectors enqueues JobClosePoll, to run at the poll's end, when a local status with a
// poll that ends is posted (on a board of an event or not), and when a local account votes in a
// remote poll that ends. The job's
// idempotency key is the poll's, so the votes of a poll enqueue it once while it is pending.
func (c *PollCloser) RegisterProjectors(r *uow.Projectors) {
	r.Register(EventStatusCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(StatusCreated)
		if !ev.Status.IsLocal || ev.Revision.Poll == nil {
			return nil
		}
		return c.enqueue(ctx, tx, *ev.Revision.Poll)
	})
	r.Register(EventBoardStatusPosted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(BoardStatusPosted)
		if ev.Revision.Poll == nil {
			return nil
		}
		return c.enqueue(ctx, tx, *ev.Revision.Poll)
	})
	r.Register(EventPollVoted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(PollVoted)
		if ev.Status.IsLocal {
			return nil // enqueued when the status was posted
		}
		return c.enqueue(ctx, tx, ev.Poll)
	})
}

func (c *PollCloser) enqueue(ctx context.Context, tx uow.Tx, p content.Poll) error {
	if p.ClosesAt == nil || c.d.Outbox == nil {
		return nil
	}
	b, err := json.Marshal(closePollPayload{PollID: p.ID})
	if err != nil {
		return err
	}
	return c.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
		Type: JobClosePoll, Payload: b, IdempotencyKey: "close_poll:" + p.ID, RunAt: *p.ClosesAt,
	})
}

// PollVoterIDs returns at most limit accounts that voted in a poll, ordered by ID, after the ID
// after ("" for the first page): the federation module sends the Update of a local poll to the
// remote ones.
func (s *Service) PollVoterIDs(ctx context.Context, pollID, after string, limit int) ([]string, error) {
	return s.d.Repository.ListPollVoterIDs(ctx, pollID, after, limit)
}

// RegisterJobs registers the JobClosePoll handler.
func (c *PollCloser) RegisterJobs(reg *queue.JobHandlerRegistry) {
	reg.Register(JobClosePoll, func(ctx context.Context, j queue.Job) queue.Result {
		var p closePollPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.PollID == "" {
			return queue.Fail(fmt.Errorf("content: close poll payload %s: %w", j.Payload, err))
		}
		wait, err := c.Close(ctx, p.PollID)
		switch {
		case err != nil:
			return queue.Retry(0, err)
		case wait > 0:
			return queue.Retry(wait, errors.New("content: the poll has not ended yet"))
		}
		return queue.Done()
	})
}

// Close emits PollClosed for the poll pollID, with its voters, once its end has passed. A poll
// that is not the one of its status's current revision any more (the status is deleted or a
// remote edit replaced it) is left alone. A poll that has not ended yet returns how long until
// it does. It is safe to repeat: the notifications are once per poll, and the federation
// module's Update of a local poll is the same document again.
func (c *PollCloser) Close(ctx context.Context, pollID string) (time.Duration, error) {
	repo := c.d.Repository
	statusID, err := repo.PollStatusID(ctx, pollID)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	st, err := repo.FindStatus(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	rev, err := repo.FindCurrentRevision(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return 0, nil // deleted
	}
	if err != nil {
		return 0, err
	}
	poll := rev.Poll
	if poll == nil || poll.ID != pollID || poll.ClosesAt == nil {
		return 0, nil
	}
	if now := c.d.Clock.Now(); !poll.Closed(now) {
		return poll.ClosesAt.Sub(now), nil
	}
	var voters []string
	for after := ""; ; {
		page, err := repo.ListPollVoterIDs(ctx, pollID, after, pollVotersPage)
		if err != nil {
			return 0, err
		}
		voters = append(voters, page...)
		if len(page) < pollVotersPage {
			break
		}
		after = page[len(page)-1]
	}
	return 0, c.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		deleted, err := repo.IsDeleted(ctx, tx, statusID)
		if err != nil || deleted {
			return err
		}
		return tx.Emit(ctx, PollClosed{Poll: *poll, Status: RefOf(st), VoterAccountIDs: voters})
	})
}
