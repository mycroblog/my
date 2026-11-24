// SPDX-License-Identifier: AGPL-3.0-only

// Command bench is the benchmarker of the server (#3183): it measures how fast the server answers
// cheap requests under concurrent clients, on SQLite and on PostgreSQL, and captures where the
// time goes, so that a change made for performance has a measurement before and after that can be
// laid side by side with another commit's. It speaks HTTP to the server and drives no browser. It
// is run by hand: nothing in CI, the slow suite or a gate runs it, and its numbers are no budget.
//
//	scripts/task.sh go:bench -- [flags]            # go run ./test/bench run [flags]; -h lists them
//	go run ./test/bench compare <a.json> <b.json>  # the two results side by side
//
// # A run
//
// For each database of -db, a run owns its server's life: a directory of its own under -out, free
// ports, a database of its own (a file for SQLite; for PostgreSQL a database created on the server
// of -postgres and dropped after), the schema applied with `my schema apply` and the data upgrades run with `my db upgrade`, and `my serve` in the
// production profile with a settings environment of its own (no MY_* of the caller's). It stops the
// server with SIGTERM and kills it a minute later, and waits for it to end before the next starts.
// Any failure ends the run with an error and a non-zero exit, and so does any answer of the load
// that is not a 2xx: the result file is still written, with the answers counted by status.
//
// The server is the build of the checkout (go build ./cmd/my, without the web client), or -my. With
// -cores, it runs under `taskset -c <cores>`, and the Go runtime sizes itself to those cores, as
// the connection pools do; the benchmarker itself is not pinned (run it under taskset to pin it).
//
// # The data
//
// The server is seeded through its own API and CLI at the size of -data: small (32 people, 8
// follows and 10 posts each: the scale of the E2E scenario) or large (200 people, 50 follows and
// 100 posts each, 20,000 posts: enough for a query's plan to show). The people are made with
// `my admin create` and lose the admin role (one more keeps it, and is never used); each signs in through the authorization code flow, as
// the web client does, from an address of its own (X-Forwarded-For, trusted from loopback). A
// post in four replies to an earlier one, one in five carries a hashtag, one in ten mentions
// someone, and each person favourites half as many posts as they write. The data and the load
// are the same for the same flags: their choices come from a fixed seed. The run waits until the
// job queue has no due job before it measures.
//
// # The load
//
// For each number of clients of -clients, the clients send -warmup requests that are not counted,
// then -requests that are, each taking the next request of one sequence: of every hundred, -writes
// are writes (a post, or a favourite) and the rest reads of the routes of a client's first screen
// in turn (the own account, the instance, the preferences, the announcements, the home timeline,
// the notifications, and the posts of someone followed). Each request is made as the next person
// in turn, so that the load stays under the rate limits of one account (1,500 requests in five
// minutes, 300 posts in three hours) without changing what a request does. A level reports its
// throughput, the latency's percentiles overall and per route, and the answers by status.
//
// # What it captures
//
//   - Profiles: after each level's measured requests, with -profile seconds, the same load runs on
//     while the server's profiler listener (pprof_listen) takes the CPU profile and the block and
//     mutex profiles, which sample only while asked (internal/platform/serve_pprof.go). They are
//     written beside the result, for `go tool pprof`, and are taken apart from the measured
//     requests, since profiling slows the server.
//   - Statements per request: the server is started again on the same database with every trace
//     recorded (traces_exporter = otlp, traces_sample_ratio = 1) and sent to a receiver of the
//     benchmarker (otlptest), and -trace requests of the same load run from one client. Each
//     trace's root is a request (its route) or a job (its type); the result holds, per root, the
//     statements, transactions, time in the database and time overall of a mean one, and the
//     statements by sqlc query name. Tracing slows the server too, so none of these requests is
//     timed for the levels. On SQLite the key-value store's statements have no spans
//     (internal/adapter/kv/db/sqlite.go), so the counts there leave out the rate limit's and the
//     token use's writes and the store's reads, which the counts on PostgreSQL hold.
//   - Plans: the queries that took the most time in the traced requests, with their plans on the
//     run's database once the server has stopped: EXPLAIN (GENERIC_PLAN) on PostgreSQL and EXPLAIN
//     QUERY PLAN on SQLite, the SQL read from the generated code of the adapters by query name.
//
// # The result
//
// result.json under -out holds the commit (and whether the tree was dirty), the machine's
// processors, the flags, and per database the data, the levels, the traced roots and the plans;
// the profiles and each server's log are beside it. `compare` prints, per database and level, the
// throughput and percentiles of two results side by side with the change, and the statements of
// each route.
package main
