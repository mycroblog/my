// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/moderationtest"
)

func TestAppealContract(t *testing.T) {
	moderationtest.AppealContract(t, func(t *testing.T, opts uow.Options) moderationtest.ModerationHarness {
		db := newTestDB(t, opts)
		return moderationtest.ModerationHarness{UnitOfWork: db, Moderation: db.ModerationRepository()}
	})
}
