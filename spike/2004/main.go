// SPDX-License-Identifier: AGPL-3.0-only

// Command spike2004 compares N worker processes with one slot against one process with N slots.
// It stays on the spike branch (ADR-0022). Each slot is a real queue.PullRunner on the real
// queue/db Source; the handler does one read on the database and one GET on a slow server.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/adapter/system"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/schema"
)

type clock struct{}

func (clock) Now() time.Time { return kernel.Normalize(time.Now()) }

var gen = system.IDGenerator{}

var (
	useDispatch bool
	useShared   bool

	flagTxSecond bool
	flagHold     time.Duration
	flagLease    time.Duration
	flagTypes    int
)

// typeName is one of the skipped-first backlog's other types.
func typeName(i int) string { return fmt.Sprintf("t%02d", i) }

// timedSource measures the claim.
type timedSource struct {
	queue.Source
	n, sum atomic.Int64
}

func (t *timedSource) Claim(ctx context.Context, worker string, now time.Time, lease time.Duration) (queue.Job, bool, error) {
	start := time.Now()
	j, ok, err := t.Source.Claim(ctx, worker, now, lease)
	t.n.Add(1)
	t.sum.Add(int64(time.Since(start)))
	return j, ok, err
}

// pollGate lets one slot poll while no job is due; the others wait until it finds one. Claims stay
// parallel while there is work.
type pollGate struct {
	mu      sync.Mutex
	polling bool
	wake    chan struct{}
}

func (g *pollGate) slot(ctx context.Context, r *queue.PullRunner) {
	for ctx.Err() == nil {
		ran, err := r.RunOnce(ctx)
		if ran && err == nil {
			g.mu.Lock()
			close(g.wake)
			g.wake = make(chan struct{})
			g.mu.Unlock()
			continue
		}
		g.mu.Lock()
		if !g.polling {
			g.polling = true
			g.mu.Unlock()
			select {
			case <-time.After(queue.DefaultPollInterval):
			case <-ctx.Done():
			}
			g.mu.Lock()
			g.polling = false
			g.mu.Unlock()
			continue
		}
		wake := g.wake
		g.mu.Unlock()
		select {
		case <-wake:
		case <-ctx.Done():
		}
	}
}

// dispatchLoop is the shape the ADR recommends: a slot is taken before the claim, so no claimed job
// waits for a slot, and the process polls once per interval however many slots it has.
func dispatchLoop(ctx context.Context, src queue.Source, reg *queue.JobHandlerRegistry, slots int) {
	sem := make(chan struct{}, slots)
	var wg sync.WaitGroup
	defer wg.Wait()
	for ctx.Err() == nil {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		now := clock{}.Now()
		j, ok, err := src.Claim(ctx, "dispatch", now, queue.DefaultLease)
		if err != nil || !ok {
			<-sem
			select {
			case <-time.After(queue.DefaultPollInterval):
			case <-ctx.Done():
			}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			h, _, _ := reg.Lookup(j.Type)
			res := h(ctx, j)
			rec, cancel := context.WithTimeout(context.WithoutCancel(ctx), queue.DefaultRecordTimeout)
			defer cancel()
			if res.IsDone() {
				_ = src.Complete(rec, "dispatch", j)
			} else {
				_ = src.Release(rec, "dispatch", j, j.Attempt, clock{}.Now().Add(time.Minute), "cancelled")
			}
		}()
	}
}

func main() {
	mode := flag.String("mode", "", "remote | schema | load | worker")
	dsn := flag.String("dsn", "", "database")
	dialect := flag.String("dialect", "postgres", "postgres | sqlite")
	addr := flag.String("addr", "127.0.0.1:18999", "the slow server")
	delay := flag.Duration("delay", 200*time.Millisecond, "the slow server's delay")
	jobs := flag.Int("jobs", 1000, "jobs to load")
	slots := flag.Int("slots", 1, "PullRunners in this process")
	perHost := flag.Int("per-host", 0, "MaxConnsPerHost of the HTTP client; 0 is none")
	dispatch := flag.Bool("dispatch", false, "one claim loop hands jobs to the slots, instead of a runner per slot")
	shared := flag.Bool("shared-poll", false, "a runner per slot, but one slot polls while the queue is empty")
	txSecond := flag.Bool("tx-second", false, "the handler reads on a second connection while it holds a transaction")
	hold := flag.Duration("hold", 0, "how long the handler holds the transaction before that read")
	lease := flag.Duration("lease", 0, "the lease, and so the handler's deadline; 0 is the default")
	types := flag.Int("types", 0, "claim only the first N of t01..t82 with the real OnlyTypes; 0 claims any")
	flag.Parse()
	useShared = *shared
	flagTxSecond, flagHold, flagLease, flagTypes = *txSecond, *hold, *lease, *types
	useDispatch = *dispatch
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	var err error
	switch *mode {
	case "remote":
		err = remote(ctx, *addr)
	case "schema":
		err = applySchema(ctx, *dialect, *dsn)
	case "backlog":
		err = backlog(ctx, *dsn, *delay)
	case "load":
		err = load(ctx, *dialect, *dsn, *jobs, *delay)
	case "worker":
		err = worker(ctx, *dialect, *dsn, *addr, *slots, *perHost)
	default:
		err = fmt.Errorf("unknown mode %q", *mode)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func remote(ctx context.Context, addr string) error {
	s := &http.Server{Addr: addr, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, _ := time.ParseDuration(r.URL.Query().Get("d"))
		time.Sleep(d)
		_, _ = io.WriteString(w, "ok")
	})}
	go func() { <-ctx.Done(); _ = s.Close() }()
	return s.ListenAndServe()
}

func applySchema(ctx context.Context, dialect, dsn string) error {
	if dialect == "sqlite" {
		d, err := sqlite.Open(ctx, dsn, uow.Options{})
		if err != nil {
			return err
		}
		defer d.Close()
		return d.Tx(ctx, func(tx uow.Tx) error {
			q, err := d.SQLTx(tx)
			if err != nil {
				return err
			}
			_, err = q.ExecContext(ctx, schema.SQLite)
			return err
		})
	}
	d, err := postgres.Open(ctx, dsn, uow.Options{})
	if err != nil {
		return err
	}
	defer d.Close()
	_, err = d.Pool().Exec(ctx, schema.PostgresBootstrap+"\n"+schema.Postgres)
	return err
}

func load(ctx context.Context, dialect, dsn string, n int, delay time.Duration) error {
	payload, _ := json.Marshal(map[string]string{"d": delay.String()})
	past := time.Now().Add(-time.Minute).UTC()
	if dialect == "sqlite" {
		d, err := sqlite.Open(ctx, dsn, uow.Options{})
		if err != nil {
			return err
		}
		defer d.Close()
		q := queuedb.NewSQLite(d, queuedb.Options{Clock: clock{}, IDs: gen})
		for start := 0; start < n; start += 500 {
			batch := make([]queue.NewJob, 0, 500)
			for range min(500, n-start) {
				batch = append(batch, queue.NewJob{Type: "spike.slow", Payload: payload, RunAt: past})
			}
			if err := q.Enqueue(ctx, batch...); err != nil {
				return err
			}
		}
		return nil
	}
	d, err := postgres.Open(ctx, dsn, uow.Options{})
	if err != nil {
		return err
	}
	defer d.Close()
	_, err = d.Pool().Exec(ctx, `INSERT INTO jobs (id, type, payload, attempt, run_at, created_at)
		SELECT gen_random_uuid(), 'spike.slow', $1::bytea, 0, $3::timestamptz, $3::timestamptz FROM generate_series(1, $2::int)`, payload, n, past)
	return err
}

// backlog loads ADR-0138's distribution: 900,000 jobs of the dominant type, due first, and 100,000
// of 82 others: 20,000 each of t01..t03, so that a worker of 1 or 3 types does not run out, and the
// rest over t04..t82.
func backlog(ctx context.Context, dsn string, delay time.Duration) error {
	payload, _ := json.Marshal(map[string]string{"d": delay.String()})
	d, err := postgres.Open(ctx, dsn, uow.Options{})
	if err != nil {
		return err
	}
	defer d.Close()
	oldest := time.Now().Add(-2 * time.Hour).UTC()
	newer := time.Now().Add(-time.Minute).UTC()
	if _, err := d.Pool().Exec(ctx, `INSERT INTO jobs (id, type, payload, attempt, run_at, created_at)
		SELECT gen_random_uuid(), 'federation.deliver', $1::bytea, 0, $2::timestamptz + (n * interval '1 millisecond'), $2::timestamptz
		FROM generate_series(1, 900000) AS n`, payload, oldest); err != nil {
		return err
	}
	add := func(typ string, n int) error {
		_, err := d.Pool().Exec(ctx, `INSERT INTO jobs (id, type, payload, attempt, run_at, created_at)
			SELECT gen_random_uuid(), $1, $2::bytea, 0, $3::timestamptz + (n * interval '1 millisecond'), $3::timestamptz
			FROM generate_series(1, $4::int) AS n`, typ, payload, newer, n)
		return err
	}
	for i := 1; i <= 3; i++ {
		if err := add(typeName(i), 20000); err != nil {
			return err
		}
	}
	for i := 4; i <= 82; i++ {
		if err := add(typeName(i), 40000/79+1); err != nil {
			return err
		}
	}
	_, err = d.Pool().Exec(ctx, `ANALYZE jobs`)
	return err
}

func worker(ctx context.Context, dialect, dsn, addr string, slots, perHost int) error {
	var (
		src  queue.Source
		read func(context.Context) error
	)
	opts := queuedb.Options{Clock: clock{}, IDs: gen}
	if dialect == "sqlite" {
		d, err := sqlite.Open(ctx, dsn, uow.Options{})
		if err != nil {
			return err
		}
		defer d.Close()
		src = queuedb.NewSQLite(d, opts)
		// The read of a real handler is a transaction here, which takes the write lock (BEGIN IMMEDIATE).
		read = func(ctx context.Context) error {
			return d.Tx(ctx, func(uow.Tx) error { return nil })
		}
	} else {
		d, err := postgres.Open(ctx, dsn, uow.Options{})
		if err != nil {
			return err
		}
		defer d.Close()
		q := queuedb.NewPostgres(d, opts)
		src = q
		if flagTypes > 0 {
			names := make([]string, 0, flagTypes)
			for i := 1; i <= flagTypes; i++ {
				names = append(names, typeName(i))
			}
			src = q.OnlyTypes(names)
		}
		read = func(ctx context.Context) error {
			var one int
			if !flagTxSecond {
				return d.Pool().QueryRow(ctx, `SELECT 1`).Scan(&one)
			}
			// The shape of content/remote_reaction.go:108: a read on another connection inside Tx.
			return d.Tx(ctx, func(uow.Tx) error {
				time.Sleep(flagHold)
				return d.Pool().QueryRow(ctx, `SELECT 1`).Scan(&one)
			})
		}
	}
	client := &http.Client{Transport: &http.Transport{MaxConnsPerHost: perHost, MaxIdleConns: 1000, MaxIdleConnsPerHost: 1000}}
	reg := &queue.JobHandlerRegistry{}
	handle := func(ctx context.Context, j queue.Job) queue.Result {
		var p struct{ D string }
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return queue.Fail(err)
		}
		if err := read(ctx); err != nil {
			return queue.Retry(0, err)
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/?d="+p.D, nil)
		resp, err := client.Do(req)
		if err != nil {
			return queue.Retry(0, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return queue.Done()
	}
	reg.Register("spike.slow", handle)
	reg.Register("federation.deliver", handle)
	for i := 1; i <= 82; i++ {
		reg.Register(typeName(i), handle)
	}
	timed := &timedSource{Source: src}
	src = timed
	defer func() {
		if n := timed.n.Load(); n > 0 {
			fmt.Fprintf(os.Stderr, "claims=%d claim_avg_ms=%.2f\n", n, float64(timed.sum.Load())/float64(n)/1e6)
		}
	}()
	// Errors go to stderr, for the driver to count; a run's line is info.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if useDispatch {
		dispatchLoop(ctx, src, reg, slots)
		return nil
	}
	var wg sync.WaitGroup
	gate := &pollGate{wake: make(chan struct{})}
	for range slots {
		wg.Add(1)
		r := queue.NewPullRunner(src, reg, queue.RunnerOptions{Clock: clock{}, Logger: logger, Lease: flagLease})
		go func() {
			defer wg.Done()
			if useShared {
				gate.slot(ctx, r)
				return
			}
			_ = r.Run(ctx)
		}()
	}
	wg.Wait()
	return nil
}
