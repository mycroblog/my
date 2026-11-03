// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"errors"
	"net/http"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// setRole makes u hold only role ("" for no role), as the CLI does.
func (e *accountsEnv) setRole(u testUser, role string) {
	e.t.Helper()
	ctx := e.t.Context()
	actor, err := e.app.Accounts.FindByID(ctx, u.id)
	if err != nil {
		e.t.Fatal(err)
	}
	userID := actor.Local.UserID
	if role != "" {
		if _, err := e.app.Identity.GrantRole(ctx, appidentity.RoleChangeInput{UserID: userID, RoleKey: role}); err != nil {
			e.t.Fatalf("GrantRole(%s): %v", role, err)
		}
	}
	if role != domainidentity.RoleAdmin {
		if _, err := e.app.Identity.RevokeRole(ctx, appidentity.RoleChangeInput{UserID: userID, RoleKey: domainidentity.RoleAdmin}); err != nil {
			e.t.Fatalf("RevokeRole(admin): %v", err)
		}
	}
}

// TestStaffCannotActOnAHigherOrEqualRank drives the moderation and recovery operations as staff of
// each rank: the built-in admin (1000) and moderator (100), a custom role (50) that holds only
// manage_login_recovery, and a member without roles.
func TestStaffCannotActOnAHigherOrEqualRank(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	ctx := t.Context()
	root, otherAdmin := e.user("root"), e.user("otheradmin")
	mod, otherMod := e.user("mod"), e.user("othermod")
	recovery, otherRecovery := e.user("recovery"), e.user("otherrecovery")
	member, secondMember := e.user("member"), e.user("secondmember")

	e.want(e.do(http.MethodPost, "/admin/roles", root.token, map[string]any{
		"key": "recovery", "position": 50, "permissions": []string{"manage_login_recovery"},
	}, nil), http.StatusCreated, "create the role")
	for _, u := range []testUser{mod, otherMod} {
		e.setRole(u, domainidentity.RoleModerator)
	}
	for _, u := range []testUser{recovery, otherRecovery} {
		e.setRole(u, "recovery")
	}
	for _, u := range []testUser{member, secondMember} {
		e.setRole(u, "")
	}

	type op struct {
		name string
		path func(target testUser) string
		body any
		ok   int
	}
	actions := func(target testUser) string { return "/admin/accounts/" + target.id + "/actions" }
	warn := op{"warn", actions, map[string]any{"kind": "warn"}, http.StatusCreated}
	ban := op{"ban", actions, map[string]any{"kind": "ban"}, http.StatusCreated}
	reveal := op{"reveal", func(target testUser) string { return "/admin/accounts/" + target.id + "/reveal" }, nil, http.StatusOK}
	resetLink := op{"reset link", func(target testUser) string { return "/admin/accounts/" + target.id + "/password_reset_link" }, nil, http.StatusCreated}
	twoFactor := op{"disable two-factor", func(target testUser) string { return "/admin/accounts/" + target.id + "/two_factor/disable" }, nil, http.StatusConflict}

	refused := func(actor testUser, target testUser, o op) {
		t.Helper()
		res := e.do(http.MethodPost, o.path(target), actor.token, o.body, nil)
		if p := e.want(res, http.StatusForbidden, o.name); p != clientapi.ProblemTypeTargetOutranksActor {
			t.Errorf("%s = %q, want %q", o.name, p, clientapi.ProblemTypeTargetOutranksActor)
		}
	}
	allowed := func(actor testUser, target testUser, o op) {
		t.Helper()
		e.want(e.do(http.MethodPost, o.path(target), actor.token, o.body, nil), o.ok, o.name)
	}

	// A moderator: not on an admin, another moderator, or their own account.
	for _, target := range []testUser{root, otherMod, mod} {
		for _, o := range []op{warn, ban, reveal} {
			refused(mod, target, o)
		}
	}
	// The low role that recovers sign-ins: not on anyone at or above it, which is how a low role
	// would take over an administrator's account.
	for _, target := range []testUser{root, mod, otherRecovery, recovery} {
		for _, o := range []op{resetLink, twoFactor} {
			refused(recovery, target, o)
		}
	}
	// An admin: not on another admin or their own account.
	for _, target := range []testUser{otherAdmin, root} {
		for _, o := range []op{warn, ban, reveal, resetLink, twoFactor} {
			refused(root, target, o)
		}
	}

	// Nothing was issued: everyone refused still works, and the audit log shows no reveal.
	for _, u := range []testUser{root, otherAdmin, mod, otherMod, recovery, otherRecovery} {
		e.want(e.do(http.MethodGet, "/accounts/verify_credentials", u.token, nil, nil), http.StatusOK, "still signed in")
	}
	var audit clientapi.AdminAuditEntryPage
	e.want(e.do(http.MethodGet, "/admin/audit_log?action=revealed_private_data", root.token, nil, &audit), http.StatusOK, "audit log")
	if len(audit.Items) != 0 {
		t.Errorf("audit log after refused reveals = %+v, want none", audit.Items)
	}

	// Each acts on a lower rank.
	for _, o := range []op{warn, reveal} {
		allowed(mod, member, o)
	}
	for _, o := range []op{resetLink, twoFactor} {
		allowed(recovery, member, o)
	}
	for _, o := range []op{warn, reveal, resetLink} {
		allowed(root, mod, o)
	}
	allowed(root, recovery, ban)

	// Lifting an action is acting on its target; the CLI acts on anyone.
	action, err := e.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: otherAdmin.id, Kind: moderation.ActionLimit, ReasonCategory: moderation.CategoryOther,
	})
	if err != nil {
		t.Fatalf("IssueAccountAction by the CLI: %v", err)
	}
	lift := "/admin/account_actions/" + action.ID + "/lift"
	for _, actor := range []testUser{mod, root} {
		if p := e.want(e.do(http.MethodPost, lift, actor.token, map[string]any{}, nil), http.StatusForbidden, "lift"); p != clientapi.ProblemTypeTargetOutranksActor {
			t.Errorf("lift = %q", p)
		}
	}
	if err := e.app.Moderation.LiftAccountAction(ctx, action.ID, "", ""); err != nil {
		t.Errorf("LiftAccountAction by the CLI: %v", err)
	}
	var onMember clientapi.AdminAccountAction
	e.want(e.do(http.MethodPost, actions(secondMember), mod.token, map[string]any{"kind": "limit"}, &onMember), http.StatusCreated, "limit a member")
	e.want(e.do(http.MethodPost, "/admin/account_actions/"+onMember.Id+"/lift", mod.token, map[string]any{}, nil), http.StatusOK, "lift on a member")
	// A local account's reset link from the CLI still works on an admin.
	if _, err := e.app.Identity.IssuePasswordResetLink(ctx, root.id, ""); err != nil {
		t.Errorf("IssuePasswordResetLink by the CLI: %v", err)
	}
}

// TestLastAdministratorIsProtected checks that the only administrator with an active account cannot
// be locked out or delete their account, and that an administrator who has left does not count.
func TestLastAdministratorIsProtected(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	ctx := t.Context()
	root := e.user("root")
	const password = "correct horse battery staple"
	deleteOwn := func(u testUser) *http.Response {
		t.Helper()
		return e.do(http.MethodPost, "/accounts/delete", u.token, map[string]any{"password": password}, nil)
	}
	wantLast := func(res *http.Response) {
		t.Helper()
		if p := e.want(res, http.StatusConflict, "delete the last administrator"); p != clientapi.ProblemTypeLastAdministrator {
			t.Errorf("problem = %q, want %q", p, clientapi.ProblemTypeLastAdministrator)
		}
	}

	wantLast(deleteOwn(root))
	for _, kind := range []moderation.ActionKind{moderation.ActionDisableLogin, moderation.ActionSuspend, moderation.ActionBan} {
		_, err := e.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
			TargetAccountID: root.id, Kind: kind, ReasonCategory: moderation.CategoryOther,
		})
		if !errors.Is(err, appmoderation.ErrLastAdministrator) {
			t.Errorf("IssueAccountAction(%s) by the CLI = %v, want ErrLastAdministrator", kind, err)
		}
	}
	if _, err := e.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: root.id, Kind: moderation.ActionWarn, ReasonCategory: moderation.CategoryOther,
	}); err != nil {
		t.Errorf("a warning by the CLI = %v, want it issued", err)
	}
	e.want(e.do(http.MethodGet, "/accounts/verify_credentials", root.token, nil, nil), http.StatusOK, "still signed in")

	// With a second administrator either may leave, but not both: the one who left no longer counts.
	second := e.user("second")
	e.want(deleteOwn(second), http.StatusNoContent, "delete a second administrator")
	wantLast(deleteOwn(root))
	if _, err := e.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: root.id, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategoryOther,
	}); !errors.Is(err, appmoderation.ErrLastAdministrator) {
		t.Errorf("suspend after the second administrator left = %v, want ErrLastAdministrator", err)
	}

	// Nor can the role be taken from them, although the one who left still holds it.
	rootActor, err := e.app.Accounts.FindByID(ctx, root.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Identity.RevokeRole(ctx, appidentity.RoleChangeInput{UserID: rootActor.Local.UserID, RoleKey: domainidentity.RoleAdmin}); !errors.Is(err, appidentity.ErrLastAdmin) {
		t.Errorf("RevokeRole after the second administrator left = %v, want ErrLastAdmin", err)
	}

	// The wrong password stays the answer to a wrong password.
	res := e.do(http.MethodPost, "/accounts/delete", root.token, map[string]any{"password": "not the password at all"}, nil)
	if p := e.want(res, http.StatusUnprocessableEntity, "wrong password"); p == clientapi.ProblemTypeLastAdministrator {
		t.Errorf("wrong password = %q", p)
	}

	// A new administrator lets the first one go.
	third := e.user("third")
	e.want(deleteOwn(root), http.StatusNoContent, "delete with another administrator")
	wantLast(deleteOwn(third))
}

// TestStaffRankOnAppealsAndEmailSuppressions checks the rank on the operations that help the target
// rather than limit it: deciding an appeal, either way, and lifting an email suppression.
func TestStaffRankOnAppealsAndEmailSuppressions(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	ctx := t.Context()
	root, other := e.user("root"), e.user("other")
	mod, member := e.user("mod"), e.user("member")
	e.setRole(mod, domainidentity.RoleModerator)
	e.setRole(member, "")

	appeal := func(u testUser) string {
		t.Helper()
		action, err := e.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
			TargetAccountID: u.id, Kind: moderation.ActionWarn, ReasonCategory: moderation.CategoryOther,
		})
		if err != nil {
			t.Fatalf("IssueAccountAction: %v", err)
		}
		var filed clientapi.ModerationAction
		e.want(e.do(http.MethodPost, "/moderation/actions/"+action.ID+"/appeal", u.token, map[string]any{"text": "please look again"}, &filed), http.StatusOK, "appeal")
		return filed.Appeal.Id
	}
	refused := func(res *http.Response, what string) {
		t.Helper()
		if p := e.want(res, http.StatusForbidden, what); p != clientapi.ProblemTypeTargetOutranksActor {
			t.Errorf("%s = %q, want %q", what, p, clientapi.ProblemTypeTargetOutranksActor)
		}
	}
	onAdmin := appeal(other)
	for _, decision := range []string{"overturn", "uphold"} {
		for _, actor := range []testUser{mod, root} {
			refused(e.do(http.MethodPost, "/admin/appeals/"+onAdmin+"/"+decision, actor.token, map[string]any{}, nil), decision+" on an admin")
		}
	}
	var pending clientapi.AdminAppeal
	e.want(e.do(http.MethodGet, "/admin/appeals/"+onAdmin, root.token, nil, &pending), http.StatusOK, "the appeal")
	if pending.DecidedAt != nil {
		t.Errorf("appeal after refused decisions = %+v, want it pending", pending)
	}
	if _, err := e.app.Moderation.DecideAppeal(ctx, onAdmin, moderation.AppealDecisionUpheld, "", ""); err != nil {
		t.Errorf("DecideAppeal by the CLI = %v", err)
	}
	e.want(e.do(http.MethodPost, "/admin/appeals/"+appeal(member)+"/overturn", mod.token, map[string]any{}, nil), http.StatusOK, "overturn on a member")
	e.want(e.do(http.MethodPost, "/admin/appeals/"+appeal(member)+"/uphold", mod.token, map[string]any{}, nil), http.StatusOK, "uphold on a member")

	lift := func(target testUser) string { return "/admin/accounts/" + target.id + "/email_suppression/lift" }
	refused(e.do(http.MethodPost, lift(other), root.token, nil, nil), "lift on an admin")
	refused(e.do(http.MethodPost, lift(root), root.token, nil, nil), "lift on the own account")
	// A lower rank passes the check and reaches the state of the address, which is not suppressed.
	if p := e.want(e.do(http.MethodPost, lift(mod), root.token, nil, nil), http.StatusConflict, "lift on a lower rank"); p != clientapi.ProblemTypeEmailNotSuppressed {
		t.Errorf("lift on a lower rank = %q", p)
	}
}

// TestRestrictedAdministratorDoesNotCount checks that an administrator whom moderation keeps from
// signing in leaves the other one as the last, and counts again once the action is lifted.
func TestRestrictedAdministratorDoesNotCount(t *testing.T) {
	t.Parallel()
	for _, kind := range []moderation.ActionKind{moderation.ActionSuspend, moderation.ActionDisableLogin} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			e := newAccountsEnv(t)
			ctx := t.Context()
			root, second := e.user("root"), e.user("second")
			rootActor, err := e.app.Accounts.FindByID(ctx, root.id)
			if err != nil {
				t.Fatal(err)
			}
			issue := func(target testUser) (moderation.Action, error) {
				return e.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
					TargetAccountID: target.id, Kind: kind, ReasonCategory: moderation.CategoryOther,
				})
			}
			deleteRoot := func() *http.Response {
				t.Helper()
				return e.do(http.MethodPost, "/accounts/delete", root.token, map[string]any{"password": "correct horse battery staple"}, nil)
			}
			action, err := issue(second)
			if err != nil {
				t.Fatalf("restrict the second administrator: %v", err)
			}
			if p := e.want(deleteRoot(), http.StatusConflict, "delete"); p != clientapi.ProblemTypeLastAdministrator {
				t.Errorf("delete = %q", p)
			}
			if _, err := issue(root); !errors.Is(err, appmoderation.ErrLastAdministrator) {
				t.Errorf("restrict the last one who can sign in = %v, want ErrLastAdministrator", err)
			}
			if _, err := e.app.Identity.RevokeRole(ctx, appidentity.RoleChangeInput{UserID: rootActor.Local.UserID, RoleKey: domainidentity.RoleAdmin}); !errors.Is(err, appidentity.ErrLastAdmin) {
				t.Errorf("RevokeRole = %v, want ErrLastAdmin", err)
			}
			if err := e.app.Moderation.LiftAccountAction(ctx, action.ID, "", ""); err != nil {
				t.Fatal(err)
			}
			e.want(deleteRoot(), http.StatusNoContent, "delete with another administrator back")
		})
	}
}
