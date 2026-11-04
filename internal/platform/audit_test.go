// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
)

// TestAuditLogOfEveryModule checks, through the composition root on SQLite, that the
// administrative use cases of the identity, instance, and moderation modules each record one
// audit log entry (ADR-0058) with their actor, target, and label, and that operations that
// fail or change nothing record none.
func TestAuditLogOfEveryModule(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": "inline", "MY_MAIL": "none"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ctx := t.Context()
	admin, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "admin", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	by := admin.User.ID

	// instance: a setting, a text, and the rules.
	if _, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingRegistrationMode, Value: "approval", RevisedByUserID: by}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingRegistrationMode, Value: "nonsense"}); err == nil {
		t.Fatal("SetSetting(nonsense) succeeded")
	}
	if _, err := app.Instance.SetText(ctx, appinstance.SetTextInput{Key: instance.TextTerms, Body: "規約"}); err != nil {
		t.Fatal(err)
	}
	first, err := app.Instance.AddRule(ctx, appinstance.AddRuleInput{Text: "Be kind", CreatedByUserID: by})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.Instance.AddRule(ctx, appinstance.AddRuleInput{Text: "No spam", CreatedByUserID: by})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Instance.ReviseRule(ctx, appinstance.ReviseRuleInput{RuleID: first.ID, Text: "Be very kind", RevisedByUserID: by}); err != nil {
		t.Fatal(err)
	}
	moved, err := app.Instance.MoveRule(ctx, appinstance.MoveRuleInput{RuleID: second.ID, Position: 1, RevisedByUserID: by})
	if err != nil {
		t.Fatal(err)
	}
	// Moving a rule to its place records nothing.
	if _, err := app.Instance.MoveRule(ctx, appinstance.MoveRuleInput{RuleID: second.ID, Position: 1, RevisedByUserID: by}); err != nil {
		t.Fatal(err)
	}
	for range 2 { // retiring a retired rule records nothing
		if _, err := app.Instance.RetireRule(ctx, first.ID, ""); err != nil {
			t.Fatal(err)
		}
	}

	// identity: a registration approved by the CLI.
	signed, err := app.Identity.SignUp(ctx, identity.SignUpInput{
		Username: "bob", Email: "bob@example.com", Password: "correct horse battery staple", Reason: "hi",
	})
	if err != nil || signed.State != domainidentity.RegistrationPending {
		t.Fatalf("SignUp = %+v, %v; want pending", signed.State, err)
	}
	decision, err := app.Identity.DecideRegistration(ctx, identity.DecideInput{UserID: signed.User.ID, Decision: domainidentity.DecisionApproved})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Identity.DecideRegistration(ctx, identity.DecideInput{UserID: signed.User.ID, Decision: domainidentity.DecisionRejected}); err == nil {
		t.Fatal("deciding a decided registration succeeded")
	}

	// moderation: a domain action.
	action, err := app.Moderation.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "bad.example", Kind: moderation.DomainLimit, IssuedByUserID: by})
	if err != nil {
		t.Fatal(err)
	}

	got, err := app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		actor         moderation.AuditActorKind
		user          string
		action        moderation.AuditAction
		kind          moderation.AuditTargetKind
		target, label string
		source        string
	}
	var rows []row
	for _, e := range got {
		rows = append(rows, row{e.ActorKind, e.ActorUserID, e.Action, e.TargetKind, e.TargetID, e.TargetLabel, e.SourceEventID})
	}
	slices.Reverse(rows)
	if len(rows) != 9 {
		t.Fatalf("audit log, oldest first = %+v, want 9 entries", rows)
	}
	user, cli := moderation.AuditActorUser, moderation.AuditActorCLI
	want := []row{
		{user, by, moderation.AuditSettingChanged, moderation.AuditTargetSetting, "registration_mode", "registration_mode", rows[0].source},
		{cli, "", moderation.AuditTextChanged, moderation.AuditTargetSetting, "terms", "terms", rows[1].source},
		{user, by, moderation.AuditRuleAdded, moderation.AuditTargetRule, first.ID, "Be kind", first.Revision.ID},
		{user, by, moderation.AuditRuleAdded, moderation.AuditTargetRule, second.ID, "No spam", second.Revision.ID},
		{user, by, moderation.AuditRuleRevised, moderation.AuditTargetRule, first.ID, "Be very kind", rows[4].source},
		{user, by, moderation.AuditRuleMoved, moderation.AuditTargetRule, second.ID, "No spam", moved.Revision.ID},
		{cli, "", moderation.AuditRuleRetired, moderation.AuditTargetRule, first.ID, "Be very kind", first.ID},
		{cli, "", moderation.AuditRegistrationApproved, moderation.AuditTargetUser, signed.User.ID, "@bob", decision.ID},
		{user, by, moderation.AuditDomainActionIssued, moderation.AuditTargetDomain, "bad.example", "bad.example", action.ID},
	}
	if !slices.Equal(rows, want) {
		t.Errorf("audit log, oldest first:\n got  %+v\n want %+v", rows, want)
	}
	for i, r := range rows {
		if r.source == "" {
			t.Errorf("entry %d %s has no source event", i, r.action)
		}
	}
}
