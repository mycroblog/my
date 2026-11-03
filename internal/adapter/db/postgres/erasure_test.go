// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestAccountErasureContract(t *testing.T) {
	testkit.AccountErasureContract(t, func(t *testing.T, opts uow.Options) testkit.AccountErasureHarness {
		db := newTestDB(t, opts)
		return testkit.AccountErasureHarness{
			RetentionHarness: testkit.RetentionHarness{
				RemoteStatusHarness: testkit.RemoteStatusHarness{
					AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
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
				q := "SELECT count(*) FROM " + table + " WHERE " + column + " = $1"
				if err := db.pool.QueryRow(t.Context(), q, value).Scan(&n); err != nil {
					t.Fatalf("count %s: %v", table, err)
				}
				return n
			},
		}
	})
}
