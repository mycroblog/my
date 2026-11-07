// SPDX-License-Identifier: AGPL-3.0-only

// Package webhook_test runs the webhook flows (testkit.WebhookFlows) once, on SQLite (ADR-0153 §1).
package webhook_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestWebhookFlows(t *testing.T) {
	t.Parallel()
	testkit.WebhookFlows(t, func(t *testing.T, opts uow.Options) testkit.WebhookHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.WebhookHarness{UnitOfWork: db, Webhooks: db.WebhookRepository(), Moderation: db.ModerationRepository()}
	})
}
