// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"context"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/pkg/testkit"
)

// identityRepo writes the rows of the identity repository directly, for the port contracts of
// that repository (ADR-0153 §1): no service of any module runs on it.
type identityRepo struct {
	h     AccountHarness
	clock *testkit.Clock
	ids   *testkit.IDGenerator
}

// newIdentityRepo returns an identityRepo on a new harness whose transactions retry.
func newIdentityRepo(t *testing.T, newHarness AccountFactory) *identityRepo {
	t.Helper()
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}})
	clock := testkit.NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
	return &identityRepo{h: h, clock: clock, ids: testkit.NewIDGenerator(clock)}
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

// localAccount inserts the local account of userID with username and its handle, and returns
// its ID.
func (r *identityRepo) localAccount(t *testing.T, userID, username string) string {
	t.Helper()
	r.clock.Advance(time.Second)
	id := r.ids.NewID()
	normalized := account.NormalizeUsername(username)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		a := account.Account{ID: id, URI: "https://" + testkit.ContractHost + "/ap/actors/" + id, Domain: account.LocalDomain, ActorType: account.ActorPerson, CreatedAt: r.clock.Now()}
		if err := r.h.Accounts.InsertAccount(ctx, tx, a); err != nil {
			return err
		}
		if err := r.h.Accounts.InsertLocalAccount(ctx, tx, account.LocalAccount{AccountID: id, UserID: userID, Username: username, UsernameNormalized: normalized}); err != nil {
			return err
		}
		return r.h.Accounts.UpsertHandle(ctx, tx, account.Handle{AccountID: id, Username: username, UsernameNormalized: normalized, Domain: account.LocalDomain})
	})
	return id
}

// remoteAccount inserts a remote account of domain, with a handle when username is not "", and
// returns its ID.
func (r *identityRepo) remoteAccount(t *testing.T, domain, username string) string {
	t.Helper()
	r.clock.Advance(time.Second)
	id := r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		a := account.Account{ID: id, URI: "https://" + domain + "/users/" + id, Domain: domain, ActorType: account.ActorPerson, CreatedAt: r.clock.Now()}
		if err := r.h.Accounts.InsertAccount(ctx, tx, a); err != nil || username == "" {
			return err
		}
		return r.h.Accounts.UpsertHandle(ctx, tx, account.Handle{AccountID: id, Username: username, UsernameNormalized: account.NormalizeUsername(username), Domain: domain})
	})
	return id
}

// lifecycle puts an account in a lifecycle state, as the account module's projector does.
func (r *identityRepo) lifecycle(t *testing.T, accountID string, state account.LifecycleStateName) {
	t.Helper()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Accounts.UpsertLifecycleState(ctx, tx, account.LifecycleState{AccountID: accountID, State: state, Since: r.clock.Now()})
	})
}

// revoke revokes a grant.
func (r *identityRepo) revoke(t *testing.T, grantID string) {
	t.Helper()
	r.clock.Advance(time.Second)
	rev := identity.RoleRevocation{GrantID: grantID, RevokedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.InsertRoleRevocation(ctx, tx, rev) })
}
