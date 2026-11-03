// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"errors"
	"log/slog"
	"net/http"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestBanAppealsThroughTheCompositionRoot checks the appeals of bans (FR-707, the decider:
// 「30 日のあいだは、BAN への異議申し立てを受け付ける」): a banned account appeals while its deletion
// is scheduled, overturning the appeal cancels the deletion and ends the ban at once so the user
// can sign in again and the deletion job leaves the account, and once the account is deleted its
// ban can no longer be appealed.
func TestBanAppealsThroughTheCompositionRoot(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ctx := t.Context()
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	mod := e.user("mod")
	member := func(name string) (testUser, string) {
		t.Helper()
		r, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
			Username: name, Password: "correct horse battery staple", DisplayName: name,
		})
		if err != nil {
			t.Fatalf("RegisterLocalUser(%s): %v", name, err)
		}
		return testUser{id: r.AccountID, token: issueTokens(t, app, r.User.ID, "").AccessToken}, r.User.ID
	}
	ban := func(who testUser) clientapi.AdminAccountAction {
		t.Helper()
		var a clientapi.AdminAccountAction
		e.want(e.do(http.MethodPost, "/admin/accounts/"+who.id+"/actions", mod.token, map[string]any{"kind": "ban"}, &a), http.StatusCreated, "ban")
		return a
	}
	alice, aliceUser := member("alice")
	bob, _ := member("bob")

	// A banned account appeals within the grace before its deletion.
	b := ban(alice)
	v, err := app.Moderation.OwnAction(ctx, alice.id, b.Id)
	if err != nil || !v.Appealable || v.AppealDeadline == nil || !v.AppealDeadline.Equal(b.IssuedAt.Add(moderation.BanDeletionDelay)) {
		t.Fatalf("OwnAction(ban) = %+v, %v; want appealable until the deletion", v, err)
	}
	v, err = app.Moderation.FileAppeal(ctx, alice.id, b.Id, "I did nothing wrong.")
	if err != nil || v.Appeal == nil {
		t.Fatalf("FileAppeal(ban) = %+v, %v", v, err)
	}
	var counts clientapi.AdminCounts
	e.want(e.do(http.MethodGet, "/admin/counts", mod.token, nil, &counts), http.StatusOK, "counts")
	if counts.PendingAppeals == nil || *counts.PendingAppeals != 1 {
		t.Errorf("counts = %+v, want 1 pending appeal", counts)
	}
	e.want(e.do(http.MethodPost, "/admin/account_actions/"+b.Id+"/lift", mod.token, nil, nil), http.StatusUnprocessableEntity, "lift the ban")

	// Overturning cancels the deletion and ends the ban.
	var decided clientapi.AdminAppeal
	e.want(e.do(http.MethodPost, "/admin/appeals/"+v.Appeal.ID+"/overturn", mod.token, map[string]any{"public_reason": "Our mistake."}, &decided),
		http.StatusOK, "overturn the ban's appeal")
	if decided.State != clientapi.ModerationAppealStateOverturned || decided.Action.State != clientapi.AdminAccountActionStateLifted {
		t.Errorf("overturned = %+v, want the ban lifted", decided)
	}
	actor, err := app.Accounts.FindByID(ctx, alice.id)
	if err != nil || actor.Lifecycle.State != account.StateActive {
		t.Errorf("alice after overturning = %+v, %v; want active", actor.Lifecycle, err)
	}
	if n, err := app.Accounts.ExecuteDueDeletions(ctx); err != nil || n != 0 {
		t.Errorf("ExecuteDueDeletions after overturning = %d, %v; want none", n, err)
	}
	again := testUser{id: alice.id, token: issueTokens(t, app, aliceUser, "").AccessToken}
	e.want(e.do(http.MethodGet, "/accounts/verify_credentials", again.token, nil, nil), http.StatusOK, "alice after overturning")
	e.want(e.do(http.MethodGet, "/accounts/"+alice.id, bob.token, nil, nil), http.StatusOK, "alice's profile after overturning")
	var audit clientapi.AdminAuditEntryPage
	e.want(e.do(http.MethodGet, "/admin/audit_log?target_kind=account&target_id="+alice.id, mod.token, nil, &audit), http.StatusOK, "audit log")
	var actions []clientapi.AdminAuditAction
	for _, en := range audit.Items {
		actions = append(actions, en.Action)
	}
	if len(actions) < 3 || actions[0] != clientapi.AdminAuditActionAccountActionLifted || actions[1] != clientapi.AdminAuditActionAccountDeletionCanceled {
		t.Errorf("audit log of alice = %v, want the lift after the canceled deletion", actions)
	}

	// Once the account is deleted, its ban is not appealed.
	b = ban(bob)
	if _, err := app.Accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: bob.id, Kind: account.LifecycleDeleted, InitiatedBy: account.InitiatedBySystem,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Moderation.FileAppeal(ctx, bob.id, b.Id, "please"); !errors.Is(err, appmoderation.ErrNotAppealable) {
		t.Errorf("FileAppeal(ban of a deleted account) = %v, want ErrNotAppealable", err)
	}
}
