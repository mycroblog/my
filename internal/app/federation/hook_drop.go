// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"

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
