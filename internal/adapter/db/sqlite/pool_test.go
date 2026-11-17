// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/schemaapply"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
)

// tiny is a schema that costs nothing to plan, for the tests of what ApplySchema does with a URL.
const tiny = "CREATE TABLE probe (id INTEGER PRIMARY KEY);\n"

// burst runs n queries that are all in progress at once, each on a connection of its own, and
// returns when they are done: it needs a pool whose maximum is n or more.
func burst(t *testing.T, db *DB, n int) {
	t.Helper()
	var arrived, done sync.WaitGroup
	arrived.Add(n)
	done.Add(n)
	for range n {
		go func() {
			defer done.Done()
			c, err := db.SQLDB().Conn(t.Context())
			if err != nil {
				t.Errorf("Conn: %v", err)
				arrived.Done()
				return
			}
			var one int
			if err := c.QueryRowContext(t.Context(), "SELECT 1").Scan(&one); err != nil {
				t.Errorf("query: %v", err)
			}
			arrived.Done()
			arrived.Wait()
			if err := c.Close(); err != nil {
				t.Errorf("Close: %v", err)
			}
		}()
	}
	done.Wait()
}

func TestPoolKeepsItsConnectionsOnceWarm(t *testing.T) {
	t.Parallel()
	const conns, rounds = 5, 4
	db, _ := newTestDB(t, "?"+ParamPoolMaxConns+"="+strconv.Itoa(conns), uow.Options{})
	burst(t, db, conns)
	warm := db.SQLDB().Stats()
	if warm.OpenConnections != conns {
		t.Fatalf("OpenConnections after the first round = %d, want %d", warm.OpenConnections, conns)
	}
	for round := 2; round <= rounds; round++ {
		burst(t, db, conns)
		s := db.SQLDB().Stats()
		if s.OpenConnections != warm.OpenConnections || s.Idle != conns {
			t.Errorf("round %d: OpenConnections = %d, Idle = %d, want %d of both", round, s.OpenConnections, s.Idle, conns)
		}
		// A connection that is opened in a round was closed in an earlier one, or is a new one: the
		// first is counted by these, the second by OpenConnections above.
		if s.MaxIdleClosed != 0 {
			t.Errorf("round %d: MaxIdleClosed = %d, want 0", round, s.MaxIdleClosed)
		}
	}
}

// Time cannot be passed for database/sql, whose timers are the real ones, so what is held is that
// no connection was ever closed by the age that the pool could have been given.
func TestPoolClosesNoConnectionByAge(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "?"+ParamPoolMaxConns+"=4", uow.Options{})
	for range 3 {
		burst(t, db, 4)
		s := db.SQLDB().Stats()
		if s.MaxIdleTimeClosed != 0 || s.MaxLifetimeClosed != 0 {
			t.Fatalf("MaxIdleTimeClosed = %d, MaxLifetimeClosed = %d, want 0 and 0", s.MaxIdleTimeClosed, s.MaxLifetimeClosed)
		}
	}
}

func TestPoolParameters(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		suffix   string
		wantMax  int
		wantIdle int // the connections kept after a burst of wantMax
	}{
		{"a maximum", "?pool_max_conns=3", 3, 3},
		{"a maximum of one", "?pool_max_conns=1", 1, 1},
		{"a maximum and an idle limit", "?pool_max_conns=6&pool_max_idle_conns=2", 6, 2},
		{"an idle limit of the maximum", "?pool_max_conns=5&pool_max_idle_conns=5", 5, 5},
		{"no idle connection", "?pool_max_conns=4&pool_max_idle_conns=0", 4, 0},
		{"with a pragma before", "?_pragma=busy_timeout(100)&pool_max_conns=3&pool_max_idle_conns=1", 3, 1},
		{"with a pragma after", "?pool_max_conns=3&_pragma=busy_timeout(100)", 3, 3},
		{"the default idle limit is the maximum", "?pool_max_conns=5", 5, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, _ := newTestDB(t, tc.suffix, uow.Options{})
			if got := db.PoolMax(); got != tc.wantMax {
				t.Errorf("PoolMax = %d, want %d", got, tc.wantMax)
			}
			if got := db.SQLDB().Stats().MaxOpenConnections; got != tc.wantMax {
				t.Errorf("MaxOpenConnections = %d, want %d", got, tc.wantMax)
			}
			// Open itself may use a connection and give it back, so count from here.
			before := db.SQLDB().Stats().MaxIdleClosed
			burst(t, db, tc.wantMax)
			s := db.SQLDB().Stats()
			if s.Idle != tc.wantIdle {
				t.Errorf("Idle after a burst = %d, want %d", s.Idle, tc.wantIdle)
			}
			if want := int64(tc.wantMax - tc.wantIdle); s.MaxIdleClosed-before < want {
				t.Errorf("MaxIdleClosed grew by %d, want at least %d", s.MaxIdleClosed-before, want)
			}
		})
	}
}

func TestPoolMaxDefaultIsTheGreaterOfFourAndTheProcessors(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "", uow.Options{})
	if got, want := db.PoolMax(), max(4, runtime.GOMAXPROCS(0)); got != want {
		t.Errorf("PoolMax of a database opened without the parameter = %d, want %d", got, want)
	}
	if got, want := db.SQLDB().Stats().MaxOpenConnections, PoolMaxDefault(); got != want {
		t.Errorf("MaxOpenConnections = %d, want %d", got, want)
	}
	if got, want := PoolMaxDefault(), max(4, runtime.GOMAXPROCS(0)); got != want {
		t.Errorf("PoolMaxDefault = %d, want %d", got, want)
	}
}

func TestPoolParametersAreRefusedByName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		suffix string
		param  string
	}{
		{"?pool_max_conns=0", ParamPoolMaxConns},
		{"?pool_max_conns=-1", ParamPoolMaxConns},
		{"?pool_max_conns=many", ParamPoolMaxConns},
		{"?pool_max_conns=", ParamPoolMaxConns},
		{"?pool_max_conns=2.5", ParamPoolMaxConns},
		{"?pool_max_conns=4&pool_max_idle_conns=-1", ParamPoolMaxIdleConns},
		{"?pool_max_conns=4&pool_max_idle_conns=some", ParamPoolMaxIdleConns},
		{"?pool_max_conns=4&pool_max_idle_conns=5", ParamPoolMaxIdleConns},
		{"?pool_max_idle_conns=" + strconv.Itoa(PoolMaxDefault()+1), ParamPoolMaxIdleConns},
	} {
		t.Run(tc.suffix, func(t *testing.T) {
			t.Parallel()
			path := sqlitetest.NewFile(t)
			db, err := Open(t.Context(), path+tc.suffix, uow.Options{})
			if err == nil {
				_ = db.Close()
				t.Fatalf("Open succeeded, want an error naming %s", tc.param)
			}
			if !strings.Contains(err.Error(), tc.param) {
				t.Errorf("Open = %v, want an error naming %s", err, tc.param)
			}
			if _, err := ApplySchema(t.Context(), path+tc.suffix, tiny, schemaapply.Options{DryRun: true}); err == nil || !strings.Contains(err.Error(), tc.param) {
				t.Errorf("ApplySchema = %v, want an error naming %s", err, tc.param)
			}
		})
	}
}

func TestPoolParametersAreNotPassedToTheDriver(t *testing.T) {
	t.Parallel()
	for _, suffix := range []string{
		"?pool_max_conns=3&pool_max_idle_conns=2",
		"?_pragma=busy_timeout(100)&pool_max_conns=3",
	} {
		t.Run(suffix, func(t *testing.T) {
			t.Parallel()
			path := newEmptyFile(t)
			r, err := ApplySchema(t.Context(), path+suffix, tiny, schemaapply.Options{})
			if err != nil {
				t.Fatalf("ApplySchema: %v", err)
			}
			if len(r.Statements) == 0 {
				t.Error("ApplySchema applied nothing to an empty file")
			}
			db, err := Open(t.Context(), path+suffix, uow.Options{})
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if err := db.Ready(t.Context(), nil); err != nil {
				t.Errorf("Ready: %v", err)
			}
			var timeout int
			if err := db.SQLDB().QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&timeout); err != nil {
				t.Fatal(err)
			}
			if want := map[bool]int{true: 100, false: DefaultBusyTimeoutMillis}[strings.Contains(suffix, "_pragma")]; timeout != want {
				t.Errorf("busy_timeout = %d, want %d", timeout, want)
			}
		})
	}
}
