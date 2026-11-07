// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
)

// AdminAccountContract verifies the reads of the moderators' account management (FR-714) in the
// identity and account repositories of a database adapter, by their methods alone, the port
// contract of ADR-0153 §1: the origins of sessions and registrations, found and purged oldest
// first in bounded batches except the held users; the users of an email domain, of an IP address
// (registered or signed in), and of a registration state, each newest first in pages; the newest
// session origins of a user; and the account search by username prefix, domain, and origin, and
// the accounts in a lifecycle state, newest first in pages. The account management on top of it
// is AdminAccountFlows, which internal/flowtest/identity runs once on SQLite.
func AdminAccountContract(t *testing.T, newHarness AdminAccountFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *identityRepo)
	}{
		{"FindsAndPurgesOrigins", adminAccountRepoOrigins},
		{"ListsUsersByEmailDomain", adminAccountRepoEmailDomain},
		{"ListsUsersByIP", adminAccountRepoIP},
		{"ListsUsersInARegistrationState", adminAccountRepoRegistrationState},
		{"SearchesAccounts", adminAccountRepoSearch},
		{"ListsAccountsInALifecycleState", adminAccountRepoLifecycle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newIdentityRepo(t, func(t *testing.T, opts uow.Options) AccountHarness {
				h := newHarness(t, opts)
				return AccountHarness{UnitOfWork: h.UnitOfWork, Identity: h.Identity, Accounts: h.Accounts}
			})
			tc.run(t, r)
		})
	}
}

// session inserts a session of userID with its origin from ip, a second later than the previous
// one, and returns it.
func (r *identityRepo) session(t *testing.T, userID, ip string) identity.Session {
	t.Helper()
	r.clock.Advance(time.Second)
	id := r.ids.NewID()
	s := identity.Session{ID: id, UserID: userID, TokenHash: []byte("token of " + id), CreatedAt: r.clock.Now(), ExpiresAt: r.clock.Now().Add(24 * time.Hour)}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.InsertSession(ctx, tx, s); err != nil {
			return err
		}
		return r.h.Identity.InsertSessionOrigin(ctx, tx, identity.SessionOrigin{SessionID: id, UserID: userID, IP: ip, UserAgent: "ua " + ip, CreatedAt: s.CreatedAt})
	})
	return s
}

// registrationOrigin records that userID registered from ip.
func (r *identityRepo) registrationOrigin(t *testing.T, userID, ip string) {
	t.Helper()
	r.clock.Advance(time.Second)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Identity.InsertRegistrationOrigin(ctx, tx, identity.RegistrationOrigin{UserID: userID, IP: ip, RecordedAt: r.clock.Now()})
	})
}

// pages collects the IDs that list returns in pages of two, following its cursor.
func pages(t *testing.T, list func(before string, limit int) ([]string, error)) []string {
	t.Helper()
	var out []string
	before := ""
	for range 5 {
		ids, err := list(before, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) > 2 {
			t.Fatalf("page of %d, want at most 2", len(ids))
		}
		if len(ids) == 0 {
			break
		}
		out = append(out, ids...)
		before = ids[len(ids)-1]
	}
	return out
}

func adminAccountRepoOrigins(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice, held := r.user(t), r.user(t)
	r.registrationOrigin(t, alice, "192.0.2.1")
	r.registrationOrigin(t, held, "192.0.2.2")
	old := r.session(t, alice, "192.0.2.1")
	heldSession := r.session(t, held, "192.0.2.2")
	older := r.session(t, alice, "192.0.2.3")
	cutoff := r.clock.Now().Add(time.Second)
	r.clock.Advance(time.Minute)
	recent := r.session(t, alice, "192.0.2.4")

	if got, err := r.h.Identity.FindSessionOrigin(ctx, old.ID); err != nil || got.IP != "192.0.2.1" || got.UserAgent != "ua 192.0.2.1" || got.UserID != alice || !got.CreatedAt.Equal(old.CreatedAt) {
		t.Errorf("FindSessionOrigin = %+v, %v", got, err)
	}
	if got, err := r.h.Identity.FindRegistrationOrigin(ctx, alice); err != nil || got.IP != "192.0.2.1" {
		t.Errorf("FindRegistrationOrigin = %+v, %v", got, err)
	}
	if _, err := r.h.Identity.FindSessionOrigin(ctx, r.ids.NewID()); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindSessionOrigin(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Identity.FindRegistrationOrigin(ctx, r.ids.NewID()); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindRegistrationOrigin(unknown) = %v, want ErrNotFound", err)
	}
	origins, err := r.h.Identity.ListSessionOriginsOfUser(ctx, alice, 2)
	if err != nil || len(origins) != 2 || origins[0].SessionID != recent.ID || origins[1].SessionID != older.ID {
		t.Errorf("ListSessionOriginsOfUser(2) = %+v, %v; want the two newest, newest first", origins, err)
	}

	// The purges delete the oldest first, at most limit at a time, and keep the held users'.
	except := []string{held}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i, want := range []int64{1, 1, 0} {
			n, err := r.h.Identity.DeleteSessionOriginsBefore(ctx, tx, cutoff, except, 1)
			if err != nil {
				return err
			}
			if n != want {
				t.Errorf("DeleteSessionOriginsBefore #%d = %d, want %d", i+1, n, want)
			}
		}
		n, err := r.h.Identity.DeleteRegistrationOriginsBefore(ctx, tx, cutoff, except, 10)
		if err == nil && n != 1 {
			t.Errorf("DeleteRegistrationOriginsBefore = %d, want 1", n)
		}
		return err
	})
	for id, want := range map[string]bool{old.ID: false, older.ID: false, heldSession.ID: true, recent.ID: true} {
		if _, err := r.h.Identity.FindSessionOrigin(ctx, id); (err == nil) != want {
			t.Errorf("FindSessionOrigin(%s) after the purge = %v; want kept %t", id, err, want)
		}
	}
	if _, err := r.h.Identity.FindRegistrationOrigin(ctx, alice); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindRegistrationOrigin(alice) after the purge = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Identity.FindRegistrationOrigin(ctx, held); err != nil {
		t.Errorf("FindRegistrationOrigin(held) after the purge = %v, want kept", err)
	}
}

func adminAccountRepoEmailDomain(t *testing.T, r *identityRepo) {
	var want []string
	for i, address := range []string{"a@example.com", "b@other.example", "c@Example.com", "d@example.com"} {
		u := r.user(t)
		r.email(t, u, address)
		if i != 1 {
			want = append([]string{u}, want...)
		}
	}
	got := pages(t, func(before string, limit int) ([]string, error) {
		return r.h.Identity.ListUserIDsByEmailDomain(t.Context(), "example.com", before, limit)
	})
	if !slices.Equal(got, want) {
		t.Errorf("ListUserIDsByEmailDomain pages = %v, want %v", got, want)
	}
}

func adminAccountRepoIP(t *testing.T, r *identityRepo) {
	registered, signedIn, both, other := r.user(t), r.user(t), r.user(t), r.user(t)
	r.registrationOrigin(t, registered, "192.0.2.1")
	r.session(t, signedIn, "192.0.2.1")
	r.registrationOrigin(t, both, "192.0.2.1")
	r.session(t, both, "192.0.2.1")
	r.session(t, both, "192.0.2.1")
	r.session(t, other, "192.0.2.9")
	got := pages(t, func(before string, limit int) ([]string, error) {
		return r.h.Identity.ListUserIDsByIP(t.Context(), "192.0.2.1", before, limit)
	})
	if want := []string{both, signedIn, registered}; !slices.Equal(got, want) {
		t.Errorf("ListUserIDsByIP pages = %v, want %v (each user once, newest first)", got, want)
	}
}

func adminAccountRepoRegistrationState(t *testing.T, r *identityRepo) {
	var want []string
	for i := range 4 {
		u := r.user(t)
		state := identity.RegistrationPending
		if i == 2 {
			state = identity.RegistrationApproved
		} else {
			want = append([]string{u}, want...)
		}
		r.apply(t, u, state)
	}
	got := pages(t, func(before string, limit int) ([]string, error) {
		return r.h.Identity.ListUserIDsInRegistrationState(t.Context(), identity.RegistrationPending, before, limit)
	})
	if !slices.Equal(got, want) {
		t.Errorf("ListUserIDsInRegistrationState pages = %v, want %v", got, want)
	}
}

func adminAccountRepoSearch(t *testing.T, r *identityRepo) {
	alice := r.localAccount(t, r.user(t), "Alice")
	alfred := r.remoteAccount(t, "remote.example", "alfred")
	unverified := r.remoteAccount(t, "remote.example", "")
	albert := r.localAccount(t, r.user(t), "albert")
	bob := r.remoteAccount(t, "other.example", "bob")
	search := func(q appaccount.SearchQuery) []string {
		t.Helper()
		return pages(t, func(before string, limit int) ([]string, error) {
			q.Before = before
			return r.h.Accounts.SearchAccountIDs(t.Context(), q, limit)
		})
	}
	for name, c := range map[string]struct {
		q    appaccount.SearchQuery
		want []string
	}{
		"all":             {appaccount.SearchQuery{}, []string{bob, albert, unverified, alfred, alice}},
		"username prefix": {appaccount.SearchQuery{UsernamePrefix: "al"}, []string{albert, alfred, alice}},
		"domain":          {appaccount.SearchQuery{Domain: "remote.example"}, []string{unverified, alfred}},
		"local":           {appaccount.SearchQuery{Origin: appaccount.OriginLocal}, []string{albert, alice}},
		"remote":          {appaccount.SearchQuery{Origin: appaccount.OriginRemote}, []string{bob, unverified, alfred}},
		"remote prefix":   {appaccount.SearchQuery{UsernamePrefix: "al", Origin: appaccount.OriginRemote}, []string{alfred}},
	} {
		if got := search(c.q); !slices.Equal(got, c.want) {
			t.Errorf("SearchAccountIDs(%s) = %v, want %v", name, got, c.want)
		}
	}
}

func adminAccountRepoLifecycle(t *testing.T, r *identityRepo) {
	var deactivated []string
	for i := range 4 {
		id := r.remoteAccount(t, "remote.example", "")
		state := account.StateDeactivated
		if i == 1 {
			state = account.StateDeleted
		} else {
			deactivated = append([]string{id}, deactivated...)
		}
		r.lifecycle(t, id, state)
	}
	r.remoteAccount(t, "remote.example", "") // active, without a row
	got := pages(t, func(before string, limit int) ([]string, error) {
		return r.h.Accounts.ListAccountIDsInLifecycleState(t.Context(), account.StateDeactivated, before, limit)
	})
	if !slices.Equal(got, deactivated) {
		t.Errorf("ListAccountIDsInLifecycleState pages = %v, want %v", got, deactivated)
	}
}
