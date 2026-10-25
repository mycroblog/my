// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// AdminAccountHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the identity, account, and moderation repositories on it.
type AdminAccountHarness struct {
	UnitOfWork uow.UnitOfWork
	Identity   appidentity.Repository
	Accounts   appaccount.Repository
	Moderation appmoderation.Repository
}

// AdminAccountFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type AdminAccountFactory func(t *testing.T, opts uow.Options) AdminAccountHarness

// AdminAccountContract verifies the reads of the moderators' account management (FR-714) in the
// identity, account, and moderation repositories of a database adapter, through
// appidentity.AccountAdmin: the search lists local and remote accounts newest first without the
// instance actor, in keyset pages, by username prefix, domain, origin, state (each derived from
// the lifecycle, the account's own actions, and the registration), email address, email domain,
// and IP address of a registration or a sign-in; the details tell the registration, the email's
// verification, and the roles; and the reveal shows the email address, the registration and
// sign-in addresses, and the other accounts of the same addresses only to a local user's
// account, recording each reveal in the audit log; and an administrator removes the two-factor
// authentication of a local user, recorded in the audit log.
func AdminAccountContract(t *testing.T, newHarness AdminAccountFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *adminAccountEnv)
	}{
		{"SearchesAccounts", adminAccountSearches},
		{"ShowsTheDetails", adminAccountDetails},
		{"RevealsAndAudits", adminAccountReveals},
		{"DisablesTwoFactorAndAudits", adminAccountDisablesTwoFactor},
		{"LiftsEmailSuppressionsAndAudits", adminAccountLiftsEmailSuppressions},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newAdminAccountEnv(t, newHarness)) })
	}
}

type adminAccountEnv struct {
	*accountEnv
	h     AdminAccountHarness
	mod   *appmoderation.Service
	admin *appidentity.AccountAdmin

	// alice signed up in open mode from 192.0.2.1; bob in approval mode from 192.0.2.1 and is
	// pending; carol is an admin made by the CLI who signed in from 192.0.2.1 and 192.0.2.9 and
	// is suspended; alfred is a remote account with a verified handle, limited; gone is a remote
	// account without a handle, deleted on its server.
	alice, bob, carol, alfred, gone string
	aliceUser, carolUser            string
}

const adminAccountPassword = "correct horse carol"

func newAdminAccountEnv(t *testing.T, newHarness AdminAccountFactory) *adminAccountEnv {
	t.Helper()
	ctx := t.Context()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appmoderation.RegisterAuditProjector(projectors, h.Moderation)
	base := newAccountEnvOn(AccountHarness{UnitOfWork: h.UnitOfWork, Identity: h.Identity, Accounts: h.Accounts}, projectors)
	e := &adminAccountEnv{accountEnv: base, h: h}
	e.mod = appmoderation.NewService(appmoderation.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Moderation, Accounts: base.accounts, Clock: base.clock, IDs: base.ids,
		Lifecycles: base.accounts,
	})
	e.admin = appidentity.NewAccountAdmin(appidentity.AccountAdminDeps{
		UnitOfWork: h.UnitOfWork, Repository: h.Identity, Accounts: base.accounts,
		Restrictions: appmoderation.NewAccountPolicy(h.Moderation, base.accounts, base.clock), Clock: base.clock, IDs: base.ids,
	})
	if _, err := base.accounts.EnsureInstanceActor(ctx); err != nil {
		t.Fatal(err)
	}

	base.clock.Advance(time.Second)
	in := signUpInput("alice")
	in.IP = "192.0.2.1"
	alice := base.registration(identity.RegistrationOpen, true).signUp(t, in)
	e.alice, e.aliceUser = alice.AccountID, alice.User.ID

	base.clock.Advance(time.Second)
	in = signUpInput("bob")
	in.Email, in.IP = "bob@other.example", "::ffff:192.0.2.1"
	e.bob = base.registration(identity.RegistrationApproval, false).signUp(t, in).AccountID

	base.clock.Advance(time.Second)
	carol, err := base.identity.CreateAdmin(ctx, appidentity.CreateAdminInput{Username: "carol", Email: "carol@example.com", Password: adminAccountPassword})
	if err != nil {
		t.Fatal(err)
	}
	e.carol, e.carolUser = carol.AccountID, carol.User.ID
	for _, ip := range []string{"192.0.2.1", "192.0.2.9"} {
		base.clock.Advance(time.Second)
		if _, err := base.identity.SignIn(ctx, appidentity.SignInInput{Email: "carol@example.com", Password: adminAccountPassword, IP: ip, UserAgent: "ua " + ip}); err != nil {
			t.Fatal(err)
		}
	}

	e.alfred = e.remote(t, "https://remote.example/users/alfred", "remote.example", "Alfred")
	e.gone = e.remote(t, "https://gone.example/actor", "gone.example", "")
	e.issue(t, e.alfred, moderation.ActionLimit)
	e.issue(t, e.carol, moderation.ActionSuspend)
	if _, err := base.accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: e.gone, Kind: account.LifecycleRemoteDeleted, InitiatedBy: account.InitiatedByRemote,
	}); err != nil {
		t.Fatal(err)
	}
	return e
}

// remote stores a remote account, with a verified handle when username is not "".
func (e *adminAccountEnv) remote(t *testing.T, uri, domain, username string) string {
	t.Helper()
	ctx := t.Context()
	e.clock.Advance(time.Second)
	a := account.Account{ID: e.ids.NewID(), URI: uri, Domain: domain, ActorType: account.ActorPerson, CreatedAt: e.clock.Now()}
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := e.h.Accounts.InsertAccount(ctx, tx, a); err != nil {
			return err
		}
		if username == "" {
			return nil
		}
		return e.h.Accounts.UpsertHandle(ctx, tx, account.Handle{
			AccountID: a.ID, Username: username, UsernameNormalized: account.NormalizeUsername(username), Domain: domain,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func (e *adminAccountEnv) issue(t *testing.T, accountID string, kind moderation.ActionKind) {
	t.Helper()
	e.clock.Advance(time.Second)
	if _, err := e.mod.IssueAccountAction(t.Context(), appmoderation.AccountActionInput{
		TargetAccountID: accountID, Kind: kind, ReasonCategory: moderation.CategorySpam,
	}); err != nil {
		t.Fatalf("IssueAccountAction(%s) = %v", kind, err)
	}
}

// search returns the IDs of every account that q selects, reading pages of limit, and checks
// the pages.
func (e *adminAccountEnv) search(t *testing.T, q appidentity.AccountSearch, limit int) []string {
	t.Helper()
	out := []string{}
	for range 20 {
		page, next, err := e.admin.Search(t.Context(), q, limit)
		if err != nil {
			t.Fatalf("Search(%+v) = %v", q, err)
		}
		if len(page) > limit {
			t.Fatalf("Search(%+v) = %d accounts, more than %d", q, len(page), limit)
		}
		for _, a := range page {
			out = append(out, a.Actor.Account.ID)
		}
		if next == "" {
			return out
		}
		q.After = next
	}
	t.Fatalf("Search(%+v) never ends", q)
	return nil
}

func adminAccountSearches(t *testing.T, e *adminAccountEnv) {
	all := []string{e.gone, e.alfred, e.carol, e.bob, e.alice}
	for _, c := range []struct {
		name string
		q    appidentity.AccountSearch
		want []string
	}{
		{"everything", appidentity.AccountSearch{}, all},
		{"local", appidentity.AccountSearch{Origin: appaccount.OriginLocal}, []string{e.carol, e.bob, e.alice}},
		{"remote", appidentity.AccountSearch{Origin: appaccount.OriginRemote}, []string{e.gone, e.alfred}},
		{"username", appidentity.AccountSearch{Username: "AL"}, []string{e.alfred, e.alice}},
		{"remote username", appidentity.AccountSearch{Username: "al", Origin: appaccount.OriginRemote}, []string{e.alfred}},
		{"domain", appidentity.AccountSearch{Domain: "Remote.Example"}, []string{e.alfred}},
		{"email", appidentity.AccountSearch{Email: "ALICE@example.com"}, []string{e.alice}},
		{"unknown email", appidentity.AccountSearch{Email: "nobody@example.com"}, []string{}},
		{"email of a remote account", appidentity.AccountSearch{Email: "alice@example.com", Origin: appaccount.OriginRemote}, []string{}},
		{"email domain", appidentity.AccountSearch{EmailDomain: "Example.com"}, []string{e.carol, e.alice}},
		{"email domain with @", appidentity.AccountSearch{EmailDomain: "@other.example"}, []string{e.bob}},
		{"ip", appidentity.AccountSearch{IP: "192.0.2.1"}, []string{e.carol, e.bob, e.alice}},
		{"mapped ip", appidentity.AccountSearch{IP: "::ffff:192.0.2.9"}, []string{e.carol}},
		{"ip and username", appidentity.AccountSearch{IP: "192.0.2.1", Username: "b"}, []string{e.bob}},
		{"active", appidentity.AccountSearch{State: appidentity.AdminAccountActive}, []string{e.alice}},
		{"pending", appidentity.AccountSearch{State: appidentity.AdminAccountPending}, []string{e.bob}},
		{"pending and username", appidentity.AccountSearch{State: appidentity.AdminAccountPending, Username: "al"}, []string{}},
		{"rejected", appidentity.AccountSearch{State: appidentity.AdminAccountRejected}, []string{}},
		{"limited", appidentity.AccountSearch{State: appidentity.AdminAccountLimited}, []string{e.alfred}},
		{"disabled", appidentity.AccountSearch{State: appidentity.AdminAccountDisabled}, []string{}},
		{"suspended", appidentity.AccountSearch{State: appidentity.AdminAccountSuspended}, []string{e.carol}},
		{"deleted", appidentity.AccountSearch{State: appidentity.AdminAccountDeleted}, []string{e.gone}},
	} {
		for _, limit := range []int{1, 2, 40} {
			if got := e.search(t, c.q, limit); !slices.Equal(got, c.want) {
				t.Errorf("%s by %d = %v, want %v", c.name, limit, got, c.want)
			}
		}
	}

	page, _, err := e.admin.Search(t.Context(), appidentity.AccountSearch{}, 40)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]appidentity.AdminAccountState{}
	for _, a := range page {
		states[a.Actor.Account.ID] = a.State
		if a.Actor.Account.IsLocal() != (a.UserID != "") {
			t.Errorf("account %s has user %q", a.Actor.Account.ID, a.UserID)
		}
	}
	want := map[string]appidentity.AdminAccountState{
		e.alice: appidentity.AdminAccountActive, e.bob: appidentity.AdminAccountPending, e.carol: appidentity.AdminAccountSuspended,
		e.alfred: appidentity.AdminAccountLimited, e.gone: appidentity.AdminAccountDeleted,
	}
	for id, s := range want {
		if states[id] != s {
			t.Errorf("state of %s = %q, want %q", id, states[id], s)
		}
	}

	for _, c := range []struct {
		q     appidentity.AccountSearch
		param string
	}{
		{appidentity.AccountSearch{Email: "a@example.com", IP: "192.0.2.1"}, "ip"},
		{appidentity.AccountSearch{Email: "a@example.com", EmailDomain: "example.com"}, "email_domain"},
		{appidentity.AccountSearch{IP: "not an address"}, "ip"},
		{appidentity.AccountSearch{EmailDomain: "a@example.com"}, "email_domain"},
		{appidentity.AccountSearch{State: "frozen"}, "state"},
		{appidentity.AccountSearch{Origin: "elsewhere"}, "origin"},
	} {
		var serr *appidentity.AccountSearchError
		if _, _, err := e.admin.Search(t.Context(), c.q, 10); !errors.As(err, &serr) || serr.Param != c.param {
			t.Errorf("Search(%+v) = %v, want an error naming %s", c.q, err, c.param)
		}
	}
}

func adminAccountDetails(t *testing.T, e *adminAccountEnv) {
	ctx := t.Context()
	alice, err := e.admin.Detail(ctx, e.alice)
	if err != nil {
		t.Fatal(err)
	}
	if alice.UserID != e.aliceUser || alice.Registration == nil || alice.Registration.State != identity.RegistrationApproved ||
		alice.Application == nil || !alice.HasEmail || alice.EmailVerified || len(alice.Roles) != 0 || alice.State != appidentity.AdminAccountActive {
		t.Errorf("alice = %+v", alice)
	}
	carol, err := e.admin.Detail(ctx, e.carol)
	if err != nil {
		t.Fatal(err)
	}
	if carol.Registration != nil || carol.Application != nil || !carol.HasEmail || len(carol.Roles) != 1 ||
		carol.Roles[0].Key != identity.RoleAdmin || carol.State != appidentity.AdminAccountSuspended ||
		!slices.Contains(carol.Restrictions, moderation.RestrictLogin) {
		t.Errorf("carol = %+v", carol)
	}
	alfred, err := e.admin.Detail(ctx, e.alfred)
	if err != nil {
		t.Fatal(err)
	}
	if alfred.UserID != "" || alfred.HasEmail || len(alfred.Roles) != 0 || alfred.State != appidentity.AdminAccountLimited ||
		!slices.Equal(alfred.Restrictions, slices.Sorted(slices.Values(moderation.ActionLimit.Restrictions()))) {
		t.Errorf("alfred = %+v, want the restrictions of limit", alfred)
	}
	if _, err := e.admin.Detail(ctx, e.ids.NewID()); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("Detail(unknown) = %v, want ErrNotFound", err)
	}
}

func adminAccountReveals(t *testing.T, e *adminAccountEnv) {
	ctx := t.Context()
	audit := func() []moderation.AuditEntry {
		t.Helper()
		got, err := e.mod.AuditLog(ctx, appmoderation.AuditQuery{Action: moderation.AuditRevealedPrivateData}, 10)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := audit(); len(got) != 0 {
		t.Fatalf("audit log before a reveal = %+v", got)
	}

	e.clock.Advance(time.Second)
	carol, err := e.admin.Reveal(ctx, e.carol, NamedID("mod-user"))
	if err != nil {
		t.Fatal(err)
	}
	if carol.Email != "carol@example.com" || carol.Registration != nil || len(carol.SignIns) != 2 ||
		carol.SignIns[0].IP != "192.0.2.9" || carol.SignIns[0].UserAgent != "ua 192.0.2.9" || carol.SignIns[1].IP != "192.0.2.1" {
		t.Errorf("carol = %+v", carol)
	}
	same := map[string][]string{}
	for _, g := range carol.SameIP {
		ids := []string{}
		for _, a := range g.Accounts {
			ids = append(ids, a.Actor.Account.ID)
		}
		same[g.IP] = ids
	}
	if len(carol.SameIP) != 2 || carol.SameIP[0].IP != "192.0.2.9" || len(same["192.0.2.9"]) != 0 ||
		!slices.Equal(same["192.0.2.1"], []string{e.bob, e.alice}) {
		t.Errorf("same IP = %v (%+v)", same, carol.SameIP)
	}
	entries := audit()
	if len(entries) != 1 {
		t.Fatalf("audit log = %+v, want one entry", entries)
	}
	if g := entries[0]; g.ActorKind != moderation.AuditActorUser || g.ActorUserID != NamedID("mod-user") || g.TargetKind != moderation.AuditTargetAccount ||
		g.TargetID != e.carol || g.TargetLabel != "@carol" || g.SourceEventID != "" || !g.OccurredAt.Equal(e.clock.Now()) {
		t.Errorf("entry = %+v", g)
	}

	alice, err := e.admin.Reveal(ctx, e.alice, "")
	if err != nil {
		t.Fatal(err)
	}
	if alice.Email != "alice@example.com" || alice.EmailVerified || alice.Registration == nil || alice.Registration.IP != "192.0.2.1" ||
		len(alice.SignIns) != 1 || alice.SignIns[0].IP != "192.0.2.1" || len(alice.SameIP) != 1 {
		t.Errorf("alice = %+v", alice)
	}
	if entries := audit(); len(entries) != 2 || entries[0].ActorKind != moderation.AuditActorCLI || entries[0].TargetLabel != "@alice" {
		t.Errorf("audit log after a reveal by the CLI = %+v", entries)
	}

	// An account without a user has nothing to reveal, and nothing is recorded.
	instance, err := e.accounts.InstanceActor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{e.alfred, instance.Account.ID} {
		if _, err := e.admin.Reveal(ctx, id, NamedID("mod-user")); !errors.Is(err, appidentity.ErrNotLocalAccount) {
			t.Errorf("Reveal(%s) = %v, want ErrNotLocalAccount", id, err)
		}
	}
	if _, err := e.admin.Reveal(ctx, e.ids.NewID(), NamedID("mod-user")); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("Reveal(unknown) = %v, want ErrNotFound", err)
	}
	if entries := audit(); len(entries) != 2 {
		t.Errorf("audit log after refused reveals = %+v", entries)
	}
}

func adminAccountDisablesTwoFactor(t *testing.T, e *adminAccountEnv) {
	ctx := t.Context()
	if err := e.admin.DisableTwoFactor(ctx, e.alice, NamedID("mod-user")); !errors.Is(err, appidentity.ErrTwoFactorNotEnabled) {
		t.Errorf("DisableTwoFactor(without TOTP) = %v, want ErrTwoFactorNotEnabled", err)
	}
	if err := e.admin.DisableTwoFactor(ctx, e.alfred, NamedID("mod-user")); !errors.Is(err, appidentity.ErrNotLocalAccount) {
		t.Errorf("DisableTwoFactor(remote) = %v, want ErrNotLocalAccount", err)
	}
	if err := e.admin.DisableTwoFactor(ctx, e.ids.NewID(), NamedID("mod-user")); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("DisableTwoFactor(unknown) = %v, want ErrNotFound", err)
	}
	e.enableTOTP(t, appidentity.Registration{User: identity.User{ID: e.aliceUser}, AccountID: e.alice})
	e.clock.Advance(time.Second)
	if err := e.admin.DisableTwoFactor(ctx, e.alice, NamedID("mod-user")); err != nil {
		t.Fatalf("DisableTwoFactor = %v", err)
	}
	if st, err := e.identity.TwoFactorStatus(ctx, e.aliceUser); err != nil || st.Enabled {
		t.Errorf("TwoFactorStatus = %+v, %v; want disabled", st, err)
	}
	got, err := e.mod.AuditLog(ctx, appmoderation.AuditQuery{Action: moderation.AuditTwoFactorDisabled}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ActorUserID != NamedID("mod-user") || got[0].TargetKind != moderation.AuditTargetAccount ||
		got[0].TargetID != e.alice || got[0].TargetLabel != "@alice" || !got[0].OccurredAt.Equal(e.clock.Now()) {
		t.Errorf("audit log = %+v, want one two_factor_disabled entry on @alice", got)
	}
}
