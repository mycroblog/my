// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib" // the pgx driver of database/sql, for the plans and the job queue
	_ "modernc.org/sqlite"             // the SQLite driver of database/sql
)

// stopGrace is how long a server has to end after SIGTERM before it is killed.
const stopGrace = time.Minute

// database is the database of one run: a SQLite file or a PostgreSQL database of its own.
type database struct {
	kind string // sqlite or postgres
	url  string // MY_DATABASE_URL
	drop func(context.Context) error
}

// newDatabase makes the database of a run on kind in dir, or on the PostgreSQL server of admin.
func newDatabase(ctx context.Context, kind, dir, admin string) (*database, error) {
	switch kind {
	case "sqlite":
		return &database{kind: kind, url: filepath.Join(dir, "my.db"), drop: func(context.Context) error { return nil }}, nil
	case "postgres":
		u, err := url.Parse(admin)
		if err != nil {
			return nil, fmt.Errorf("-postgres: %w", err)
		}
		name := "mybench_" + randomHex(6)
		conn, err := pgx.Connect(ctx, admin)
		if err != nil {
			return nil, fmt.Errorf("connect to %s: %w", u.Redacted(), err)
		}
		defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
		if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
			return nil, fmt.Errorf("create database %s: %w", name, err)
		}
		u.Path = "/" + name
		return &database{kind: kind, url: u.String(), drop: func(ctx context.Context) error {
			conn, err := pgx.Connect(ctx, admin)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close(ctx) }()
			_, err = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			return err
		}}, nil
	}
	return nil, fmt.Errorf("-db: unknown database %q (sqlite or postgres)", kind)
}

// open opens the run's database for the benchmarker's own reads: the job queue and the plans.
func (d *database) open() (*sql.DB, error) {
	if d.kind == "postgres" {
		return sql.Open("pgx", d.url)
	}
	return sql.Open("sqlite", "file:"+d.url+"?mode=ro&_pragma=busy_timeout(5000)")
}

// server is one `my serve` process of a run and the settings that `my` commands share with it.
type server struct {
	bin, dir string
	url      string // the origin of the API's listener
	web      string // the web origin (web_origin), which the sign-in's redirect names
	pprof    string // the origin of the profiler's listener
	env      []string
	cmd      *exec.Cmd
	done     chan error
	log      *os.File
}

// newServer returns the settings of a server of bin on db in dir, not started. Its settings
// environment holds none of the caller's MY_* or OTEL_*.
func newServer(bin, dir string, db *database) (*server, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	pprofPort, err := freePort()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "blobs"), 0o750); err != nil {
		return nil, err
	}
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	if err := os.WriteFile(filepath.Join(dir, "kek"), []byte(base64.StdEncoding.EncodeToString(kek)), 0o600); err != nil {
		return nil, err
	}
	host := fmt.Sprintf("localhost:%d", port)
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "MY_") && !strings.HasPrefix(kv, "OTEL_") {
			env = append(env, kv)
		}
	}
	env = append(env,
		"MY_PROFILE=production",
		"MY_HOST="+host,
		fmt.Sprintf("MY_LISTEN=127.0.0.1:%d", port),
		"MY_WEB_ORIGIN=http://"+host,
		"MY_DATABASE="+db.kind,
		"MY_DATABASE_URL="+db.url,
		"MY_BLOB=fs",
		"MY_BLOB_FS_DIR="+filepath.Join(dir, "blobs"),
		"MY_KEK_FILE="+filepath.Join(dir, "kek"),
		// Each person comes from an address of their own (X-Forwarded-For from this machine), so
		// that the people do not share one sign-in limit.
		"MY_TRUSTED_PROXIES=127.0.0.1",
		fmt.Sprintf("MY_PPROF_LISTEN=127.0.0.1:%d", pprofPort),
	)
	return &server{
		bin: bin, dir: dir, env: env, web: "http://" + host,
		url: fmt.Sprintf("http://127.0.0.1:%d", port), pprof: fmt.Sprintf("http://127.0.0.1:%d", pprofPort),
	}, nil
}

// my runs `my args...` with input on its standard input under the server's settings.
func (s *server) my(ctx context.Context, input string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, s.bin, args...) //nolint:gosec // the server binary of -my or of the run's build
	cmd.Dir, cmd.Env = s.dir, s.env
	cmd.Stdin = strings.NewReader(input)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("my %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

// start starts `my serve` with extra settings, under taskset when cores names some, logging to
// logName in the directory, and returns once it answers /readyz.
func (s *server) start(ctx context.Context, cores, logName string, extra ...string) error {
	if s.cmd != nil {
		return errors.New("bench: the server is running already")
	}
	log, err := os.Create(filepath.Join(s.dir, logName)) //nolint:gosec // a file of the run's own directory
	if err != nil {
		return err
	}
	name, args := s.bin, []string{"serve"}
	if cores != "" {
		name, args = "taskset", append([]string{"-c", cores, s.bin}, args...)
	}
	// Not CommandContext: the server is stopped by stop, with its grace, also when ctx ends.
	cmd := exec.Command(name, args...) //nolint:gosec // the server binary of -my or of the run's build, under taskset
	cmd.Dir, cmd.Env = s.dir, append(append([]string(nil), s.env...), extra...)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = log.Close()
		return fmt.Errorf("start my serve: %w", err)
	}
	s.cmd, s.log, s.done = cmd, log, make(chan error, 1)
	go func() { s.done <- cmd.Wait() }()
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		select {
		case err := <-s.done:
			s.done <- err
			return fmt.Errorf("my serve exited before it was ready (%w); see %s", err, log.Name())
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		if ready(ctx, s.url) {
			return nil
		}
	}
	return fmt.Errorf("my serve did not answer /readyz within a minute; see %s", log.Name())
}

func ready(ctx context.Context, origin string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/readyz", nil)
	resp, err := local.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// stop ends the server: SIGTERM to its process group, SIGKILL after stopGrace, and waits for the
// process to end, so that the next server never starts beside it.
func (s *server) stop() error {
	if s.cmd == nil {
		return nil
	}
	defer func() { s.cmd, s.done = nil, nil; _ = s.log.Close() }()
	pgid := s.cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	select {
	case <-s.done:
		return nil
	case <-time.After(stopGrace):
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	<-s.done
	return fmt.Errorf("my serve did not end within %s of SIGTERM and was killed; see %s", stopGrace, s.log.Name())
}

// local sends the benchmarker's own requests to the server under test and its profiler, on
// loopback: no outbound request of the server's (kernel.HTTPClient guards those).
var local = &http.Client{}

// freePort returns a TCP port on loopback that nothing listens on now.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// drainJobs waits until the job queue of db has no job due, so that a measurement does not share
// the server with the work that the seed queued.
func drainJobs(ctx context.Context, db *database, timeout time.Duration) error {
	conn, err := db.open()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	q := "SELECT COUNT(*) FROM jobs WHERE run_at <= ?"
	if db.kind == "postgres" {
		q = "SELECT COUNT(*) FROM jobs WHERE run_at <= $1"
	}
	deadline := time.Now().Add(timeout)
	for {
		var due int
		now := any(time.Now())
		if db.kind == "sqlite" {
			now = time.Now().UnixMilli()
		}
		if err := conn.QueryRowContext(ctx, q, now).Scan(&due); err != nil {
			return fmt.Errorf("count the due jobs: %w", err)
		}
		if due == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%d jobs still due after %s", due, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
