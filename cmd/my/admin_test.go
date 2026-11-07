// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
)

// newAdminEnv points the configuration at a new SQLite database with the product schema applied
// and returns the database path.
func newAdminEnv(t *testing.T) string {
	t.Helper()
	path := sqlitetest.NewFile(t)
	t.Setenv(config.EnvConfigFile, "")
	t.Setenv(config.EnvPrefix+"HOST", "social.example")
	t.Setenv(config.EnvPrefix+"DATABASE", "sqlite")
	t.Setenv(config.EnvPrefix+"DATABASE_URL", path)
	t.Setenv(config.EnvPrefix+"BLOB_FS_DIR", t.TempDir())
	t.Setenv(config.EnvPrefix+"KEK", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	return path
}

func runAdminCreate(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = runAdmin(append([]string{"create"}, args...), strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

const adminPassword = "Tr0ub4dor&3-horse-battery" //nolint:gosec // G101: a test password

func TestAdminCreate(t *testing.T) {
	path := newAdminEnv(t)

	code, stdout, stderr := runAdminCreate(t, adminPassword+"\n", "-username", "root", "-email", "Root@Example.com")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "created admin @root@social.example") {
		t.Errorf("stdout = %q, want the created handle", stdout)
	}
	assertNoSecret(t, stdout, stderr)

	db, err := sqlite.Open(t.Context(), path, uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	repo := db.IdentityRepository()
	userID, err := repo.FindUserIDByEmail(t.Context(), "root@example.com")
	if err != nil {
		t.Fatalf("FindUserIDByEmail = %v", err)
	}
	cred, err := repo.FindCurrentPasswordCredential(t.Context(), userID)
	if err != nil {
		t.Fatalf("FindCurrentPasswordCredential = %v", err)
	}
	hasher := appidentity.NewPasswordHasher(appidentity.DefaultArgon2Params, 1)
	if ok, rehash, err := hasher.Verify(t.Context(), adminPassword, cred.Hash); !ok || rehash || err != nil {
		t.Errorf("Verify(password) = %v, %v, %v; want true, false, nil", ok, rehash, err)
	}
	if ok, _, _ := hasher.Verify(t.Context(), adminPassword+"x", cred.Hash); ok {
		t.Error("Verify(wrong password) = true")
	}
	roles, err := repo.ListUserRoles(t.Context(), userID)
	if err != nil || len(roles) != 1 || roles[0].Key != identity.RoleAdmin {
		t.Errorf("roles = %+v, %v; want admin", roles, err)
	}

	// Re-running fails clearly and changes nothing.
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-username", "ROOT", "-email", "other@example.com"}, `username "ROOT" is already taken; nothing was changed`},
		{[]string{"-username", "other", "-email", "root@example.COM"}, `email address "root@example.COM" is already in use; nothing was changed`},
	} {
		code, stdout, stderr := runAdminCreate(t, adminPassword+"\n", tc.args...)
		if code != 1 || !strings.Contains(stderr, tc.want) {
			t.Errorf("create %v = %d, stderr %q; want 1 and %q", tc.args, code, stderr, tc.want)
		}
		assertNoSecret(t, stdout, stderr)
	}
	if _, err := repo.FindUserIDByEmail(t.Context(), "other@example.com"); err == nil {
		t.Error("a failed run left the other email address behind")
	}
}

func TestAdminCreateRejectsBadInput(t *testing.T) {
	newAdminEnv(t)
	for _, tc := range []struct {
		name, stdin string
		args        []string
		code        int
		want        string
	}{
		{"short password", "short\n", []string{"-username", "root", "-email", "root@example.com"}, 1, "invalid password"},
		{"no password", "", []string{"-username", "root", "-email", "root@example.com"}, 1, "no password on standard input"},
		{"long input", strings.Repeat("p", maxPasswordInput+1), []string{"-username", "root", "-email", "root@example.com"}, 1, "too long"},
		{"bad email", adminPassword, []string{"-username", "root", "-email", "Root <root@example.com>"}, 1, "invalid email address"},
		{"bad username", adminPassword, []string{"-username", "no spaces", "-email", "root@example.com"}, 1, "invalid username"},
		{"missing email", adminPassword, []string{"-username", "root"}, 2, "-username and -email are required"},
		{"extra argument", adminPassword, []string{"-username", "root", "-email", "root@example.com", "x"}, 2, `unexpected argument "x"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runAdminCreate(t, tc.stdin, tc.args...)
			if code != tc.code || !strings.Contains(stderr, tc.want) {
				t.Errorf("exit code = %d, stderr = %q; want %d and %q", code, stderr, tc.code, tc.want)
			}
			if strings.Contains(stdout+stderr, strings.TrimSpace(tc.stdin)) && strings.TrimSpace(tc.stdin) != "" {
				t.Errorf("output echoes the password: stdout %q, stderr %q", stdout, stderr)
			}
		})
	}
}

func TestAdminUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"admin"}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "Usage: my admin create") {
		t.Errorf("my admin = %d, %q", code, stderr.String())
	}
	stderr.Reset()
	if code := runAdmin([]string{"nope"}, strings.NewReader(""), &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), `unknown subcommand "nope"`) {
		t.Errorf("my admin nope = %d, %q", code, stderr.String())
	}
}

func TestReadPasswordStripsOnlyTheLineEnd(t *testing.T) {
	for in, want := range map[string]string{
		"pass word \r\n":  "pass word ",
		"first\nsecond\n": "first",
		"no newline":      "no newline",
	} {
		got, err := readPassword(strings.NewReader(in), &bytes.Buffer{})
		if err != nil || got != want {
			t.Errorf("readPassword(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func assertNoSecret(t *testing.T, outputs ...string) {
	t.Helper()
	for _, o := range outputs {
		if strings.Contains(o, adminPassword) || strings.Contains(o, "argon2id") {
			t.Errorf("output reveals the password or its hash: %q", o)
		}
	}
}
