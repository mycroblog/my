// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestTokenGrantsLockSerializesRotationAndRevocation rotates a refresh token in one transaction
// while another revokes the token's family. Under READ COMMITTED the revocation would not see
// the new token of a rotation that commits while the revocation runs; LockTokenGrants makes it
// wait for the rotation, so the new token ends up revoked with its family.
func TestTokenGrantsLockSerializesRotationAndRevocation(t *testing.T) {
	db := newTestDB(t, uow.Options{})
	repo := db.IdentityRepository()
	ctx := t.Context()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	userID, familyID, rotatedID := testkit.NamedID("user"), testkit.NamedID("family"), testkit.NamedID("rotated")
	token := func(id, name string) identity.AccessToken {
		return identity.AccessToken{
			ID: id, FamilyID: familyID, ClientID: "client", UserID: userID, Scope: "read",
			AccessTokenHash: appidentity.HashToken("at-" + name), RefreshTokenHash: appidentity.HashToken("rt-" + name),
			IssuedAt: now, ExpiresAt: now.Add(time.Hour), RefreshExpiresAt: now.Add(24 * time.Hour),
		}
	}
	if err := db.Tx(ctx, func(tx uow.Tx) error { return repo.InsertAccessToken(ctx, tx, token(familyID, "first")) }); err != nil {
		t.Fatal(err)
	}

	locked, release := make(chan struct{}), make(chan struct{})
	rotation := make(chan error, 1)
	go func() {
		rotation <- db.Tx(ctx, func(tx uow.Tx) error {
			if err := repo.LockTokenGrants(ctx, tx, userID); err != nil {
				return err
			}
			rev := identity.TokenRevocation{TokenID: familyID, Reason: identity.TokenRevokedRotation, RevokedAt: now}
			if _, err := repo.InsertTokenRevocation(ctx, tx, rev); err != nil {
				return err
			}
			close(locked)
			<-release
			next := token(rotatedID, "rotated")
			next.RotatedFromTokenID = familyID
			return repo.InsertAccessToken(ctx, tx, next)
		})
	}()
	<-locked
	revocation := make(chan error, 1)
	go func() {
		revocation <- db.Tx(ctx, func(tx uow.Tx) error {
			if err := repo.LockTokenGrants(ctx, tx, userID); err != nil {
				return err
			}
			return repo.RevokeTokenFamily(ctx, tx, familyID, identity.TokenRevokedUser, now)
		})
	}()
	waitForLockWaiter(t, db)
	close(release)
	if err := <-rotation; err != nil {
		t.Fatalf("rotation: %v", err)
	}
	if err := <-revocation; err != nil {
		t.Fatalf("revocation: %v", err)
	}
	_, state, err := repo.FindAccessTokenByRefreshHash(ctx, appidentity.HashToken("rt-rotated"))
	if err != nil || !state.Revoked {
		t.Errorf("rotated token = %+v, %v; want it revoked with its family", state, err)
	}
}

// waitForLockWaiter waits until a transaction of the test's database waits for a lock.
func waitForLockWaiter(t *testing.T, db *DB) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		err := db.pool.QueryRow(t.Context(), `SELECT count(*) FROM pg_locks l JOIN pg_stat_activity a ON a.pid = l.pid
			WHERE NOT l.granted AND a.datname = current_database()`).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no transaction waits for a lock")
}
