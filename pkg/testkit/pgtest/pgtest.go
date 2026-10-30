// SPDX-License-Identifier: AGPL-3.0-only

// Package pgtest gives tests a PostgreSQL database with the product schema applied (ADR-0041).
//
// One server serves every test of a test binary, and every test gets its own database, copied
// with CREATE DATABASE ... TEMPLATE from a template database that holds the bootstrap SQL and the
// product schema (see package schema for why executing the files matches an Atlas apply).
// Environment variables choose the server:
//
//   - TEST_POSTGRES_URL: an existing server, as a postgres:// URL whose user may create databases.
//   - TEST_POSTGRES: without TEST_POSTGRES_URL, "pgmem" (the default) runs pgmem, PostgreSQL in
//     memory inside the test process, served on a Unix socket so that the tests' connections
//     take no TCP port, and "container" starts a container with testcontainers-go.
//   - TEST_POSTGRES_IMAGE: the container image; it defaults to the one `scripts/task.sh db:up` uses.
//   - TEST_POSTGRES_REQUIRED: when non-empty, a container that cannot start because Docker is
//     unavailable fails the tests instead of skipping them. CI sets it.
//
// The server starts on the first NewDatabase call, so a test binary whose tests mostly use
// SQLite pays nothing for it until a PostgreSQL test runs. The TestMain of a package that uses
// pgtest calls os.Exit(pgtest.Main(m)) to stop the server at the end.
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shibukawa/pgmem"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mycroblog/my/schema"
)

// The environment variables that choose the server; see the package documentation.
const (
	EnvURL      = "TEST_POSTGRES_URL"
	EnvServer   = "TEST_POSTGRES"
	EnvImage    = "TEST_POSTGRES_IMAGE"
	EnvRequired = "TEST_POSTGRES_REQUIRED"
)

// The values of TEST_POSTGRES.
const (
	ServerPgmem     = "pgmem"
	ServerContainer = "container"
)

// DefaultImage is the container image of TEST_POSTGRES=container, the one `scripts/task.sh db:up` uses.
const DefaultImage = "postgres:18-alpine"

// server is the PostgreSQL server of this test binary and its template database.
var server struct {
	once     sync.Once
	url      string // empty when there is no server
	skip     string // why url is empty, when the tests skip
	err      error  // why url is empty, when the tests fail
	template string
	stop     func()
}

// Main runs the tests of m and then drops the template database and stops the server, if
// NewDatabase started one. A TestMain calls os.Exit(pgtest.Main(m)).
func Main(m *testing.M) int {
	code := m.Run()
	Close()
	return code
}

// Close drops the template database and stops the server, if NewDatabase started one. Main calls
// it; a TestMain that needs more than Main calls it after m.Run.
func Close() {
	if server.template != "" {
		ctx := context.Background()
		if c, err := pgx.Connect(ctx, server.url); err == nil {
			_, _ = c.Exec(ctx, "DROP DATABASE "+server.template+" WITH (FORCE)")
			_ = c.Close(ctx)
		}
		server.template = ""
	}
	if server.stop != nil {
		server.stop()
		server.stop = nil
	}
}

// NewDatabase creates a new database with the bootstrap SQL and the product schema applied and
// returns its URL. It skips t when no server is available and drops the database when t ends, so
// a caller that opens a pool on the URL registers its Close after this call.
func NewDatabase(t testing.TB) string {
	t.Helper()
	return newDatabase(t, true)
}

// NewEmptyDatabase is NewDatabase without the bootstrap SQL and the schema: a new database
// with no table and no extension, for the tests of applying the schema.
func NewEmptyDatabase(t testing.TB) string {
	t.Helper()
	return newDatabase(t, false)
}

func newDatabase(t testing.TB, withSchema bool) string {
	t.Helper()
	server.once.Do(setUp)
	if server.err != nil {
		t.Fatalf("pgtest: %v", server.err)
	}
	if server.url == "" {
		t.Skip(server.skip)
	}
	ctx := t.Context()
	admin, err := pgx.Connect(ctx, server.url)
	if err != nil {
		t.Fatalf("pgtest: connect: %v", err)
	}
	defer func() { _ = admin.Close(context.WithoutCancel(ctx)) }()

	name := randomName("test_")
	create := "CREATE DATABASE " + name
	if withSchema {
		create += " TEMPLATE " + server.template
	}
	// The backend of a connection that just left the template may still be exiting, and
	// PostgreSQL refuses to copy a database that has a session (55006 object_in_use).
	for attempt := 0; ; attempt++ {
		_, err = admin.Exec(ctx, create)
		var pgErr *pgconn.PgError
		if err == nil || attempt == 50 || !errors.As(err, &pgErr) || pgErr.Code != "55006" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("pgtest: create database: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		if c, err := pgx.Connect(ctx, server.url); err == nil {
			_, _ = c.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
			_ = c.Close(ctx)
		}
	})
	u, err := databaseURL(server.url, name)
	if err != nil {
		t.Fatalf("pgtest: %v", err)
	}
	return u
}

// setUp finds or starts the server and creates the template database.
func setUp() {
	s := &server
	switch u, kind := os.Getenv(EnvURL), os.Getenv(EnvServer); {
	case u != "":
		s.url = u
	case kind == "" || kind == ServerPgmem:
		s.err = startPgmem()
	case kind == ServerContainer:
		s.err = startContainer()
	default:
		s.err = fmt.Errorf("%s=%q: want %q or %q", EnvServer, kind, ServerPgmem, ServerContainer)
	}
	if s.err != nil || s.url == "" {
		return
	}
	s.template, s.err = createTemplate(context.Background())
}

// startPgmem starts pgmem and serves it on a Unix socket in a new temporary directory, so the
// connections of the tests take no loopback TCP port: ephemeral ports are few on some machines
// (4,096 under WSL2 mirrored networking) and every closed connection holds one for a minute in
// TIME_WAIT (CONTRIBUTING.md). pgmem itself still listens on one TCP port, which nothing uses.
func startPgmem() error {
	srv, err := pgmem.Start(context.Background(), pgmem.Options{Database: "my", User: "my"})
	if err != nil {
		return fmt.Errorf("start pgmem: %w", err)
	}
	dir, err := os.MkdirTemp("", "pgtest")
	if err != nil {
		_ = srv.Close()
		return fmt.Errorf("start pgmem: %w", err)
	}
	// The socket's name is the one pgx and libpq look for in the host directory on port 5432.
	ln, err := net.Listen("unix", filepath.Join(dir, ".s.PGSQL.5432"))
	if err != nil {
		_ = srv.Close()
		_ = os.RemoveAll(dir)
		return fmt.Errorf("start pgmem: %w", err)
	}
	go servePgmem(ln, srv)
	// A host that starts with "/" is the directory of a Unix socket.
	server.url = (&url.URL{
		Scheme:   "postgres",
		User:     url.User("my"),
		Path:     "/my",
		RawQuery: url.Values{"host": {dir}, "sslmode": {"disable"}}.Encode(),
	}).String()
	server.stop = func() {
		_ = ln.Close()
		_ = srv.Close()
		_ = os.RemoveAll(dir)
	}
	return nil
}

// servePgmem joins every connection accepted on ln to an in-process connection of srv, until ln
// is closed.
func servePgmem(ln net.Listener, srv *pgmem.Server) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer func() { _ = c.Close() }()
			s, err := srv.Dial(context.Background(), "", "")
			if err != nil {
				return
			}
			defer func() { _ = s.Close() }()
			done := make(chan struct{}, 2)
			go func() { _, _ = io.Copy(s, c); done <- struct{}{} }()
			go func() { _, _ = io.Copy(c, s); done <- struct{}{} }()
			<-done // one side closed; the deferred Closes end the other copy
		}()
	}
}

func startContainer() error {
	if err := exec.Command("docker", "info").Run(); err != nil {
		reason := fmt.Sprintf("%s=%s and Docker is unavailable (%v)", EnvServer, ServerContainer, err)
		if os.Getenv(EnvRequired) != "" {
			return fmt.Errorf("%s, but %s is set", reason, EnvRequired)
		}
		server.skip = reason
		return nil
	}
	image := os.Getenv(EnvImage)
	if image == "" {
		image = DefaultImage
	}
	ctx := context.Background()
	c, err := tcpostgres.Run(ctx, image,
		tcpostgres.WithDatabase("my"), tcpostgres.WithUsername("my"), tcpostgres.WithPassword("my"),
		tcpostgres.BasicWaitStrategies(),
	)
	if c != nil {
		server.stop = func() { _ = testcontainers.TerminateContainer(c) }
	}
	if err != nil {
		return fmt.Errorf("start container %s: %w", image, err)
	}
	server.url, err = c.ConnectionString(ctx, "sslmode=disable")
	return err
}

// createTemplate creates the template database and applies the schema to it.
func createTemplate(ctx context.Context) (string, error) {
	name := randomName("test_template_")
	admin, err := pgx.Connect(ctx, server.url)
	if err != nil {
		return "", fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = admin.Close(ctx) }()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		return "", fmt.Errorf("create template database: %w", err)
	}
	u, err := databaseURL(server.url, name)
	if err != nil {
		return "", err
	}
	conn, err := pgx.Connect(ctx, u)
	if err != nil {
		return "", fmt.Errorf("connect to the template: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	for _, ddl := range []string{schema.PostgresBootstrap, schema.Postgres} {
		if _, err := conn.Exec(ctx, ddl); err != nil {
			return "", fmt.Errorf("apply schema to the template: %w", err)
		}
	}
	return name, nil
}

// databaseURL returns serverURL with its database replaced by name.
func databaseURL(serverURL, name string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("the server URL must be a postgres:// URL: %w", err)
	}
	u.Path = "/" + name
	return u.String(), nil
}

func randomName(prefix string) string {
	suffix := make([]byte, 8)
	_, _ = rand.Read(suffix)
	return prefix + hex.EncodeToString(suffix)
}
