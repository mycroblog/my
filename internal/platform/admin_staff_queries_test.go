// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// adminStaffQueries counts the round trips of the audit log, the actions on an account, a report
// with its notes, and the invites, each written by n members of the staff or n members.
func adminStaffQueries(t *testing.T, n int) map[string]int {
	t.Helper()
	tracer := testkit.NewTracer()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Tracer: tracer})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	ctx := t.Context()
	var staff []identity.Registration
	for i := range n + 1 {
		r, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
			Username: fmt.Sprintf("staff%d", i), Email: fmt.Sprintf("staff%d@example.com", i), Password: "correct horse battery staple",
		})
		if err != nil {
			t.Fatal(err)
		}
		staff = append(staff, r)
	}
	viewer := issueTokens(t, app, staff[0].User.ID, "").AccessToken
	target, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "target", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := app.LocalReports.File(ctx, appmoderation.LocalReportInput{
		ReporterAccountID: staff[0].AccountID, TargetAccountID: target.AccountID, Category: moderation.CategorySpam,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingInviteCreators, Value: string(instance.InviteCreatorsEveryone)}); err != nil {
		t.Fatal(err)
	}
	for _, s := range staff[1:] {
		if _, err := app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
			TargetAccountID: target.AccountID, Kind: moderation.ActionWarn, ReasonCategory: moderation.CategoryOther, IssuedByUserID: s.User.ID,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := app.Moderation.AddReportNote(ctx, report.ID, "noted", s.User.ID); err != nil {
			t.Fatal(err)
		}
		member, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "member" + s.User.ID[len(s.User.ID)-6:], Password: "correct horse battery staple"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := app.Identity.CreateOwnInvite(ctx, identity.CreateOwnInviteInput{UserID: member.User.ID}); err != nil {
			t.Fatal(err)
		}
	}
	out := map[string]int{}
	for _, path := range []string{"/admin/audit_log", "/admin/accounts/" + target.AccountID + "/actions", "/admin/reports/" + report.ID, "/admin/invites"} {
		was := ops(tracer)
		e.want(e.do(http.MethodGet, path, viewer, nil, nil), http.StatusOK, path)
		count, names := opsSince(tracer, was)
		t.Logf("GET %s with %d staff: %d queries: %v", path, n, count, names)
		out[path[:min(len(path), 18)]] = count
	}
	var page clientapi.AdminInvitePage
	e.want(e.do(http.MethodGet, "/admin/invites", viewer, nil, &page), http.StatusOK, "invites")
	for _, inv := range page.Items {
		if inv.State != clientapi.AdminInviteStateUsable {
			t.Errorf("invite %s = %s, want usable", inv.Id, inv.State)
		}
	}
	return out
}

// TestAdminStaffQueries checks that the round trips of the administration's smaller lists do not
// grow with the staff and the creators on them.
func TestAdminStaffQueries(t *testing.T) {
	t.Parallel()
	small, large := adminStaffQueries(t, 2), adminStaffQueries(t, 5)
	for k, v := range large {
		if v > small[k] {
			t.Errorf("GET %s makes %d queries with 5 staff, more than the %d with 2", k, v, small[k])
		}
	}
}
