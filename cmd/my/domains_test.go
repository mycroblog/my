// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform/config"
)

// recordedDomains returns the row of instance_domains of the database at path, "" without one.
func recordedDomains(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var host, accountDomain string
	if err := db.QueryRowContext(context.Background(), `SELECT host, account_domain FROM instance_domains`).Scan(&host, &accountDomain); err != nil {
		return ""
	}
	return host + " " + accountDomain
}

func TestCommandsRecordTheDomainsOnFirstUse(t *testing.T) {
	path := newAdminEnv(t)
	t.Setenv(config.EnvPrefix+"ACCOUNT_DOMAIN", "example")
	// The commands that only check change nothing.
	if code, stdout, stderr := runCmd(t, dbCmd, "upgrade", "-check"); code != 0 {
		t.Fatalf("db upgrade -check = %d %q %q", code, stdout, stderr)
	}
	if got := recordedDomains(t, path); got != "" {
		t.Fatalf("after db upgrade -check, instance_domains = %q, want no row", got)
	}
	if code, stdout, stderr := runAdminCreate(t, adminPassword+"\n", "-username", "alice", "-email", "alice@social.example"); code != 0 {
		t.Fatalf("admin create = %d %q %q", code, stdout, stderr)
	}
	if got, want := recordedDomains(t, path), "social.example example"; got != want {
		t.Errorf("after admin create, instance_domains = %q, want %q", got, want)
	}
}

func TestCommandsRefuseOtherDomains(t *testing.T) {
	path := newAdminEnv(t)
	if code, stdout, stderr := runCmd(t, dbCmd, "upgrade"); code != 0 {
		t.Fatalf("db upgrade = %d %q %q", code, stdout, stderr)
	}
	t.Setenv(config.EnvPrefix+"HOST", "other.example")
	t.Setenv(config.EnvPrefix+"KEK", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	wants := []string{"host social.example", "host other.example", "cannot change"}
	checkStderr := func(name string, code int, stderr string) {
		t.Helper()
		if code != 1 {
			t.Errorf("%s = %d, want 1", name, code)
		}
		for _, want := range wants {
			if !strings.Contains(stderr, want) {
				t.Errorf("%s stderr = %q, want it to contain %q", name, stderr, want)
			}
		}
	}
	code, _, stderr := runAdminCreate(t, adminPassword+"\n", "-username", "bob", "-email", "bob@other.example")
	checkStderr("admin create", code, stderr)
	code, _, stderr = runCmd(t, dbCmd, "upgrade")
	checkStderr("db upgrade", code, stderr)
	code, _, stderr = runCmd(t, dbCmd, "upgrade", "-check")
	checkStderr("db upgrade -check", code, stderr)

	var stdout, errOut bytes.Buffer
	code = run([]string{"instance", "list"}, &stdout, &errOut)
	checkStderr("instance list", code, errOut.String())

	stdout.Reset()
	errOut.Reset()
	code = run([]string{"config", "check", "-instance"}, &stdout, &errOut)
	checkStderr("config check -instance", code, errOut.String())
	if strings.Contains(stdout.String(), "configuration OK") {
		t.Errorf("config check -instance stdout = %q, want no OK", stdout.String())
	}

	stdout.Reset()
	errOut.Reset()
	code = run([]string{"serve"}, &stdout, &errOut)
	checkStderr("serve", code, errOut.String())

	if got, want := recordedDomains(t, path), "social.example social.example"; got != want {
		t.Errorf("instance_domains = %q, want %q", got, want)
	}
}
