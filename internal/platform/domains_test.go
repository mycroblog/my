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
	"github.com/mycroblog/my/pkg/testkit"
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
	upgrade(t, s.app)
	var resp *http.Response
	var body string
	ready := testkit.Eventually(t, 10*time.Second, func() bool {
		resp, body = s.do(t, http.MethodGet, platform.PathReadiness, nil)
		return resp.StatusCode == http.StatusOK
	})
	if !ready {
		t.Fatalf("readiness after the schema = %d %q, want 200", resp.StatusCode, body)
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

// A process that could not check its domains serves only the health checks, and runs no jobs, until
// it has.
func TestServeHoldsEverythingButHealthChecksUntilTheDomainsAreChecked(t *testing.T) {
	t.Parallel()
	path := newDatabase(t, false)
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": path})
	s := start(t, cfg, platform.ServeOptions{DomainsRetry: 10 * time.Millisecond}, nil)
	waitServing(t, s)
	for _, target := range []string{"/", "/api/v1/instance", "/.well-known/webfinger?resource=acct:alice@example.test", "/ap/actors/x"} {
		resp, _ := s.do(t, http.MethodGet, target, nil)
		if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Retry-After") == "" {
			t.Errorf("GET %s = %d, Retry-After %q; want 503 and the header", target, resp.StatusCode, resp.Header.Get("Retry-After"))
		}
	}
	if resp, _ := s.do(t, http.MethodPost, "/inbox", nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("POST /inbox = %d, want 503", resp.StatusCode)
	}
	if resp, _ := s.do(t, http.MethodGet, platform.PathLiveness, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("liveness = %d, want 200", resp.StatusCode)
	}
	// The job runner starts once the domains are checked, which needs the schema: its start, and any
	// line of a runner (one on a database without tables would say so), come after the schema.
	before := len(s.logs.String())
	applySchema(t, path, "")
	served := testkit.Eventually(t, 10*time.Second, func() bool {
		resp, _ := s.do(t, http.MethodGet, "/api/v1/instance", nil)
		return resp.StatusCode != http.StatusServiceUnavailable
	})
	if !served {
		t.Fatal("still 503 after the schema was applied")
	}
	const started = `"msg":"job runner started"`
	if !testkit.Eventually(t, 10*time.Second, func() bool { return strings.Contains(s.logs.String(), started) }) {
		t.Fatalf("the job runner did not start once the domains were checked:\n%s", s.logs.String())
	}
	logs := s.logs.String()
	if i := strings.Index(logs, `"msg":"job runner`); i < before || i != strings.Index(logs, started) {
		t.Errorf("a job runner logged before the domains were checked:\n%s", logs)
	}
}
