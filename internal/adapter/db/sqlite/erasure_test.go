// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestAccountErasureContract(t *testing.T) {
	t.Parallel()
	federationtest.AccountErasureContract(t, func(t *testing.T, opts uow.Options) federationtest.AccountErasureHarness {
		db, _ := newTestDB(t, "", opts)
		return federationtest.AccountErasureHarness{
			RetentionHarness: federationtest.RetentionHarness{
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
			},
			AccountErasure:      db.AccountRepository(),
			IdentityErasure:     db.IdentityRepository(),
			ContentErasure:      db.StatusRepository(),
			GraphErasure:        db.GraphRepository(),
			TimelineErasure:     db.TimelineRepository(),
			NotificationErasure: db.NotificationRepository(),
			Rows: func(t *testing.T, table, column, value string) int {
				t.Helper()
				var n int
				// The table and column names come from the contract, never from input.
				q := "SELECT count(*) FROM " + table + " WHERE " + column + " = ?"
				if err := db.db.QueryRowContext(t.Context(), q, value).Scan(&n); err != nil {
					t.Fatalf("count %s: %v", table, err)
				}
				return n
			},
		}
	})
}
