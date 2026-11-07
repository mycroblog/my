// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

// retentionHarness is the harness of the retention flows on db.
func retentionHarness(db *sqlite.DB) federationtest.RetentionHarness {
	return federationtest.RetentionHarness{
		RemoteStatusHarness: federationtest.RemoteStatusHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
			Statuses:       db.StatusRepository(),
			Media:          db.MediaRepository(),
			Timeline:       db.TimelineRepository(),
			Notifications:  db.NotificationRepository(),
			Inbox:          db.FederationRepository(),
		},
		Interactions: db.InteractionRepository(),
		Federation:   db.FederationRepository(),
		Retention:    db.StatusRepository(),
		Purges:       db.FederationRepository(),
		Instances:    db.FederationRepository(),
		Profiles:     db.AccountRepository(),
		Emojis:       db.CustomEmojiRepository(),
	}
}

func TestRetentionFlows(t *testing.T) {
	t.Parallel()
	federationtest.RetentionFlows(t, func(t *testing.T, opts uow.Options) federationtest.RetentionHarness {
		return retentionHarness(flowtest.SQLite(t, opts))
	})
}
