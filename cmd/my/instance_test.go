// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

func runInstanceCmd(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = runInstance(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func mustInstance(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	code, stdout, stderr := runInstanceCmd(t, stdin, args...)
	if code != 0 {
		t.Fatalf("my instance %q: exit code %d, stderr %q", args, code, stderr)
	}
	return stdout
}

func TestInstanceSetGetList(t *testing.T) {
	newAdminEnv(t)

	if got := mustInstance(t, "", "get", "registration_mode"); got != "approval\n" {
		t.Errorf("get registration_mode before set = %q, want the default", got)
	}
	if got := mustInstance(t, "", "set", "registration_mode", "open"); !strings.HasPrefix(got, "set registration_mode = open (revision ") {
		t.Errorf("set stdout = %q", got)
	}
	mustInstance(t, "", "set", "registration_mode", "closed")
	if got := mustInstance(t, "", "get", "registration_mode"); got != "closed\n" {
		t.Errorf("get registration_mode = %q, want closed", got)
	}
	history := mustInstance(t, "", "history", "registration_mode")
	if lines := strings.Split(strings.TrimSpace(history), "\n"); len(lines) != 3 ||
		!strings.HasSuffix(lines[1], "cli  closed") || !strings.HasSuffix(lines[2], "cli  open") {
		t.Errorf("history = %q, want two CLI revisions, newest first", history)
	}

	mustInstance(t, "", "set", "name", "Example")
	if got := mustInstance(t, "", "set", "name", "例"); !strings.HasPrefix(got, "set name (revision ") {
		t.Errorf("set name stdout = %q", got)
	}
	mustInstance(t, "説明文\n2 行目\n", "set", "description", "-")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"get", "name"}, "例\n"},
		{[]string{"get", "description"}, "説明文\n2 行目\n"},
	} {
		if got := mustInstance(t, "", tc.args...); got != tc.want {
			t.Errorf("%q = %q, want %q", tc.args, got, tc.want)
		}
	}

	// The columns are as wide as the longest key, so compare with single spaces.
	list := strings.Join(strings.Fields(mustInstance(t, "", "list")), " ")
	for _, want := range []string{"registration_mode closed set", "max_status_length 500 default", `name `, `"例"`} {
		if !strings.Contains(list, want) {
			t.Errorf("list = %q, missing %q", list, want)
		}
	}
}

func TestInstanceRejectsBadInput(t *testing.T) {
	newAdminEnv(t)
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{nil, 2, "Usage:"},
		{[]string{"frobnicate"}, 2, "unknown subcommand"},
		{[]string{"get"}, 2, "want 1 argument"},
		{[]string{"get", "no_such_key"}, 2, `unknown key "no_such_key"`},
		{[]string{"set", "-locale", "ja", "name", "x"}, 2, "flag provided but not defined: -locale"},
		{[]string{"set", "max_status_length", "0"}, 1, "outside 1..100000"},
		{[]string{"set", "registration_mode", "everyone"}, 1, "not one of open, approval, invite, closed"},
		{[]string{"get", "terms"}, 1, "terms is not set"},
	} {
		code, _, stderr := runInstanceCmd(t, "", tc.args...)
		if code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("my instance %q = %d, %q; want %d and %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}

// TestInstancePreviewImage checks that `my instance set -owner` uploads an image file as the
// preview_image setting, which also takes the ID of an uploaded media file.
func TestInstancePreviewImage(t *testing.T) {
	newAdminEnv(t)
	if code, _, stderr := runAdminCreate(t, adminPassword+"\n", "-username", "admin", "-email", "admin@example.com"); code != 0 {
		t.Fatalf("admin create: exit code %d, stderr %q", code, stderr)
	}
	file := filepath.Join(t.TempDir(), "preview.png")
	if err := os.WriteFile(file, testkit.FixturePNG(1200, 630, false), 0o600); err != nil {
		t.Fatal(err)
	}
	out := mustInstance(t, "", "set", "-owner", "admin", "preview_image", file)
	m := regexp.MustCompile(`^uploaded .+ as media ([0-9a-f-]{36}); .*\nset preview_image = ([0-9a-f-]{36}) \(revision `).FindStringSubmatch(out)
	if m == nil || m[1] != m[2] {
		t.Fatalf("set -owner stdout = %q", out)
	}
	if got := mustInstance(t, "", "get", "preview_image"); got != m[1]+"\n" {
		t.Errorf("get preview_image = %q, want %s", got, m[1])
	}
	mustInstance(t, "", "set", "preview_image", "")
	mustInstance(t, "", "set", "preview_image", strings.ToUpper(m[1]))
	if got := mustInstance(t, "", "get", "preview_image"); got != m[1]+"\n" {
		t.Errorf("get preview_image after setting the ID = %q, want %s", got, m[1])
	}
	mustInstance(t, "", "set", "preview_image_alt", "マスコット")

	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"set", "preview_image", "preview.png"}, 1, "is not a media ID"},
		{[]string{"set", "preview_image", "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f"}, 1, "not found"},
		{[]string{"set", "-owner", "admin", "registration_mode", "open"}, 2, "-owner applies only to media keys"},
		{[]string{"set", "-owner", "nobody", "preview_image", file}, 1, "owner nobody"},
		{[]string{"set", "-owner", "admin", "preview_image", file + ".missing"}, 1, "no such file"},
	} {
		code, _, stderr := runInstanceCmd(t, "", tc.args...)
		if code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("my instance %q = %d, %q; want %d and %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}

// TestNewUsersGetTheInstanceDefaultLocale checks the composition root: identity reads the
// default_locale instance setting from the database for each registration.
func TestNewUsersGetTheInstanceDefaultLocale(t *testing.T) {
	path := newAdminEnv(t)
	create := func(name string) string {
		t.Helper()
		code, _, stderr := runAdminCreate(t, adminPassword+"\n", "-username", name, "-email", name+"@example.com")
		if code != 0 {
			t.Fatalf("admin create %s: exit code %d, stderr %q", name, code, stderr)
		}
		db, err := sqlite.Open(t.Context(), path, uow.Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		repo := sqliteidentity.NewRepository(db)
		userID, err := repo.FindUserIDByEmail(t.Context(), name+"@example.com")
		if err != nil {
			t.Fatal(err)
		}
		prefs, err := repo.FindPreferences(t.Context(), userID)
		if err != nil {
			t.Fatal(err)
		}
		return prefs.Locale
	}
	if got := create("before"); got != "en" {
		t.Errorf("locale with default_locale unset = %q, want en", got)
	}
	mustInstance(t, "", "set", "default_locale", "ja_jp")
	if got := create("after"); got != "ja-JP" {
		t.Errorf("locale after setting default_locale = %q, want ja-JP", got)
	}
}
