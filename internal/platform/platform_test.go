// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for applying the schema

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/schema"
)

// syncBuffer is a bytes.Buffer safe for the concurrent writes of a logger.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// sqliteTemplate is the SQLite database file with the schema applied that newDatabase copies,
// created once per test binary: applying the schema commits every statement on its own, which
// costs more than the rest of most tests. TestMain removes its directory.
var sqliteTemplate struct {
	once sync.Once
	dir  string
	path string
	err  error
}

func createSQLiteTemplate() {
	s := &sqliteTemplate
	if s.dir, s.err = os.MkdirTemp("", "platform-sqlite-template-"); s.err != nil {
		return
	}
	s.path = filepath.Join(s.dir, "my.db")
	db, err := sql.Open("sqlite", s.path)
	if err != nil {
		s.err = err
		return
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(context.Background(), schema.SQLite); err != nil {
		s.err = fmt.Errorf("apply schema: %w", err)
	}
}

// removeSQLiteTemplate removes the template that newDatabase created, if any.
func removeSQLiteTemplate() {
	if sqliteTemplate.dir != "" {
		_ = os.RemoveAll(sqliteTemplate.dir)
	}
}

// newDatabase returns the path of a new SQLite database file, a copy of the template with the
// schema applied when applied is true, and a path to no file yet otherwise.
func newDatabase(t *testing.T, applied bool) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "my.db")
	if !applied {
		return path
	}
	sqliteTemplate.once.Do(createSQLiteTemplate)
	if sqliteTemplate.err != nil {
		t.Fatalf("SQLite template: %v", sqliteTemplate.err)
	}
	b, err := os.ReadFile(sqliteTemplate.path)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile("my.db", b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// loadConfig loads a configuration from env alone, with the required settings filled in.
func loadConfig(t *testing.T, env map[string]string) *config.Config {
	t.Helper()
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	full := map[string]string{
		"MY_HOST": "example.test", "MY_KEK": base64.StdEncoding.EncodeToString(kek), "MY_BLOB_FS_DIR": t.TempDir(),
	}
	for k, v := range env {
		full[k] = v
	}
	cfg, _, err := config.Load(config.Source{
		LookupEnv: func(k string) (string, bool) { v, ok := full[k]; return v, ok },
	})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

type server struct {
	app  *platform.App
	url  string
	logs *syncBuffer
	stop context.CancelFunc
	done chan error
}

// start builds the App for cfg and serves it on a free local port. setup runs before Serve.
func start(t *testing.T, cfg *config.Config, opts platform.ServeOptions, setup func(*platform.App)) *server {
	t.Helper()
	return startApp(t, cfg, platform.AppOptions{}, opts, setup)
}

// startApp is start with AppOptions; their Logger is replaced by one that the server keeps.
func startApp(t *testing.T, cfg *config.Config, appOpts platform.AppOptions, opts platform.ServeOptions, setup func(*platform.App)) *server {
	t.Helper()
	logs := &syncBuffer{}
	appOpts.Logger = platform.NewLogger(logs, slog.LevelDebug)
	app, err := platform.NewApp(t.Context(), cfg, appOpts)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if setup != nil {
		setup(app)
	}
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	opts.Listener = ln
	ctx, cancel := context.WithCancel(context.Background())
	s := &server{app: app, url: "http://" + ln.Addr().String(), logs: logs, stop: cancel, done: make(chan error, 1)}
	go func() { s.done <- platform.Serve(ctx, app, opts) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-s.done:
		case <-time.After(10 * time.Second):
			t.Error("Serve did not return")
		}
	})
	return s
}

func (s *server) do(t *testing.T, method, path string, header map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, s.url+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

type problemDoc struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	RequestID string `json:"request_id"`
}

func decodeProblem(t *testing.T, resp *http.Response, body string) problemDoc {
	t.Helper()
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	var p problemDoc
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("problem body %q: %v", body, err)
	}
	if p.Type != "about:blank" || p.Status != resp.StatusCode || p.Title == "" {
		t.Errorf("problem = %+v, want type about:blank, status %d, and a title", p, resp.StatusCode)
	}
	if p.RequestID == "" || p.RequestID != resp.Header.Get(platform.HeaderRequestID) {
		t.Errorf("problem request_id = %q, want the response's %s %q", p.RequestID, platform.HeaderRequestID, resp.Header.Get(platform.HeaderRequestID))
	}
	return p
}

func TestServeSQLite(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	s := start(t, cfg, platform.ServeOptions{}, nil)

	for _, path := range []string{platform.PathLiveness, platform.PathReadiness} {
		resp, body := s.do(t, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusOK || body != "{\"status\":\"ok\"}\n" {
			t.Errorf("GET %s = %d %q, want 200 {\"status\":\"ok\"}", path, resp.StatusCode, body)
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s: Cache-Control = %q, want no-store", path, got)
		}
	}

	t.Run("security headers", func(t *testing.T) {
		resp, _ := s.do(t, http.MethodGet, platform.PathLiveness, nil)
		for k, want := range map[string]string{
			"Strict-Transport-Security":    "max-age=63072000; includeSubDomains",
			"X-Content-Type-Options":       "nosniff",
			"Referrer-Policy":              "strict-origin-when-cross-origin",
			"Permissions-Policy":           "camera=(), microphone=(), geolocation=(), payment=(), usb=()",
			"Cross-Origin-Opener-Policy":   "same-origin",
			"Cross-Origin-Resource-Policy": "same-site",
			"X-Frame-Options":              "DENY",
			"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'; base-uri 'none'",
		} {
			if got := resp.Header.Get(k); got != want {
				t.Errorf("%s = %q, want %q", k, got, want)
			}
		}
	})

	t.Run("request IDs", func(t *testing.T) {
		resp, _ := s.do(t, http.MethodGet, platform.PathLiveness, map[string]string{platform.HeaderRequestID: "lb-1234"})
		if got := resp.Header.Get(platform.HeaderRequestID); got != "lb-1234" {
			t.Errorf("kept request ID = %q, want lb-1234", got)
		}
		resp, _ = s.do(t, http.MethodGet, platform.PathLiveness, map[string]string{platform.HeaderRequestID: "bad id <script>"})
		if got := resp.Header.Get(platform.HeaderRequestID); len(got) != 36 {
			t.Errorf("replacement request ID = %q, want a UUID", got)
		}
		if !strings.Contains(s.logs.String(), `"request_id":"lb-1234"`) {
			t.Errorf("access log lacks the request ID:\n%s", s.logs.String())
		}
	})

	t.Run("not found is a localized problem", func(t *testing.T) {
		resp, body := s.do(t, http.MethodGet, "/ap/nope", map[string]string{"Accept-Language": "ja-JP,en;q=0.5"})
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
		p := decodeProblem(t, resp, body)
		if resp.Header.Get("Content-Language") != "ja" || p.Title == "" || p.Title == "The requested resource was not found." {
			t.Errorf("Content-Language = %q, title = %q; want a Japanese title", resp.Header.Get("Content-Language"), p.Title)
		}
		resp, body = s.do(t, http.MethodGet, "/ap/nope", nil)
		if p := decodeProblem(t, resp, body); p.Title != "The requested resource was not found." || resp.Header.Get("Content-Language") != "en" {
			t.Errorf("default title = %q (%s), want the English message", p.Title, resp.Header.Get("Content-Language"))
		}
	})

	t.Run("method not allowed is a problem", func(t *testing.T) {
		resp, body := s.do(t, http.MethodPost, platform.PathLiveness, nil)
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", resp.StatusCode)
		}
		if p := decodeProblem(t, resp, body); !strings.Contains(p.Title, "POST") {
			t.Errorf("title = %q, want it to name the method", p.Title)
		}
		if allow := resp.Header.Get("Allow"); !strings.Contains(allow, "GET") {
			t.Errorf("Allow = %q, want GET", allow)
		}
	})

	s.stop()
	if err := <-s.done; err != nil {
		t.Errorf("Serve = %v, want nil after a graceful shutdown", err)
	}
	s.done <- nil // for the cleanup
}

func TestReadinessWithoutSchema(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, false)})
	s := start(t, cfg, platform.ServeOptions{}, nil)

	if resp, _ := s.do(t, http.MethodGet, platform.PathLiveness, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("liveness = %d, want 200 without the schema", resp.StatusCode)
	}
	resp, body := s.do(t, http.MethodGet, platform.PathReadiness, map[string]string{"Accept-Language": "ja"})
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readiness = %d %q, want 503", resp.StatusCode, body)
	}
	if p := decodeProblem(t, resp, body); strings.Contains(p.Title, "jobs") {
		t.Errorf("title %q reveals internal details", p.Title)
	}
	if !strings.Contains(s.logs.String(), "schema not applied") {
		t.Errorf("the readiness failure is not logged:\n%s", s.logs.String())
	}
}

func TestWorkerRoleServesOnlyHealthChecks(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_ROLE": "worker"})
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		a.Handle("GET /web", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	})
	if resp, _ := s.do(t, http.MethodGet, platform.PathReadiness, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("readiness = %d, want 200", resp.StatusCode)
	}
	if resp, _ := s.do(t, http.MethodGet, "/web", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("web route on the worker role = %d, want 404", resp.StatusCode)
	}
}

func TestPanicIsAProblem(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_ROLE": "web"})
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		a.Handle("GET /panic/{id}", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom at https://remote.example/x") }))
		a.Handle("GET /missing", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	})
	for _, path := range []string{"/panic/1", "/panic/2"} {
		resp, body := s.do(t, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", resp.StatusCode)
		}
		if p := decodeProblem(t, resp, body); strings.Contains(p.Title, "boom") {
			t.Errorf("title %q reveals the panic", p.Title)
		}
	}
	for _, path := range []string{"/missing", "/panic/1?x=%ff"} {
		if resp, _ := s.do(t, http.MethodGet, path, nil); resp.StatusCode < 400 || resp.StatusCode >= 500 {
			t.Errorf("GET %s = %d, want a 4xx", path, resp.StatusCode)
		}
	}
	// One scrubbed report line for each panic, with the same fingerprint, and none for a 4xx.
	reports := errorReportLines(t, s.logs.String())
	if len(reports) != 2 {
		t.Fatalf("%d report lines, want 2:\n%s", len(reports), s.logs.String())
	}
	for _, rep := range reports {
		msg, _ := rep["message"].(string)
		if rep["my.error.kind"] != "panic" || rep["my.error.where"] != "web GET /panic/{id}" || rep["severity"] != "ERROR" ||
			!strings.HasPrefix(msg, "string: boom at <url>\ngoroutine ") || rep["request_id"] == nil {
			t.Errorf("report = %v", rep)
		}
	}
	if reports[0]["my.error.fingerprint"] != reports[1]["my.error.fingerprint"] {
		t.Errorf("fingerprints %v and %v differ", reports[0]["my.error.fingerprint"], reports[1]["my.error.fingerprint"])
	}
}

// errorReportLines returns the error report lines of logs.
func errorReportLines(t *testing.T, logs string) []map[string]any {
	t.Helper()
	var reports []map[string]any
	for line := range strings.Lines(logs) {
		if !strings.Contains(line, `"msg":"error report"`) {
			continue
		}
		var rep map[string]any
		if err := json.Unmarshal([]byte(line), &rep); err != nil {
			t.Fatalf("%v: %s", err, line)
		}
		reports = append(reports, rep)
	}
	return reports
}

// A dead job writes one scrubbed report line, with the same fingerprint for each such job, and
// a job that only retries writes none.
func TestDeadJobIsReported(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		a.Handlers.Register("test.dead", func(context.Context, queue.Job) queue.Result {
			return queue.Fail(errors.New("mail to bob@example.com refused"))
		})
		a.Handlers.Register("test.retry", func(context.Context, queue.Job) queue.Result {
			return queue.Retry(time.Hour, errors.New("try later"))
		})
	})
	for _, typ := range []string{"test.retry", "test.dead", "test.dead"} {
		if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: typ}); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && strings.Count(s.logs.String(), `"my.outcome":"dead"`) < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	reports := errorReportLines(t, s.logs.String())
	if len(reports) != 2 {
		t.Fatalf("%d report lines, want 2:\n%s", len(reports), s.logs.String())
	}
	for _, rep := range reports {
		msg, _ := rep["message"].(string)
		if rep["my.error.kind"] != "error" || rep["my.error.where"] != "test.dead" || rep["job_id"] == nil ||
			!strings.HasPrefix(msg, "*errors.errorString: mail to <email> refused\ngoroutine ") {
			t.Errorf("report = %v", rep)
		}
	}
	if reports[0]["my.error.fingerprint"] != reports[1]["my.error.fingerprint"] {
		t.Errorf("fingerprints %v and %v differ", reports[0]["my.error.fingerprint"], reports[1]["my.error.fingerprint"])
	}
}

// Shutdown waits for the request and the job in progress, then returns.
func TestShutdownWaitsForRequestsAndJobs(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	reqStarted, reqRelease := make(chan struct{}), make(chan struct{})
	jobStarted, jobRelease := make(chan struct{}), make(chan struct{})
	var jobCtxErr error
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		a.Handle("GET /slow", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(reqStarted)
			<-reqRelease
			_, _ = w.Write([]byte("done"))
		}))
		a.Handlers.Register("test.slow", func(ctx context.Context, _ queue.Job) queue.Result {
			close(jobStarted)
			<-jobRelease
			jobCtxErr = ctx.Err()
			return queue.Done()
		})
	})
	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "test.slow"}); err != nil {
		t.Fatal(err)
	}
	type result struct {
		status int
		body   string
	}
	resc := make(chan result, 1)
	go func() {
		resp, body := s.do(t, http.MethodGet, "/slow", nil)
		resc <- result{resp.StatusCode, body}
	}()
	<-reqStarted
	<-jobStarted

	s.stop()
	select {
	case err := <-s.done:
		t.Fatalf("Serve returned %v with a request and a job in progress", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(reqRelease)
	if r := <-resc; r.status != http.StatusOK || r.body != "done" {
		t.Errorf("in-flight request = %d %q, want 200 done", r.status, r.body)
	}
	select {
	case err := <-s.done:
		t.Fatalf("Serve returned %v with a job in progress", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(jobRelease)
	if err := <-s.done; err != nil {
		t.Errorf("Serve = %v, want nil", err)
	}
	s.done <- nil // for the cleanup
	if jobCtxErr != nil {
		t.Errorf("the job's context ended during the graceful shutdown: %v", jobCtxErr)
	}
	dead, err := s.app.DeadJobs.ListDeadJobs(context.Background(), "", queue.DeadJobPosition{}, 10)
	if err != nil || len(dead) != 0 {
		t.Errorf("dead jobs = %v (%v), want none", dead, err)
	}
}

// After the shutdown timeout, Serve cancels the requests still running and reports the timeout.
func TestShutdownTimeout(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_ROLE": "web"})
	started := make(chan struct{})
	s := start(t, cfg, platform.ServeOptions{ShutdownTimeout: 100 * time.Millisecond}, func(a *platform.App) {
		a.Handle("GET /stuck", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			close(started)
			<-r.Context().Done()
		}))
	})
	go func() {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, s.url+"/stuck", nil)
		if resp, err := http.DefaultTransport.RoundTrip(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	s.stop()
	select {
	case err := <-s.done:
		if err == nil {
			t.Error("Serve = nil, want an error after the shutdown timeout")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the shutdown timeout")
	}
	s.done <- nil // for the cleanup
}

// TestConfigAndInstanceSettingsShareNoKey enforces the precedence rule between the deployment
// configuration and the instance settings in the database: a key lives in exactly one of them,
// so neither overrides the other (docs/reference/configuration.md).
func TestConfigAndInstanceSettingsShareNoKey(t *testing.T) {
	t.Parallel()
	db := map[string]bool{}
	for _, d := range instance.Definitions() {
		db[string(d.Key)] = true
	}
	for _, d := range instance.TextDefinitions() {
		db[string(d.Key)] = true
	}
	for _, k := range config.Keys() {
		if db[k] {
			t.Errorf("%q is both a configuration key and an instance setting or text key", k)
		}
	}
}
