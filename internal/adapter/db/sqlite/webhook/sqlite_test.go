// SPDX-License-Identifier: AGPL-3.0-only

package webhook

import (
	"testing"

	sqlitemoderation "github.com/mycroblog/my/internal/adapter/db/sqlite/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/webhooktest"
)

func TestWebhookRepositoryContract(t *testing.T) {
	t.Parallel()
	webhooktest.WebhookRepositoryContract(t, func(t *testing.T, opts uow.Options) webhooktest.WebhookHarness {
		db, _ := newTestDB(t, "", opts)
		return webhooktest.WebhookHarness{UnitOfWork: db, Webhooks: NewRepository(db), Moderation: sqlitemoderation.NewRepository(db)}
	})
}
