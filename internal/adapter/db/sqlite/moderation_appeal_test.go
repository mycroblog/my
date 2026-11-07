// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestAppealContract(t *testing.T) {
	t.Parallel()
	testkit.AppealContract(t, func(t *testing.T, opts uow.Options) testkit.ModerationHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.ModerationHarness{UnitOfWork: db, Moderation: db.ModerationRepository()}
	})
}
