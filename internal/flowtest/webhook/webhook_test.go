// SPDX-License-Identifier: AGPL-3.0-only

// Package webhook_test runs the webhook flows (webhooktest.WebhookFlows) once, on SQLite (ADR-0153 §1).
package webhook_test

import (
	"testing"

	sqlitemoderation "github.com/mycroblog/my/internal/adapter/db/sqlite/moderation"
	sqlitewebhook "github.com/mycroblog/my/internal/adapter/db/sqlite/webhook"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/webhooktest"
)

func TestWebhookFlows(t *testing.T) {
	t.Parallel()
	webhooktest.WebhookFlows(t, func(t *testing.T, opts uow.Options) webhooktest.WebhookHarness {
		db := flowtest.SQLite(t, opts)
		return webhooktest.WebhookHarness{UnitOfWork: db, Webhooks: sqlitewebhook.NewRepository(db), Moderation: sqlitemoderation.NewRepository(db)}
	})
}
