// SPDX-License-Identifier: AGPL-3.0-only

package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/pkg/testkit/pgtest"
)

// Spike #3535: does a temporary table of the same name shadow the schema's table for a statement
// that pgx prepared and cached on the connection before the temporary table existed?
func TestSpikeTempTableShadowsCachedStatement(t *testing.T) {
	ctx := context.Background()
	url := pgtest.NewDatabase(t)
	for _, mode := range []pgx.QueryExecMode{pgx.QueryExecModeCacheStatement, pgx.QueryExecModeCacheDescribe, pgx.QueryExecModeExec} {
		t.Run(mode.String(), func(t *testing.T) {
			cfg, err := pgx.ParseConfig(url)
			if err != nil {
				t.Fatal(err)
			}
			cfg.DefaultQueryExecMode = mode
			conn, err := pgx.ConnectConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close(ctx)
			if _, err := conn.Exec(ctx, "DELETE FROM poll_tallies"); err != nil {
				t.Fatal(err)
			}
			count := func(tx pgx.Tx) int {
				var n int
				if err := tx.QueryRow(ctx, "SELECT count(*) FROM poll_tallies").Scan(&n); err != nil {
					t.Fatal(err)
				}
				return n
			}
			insert := func(tx pgx.Tx, poll string) {
				if _, err := tx.Exec(ctx, "INSERT INTO poll_tallies (poll_id, position, votes) VALUES ($1, 0, 1)", poll); err != nil {
					t.Fatal(err)
				}
			}
			// Warm the cache on the live table.
			tx, _ := conn.Begin(ctx)
			insert(tx, "01a00000-0000-7000-8000-000000000001")
			live := count(tx)
			_ = tx.Commit(ctx)
			// In a later transaction, shadow it.
			tx, _ = conn.Begin(ctx)
			if _, err := tx.Exec(ctx, "CREATE TEMP TABLE poll_tallies (LIKE public.poll_tallies INCLUDING ALL) ON COMMIT DROP"); err != nil {
				t.Fatal(err)
			}
			insert(tx, "01a00000-0000-7000-8000-000000000002")
			insert(tx, "01a00000-0000-7000-8000-000000000003")
			shadowed := count(tx)
			var publicN int
			_ = tx.QueryRow(ctx, "SELECT count(*) FROM public.poll_tallies").Scan(&publicN)
			_ = tx.Rollback(ctx)
			tx, _ = conn.Begin(ctx)
			after := count(tx)
			_ = tx.Rollback(ctx)
			t.Logf("live=%d shadowed=%d public-during=%d after=%d", live, shadowed, publicN, after)
			if shadowed != 2 || publicN != 1 || after != 1 {
				t.Errorf("the temporary table did not shadow the cached statements: live=%d shadowed=%d public=%d after=%d", live, shadowed, publicN, after)
			}
		})
	}
}
