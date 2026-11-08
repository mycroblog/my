// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/app/uow"
)

// TestAccountSearchUsesTermIndex checks that the range of the prefix search (SearchAccountTerms
// in queries/postgres/account/account_search.sql) can read account_search_terms through its term index
// in order (ADR-0092 §1): term is COLLATE "C", so the range and the order are the index's.
// Sequential and bitmap scans are off, since the planner prefers them on an empty table.
func TestAccountSearchUsesTermIndex(t *testing.T) {
	db := newTestDB(t, uow.Options{})
	ctx := t.Context()
	conn, err := db.Pool().Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET enable_seqscan = off; SET enable_bitmapscan = off"); err != nil {
		t.Fatal(err)
	}
	rows, err := conn.Query(ctx, `EXPLAIN
SELECT t.account_id, t.kind, t.term, t.is_local, a.domain
FROM account_search_terms t JOIN accounts a ON a.id = t.account_id
WHERE t.term >= $1::text AND t.term < $2::text
ORDER BY t.term, t.account_id
LIMIT $3`, "al", "al\U0010FFFF", 100)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan, "\n")
	t.Logf("plan:\n%s", joined)
	if !strings.Contains(joined, "account_search_terms_term_account_id") || strings.Contains(joined, "Sort") {
		t.Errorf("plan =\n%s\nwant a range of account_search_terms_term_account_id without sorting", joined)
	}
}
