// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeAccountAdmin is the account management over the accounts of the test: alice is a local
// user, the reporter a remote account. The search returns every account after the position,
// and records the queries; Reveal records who asked.
type fakeAccountAdmin struct {
	accounts fakeAccounts
	queries  []appidentity.AccountSearch
	reveals  []string // "account by user"
	reports  []moderation.ReportRecord
	// twoFactor holds the accounts whose users have two-factor authentication, and
	// twoFactorRemovals the removals ("account by user").
	twoFactor         map[string]bool
	twoFactorRemovals []string
	// resetLinks are the password reset links issued ("account by user").
	resetLinks []string
	// suppressed holds the accounts whose addresses are suppressed, and lifts the lifts of
	// their suppression ("account by user").
	suppressed map[string]identity.EmailSuppression
	lifts      []string
	// perms are the permissions of the env's users.
	perms fakePermissions
}

func (f *fakeAccountAdmin) present(a appaccount.Actor) appidentity.AdminAccount {
	out := appidentity.AdminAccount{Actor: a, State: appidentity.AdminAccountActive, Restrictions: []appmoderation.Restriction{}}
	if a.Local != nil {
		out.UserID = a.Local.UserID
		out.State = appidentity.AdminAccountLimited
		out.Restrictions = []appmoderation.Restriction{moderation.RestrictVisibilityCeilingPrivate, moderation.RestrictPublicDiscovery}
	}
	return out
}

func (f *fakeAccountAdmin) Search(_ context.Context, q appidentity.AccountSearch, limit int) ([]appidentity.AdminAccount, string, error) {
	f.queries = append(f.queries, q)
	if q.IP == "not an address" {
		return nil, "", &appidentity.AccountSearchError{Param: "ip"}
	}
	ids := []string{aliceAccountID, reporterAccountID} // newest first
	var out []appidentity.AdminAccount
	for _, id := range ids {
		if q.After == "" || id < q.After {
			out = append(out, f.present(f.accounts[id]))
		}
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = out[limit-1].Actor.Account.ID
	}
	return out, next, nil
}

func (f *fakeAccountAdmin) Detail(_ context.Context, id string) (appidentity.AdminAccountDetail, error) {
	a, ok := f.accounts[id]
	if !ok {
		return appidentity.AdminAccountDetail{}, appaccount.ErrNotFound
	}
	d := appidentity.AdminAccountDetail{AdminAccount: f.present(a), Roles: []identity.Role{}}
	if a.Local != nil {
		d.Registration = &identity.UserRegistrationState{UserID: a.Local.UserID, State: identity.RegistrationApproved, Since: adminNow}
		d.Application = &identity.RegistrationApplication{UserID: a.Local.UserID, Reason: "hi", InviteID: adminInviteID, SubmittedAt: adminNow.Add(-time.Hour)}
		// Bob invited the user (ADR-0108 §5).
		d.Invite = &identity.Invite{ID: adminInviteID, Code: "ABCDEFGHIJKLMNOP", CreatedByUserID: userOf(bobAccountID), CreatorKind: identity.InviteByMember, CreatedAt: adminNow}
		d.HasEmail, d.EmailVerified = true, true
		d.Roles = []identity.Role{{Key: identity.RoleAdmin}}
		d.TwoFactorEnabled = f.twoFactor[id]
		if sup, ok := f.suppressed[id]; ok {
			d.EmailSuppression = &sup
		}
	}
	return d, nil
}

func (f *fakeAccountAdmin) LiftEmailSuppression(_ context.Context, id, by string) error {
	a, ok := f.accounts[id]
	switch {
	case !ok:
		return appaccount.ErrNotFound
	case a.Local == nil:
		return appidentity.ErrNotLocalAccount
	}
	if _, ok := f.suppressed[id]; !ok {
		return appidentity.ErrNotSuppressed
	}
	delete(f.suppressed, id)
	f.lifts = append(f.lifts, id+" by "+by)
	return nil
}

func (f *fakeAccountAdmin) Reveal(_ context.Context, id, by string) (appidentity.PrivateData, error) {
	a, ok := f.accounts[id]
	switch {
	case !ok:
		return appidentity.PrivateData{}, appaccount.ErrNotFound
	case a.Local == nil:
		return appidentity.PrivateData{}, appidentity.ErrNotLocalAccount
	}
	f.reveals = append(f.reveals, id+" by "+by)
	return appidentity.PrivateData{
		Email: "alice@example.com", EmailVerified: true,
		Registration: &identity.RegistrationOrigin{UserID: a.Local.UserID, IP: "192.0.2.1", RecordedAt: adminNow.Add(-time.Hour)},
		SignIns: []identity.SessionOrigin{
			{SessionID: testkit.NamedID("session-2"), UserID: a.Local.UserID, IP: "192.0.2.2", UserAgent: "ua", CreatedAt: adminNow},
			{SessionID: testkit.NamedID("session-1"), UserID: a.Local.UserID, UserAgent: "ua", CreatedAt: adminNow.Add(-time.Minute)},
		},
		SameIP: []appidentity.SameIPAccounts{
			{IP: "192.0.2.2", Accounts: []appidentity.AdminAccount{}},
			{IP: "192.0.2.1", Accounts: []appidentity.AdminAccount{f.present(f.accounts[bobAccountID])}},
		},
	}, nil
}

func (f *fakeAccountAdmin) DisableTwoFactor(_ context.Context, id, by string) error {
	a, ok := f.accounts[id]
	switch {
	case !ok:
		return appaccount.ErrNotFound
	case a.Local == nil:
		return appidentity.ErrNotLocalAccount
	case !f.twoFactor[id]:
		return appidentity.ErrTwoFactorNotEnabled
	}
	delete(f.twoFactor, id)
	f.twoFactorRemovals = append(f.twoFactorRemovals, id+" by "+by)
	return nil
}

func (f *fakeAccountAdmin) IssuePasswordResetLink(_ context.Context, id, by string) (appidentity.PasswordResetLink, error) {
	a, ok := f.accounts[id]
	switch {
	case !ok:
		return appidentity.PasswordResetLink{}, appaccount.ErrNotFound
	case a.Local == nil:
		return appidentity.PasswordResetLink{}, appidentity.ErrNotLocalAccount
	}
	f.resetLinks = append(f.resetLinks, id+" by "+by)
	return appidentity.PasswordResetLink{URL: "https://social.example/auth/reset_password?token=rst_secret", ExpiresAt: adminNow.Add(time.Hour)}, nil
}

func (f *fakeAccountAdmin) RecentReportsOf(_ context.Context, target string, limit int) ([]moderation.ReportRecord, error) {
	var out []moderation.ReportRecord
	for _, r := range slices.Backward(f.reports) {
		if r.Report.TargetAccountID == target && len(out) < limit {
			out = append(out, r)
		}
	}
	return out, nil
}

// newAccountAdminEnv is newAccountModerationEnv with the account management.
// adminInviteID is the invite that the accounts of fakeAccountAdmin signed up with.
const adminInviteID = "01890a5d-ac96-774b-bcce-b302099a8f01"

func newAccountAdminEnv(t *testing.T) (*authEnv, *fakeAccountAdmin, *fakeAccountModeration) {
	t.Helper()
	admin := &fakeAccountAdmin{
		twoFactor:  map[string]bool{aliceAccountID: true},
		suppressed: map[string]identity.EmailSuppression{aliceAccountID: {Kind: identity.EmailHardBounce, FailureID: "f", Since: adminNow.Add(-time.Minute)}},
	}
	mod := &fakeAccountModeration{now: adminNow}
	e, reports := newModerationEnvWith(t, func(o *clientapi.Options) {
		accounts := o.Accounts.(fakeAccounts)
		accounts[bobAccountID] = appaccount.Actor{
			Account:   account.Account{ID: bobAccountID, URI: "https://social.example/ap/actors/" + bobAccountID, CreatedAt: adminNow},
			Local:     &account.LocalAccount{AccountID: bobAccountID, UserID: userOf(bobAccountID), Username: "bob"},
			Lifecycle: account.InitialLifecycleState(bobAccountID),
		}
		admin.accounts, mod.accounts = accounts, accounts
		admin.perms = o.Permissions.(fakePermissions)
		o.AccountAdmin, o.AccountReports, o.AccountModeration, o.PasswordResetLinks = admin, admin, mod, admin
		o.UserAccounts = fakeUserAccounts{}
	})
	admin.reports = reports.reports
	return e, admin, mod
}

func TestAccountAdminOperationsNeedAPermission(t *testing.T) {
	e, admin, _ := newAccountAdminEnv(t)
	for _, op := range []struct{ method, path string }{
		{http.MethodGet, "/admin/accounts"},
		{http.MethodGet, "/admin/accounts/" + aliceAccountID},
		{http.MethodPost, "/admin/accounts/" + aliceAccountID + "/reveal"},
	} {
		res := e.send(t, op.method, op.path, "bob-admin", "")
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_accounts") {
			t.Errorf("%s %s by bob = %d %+v, want 403 for manage_accounts", op.method, op.path, res.StatusCode, p)
		}
	}
	if len(admin.queries) != 0 || len(admin.reveals) != 0 || len(admin.twoFactorRemovals) != 0 {
		t.Errorf("a refused operation ran: %v %v %v", admin.queries, admin.reveals, admin.twoFactorRemovals)
	}
}

func TestSearchAdminAccounts(t *testing.T) {
	e, admin, _ := newAccountAdminEnv(t)
	q := url.Values{
		"username": {"Al"}, "domain": {"remote.example"}, "origin": {"local"}, "state": {"limited"},
		"email": {"alice@example.com"}, "limit": {"1"},
	}
	res := e.send(t, http.MethodGet, "/admin/accounts?"+q.Encode(), "admin-read", "")
	page := decodeJSON[clientapi.AdminAccountPage](t, res)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("search = %d (%s)", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	want := appidentity.AccountSearch{
		Username: "Al", Domain: "remote.example", Origin: "local", State: appidentity.AdminAccountLimited, Email: "alice@example.com",
	}
	if len(admin.queries) != 1 || admin.queries[0] != want {
		t.Errorf("queries = %+v, want %+v", admin.queries, want)
	}
	if len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("first page = %+v", page)
	}
	alice := page.Items[0]
	if alice.Id != aliceAccountID || !alice.Local || alice.Account.Username != "alice" ||
		alice.State != clientapi.AdminAccountStateLimited || alice.RegistrationState == nil ||
		*alice.RegistrationState != clientapi.AdminAccountRegistrationStateApproved ||
		!slices.Equal(alice.Restrictions, []clientapi.AdminAccountRestriction{
			clientapi.AdminAccountRestrictionVisibilityCeilingPrivate, clientapi.AdminAccountRestrictionPublicDiscovery,
		}) {
		t.Errorf("first page = %+v", alice)
	}
	res = e.send(t, http.MethodGet, "/admin/accounts?limit=1&cursor="+*page.NextCursor, "admin-read", "")
	rest := decodeJSON[clientapi.AdminAccountPage](t, res)
	if len(rest.Items) != 1 || rest.NextCursor != nil || admin.queries[1].After != aliceAccountID {
		t.Fatalf("second page = %+v after %q", rest, admin.queries[1].After)
	}
	if r := rest.Items[0]; r.Id != reporterAccountID || r.Local || r.RegistrationState != nil ||
		r.State != clientapi.AdminAccountStateActive || len(r.Restrictions) != 0 {
		t.Errorf("second page = %+v", r)
	}
	for _, query := range []string{"ip=not+an+address", "limit=0", "cursor=%25", "username=" + strings.Repeat("a", 321)} {
		res := e.send(t, http.MethodGet, "/admin/accounts?"+query, "admin-read", "")
		if p := decodeProblem(t, res); res.StatusCode != http.StatusBadRequest {
			t.Errorf("?%.40s = %d %+v, want 400", query, res.StatusCode, p)
		}
	}
}

func TestGetAdminAccount(t *testing.T) {
	e, admin, mod := newAccountAdminEnv(t)
	if _, err := mod.IssueAccountAction(t.Context(), appmoderation.AccountActionInput{
		TargetAccountID: aliceAccountID, Kind: moderation.ActionLimit, ReasonCategory: moderation.CategorySpam, IssuedByUserID: userOf(aliceAccountID),
	}); err != nil {
		t.Fatal(err)
	}
	res := e.send(t, http.MethodGet, "/admin/accounts/"+aliceAccountID, "admin-read", "")
	d := decodeJSON[clientapi.AdminAccountDetail](t, res)
	if res.StatusCode != http.StatusOK || d.Id != aliceAccountID || d.State != clientapi.AdminAccountStateLimited ||
		!slices.Equal(d.Roles, []string{identity.RoleAdmin}) || d.EmailVerified == nil || !*d.EmailVerified ||
		d.Registration == nil || d.Registration.Reason == nil || *d.Registration.Reason != "hi" || !d.Registration.StateChangedAt.Equal(adminNow) ||
		len(d.Actions) != 1 || d.Actions[0].Kind != clientapi.AdminAccountActionKindLimit ||
		d.TwoFactorEnabled == nil || !*d.TwoFactorEnabled ||
		d.EmailSuppression == nil || d.EmailSuppression.Kind != clientapi.EmailSuppressionKindHardBounce || !d.EmailSuppression.Since.Equal(adminNow.Add(-time.Minute)) {
		t.Errorf("alice = %d %+v", res.StatusCode, d)
	}
	// The invite the user signed up with, and who created it.
	if inv := d.Registration.Invite; d.Registration.InviteId == nil || *d.Registration.InviteId != adminInviteID || inv == nil || inv.Id != adminInviteID ||
		inv.CreatorKind != clientapi.InviteCreatorKindMember || inv.CreatedBy == nil || inv.CreatedBy.Id != bobAccountID || inv.CreatedByDeleted {
		t.Errorf("the registration's invite = %+v", d.Registration)
	}
	// The reports of the account, newest first.
	if len(d.Reports) != len(admin.reports) || d.Reports[0].Id != reportThree {
		t.Errorf("reports = %+v", d.Reports)
	}

	res = e.send(t, http.MethodGet, "/admin/accounts/"+reporterAccountID, "admin-read", "")
	remote := decodeJSON[clientapi.AdminAccountDetail](t, res)
	if res.StatusCode != http.StatusOK || remote.Local || remote.Registration != nil || remote.EmailVerified != nil || remote.TwoFactorEnabled != nil ||
		remote.EmailSuppression != nil ||
		len(remote.Roles) != 0 || len(remote.Actions) != 0 || len(remote.Reports) != 0 {
		t.Errorf("remote = %d %+v", res.StatusCode, remote)
	}
	res = e.send(t, http.MethodGet, "/admin/accounts/"+reportThree, "admin-read", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown = %d %+v", res.StatusCode, p)
	}
	// Neither the search nor the details reveal anything.
	if len(admin.reveals) != 0 {
		t.Errorf("reveals = %v", admin.reveals)
	}
}

func TestRevealAdminAccount(t *testing.T) {
	e, admin, _ := newAccountAdminEnv(t)
	res := e.send(t, http.MethodPost, "/admin/accounts/"+aliceAccountID+"/reveal", "admin-read", "")
	d := decodeJSON[clientapi.AdminAccountPrivateData](t, res)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("reveal = %d (%s)", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	if d.Email == nil || *d.Email != "alice@example.com" || d.EmailVerified == nil || !*d.EmailVerified ||
		d.RegistrationIp == nil || *d.RegistrationIp != "192.0.2.1" || d.RegisteredAt == nil ||
		len(d.SignIns) != 2 || d.SignIns[0].Ip == nil || *d.SignIns[0].Ip != "192.0.2.2" || d.SignIns[1].Ip != nil ||
		len(d.SameIpAccounts) != 2 || len(d.SameIpAccounts[0].Accounts) != 0 || d.SameIpAccounts[1].Accounts[0].Id != bobAccountID {
		t.Errorf("revealed = %+v", d)
	}
	if want := []string{aliceAccountID + " by " + userOf(aliceAccountID)}; !slices.Equal(admin.reveals, want) {
		t.Errorf("reveals = %v, want %v", admin.reveals, want)
	}

	res = e.send(t, http.MethodPost, "/admin/accounts/"+reporterAccountID+"/reveal", "admin-read", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "id") {
		t.Errorf("remote = %d %+v, want 422 naming id", res.StatusCode, p)
	}
	res = e.send(t, http.MethodPost, "/admin/accounts/"+reportThree+"/reveal", "admin-read", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown = %d %+v", res.StatusCode, p)
	}
	if len(admin.reveals) != 1 {
		t.Errorf("reveals = %v, want only alice's", admin.reveals)
	}
}

func TestDisableAdminAccountTwoFactor(t *testing.T) {
	e, admin, _ := newAccountAdminEnv(t)
	path := func(id string) string { return "/admin/accounts/" + id + "/two_factor/disable" }
	// A moderator's manage_accounts is not enough: removing 2FA is the administrators' (ADR-0058).
	admin.perms[userOf(bobAccountID)] = []identity.Permission{identity.PermManageAccounts}
	res := e.send(t, http.MethodPost, path(aliceAccountID), "bob-admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
		p.Detail == nil || !strings.Contains(*p.Detail, "manage_login_recovery") {
		t.Errorf("by a moderator = %d %+v, want 403 for manage_login_recovery", res.StatusCode, p)
	}
	if len(admin.twoFactorRemovals) != 0 {
		t.Fatalf("a refused removal ran: %v", admin.twoFactorRemovals)
	}
	res = e.send(t, http.MethodPost, path(aliceAccountID), "admin-read", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
		t.Errorf("with admin:read = %d %+v, want 403 insufficient scope", res.StatusCode, p)
	}
	res = e.send(t, http.MethodPost, path(aliceAccountID), "admin", "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("disable = %d, want 204", res.StatusCode)
	}
	if want := []string{aliceAccountID + " by " + userOf(aliceAccountID)}; !slices.Equal(admin.twoFactorRemovals, want) {
		t.Errorf("removals = %v, want %v", admin.twoFactorRemovals, want)
	}
	res = e.send(t, http.MethodPost, path(aliceAccountID), "admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeTwoFactorNotEnabled {
		t.Errorf("disable twice = %d %+v, want 409 two-factor-not-enabled", res.StatusCode, p)
	}
	res = e.send(t, http.MethodPost, path(reporterAccountID), "admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "id") {
		t.Errorf("remote = %d %+v, want 422 naming id", res.StatusCode, p)
	}
	res = e.send(t, http.MethodPost, path(reportThree), "admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown = %d %+v", res.StatusCode, p)
	}
}

func TestIssueAdminAccountPasswordResetLink(t *testing.T) {
	e, admin, _ := newAccountAdminEnv(t)
	path := func(id string) string { return "/admin/accounts/" + id + "/password_reset_link" }
	// A moderator's manage_accounts is not enough: the link lets its holder into the account.
	admin.perms[userOf(bobAccountID)] = []identity.Permission{identity.PermManageAccounts}
	res := e.send(t, http.MethodPost, path(aliceAccountID), "bob-admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
		p.Detail == nil || !strings.Contains(*p.Detail, "manage_login_recovery") {
		t.Errorf("by a moderator = %d %+v, want 403 for manage_login_recovery", res.StatusCode, p)
	}
	res = e.send(t, http.MethodPost, path(aliceAccountID), "admin-read", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
		t.Errorf("with admin:read = %d %+v, want 403 insufficient scope", res.StatusCode, p)
	}
	if len(admin.resetLinks) != 0 {
		t.Fatalf("a refused issuance ran: %v", admin.resetLinks)
	}
	res = e.send(t, http.MethodPost, path(aliceAccountID), "admin", "")
	link := decodeJSON[clientapi.AdminPasswordResetLink](t, res)
	if res.StatusCode != http.StatusCreated || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("issue = %d (%s), want 201 no-store", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	if link.Url != "https://social.example/auth/reset_password?token=rst_secret" || !link.ExpiresAt.Equal(adminNow.Add(time.Hour)) {
		t.Errorf("link = %+v", link)
	}
	if want := []string{aliceAccountID + " by " + userOf(aliceAccountID)}; !slices.Equal(admin.resetLinks, want) {
		t.Errorf("issued = %v, want %v", admin.resetLinks, want)
	}
	res = e.send(t, http.MethodPost, path(reporterAccountID), "admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "id") {
		t.Errorf("remote = %d %+v, want 422 naming id", res.StatusCode, p)
	}
	res = e.send(t, http.MethodPost, path(reportThree), "admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown = %d %+v", res.StatusCode, p)
	}
}

func TestLiftAdminAccountEmailSuppression(t *testing.T) {
	e, admin, _ := newAccountAdminEnv(t)
	path := func(id string) string { return "/admin/accounts/" + id + "/email_suppression/lift" }
	res := e.send(t, http.MethodPost, path(aliceAccountID), "bob-admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
		p.Detail == nil || !strings.Contains(*p.Detail, "manage_accounts") {
		t.Errorf("by bob = %d %+v, want 403 for manage_accounts", res.StatusCode, p)
	}
	res = e.send(t, http.MethodPost, path(aliceAccountID), "admin-read", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
		t.Errorf("with admin:read = %d %+v, want 403 insufficient scope", res.StatusCode, p)
	}
	if len(admin.lifts) != 0 {
		t.Fatalf("a refused lift ran: %v", admin.lifts)
	}
	res = e.send(t, http.MethodPost, path(aliceAccountID), "admin", "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("lift = %d, want 204", res.StatusCode)
	}
	if want := []string{aliceAccountID + " by " + userOf(aliceAccountID)}; !slices.Equal(admin.lifts, want) {
		t.Errorf("lifts = %v, want %v", admin.lifts, want)
	}
	res = e.send(t, http.MethodPost, path(aliceAccountID), "admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeEmailNotSuppressed {
		t.Errorf("lift twice = %d %+v, want 409 email-not-suppressed", res.StatusCode, p)
	}
	res = e.send(t, http.MethodPost, path(reporterAccountID), "admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "id") {
		t.Errorf("remote = %d %+v, want 422 naming id", res.StatusCode, p)
	}
	res = e.send(t, http.MethodPost, path(reportThree), "admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown = %d %+v", res.StatusCode, p)
	}
}
