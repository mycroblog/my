// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestAccountRestrictionsLockSerializesLifts lifts two actions on one account in two
// transactions at once. Under READ COMMITTED each would read the other's row as still in place
// and project a restriction that no action imposes any more; LockAccountRestrictions makes the
// second wait for the first, so it reads the committed rows.
func TestAccountRestrictionsLockSerializesLifts(t *testing.T) {
	db := newTestDB(t, uow.Options{})
	repo := db.ModerationRepository()
	ctx := t.Context()
	accountID, first, second := testkit.NamedID("account"), testkit.NamedID("first"), testkit.NamedID("second")
	err := db.Tx(ctx, func(tx uow.Tx) error {
		for _, id := range []string{first, second} {
			r := moderation.RestrictionRow{Target: accountID, Restriction: moderation.RestrictPost, SourceActionID: id}
			if err := repo.InsertAccountRestriction(ctx, tx, r); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	locked, release := make(chan struct{}), make(chan struct{})
	lift := make(chan error, 1)
	go func() {
		lift <- db.Tx(ctx, func(tx uow.Tx) error {
			if err := repo.LockAccountRestrictions(ctx, tx, accountID); err != nil {
				return err
			}
			if err := repo.DeleteAccountRestrictionsOf(ctx, tx, first); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	var after []moderation.RestrictionRow
	other := make(chan error, 1)
	go func() {
		other <- db.Tx(ctx, func(tx uow.Tx) error {
			if err := repo.LockAccountRestrictions(ctx, tx, accountID); err != nil {
				return err
			}
			if err := repo.DeleteAccountRestrictionsOf(ctx, tx, second); err != nil {
				return err
			}
			rows, err := repo.AccountRestrictionRows(ctx, tx, accountID)
			after = rows
			return err
		})
	}()
	waitForLockWaiter(t, db)
	close(release)
	if err := <-lift; err != nil {
		t.Fatalf("first lift: %v", err)
	}
	if err := <-other; err != nil {
		t.Fatalf("second lift: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("rows that the second lift read = %+v, want none", after)
	}
}
