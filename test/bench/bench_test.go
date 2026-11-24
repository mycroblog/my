// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel/otlptest"
)

func TestLatencyOf(t *testing.T) {
	var ds []time.Duration
	for i := 100; i >= 1; i-- {
		ds = append(ds, time.Duration(i)*time.Millisecond)
	}
	got := latencyOf(ds)
	if got != (Latency{P50: 50, P95: 95, P99: 99, Max: 100}) {
		t.Errorf("latencyOf(1..100 ms) = %+v", got)
	}
	if got := latencyOf(nil); got != (Latency{}) {
		t.Errorf("latencyOf(nil) = %+v, want zero", got)
	}
}

func TestSqliteParams(t *testing.T) {
	for _, c := range []struct {
		sql  string
		want int
	}{
		{"SELECT a FROM t WHERE x = ? AND y = ?", 2},
		{"SELECT a FROM t WHERE x = ?1 AND y = ?3 OR z = ?1", 3},
		{"SELECT '?' FROM t WHERE x = ? -- is it ?\n AND y = \"?\"", 1},
		{"SELECT 1", 0},
	} {
		if got := sqliteParams(c.sql); got != c.want {
			t.Errorf("sqliteParams(%q) = %d, want %d", c.sql, got, c.want)
		}
	}
}

func TestLoadMix(t *testing.T) {
	l := &load{writes: 10}
	counts := map[string]int{}
	for n := range 1000 {
		counts[l.route(n)]++
	}
	w := counts["POST /statuses"] + counts["POST /statuses/{id}/favourite"]
	if w != 100 || counts["POST /statuses"] == 0 || counts["POST /statuses/{id}/favourite"] == 0 {
		t.Errorf("writes of 1,000 requests = %v, want 100 of both kinds", counts)
	}
	for _, r := range reads {
		if counts[r] == 0 {
			t.Errorf("the mix never reads %s", r)
		}
	}
}

func TestSummarize(t *testing.T) {
	span := func(trace, id, parent, name string, attrs map[string]string, start, end uint64) otlptest.Span {
		return otlptest.Span{TraceID: trace, SpanID: id, ParentSpanID: parent, Name: name, Attrs: attrs, StartUnixNano: start, EndUnixNano: end}
	}
	op := func(name string) map[string]string { return map[string]string{attrOperation: name} }
	const msec = uint64(time.Millisecond)
	spans := []otlptest.Span{
		// Two requests of one route: three statements and one transaction, then one statement.
		{TraceID: "t1", SpanID: "a", Name: "GET /api/v1/timelines/home", Kind: "server", EndUnixNano: 10 * msec},
		span("t1", "b", "a", spanTx, nil, 1*msec, 5*msec),
		span("t1", "c", "b", "ListHome", op("ListHome"), 1*msec, 3*msec),
		span("t1", "d", "b", "GetAccount", op("GetAccount"), 3*msec, 4*msec),
		span("t1", "e", "a", "IncrEntry", op("IncrEntry"), 6*msec, 7*msec),
		{TraceID: "t2", SpanID: "f", Name: "GET /api/v1/timelines/home", Kind: "server", EndUnixNano: 4 * msec},
		span("t2", "g", "f", "IncrEntry", op("IncrEntry"), 1*msec, 2*msec),
		// A job, whose root has a job type.
		span("t3", "h", "", "my.job", map[string]string{attrJobType: "timeline.fanout"}, 0, 2*msec),
		span("t3", "i", "h", "InsertEntry", op("InsertEntry"), 0, 1*msec),
		// A claim of the job runner, outside any request or job.
		span("t4", "j", "", "ClaimJob", op("ClaimJob"), 0, 1*msec),
	}
	got := summarize(spans, 2)
	if len(got.Roots) != 3 || got.Roots[0].Root != rootOther || got.Roots[0].Statements != 1 {
		t.Fatalf("roots = %+v, want the claim apart, the route and the job", got.Roots)
	}
	home, job := got.Roots[1], got.Roots[2]
	if home.Root != "GET /api/v1/timelines/home" || home.Count != 2 || home.Statements != 2 || home.Transactions != 0.5 ||
		home.DBMillis != 2.5 || home.Millis != 7 {
		t.Errorf("home = %+v", home)
	}
	if len(home.Queries) != 3 || home.Queries[0] != (QueryCount{Name: "IncrEntry", PerRequest: 1}) {
		t.Errorf("home's queries = %+v, want IncrEntry first, once a request", home.Queries)
	}
	if job.Root != "job timeline.fanout" || job.Count != 1 || job.Statements != 1 {
		t.Errorf("job = %+v", job)
	}
	// IncrEntry and ListHome took 2 ms each: the names break the tie.
	if len(got.Queries) != 5 || got.Queries[0] != (QueryStats{Name: "IncrEntry", Calls: 2, TotalMillis: 2, MeanMillis: 1}) ||
		got.Queries[1].Name != "ListHome" || got.Queries[2].Name != "ClaimJob" || got.Queries[4].Name != "InsertEntry" {
		t.Errorf("queries = %+v, want IncrEntry (2 calls, 2 ms), ListHome (2 ms), then the 1 ms ones by name", got.Queries)
	}
}

func TestQuerySQL(t *testing.T) {
	got, err := querySQL("../..", "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if sql := got["IncrEntry"]; !strings.Contains(sql, "kv_entries") {
		t.Errorf("IncrEntry = %q, want the key-value store's SQL", sql)
	}
	if sql := got["ListCurrentStatusRevisions"]; !strings.Contains(sql, "json_each") {
		t.Errorf("ListCurrentStatusRevisions on sqlite = %q, want its SQLite SQL", sql)
	}
}

func TestCompare(t *testing.T) {
	run := func(commit string, rps, p95, statements float64) *Result {
		return &Result{Commit: commit, CPUs: 4, Options: Options{Data: "small"}, Runs: []DBRun{{
			Database: "sqlite",
			Levels:   []Level{{Clients: 12, Throughput: rps, Latency: Latency{P95: p95}}},
			Trace:    &TraceResult{Roots: []RootStats{{Root: "GET /api/v1/instance", Statements: statements}}},
		}}}
	}
	var b bytes.Buffer
	compare(&b, run("aaaaaaaaaaaaaaaa", 800, 40, 4), run("bbbbbbbbbbbbbbbb", 1000, 30, 2))
	out := b.String()
	for _, want := range []string{"a: aaaaaaaaaaaa, small data, cores all 4", "| 12 | 800 → 1000 (+25%) |", "40.0 → 30.0 (-25%)",
		"| GET /api/v1/instance | 4.0 → 2.0 (-50%) |"} {
		if !strings.Contains(out, want) {
			t.Errorf("compare is missing %q:\n%s", want, out)
		}
	}
}
