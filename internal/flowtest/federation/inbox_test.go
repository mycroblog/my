// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

// inboxHarness is the inbox's harness on a new SQLite database with every repository that the
// inbox flows write through.
func inboxHarness(t *testing.T, opts uow.Options) testkit.InboxHarness {
	db := flowtest.SQLite(t, opts)
	feds := db.FederationRepository()
	return testkit.InboxHarness{
		UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: db.AccountRepository(), Statuses: db.StatusRepository(),
		Graph: db.GraphRepository(), Moderation: db.ModerationRepository(),
	}
}

func TestInboxFlows(t *testing.T) {
	t.Parallel()
	testkit.InboxFlows(t, inboxHarness)
}

func TestInboxProcessingFlows(t *testing.T) {
	t.Parallel()
	testkit.InboxProcessingFlows(t, inboxHarness)
}
