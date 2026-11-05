// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"net/url"
	"strings"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

func TestAdminAccountPasswordResetLink(t *testing.T) {
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
	ctx := t.Context()
	root, err := app.Accounts.FindByHandle(ctx, "root", "")
	if err != nil {
		t.Fatal(err)
	}

	if code, _, stderr := runCmd(t, adminCmd, "account", "password-reset-link", "@nobody"); code != 1 || stderr == "" {
		t.Errorf("password-reset-link of an unknown account = %d %q", code, stderr)
	}
	code, stdout, stderr := runCmd(t, adminCmd, "account", "password-reset-link", "@root")
	if code != 0 || !strings.Contains(stderr, root.Account.ID) {
		t.Fatalf("password-reset-link = %d %q %q", code, stdout, stderr)
	}
	link, err := url.Parse(strings.TrimSpace(stdout))
	if err != nil || link.Path != appidentity.PathResetPassword {
		t.Fatalf("link = %q, %v; want a link to %s", stdout, err, appidentity.PathResetPassword)
	}
	token := link.Query().Get("token")
	if err := app.Identity.CheckPasswordResetToken(ctx, token); err != nil {
		t.Fatalf("CheckPasswordResetToken = %v", err)
	}
	if _, err := app.Identity.ResetPassword(ctx, token, "a new password"); err != nil {
		t.Fatalf("ResetPassword = %v", err)
	}
	entries, err := app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{Action: moderation.AuditPasswordResetLinkIssued}, 10)
	if err != nil || len(entries) != 1 || entries[0].ActorKind != moderation.AuditActorCLI || entries[0].TargetID != root.Account.ID {
		t.Errorf("audit log = %+v, %v; want one entry by the CLI", entries, err)
	}
}

// TestAdminCreatePasswordLink covers the platforms whose one-off commands have no standard input and no terminal: the
// command reads neither, prints a link that sets the password, and leaves a password nobody knows.
func TestAdminCreatePasswordLink(t *testing.T) {
	newAdminEnv(t)
	code, stdout, stderr := runAdminCreate(t, "", "-username", "root", "-email", "root@example.com", "-password-link")
	if code != 0 {
		t.Fatalf("admin create -password-link = %d %q %q", code, stdout, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "created admin @root@social.example") {
		t.Fatalf("stdout = %q, want the created handle and the link", stdout)
	}
	link, err := url.Parse(lines[1])
	if err != nil || link.Path != appidentity.PathResetPassword {
		t.Fatalf("link = %q, %v; want a link to %s", lines[1], err, appidentity.PathResetPassword)
	}
	if !strings.Contains(stderr, "once, until") {
		t.Errorf("stderr = %q, want how long the link lasts", stderr)
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
	ctx := t.Context()
	token := link.Query().Get("token")
	if _, err := app.Identity.ResetPassword(ctx, token, adminPassword); err != nil {
		t.Fatalf("ResetPassword = %v", err)
	}
	if _, err := app.Identity.ResetPassword(ctx, token, adminPassword+"x"); err == nil {
		t.Error("the link worked a second time")
	}
	root, err := app.Accounts.FindByHandle(ctx, "root", "")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{Action: moderation.AuditPasswordResetLinkIssued}, 10)
	if err != nil || len(entries) != 1 || entries[0].ActorKind != moderation.AuditActorCLI || entries[0].TargetID != root.Account.ID {
		t.Errorf("audit log = %+v, %v; want one entry by the CLI", entries, err)
	}

	// Taken names fail as without the flag, and no second link appears.
	code, stdout, stderr = runAdminCreate(t, "", "-username", "root", "-email", "other@example.com", "-password-link")
	if code != 1 || !strings.Contains(stderr, "already taken") || strings.Contains(stdout, appidentity.PathResetPassword) {
		t.Errorf("second create = %d %q %q", code, stdout, stderr)
	}
}
