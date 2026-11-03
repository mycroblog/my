// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/content"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	domaincontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

var issuedAccountAction = regexp.MustCompile(`^issued disable_login on account (\S+) \(action (\S+), never expires\)\n$`)

func TestAdminAccountActionCommands(t *testing.T) {
	newAdminEnv(t)
	if code, _, stderr := runAdminCreate(t, adminPassword+"\n", "-username", "root", "-email", "root@example.com"); code != 0 {
		t.Fatalf("admin create: %d %q", code, stderr)
	}
	// With a second administrator, @root is not the last one and can be locked out.
	if code, _, stderr := runAdminCreate(t, adminPassword+"\n", "-username", "second", "-email", "second@example.com"); code != 0 {
		t.Fatalf("admin create: %d %q", code, stderr)
	}
	code, stdout, stderr := runCmd(t, adminCmd, "account", "action", "issue", "-kind", "disable_login", "-category", "spam",
		"-public-reason", "spam\nwave", "@root")
	m := issuedAccountAction.FindStringSubmatch(stdout)
	if code != 0 || m == nil {
		t.Fatalf("account action issue = %d %q %q", code, stdout, stderr)
	}
	accountID, id := m[1], m[2]
	if code, stdout, _ = runCmd(t, adminCmd, "account", "action", "issue", "-kind", "restrict_posting", "-duration", "24h", "-no-notify", accountID); code != 0 ||
		!strings.Contains(stdout, "issued restrict_posting on account "+accountID) || !strings.Contains(stdout, "expires 20") {
		t.Errorf("account action issue -duration = %d %q", code, stdout)
	}
	posting := regexp.MustCompile(`\(action (\S+),`).FindStringSubmatch(stdout)
	cfg, _, err := config.Load(config.Source{})
	if err != nil {
		t.Fatal(err)
	}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = app.Close() }()
	// The login restriction is in effect at once.
	if ok, err := app.AccountPolicy.LoginRestricted(t.Context(), accountID); err != nil || !ok {
		t.Errorf("LoginRestricted = %v, %v; want true", ok, err)
	}
	if code, stdout, _ = runCmd(t, adminCmd, "account", "action", "list", "@root"); code != 0 ||
		!regexp.MustCompile(id+` +disable_login +active +spam +\S+ +- +cli +- +spam wave`).MatchString(stdout) ||
		!strings.Contains(stdout, "restrict_posting") {
		t.Errorf("account action list = %d\n%s", code, stdout)
	}
	if code, stdout, _ = runCmd(t, adminCmd, "account", "action", "lift", "-reason", "appealed", id); code != 0 || stdout != "lifted account action "+id+"\n" {
		t.Errorf("account action lift = %d %q", code, stdout)
	}
	if ok, err := app.AccountPolicy.LoginRestricted(t.Context(), accountID); err != nil || ok {
		t.Errorf("LoginRestricted after the lift = %v, %v; want false", ok, err)
	}
	if _, stdout, _ = runCmd(t, adminCmd, "account", "action", "list", "-active", accountID); strings.Contains(stdout, id) || !strings.Contains(stdout, "restrict_posting") {
		t.Errorf("account action list -active:\n%s", stdout)
	}

	// delete_statuses deletes the statuses it names.
	if posting == nil {
		t.Fatal("no restrict_posting action to lift")
	}
	if code, _, stderr = runCmd(t, adminCmd, "account", "action", "lift", posting[1]); code != 0 {
		t.Fatalf("account action lift restrict_posting = %d %q", code, stderr)
	}
	post, err := app.Content.Create(t.Context(), content.CreateInput{
		TextInput: content.TextInput{Text: "spam"}, AuthorAccountID: accountID, Visibility: domaincontent.VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr = runCmd(t, adminCmd, "account", "action", "issue", "-kind", "delete_statuses", "-status", post.Status.ID, "@root"); code != 0 ||
		!strings.Contains(stdout, "issued delete_statuses") || !strings.Contains(stdout, "statuses: "+post.Status.ID) {
		t.Errorf("account action issue delete_statuses = %d %q %q", code, stdout, stderr)
	}
	if _, err := app.Content.Find(t.Context(), post.Status.ID); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("Find(deleted status) = %v, want ErrNotFound", err)
	}
	// A ban is never lifted.
	code, stdout, stderr = runCmd(t, adminCmd, "account", "action", "issue", "-kind", "ban", "@root")
	ban := regexp.MustCompile(`^issued ban on account \S+ \(action (\S+), never expires\)\n$`).FindStringSubmatch(stdout)
	if code != 0 || ban == nil {
		t.Fatalf("account action issue ban = %d %q %q", code, stdout, stderr)
	}
	if code, _, stderr = runCmd(t, adminCmd, "account", "action", "lift", ban[1]); code != 1 || !strings.Contains(stderr, "never lifted") {
		t.Errorf("account action lift ban = %d %q", code, stderr)
	}

	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"account", "action", "issue", "-kind", "silence", "@root"}, 1, `invalid action: kind "silence"`},
		{[]string{"account", "action", "issue", "-kind", "ban", "-duration", "24h", "@root"}, 1, "never expires"},
		{[]string{"account", "action", "issue", "-kind", "delete_statuses", "@root"}, 1, "statuses of the action"},
		{[]string{"account", "action", "issue", "-kind", "mark_statuses_sensitive", "-status", post.Status.ID, "@root"}, 1, "the status does not exist"},
		{[]string{"account", "action", "issue", "-kind", "warn", "-category", "rude", "@root"}, 1, `category "rude"`},
		{[]string{"account", "action", "issue", "-kind", "warn", "-report", "01890a5d-ac96-774b-bcce-b302099a0000", "@root"}, 1, "the report does not exist"},
		{[]string{"account", "action", "issue", "-kind", "warn", "@nobody"}, 1, `no account is "@nobody"`},
		{[]string{"account", "action", "issue", "@root"}, 2, "-kind is required"},
		{[]string{"account", "action", "lift", "01890a5d-ac96-774b-bcce-b302099a0000"}, 1, "no account action has the ID"},
		{[]string{"account", "action", "lift"}, 2, "want 1 argument(s)"},
		{[]string{"account", "action", "burn"}, 2, `unknown subcommand "burn"`},
		{[]string{"account", "burn"}, 2, `unknown subcommand "burn"`},
		{[]string{"account", "action", "list", "-limit", "0", "@root"}, 2, "-limit must be at least 1"},
	} {
		if code, _, stderr := runCmd(t, adminCmd, tc.args...); code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("admin %q = %d %q; want %d with %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}

// TestAdminAccountActionOnRemoteAccount issues and lifts actions on a remote account with the
// CLI: a suspend takes away its profile and is audited as the CLI's, and a kind that only a
// local account takes fails.
func TestAdminAccountActionOnRemoteAccount(t *testing.T) {
	newAdminEnv(t)
	cfg, _, err := config.Load(config.Source{})
	if err != nil {
		t.Fatal(err)
	}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = app.Close() }()
	const bob = "https://remote.example/users/bob"
	err = app.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		_, err := app.Accounts.UpsertRemoteActor(t.Context(), tx, appaccount.RemoteActorInput{
			URI: bob, Domain: "remote.example", Type: "Person", PreferredUsername: "bob", InboxURL: bob + "/inbox",
			Document: []byte(`{"id":"` + bob + `"}`),
		})
		return err
	})
	if err != nil {
		t.Fatalf("UpsertRemoteActor: %v", err)
	}
	actor, err := app.Accounts.FindByURI(t.Context(), bob)
	if err != nil {
		t.Fatal(err)
	}
	id := actor.Account.ID

	if code, _, stderr := runCmd(t, adminCmd, "account", "action", "issue", "-kind", "disable_login", id); code != 1 ||
		!strings.Contains(stderr, "disable_login on a remote account") {
		t.Errorf("account action issue disable_login on a remote account = %d %q", code, stderr)
	}
	code, stdout, stderr := runCmd(t, adminCmd, "account", "action", "issue", "-kind", "suspend", "-duration", "24h", id)
	m := regexp.MustCompile(`^issued suspend on account ` + id + ` \(action (\S+), expires \S+\)\n$`).FindStringSubmatch(stdout)
	if code != 0 || m == nil {
		t.Fatalf("account action issue suspend = %d %q %q", code, stdout, stderr)
	}
	if hidden, err := app.AccountPolicy.ProfileHidden(t.Context(), id); err != nil || !hidden {
		t.Errorf("ProfileHidden while suspended = %v, %v; want true", hidden, err)
	}
	if code, _, stderr = runCmd(t, adminCmd, "account", "action", "lift", m[1]); code != 0 {
		t.Fatalf("account action lift = %d %q", code, stderr)
	}
	if hidden, err := app.AccountPolicy.ProfileHidden(t.Context(), id); err != nil || hidden {
		t.Errorf("ProfileHidden after the lift = %v, %v; want false", hidden, err)
	}
	entries, err := app.Moderation.AuditLog(t.Context(), appmoderation.AuditQuery{TargetKind: moderation.AuditTargetAccount, TargetID: id}, 10)
	if err != nil || len(entries) != 2 || entries[0].Action != moderation.AuditAccountActionLifted ||
		entries[1].Action != moderation.AuditAccountActionIssued || entries[0].ActorKind != moderation.AuditActorCLI {
		t.Errorf("audit log of the remote account = %+v, %v; want the issue and the lift by the CLI", entries, err)
	}
}
