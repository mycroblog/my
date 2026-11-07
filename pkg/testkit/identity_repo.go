// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// identityRepo writes the rows of the identity repository directly, for the port contracts of
// that repository (ADR-0153 §1): no service of any module runs on it.
type identityRepo struct {
	h     AccountHarness
	clock *Clock
	ids   *IDGenerator
}

// newIdentityRepo returns an identityRepo on a new harness whose transactions retry.
func newIdentityRepo(t *testing.T, newHarness AccountFactory) *identityRepo {
	t.Helper()
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}})
	clock := NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
	return &identityRepo{h: h, clock: clock, ids: NewIDGenerator(clock)}
}

func (r *identityRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// user inserts a user a second later than the previous one and returns its ID.
func (r *identityRepo) user(t *testing.T) string {
	t.Helper()
	r.clock.Advance(time.Second)
	u := identity.User{ID: r.ids.NewID(), CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.InsertUser(ctx, tx, u) })
	return u.ID
}

// role ensures a role with key at position with perms and returns it.
func (r *identityRepo) role(t *testing.T, key string, position int, perms ...identity.Permission) identity.Role {
	t.Helper()
	var out identity.Role
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		out, err = r.h.Identity.EnsureRole(ctx, tx, identity.Role{ID: r.ids.NewID(), Key: key, Position: position, CreatedAt: r.clock.Now()}, perms)
		return err
	})
	return out
}

// grant grants the role to the user and returns the grant.
func (r *identityRepo) grant(t *testing.T, userID, roleID string) identity.RoleGrant {
	t.Helper()
	r.clock.Advance(time.Second)
	g := identity.RoleGrant{ID: r.ids.NewID(), UserID: userID, RoleID: roleID, GrantedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.InsertRoleGrant(ctx, tx, g) })
	return g
}

// revoke revokes a grant.
func (r *identityRepo) revoke(t *testing.T, grantID string) {
	t.Helper()
	r.clock.Advance(time.Second)
	rev := identity.RoleRevocation{GrantID: grantID, RevokedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.InsertRoleRevocation(ctx, tx, rev) })
}
