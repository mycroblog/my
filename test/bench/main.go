// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel/otlptest"
)

const usage = `Usage:
  bench run [flags]          measure the server (go run ./test/bench run -h)
  bench compare <a> <b>      print two result.json side by side`

func main() { os.Exit(run(os.Args[1:])) }

// run runs the command of args and returns the exit code: 2 for a usage error, 1 for a failure.
func run(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch {
	case len(args) >= 1 && args[0] == "run":
		err = runCommand(ctx, args[1:], os.Stdout)
	case len(args) == 3 && args[0] == "compare":
		err = compareCommand(args[1], args[2], os.Stdout)
	default:
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		return 1
	}
	return 0
}

// config is the flags of run.
type config struct {
	my, postgres, out, cores string
	dbs                      []string
	drain                    time.Duration
	opts                     Options
}

func parseFlags(args []string) (config, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var c config
	var dbs, clients string
	fs.StringVar(&c.my, "my", "", "the server binary; empty builds ./cmd/my of the checkout into -out")
	fs.StringVar(&dbs, "db", "sqlite", "the databases to measure on, comma-separated: sqlite, postgres")
	fs.StringVar(&c.postgres, "postgres", "postgres://my:my@localhost:5432/my?sslmode=disable",
		"a PostgreSQL URL whose user may create databases (scripts/task.sh db:up); the run makes a database of its own there")
	fs.StringVar(&c.opts.Data, "data", "small", "the size of the seed: small or large")
	fs.StringVar(&clients, "clients", "1,12,48", "the numbers of concurrent clients, comma-separated")
	fs.IntVar(&c.opts.Requests, "requests", 2000, "the requests counted at each number of clients")
	fs.IntVar(&c.opts.Warmup, "warmup", 200, "the requests sent before those counted, at each number of clients")
	fs.IntVar(&c.opts.Writes, "writes", 10, "the writes in every hundred requests")
	fs.IntVar(&c.opts.ProfileSeconds, "profile", 5, "the seconds of the CPU, block and mutex profiles at each number of clients; 0 takes none")
	fs.IntVar(&c.opts.TraceRequests, "trace", 500, "the requests whose statements are counted from the traces; 0 counts none")
	fs.StringVar(&c.cores, "cores", "", "the cores the server runs on, as taskset -c takes them (0-3); empty for all")
	fs.DurationVar(&c.drain, "drain", 30*time.Minute, "how long the run waits for the jobs the seed and each level queued before it fails")
	fs.StringVar(&c.out, "out", "", "the directory of the result; empty for .tmp/bench/<time>-<commit>")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() > 0 {
		return c, fmt.Errorf("unexpected arguments %q", fs.Args())
	}
	c.dbs = strings.Split(dbs, ",")
	for _, s := range strings.Split(clients, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 1 {
			return c, fmt.Errorf("-clients: %q is not a positive number", s)
		}
		c.opts.Clients = append(c.opts.Clients, n)
	}
	if _, ok := dataSizes[c.opts.Data]; !ok {
		return c, fmt.Errorf("-data: %q is not small or large", c.opts.Data)
	}
	if c.opts.Requests < 1 || c.opts.Writes < 0 || c.opts.Writes > 100 {
		return c, errors.New("-requests must be positive and -writes from 0 to 100")
	}
	return c, nil
}

func runCommand(ctx context.Context, args []string, stdout io.Writer) error {
	c, err := parseFlags(args)
	if err != nil {
		return err
	}
	commit, dirty := gitCommit()
	started := time.Now().UTC()
	if c.out == "" {
		c.out = filepath.Join(".tmp", "bench", started.Format("20060102-150405")+"-"+commit[:min(12, len(commit))])
	}
	if err := os.MkdirAll(c.out, 0o750); err != nil {
		return err
	}
	logf := func(format string, a ...any) {
		fmt.Fprintf(stdout, "%s "+format+"\n", append([]any{time.Now().Format("15:04:05")}, a...)...)
	}
	if c.my == "" {
		c.my = filepath.Join(c.out, "my")
		logf("building %s", c.my)
		build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", c.my, "./cmd/my") //nolint:gosec // -out names where the build goes
		build.Stdout, build.Stderr = os.Stderr, os.Stderr
		if err := build.Run(); err != nil {
			return fmt.Errorf("build the server: %w", err)
		}
	}
	if c.my, err = filepath.Abs(c.my); err != nil {
		return err
	}
	res := &Result{Commit: commit, Dirty: dirty, Started: started, CPUs: runtime.NumCPU(), Cores: c.cores, Options: c.opts}
	resultPath := filepath.Join(c.out, "result.json")
	var failed error
	for _, kind := range c.dbs {
		run, err := measure(ctx, c, kind, logf)
		if run != nil {
			res.Runs = append(res.Runs, *run)
		}
		if err != nil {
			failed = errors.Join(failed, fmt.Errorf("%s: %w", kind, err))
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", kind, err))
		}
		if werr := writeResult(resultPath, res); werr != nil {
			return errors.Join(failed, werr)
		}
		if ctx.Err() != nil {
			break
		}
	}
	printResult(stdout, res)
	logf("result: %s", resultPath)
	return failed
}

// measure runs the whole measurement on one database (doc.go, A run).
func measure(ctx context.Context, c config, kind string, logf func(string, ...any)) (run *DBRun, err error) {
	dir, err := filepath.Abs(filepath.Join(c.out, kind))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	db, err := newDatabase(ctx, kind, dir, c.postgres)
	if err != nil {
		return nil, err
	}
	defer func() {
		if derr := db.drop(context.WithoutCancel(ctx)); derr != nil {
			err = errors.Join(err, fmt.Errorf("drop the database: %w", derr))
		}
	}()
	s, err := newServer(c.my, dir, db)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, s.stop()) }()
	logf("%s: applying the schema", kind)
	if _, err := s.my(ctx, "", "schema", "apply"); err != nil {
		return nil, err
	}
	if _, err := s.my(ctx, "", "db", "upgrade"); err != nil {
		return nil, err
	}
	if err := s.start(ctx, c.cores, "server.log"); err != nil {
		return nil, err
	}
	maxClients := 1
	for _, n := range c.opts.Clients {
		maxClients = max(maxClients, n)
	}
	a := newAPI(s.url, maxClients)
	run = &DBRun{Database: kind}
	t0 := time.Now()
	w, err := seed(ctx, s, a, dataSizes[c.opts.Data], func(format string, args ...any) { logf(kind+": "+format, args...) })
	if err != nil {
		return run, fmt.Errorf("seed: %w", err)
	}
	if err := drainJobs(ctx, db, c.drain, logf); err != nil {
		return run, err
	}
	run.Data, run.SeedSeconds = w.info, time.Since(t0).Seconds()
	logf("%s: seeded %+v in %.0f s", kind, w.info, run.SeedSeconds)

	l := &load{api: a, world: w, writes: c.opts.Writes, tag: randomHex(4)}
	next := 0
	var refused error
	for _, clients := range c.opts.Clients {
		if _, next, err = l.run(ctx, clients, next, c.opts.Warmup, time.Time{}); err != nil {
			return run, err
		}
		at := time.Now()
		var samples []sample
		if samples, next, err = l.run(ctx, clients, next, c.opts.Requests, time.Time{}); err != nil {
			return run, err
		}
		lv := levelOf(clients, samples, time.Since(at))
		if c.opts.ProfileSeconds > 0 {
			// The same load runs on while the profiles are taken, a second longer than them.
			until := time.Now().Add(time.Duration(c.opts.ProfileSeconds+1) * time.Second)
			done := make(chan error, 1)
			go func() {
				var lerr error
				_, next, lerr = l.run(ctx, clients, next, 0, until)
				done <- lerr
			}()
			names, perr := profiles(ctx, s, c.opts.ProfileSeconds, dir, fmt.Sprintf("c%d", clients))
			if err := errors.Join(perr, <-done); err != nil {
				return run, err
			}
			for _, n := range names {
				lv.Profiles = append(lv.Profiles, filepath.Join(kind, n))
			}
		}
		run.Levels = append(run.Levels, lv)
		logf("%s: %d clients: %.0f requests/s, p50 %.1f ms, p95 %.1f ms, p99 %.1f ms", kind, clients, lv.Throughput,
			lv.Latency.P50, lv.Latency.P95, lv.Latency.P99)
		if n := lv.failures(); n > 0 {
			refused = errors.Join(refused, fmt.Errorf("%d clients: %d answers were not 2xx: %v", clients, n, lv.Statuses))
		}
		if err := drainJobs(ctx, db, c.drain, logf); err != nil {
			return run, err
		}
	}
	if refused != nil {
		return run, refused
	}
	if c.opts.TraceRequests > 0 {
		if run.Trace, run.Plans, err = traced(ctx, c, s, db, l, next, logf); err != nil {
			return run, err
		}
	}
	return run, nil
}

// traced runs the traced requests on a server started again with every trace recorded, and
// returns their summary and the plans of the queries that took the most time.
func traced(ctx context.Context, c config, s *server, db *database, l *load, from int, logf func(string, ...any)) (*TraceResult, []Plan, error) {
	if err := s.stop(); err != nil {
		return nil, nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	var collector otlptest.Collector
	receiver := &http.Server{Handler: &collector, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = receiver.Serve(ln) }()
	defer func() { _ = receiver.Close() }()
	if err := s.start(ctx, c.cores, "server-traced.log", traceEnv("http://"+ln.Addr().String())...); err != nil {
		return nil, nil, err
	}
	logf("%s: counting the statements of %d requests", db.kind, c.opts.TraceRequests)
	samples, _, err := l.run(ctx, 1, from, c.opts.TraceRequests, time.Time{})
	if err != nil {
		return nil, nil, err
	}
	if err := drainJobs(ctx, db, c.drain, logf); err != nil {
		return nil, nil, err
	}
	// Stopping the server sends the spans it has not sent yet.
	if err := s.stop(); err != nil {
		return nil, nil, err
	}
	spans := collector.Spans()
	if len(spans) == 0 {
		return nil, nil, errors.New("the traced server sent no spans")
	}
	tr := summarize(spans, len(samples))
	text, err := querySQL(".", db.kind)
	if err != nil {
		return tr, nil, fmt.Errorf("read the SQL of the queries: %w", err)
	}
	ps, err := plans(ctx, db, tr.Queries, text, 10)
	return tr, ps, err
}

// gitCommit returns the commit of the checkout and whether its tree differs from it.
func gitCommit() (string, bool) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown", false
	}
	status, _ := exec.Command("git", "status", "--porcelain", "--untracked-files=no").Output()
	return strings.TrimSpace(string(out)), len(strings.TrimSpace(string(status))) > 0
}
