// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/moderationtest"
)

func TestDomainListContract(t *testing.T) {
	t.Parallel()
	moderationtest.DomainListContract(t, func(t *testing.T, opts uow.Options) moderationtest.ModerationHarness {
		db, _ := newTestDB(t, "", opts)
		return moderationtest.ModerationHarness{UnitOfWork: db, Moderation: NewRepository(db)}
	})
}
