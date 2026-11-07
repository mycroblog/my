// SPDX-License-Identifier: AGPL-3.0-only

package webhook

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/webhookgen"
	"github.com/mycroblog/my/internal/app/uow"
)

// RebuildHeads implements webhook.Repository.
func (r *Repository) RebuildHeads(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild webhook endpoint heads",
		(*webhookgen.Queries).DeleteAllWebhookEndpointHeads, (*webhookgen.Queries).RebuildWebhookEndpointHeads)
}

// RebuildFailures implements webhook.Repository.
func (r *Repository) RebuildFailures(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild webhook endpoint failures",
		(*webhookgen.Queries).DeleteDerivableWebhookEndpointFailures, (*webhookgen.Queries).RebuildWebhookEndpointFailures)
}
