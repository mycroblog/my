// SPDX-License-Identifier: AGPL-3.0-only

// Package platformtest runs the whole server inside a test process, for the tests that drive it
// over HTTP as a client would: the integration tests of the client libraries
// (pkg/mykit/integration, ADR-0093 §8). It lives beside the composition root because it builds
// the App, which pkg/testkit may not import (ADR-0002).
//
// Start builds the App on a new SQLite database with the schema applied and serves it with
// platform.Serve on a free local port, so the job runner, the scheduler, and the streams run as in
// production. The server's users and tokens come from the application services: User creates one,
// and AuthorizationCode stands in for the sign-in of the authorization page, whose code the client
// then exchanges at the token endpoint.
package platformtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for applying the schema

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/schema"
)

// WebOrigin is the origin of the web client that Start configures; the authorization codes of
// AuthorizationCode are for its redirect URI.
const WebOrigin = "http://localhost:5173"

// Options configure the server that Start runs.
type Options struct {
	// Env adds or overrides settings (MY_* environment variables) of the configuration. MY_HOST,
	// MY_KEK, MY_DATABASE_URL, MY_BLOB_FS_DIR, and MY_WEB_ORIGIN are filled in.
	Env map[string]string
	// AccountLimit, when its Max is set, replaces the client API's per-account rate limit.
	AccountLimit kernel.Limit
	// Logger receives the server's logs; nil discards them.
	Logger *slog.Logger
}

// Server is a server that Start runs until the test ends.
type Server struct {
	// App is the composition root, whose services set up what the API cannot (users, codes).
	App *platform.App
	// URL is the origin that the server listens on, such as "http://127.0.0.1:41234".
	URL string
}

// Start runs a server for the test, on a database of its own, and stops it when the test ends.
func Start(t testing.TB, opts Options) *Server {
	t.Helper()
	dir := t.TempDir()
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	env := map[string]string{
		"MY_HOST": "example.test", "MY_KEK": base64.StdEncoding.EncodeToString(kek),
		"MY_DATABASE_URL": newDatabase(t, dir), "MY_BLOB_FS_DIR": filepath.Join(dir, "blobs"), "MY_WEB_ORIGIN": WebOrigin,
	}
	for k, v := range opts.Env {
		env[k] = v
	}
	cfg, _, err := config.Load(config.Source{LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok }})
	if err != nil {
		t.Fatalf("platformtest: configuration: %v", err)
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	app, err := platform.NewApp(context.Background(), cfg, platform.AppOptions{
		Logger: logger, Version: "0.0.0-test", ClientAPIAccountLimit: opts.AccountLimit,
	})
	if err != nil {
		t.Fatalf("platformtest: NewApp: %v", err)
	}
	// Serve ensures them too, but concurrently with the test's first requests: User needs them.
	if err := app.Identity.EnsureBuiltinRoles(context.Background()); err != nil {
		_ = app.Close()
		t.Fatalf("platformtest: built-in roles: %v", err)
	}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		_ = app.Close()
		t.Fatalf("platformtest: listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- platform.Serve(ctx, app, platform.ServeOptions{Listener: ln, ShutdownTimeout: 5 * time.Second})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("platformtest: Serve: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("platformtest: Serve did not return")
		}
		_ = app.Close()
	})
	return &Server{App: app, URL: "http://" + ln.Addr().String()}
}

// User is a user of the server with the local account it signs in as.
type User struct {
	UserID, AccountID, Username string
}

// User creates a user called username, with a verified email address. The user holds the
// built-in admin role, as the first user of an instance does.
func (s *Server) User(t testing.TB, username string) User {
	t.Helper()
	reg, err := s.App.Identity.CreateAdmin(context.Background(), identity.CreateAdminInput{
		Username: username, Email: username + "@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("platformtest: create user %s: %v", username, err)
	}
	return User{UserID: reg.User.ID, AccountID: reg.AccountID, Username: username}
}

// WebClient is the registration of the first-party web client, a public client, that
// AuthorizationCode issues codes to.
const (
	WebClientID          = identity.DefaultWebClientID
	WebClientRedirectURI = WebOrigin + platform.WebClientCallbackPath
)

// AuthorizationCode returns the code that the authorization page would send to the web
// client's redirect URI after the user signed in and allowed scope ("" for every scope of the
// client), for the S256 code_challenge of the client's verifier. The client exchanges it at
// the token endpoint (POST /oauth/token) with WebClientID and WebClientRedirectURI.
func (s *Server) AuthorizationCode(t testing.TB, u User, scope, codeChallenge string) string {
	t.Helper()
	ctx := context.Background()
	a, err := s.App.Identity.ValidateAuthorization(ctx, identity.AuthorizationRequest{
		ResponseType: "code", ClientID: WebClientID, RedirectURI: WebClientRedirectURI, Scope: scope,
		CodeChallenge: codeChallenge, CodeChallengeMethod: domain.CodeChallengeMethodS256,
	})
	if err != nil {
		t.Fatalf("platformtest: authorization request for %s: %v", u.Username, err)
	}
	code, err := s.App.Identity.IssueAuthorizationCode(ctx, a, u.UserID)
	if err != nil {
		t.Fatalf("platformtest: authorization code for %s: %v", u.Username, err)
	}
	return code
}

// sqliteTemplate is the SQLite database file with the schema applied that newDatabase copies,
// created once per test binary: applying the schema costs more than most tests.
var sqliteTemplate struct {
	once sync.Once
	b    []byte
	err  error
}

func createSQLiteTemplate() {
	s := &sqliteTemplate
	dir, err := os.MkdirTemp("", "platformtest-sqlite-")
	if err != nil {
		s.err = err
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "my.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		s.err = err
		return
	}
	if _, err := db.ExecContext(context.Background(), schema.SQLite); err != nil {
		_ = db.Close()
		s.err = fmt.Errorf("apply schema: %w", err)
		return
	}
	if err := db.Close(); err != nil {
		s.err = err
		return
	}
	s.b, s.err = os.ReadFile(path) //nolint:gosec // G304: the file this function just created
}

// newDatabase writes a copy of the template into dir and returns its path.
func newDatabase(t testing.TB, dir string) string {
	t.Helper()
	sqliteTemplate.once.Do(createSQLiteTemplate)
	if sqliteTemplate.err != nil {
		t.Fatalf("platformtest: SQLite template: %v", sqliteTemplate.err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile("my.db", sqliteTemplate.b, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "my.db")
}
