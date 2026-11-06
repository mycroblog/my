// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

func adminCmd(args []string, out, errOut *bytes.Buffer) int {
	return runAdmin(args, strings.NewReader(""), out, errOut)
}

var issuedAction = regexp.MustCompile(`^issued suspend on xn--r8jz45g\.jp and its subdomains \(action (\S+), never expires\)\n$`)

func TestAdminDomainCommands(t *testing.T) {
	newAdminEnv(t)
	code, stdout, stderr := runCmd(t, adminCmd, "domain", "issue", "-kind", "suspend", "-public-comment", "spam\nwave", "例え.JP.")
	m := issuedAction.FindStringSubmatch(stdout)
	if code != 0 || m == nil {
		t.Fatalf("domain issue = %d %q %q", code, stdout, stderr)
	}
	id := m[1]
	if code, stdout, _ = runCmd(t, adminCmd, "domain", "issue", "-kind", "limit", "-duration", "24h", "limit.example"); code != 0 ||
		!strings.Contains(stdout, "issued limit on limit.example") || !strings.Contains(stdout, "expires 20") {
		t.Errorf("domain issue -duration = %d %q", code, stdout)
	}
	cfg, _, err := config.Load(config.Source{})
	if err != nil {
		t.Fatal(err)
	}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = app.Close() }()
	// The suspension is in effect for federation at once, subdomains included.
	if ok, err := app.ModerationPolicy.FederationSuspended(t.Context(), "sub.xn--r8jz45g.jp"); err != nil || !ok {
		t.Errorf("FederationSuspended = %v, %v; want true", ok, err)
	}
	if code, stdout, _ = runCmd(t, adminCmd, "domain", "list"); code != 0 ||
		!regexp.MustCompile(id+` +xn--r8jz45g\.jp +suspend +active +\S+ +- +cli +spam wave`).MatchString(stdout) ||
		!strings.Contains(stdout, "limit.example") {
		t.Errorf("domain list = %d\n%s", code, stdout)
	}
	if code, stdout, _ = runCmd(t, adminCmd, "domain", "lift", id); code != 0 || stdout != "lifted domain action "+id+"\n" {
		t.Errorf("domain lift = %d %q", code, stdout)
	}
	if ok, err := app.ModerationPolicy.FederationSuspended(t.Context(), "xn--r8jz45g.jp"); err != nil || ok {
		t.Errorf("FederationSuspended after the lift = %v, %v; want false", ok, err)
	}
	if _, stdout, _ = runCmd(t, adminCmd, "domain", "list", "-domain", "例え.jp"); !regexp.MustCompile(id+` +\S+ +suspend +lifted`).MatchString(stdout) || strings.Contains(stdout, "limit.example") {
		t.Errorf("domain list -domain:\n%s", stdout)
	}
	if _, stdout, _ = runCmd(t, adminCmd, "domain", "list", "-active"); strings.Contains(stdout, id) || !strings.Contains(stdout, "limit.example") {
		t.Errorf("domain list -active:\n%s", stdout)
	}

	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"domain", "issue", "-kind", "silence", "bad.example"}, 1, `invalid action: kind "silence"`},
		{[]string{"domain", "issue", "-kind", "suspend", "https://bad.example/"}, 1, "invalid host"},
		{[]string{"domain", "issue", "bad.example"}, 2, "-kind is required"},
		{[]string{"domain", "lift", "01890a5d-ac96-774b-bcce-b302099a0000"}, 1, "no domain action has the ID"},
		{[]string{"domain", "lift"}, 2, "want 1 argument(s)"},
		{[]string{"domain", "burn"}, 2, `unknown subcommand "burn"`},
		{[]string{"domain", "list", "-limit", "0"}, 2, "-limit must be at least 1"},
	} {
		if code, _, stderr := runCmd(t, adminCmd, tc.args...); code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("admin %q = %d %q; want %d with %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}

func TestAdminReportCommands(t *testing.T) {
	newAdminEnv(t)
	if code, _, stderr := runAdminCreate(t, adminPassword+"\n", "-username", "root", "-email", "root@example.com"); code != 0 {
		t.Fatalf("admin create: %d %q", code, stderr)
	}
	cfg, _, err := config.Load(config.Source{})
	if err != nil {
		t.Fatal(err)
	}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = app.Close() }()
	root, err := app.Accounts.FindByHandle(t.Context(), "root", "")
	if err != nil {
		t.Fatal(err)
	}
	r, _, err := app.Moderation.FileRemoteReport(t.Context(), appmoderation.RemoteReportInput{
		URI: "https://remote.example/flags/1", ReporterAccountID: "01890a5d-ac96-774b-bcce-b302099a0001",
		TargetAccountID: root.Account.ID, Comment: "spam\nspam",
	})
	if err != nil {
		t.Fatal(err)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "report", "list"); code != 0 ||
		!regexp.MustCompile(r.ID+` +open +\S+ +@root +01890a5d-ac96-774b-bcce-b302099a0001 +- +0 +spam spam`).MatchString(stdout) {
		t.Errorf("report list = %d\n%s", code, stdout)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "report", "assign", r.ID, "@root"); code != 0 || stdout != "report "+r.ID+" is assigned to @root\n" {
		t.Errorf("report assign = %d %q", code, stdout)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "report", "show", r.ID); code != 0 || !regexp.MustCompile(`assignee: +@root`).MatchString(stdout) ||
		!strings.Contains(stdout, "uri:       https://remote.example/flags/1") || !strings.HasSuffix(stdout, "\nspam\nspam\n") {
		t.Errorf("report show = %d\n%s", code, stdout)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "report", "unassign", r.ID); code != 0 || stdout != "report "+r.ID+" is assigned to nobody\n" {
		t.Errorf("report unassign = %d %q", code, stdout)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "report", "resolve", "-outcome", "dismissed", r.ID); code != 0 || stdout != "report "+r.ID+" is resolved (dismissed)\n" {
		t.Errorf("report resolve = %d %q", code, stdout)
	}
	if _, stdout, _ := runCmd(t, adminCmd, "report", "list", "-state", "open"); strings.Contains(stdout, r.ID) {
		t.Errorf("report list -state open:\n%s", stdout)
	}
	if _, stdout, _ := runCmd(t, adminCmd, "report", "list", "-state", "resolved"); !regexp.MustCompile(r.ID + ` +resolved \(dismissed\)`).MatchString(stdout) {
		t.Errorf("report list -state resolved:\n%s", stdout)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "report", "note", r.ID, "seen before"); code != 0 ||
		!regexp.MustCompile(`^note \S+ is added to report `+r.ID+"\n$").MatchString(stdout) {
		t.Errorf("report note = %d %q", code, stdout)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "report", "reopen", r.ID); code != 0 || stdout != "report "+r.ID+" is open again\n" {
		t.Errorf("report reopen = %d %q", code, stdout)
	}
	// A report received from another server is not forwarded again.
	if code, _, stderr := runCmd(t, adminCmd, "report", "forward", r.ID); code != 1 || !strings.Contains(stderr, "cannot be forwarded") {
		t.Errorf("report forward of a received report = %d %q", code, stderr)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "report", "show", r.ID); code != 0 || !regexp.MustCompile(`state: +open`).MatchString(stdout) ||
		!regexp.MustCompile(`history: +0 other reports, 0 actions`).MatchString(stdout) ||
		!regexp.MustCompile(`\nnote \S+ by - at \S+:\nseen before\n$`).MatchString(stdout) {
		t.Errorf("report show after a note = %d\n%s", code, stdout)
	}
	// An action that follows from the report resolves it as actioned.
	if code, _, stderr := runCmd(t, adminCmd, "account", "action", "issue", "-kind", "warn", "-category", "spam", "-report", r.ID, "@root"); code != 0 {
		t.Fatalf("account action issue = %d %s", code, stderr)
	}
	if _, stdout, _ := runCmd(t, adminCmd, "report", "show", r.ID); !regexp.MustCompile(`state: +resolved \(actioned\)`).MatchString(stdout) ||
		!regexp.MustCompile(`history: +0 other reports, 1 actions`).MatchString(stdout) {
		t.Errorf("report show after an action:\n%s", stdout)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "audit", "list", "-target-kind", "report", "-target-id", r.ID); code != 0 ||
		!regexp.MustCompile(`(?s)report_resolved.*report_reopened.*report_note_added.*report_resolved`).MatchString(stdout) {
		t.Errorf("audit list of the report = %d\n%s", code, stdout)
	}

	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"report", "assign", r.ID, "nobody"}, 1, `no local account has the username "nobody"`},
		{[]string{"report", "show", "01890a5d-ac96-774b-bcce-b302099a0000"}, 1, "no report has the ID"},
		{[]string{"report", "resolve", "-outcome", "ignored", r.ID}, 1, "invalid report"},
		{[]string{"report", "resolve", r.ID}, 2, "-outcome is required"},
		{[]string{"report", "list", "-state", "closed"}, 1, "invalid query"},
		{[]string{"report"}, 2, "Usage:"},
	} {
		if code, _, stderr := runCmd(t, adminCmd, tc.args...); code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("admin %q = %d %q; want %d with %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}
