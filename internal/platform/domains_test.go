// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/schema"
)

func newDomainsApp(t *testing.T, path, host, accountDomain string, opts platform.AppOptions) error {
	t.Helper()
	env := map[string]string{"MY_DATABASE_URL": path, "MY_HOST": host}
	if accountDomain != "" {
		env["MY_ACCOUNT_DOMAIN"] = accountDomain
	}
	opts.Logger = slog.New(slog.DiscardHandler)
	app, err := platform.NewApp(t.Context(), loadConfig(t, env), opts)
	if err != nil {
		return err
	}
	return app.Close()
}

func TestNewAppKeepsTheDomainsOfTheDatabase(t *testing.T) {
	t.Parallel()
	path := newDatabase(t, true)
	if err := newDomainsApp(t, path, "social.example.org", "example.org", platform.AppOptions{}); err != nil {
		t.Fatalf("first start = %v", err)
	}
	if err := newDomainsApp(t, path, "social.example.org", "example.org", platform.AppOptions{}); err != nil {
		t.Fatalf("second start = %v", err)
	}
	for _, tc := range []struct{ name, host, accountDomain string }{
		{"another host", "other.example.org", "example.org"},
		{"another account_domain", "social.example.org", "other.example.org"},
		// Without account_domain it defaults to host, which is a change here.
		{"account_domain dropped", "social.example.org", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := newDomainsApp(t, path, tc.host, tc.accountDomain, platform.AppOptions{})
			var mismatch *instance.DomainMismatchError
			if !errors.As(err, &mismatch) {
				t.Fatalf("NewApp = %v, want a DomainMismatchError", err)
			}
			for _, want := range []string{"host social.example.org", "account_domain example.org", "sets host " + tc.host} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %s", err, want)
				}
			}
		})
	}
}

func TestNewAppCheckDomainsOnlyRecordsNothing(t *testing.T) {
	t.Parallel()
	path := newDatabase(t, true)
	check := platform.AppOptions{CheckDomainsOnly: true}
	if err := newDomainsApp(t, path, "first.example.org", "", check); err != nil {
		t.Fatalf("check on a new database = %v", err)
	}
	if err := newDomainsApp(t, path, "second.example.org", "", platform.AppOptions{}); err != nil {
		t.Fatalf("start after the check = %v, want the check to have recorded nothing", err)
	}
	var mismatch *instance.DomainMismatchError
	if err := newDomainsApp(t, path, "first.example.org", "", check); !errors.As(err, &mismatch) {
		t.Errorf("check of other domains = %v, want a DomainMismatchError", err)
	}
}

// applySchema applies the schema to the database at path while the server runs, and runs then in
// the same transaction, so that the server sees both or neither.
func applySchema(t *testing.T, path, then string) {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(10000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(t.Context(), "BEGIN IMMEDIATE;\n"+schema.SQLite+"\n"+then+"\nCOMMIT;"); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
}

func TestServeRecordsTheDomainsOnceTheSchemaIsApplied(t *testing.T) {
	t.Parallel()
	path := newDatabase(t, false)
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": path})
	s := start(t, cfg, platform.ServeOptions{DomainsRetry: 10 * time.Millisecond}, nil)
	waitServing(t, s)
	if resp, _ := s.do(t, http.MethodGet, platform.PathReadiness, nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readiness without the schema = %d, want 503", resp.StatusCode)
	}
	applySchema(t, path, "")
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, body := s.do(t, http.MethodGet, platform.PathReadiness, nil)
		if resp.StatusCode == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("readiness after the schema = %d %q, want 200", resp.StatusCode, body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	var mismatch *instance.DomainMismatchError
	if err := newDomainsApp(t, path, "other.example.org", "", platform.AppOptions{}); !errors.As(err, &mismatch) {
		t.Errorf("NewApp of another host = %v, want a DomainMismatchError", err)
	}
}

// A process that could not check its domains at the start stops when it finds that they differ.
func TestServeStopsWhenTheDomainsDifferLater(t *testing.T) {
	t.Parallel()
	path := newDatabase(t, false)
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": path})
	s := start(t, cfg, platform.ServeOptions{DomainsRetry: 10 * time.Millisecond}, nil)
	waitServing(t, s)
	applySchema(t, path, `INSERT INTO instance_domains (id, host, account_domain, recorded_at) VALUES (1, 'other.example.org', 'other.example.org', 0);`)
	select {
	case err := <-s.done:
		s.done <- err // for the cleanup of startApp
		var mismatch *instance.DomainMismatchError
		if !errors.As(err, &mismatch) {
			t.Errorf("Serve = %v, want a DomainMismatchError", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not stop")
	}
}
