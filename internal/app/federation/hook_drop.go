// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"time"

	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// RequeueDropped returns what cancelling a decision of an inbound hook undoes (ADR-0147 §9): a
// dropped activity whose document is still kept goes back in the queue, and its run reads the
// cancelled decision as an allow without calling the plugin. Past ReceivedDocumentRetention there
// is nothing to run, and the cancellation stands alone. The key of an inbound decision is the URI of
// the received activity.
func RequeueDropped(repo InboxRepository, outbox queue.Outbox) apphook.OverrideEffect {
	return func(ctx context.Context, tx uow.Tx, r apphook.Record) error {
		if r.Decision.Outcome != apphook.Drop {
			return nil
		}
		uri := federation.IRI(r.Decision.SubjectKey)
		received, err := repo.KeptReceivedActivityTx(ctx, tx, uri)
		switch {
		case errors.Is(err, ErrNotFound):
			return nil
		case err != nil:
			return fmt.Errorf("read the dropped activity %s: %w", uri, err)
		}
		job, err := InboxJob(received)
		if err != nil {
			return err
		}
		return outbox.EnqueueTx(ctx, tx, job)
	}
}

// RestorableUntilOf is RestorableUntil of each of activityURIs whose record is left, by URI,
// with one query.
func (i *Inbox) RestorableUntilOf(ctx context.Context, activityURIs []string) (map[string]time.Time, error) {
	iris := make([]federation.IRI, len(activityURIs))
	for k, u := range activityURIs {
		iris[k] = federation.IRI(u)
	}
	received, err := i.d.Repository.FindReceivedActivities(ctx, iris)
	if err != nil {
		return nil, err
	}
	out := make(map[string]time.Time, len(received))
	for uri, r := range received {
		out[string(uri)] = r.ReceivedAt.Add(ReceivedDocumentRetention)
	}
	return out, nil
}

// RestorableUntil returns until when cancelling the drop of the received activity activityURI runs
// it again: ReceivedDocumentRetention after the activity was received, when the purge removes its
// document. A decision can come long after the activity arrived (a job retried, or re-run by hand),
// so the time of the decision does not tell. ok is false when the record of the activity is gone,
// which the purge removes only after the document.
func (i *Inbox) RestorableUntil(ctx context.Context, activityURI string) (until time.Time, ok bool, err error) {
	received, err := i.d.Repository.FindReceivedActivity(ctx, federation.IRI(activityURI))
	switch {
	case errors.Is(err, ErrNotFound):
		return time.Time{}, false, nil
	case err != nil:
		return time.Time{}, false, err
	}
	return received.ReceivedAt.Add(ReceivedDocumentRetention), true, nil
}
