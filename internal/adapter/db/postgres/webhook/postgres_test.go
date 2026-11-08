// SPDX-License-Identifier: AGPL-3.0-only

package webhook

import (
	"testing"

	postgresmoderation "github.com/mycroblog/my/internal/adapter/db/postgres/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/webhooktest"
)

func TestWebhookRepositoryContract(t *testing.T) {
	webhooktest.WebhookRepositoryContract(t, func(t *testing.T, opts uow.Options) webhooktest.WebhookHarness {
		db := newTestDB(t, opts)
		return webhooktest.WebhookHarness{UnitOfWork: db, Webhooks: NewRepository(db), Moderation: postgresmoderation.NewRepository(db)}
	})
}
