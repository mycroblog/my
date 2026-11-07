// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/moderationtest"
)

func TestModerationRetentionContract(t *testing.T) {
	t.Parallel()
	moderationtest.ModerationRetentionContract(t, func(t *testing.T, opts uow.Options) moderationtest.ModerationHarness {
		db, _ := newTestDB(t, "", opts)
		return moderationtest.ModerationHarness{UnitOfWork: db, Moderation: db.ModerationRepository()}
	})
}
