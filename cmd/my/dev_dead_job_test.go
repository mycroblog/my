// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package main

import (
	"bytes"
	"database/sql"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform/config"
)

func TestE2EFakesDevDeadJobEnqueuesAJobItsHandlerCannotRead(t *testing.T) {
	path := newAdminEnv(t)
	t.Setenv(config.EnvPrefix+"PROFILE", "development")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"dev", "dead-job"}, &stdout, &stderr); code != 0 {
		t.Fatalf("my dev dead-job = %d, %q", code, stderr.String())
	}
	key := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(key, "dev.dead-job:") {
		t.Fatalf("stdout = %q, want the job's idempotency key", stdout.String())
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	var typ, payload string
	if err := raw.QueryRowContext(t.Context(), `SELECT type, CAST(payload AS TEXT) FROM jobs WHERE idempotency_key = ?`, key).Scan(&typ, &payload); err != nil {
		t.Fatal(err)
	}
	if typ != "notification.post_subscribers" || payload != `"not a post"` {
		t.Errorf("the job = %q %q, want notification.post_subscribers with a string for a payload", typ, payload)
	}
}

func TestE2EFakesDevDeadJobRefusesProduction(t *testing.T) {
	newAdminEnv(t)
	t.Setenv(config.EnvPrefix+"PROFILE", "production")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"dev", "dead-job"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "needs profile = development") {
		t.Errorf("my dev dead-job = %d, %q, want 1 and the profile it needs", code, stderr.String())
	}
}
