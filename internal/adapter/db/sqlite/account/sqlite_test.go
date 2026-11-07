// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
)

// TestAccountSearchUsesTermIndex checks that the range of the prefix search (SearchAccountTerms
// in queries/sqlite/account/account_search.sql) reads account_search_terms through its term index, in
// order, without sorting (ADR-0092 §1).
func TestAccountSearchUsesTermIndex(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "", uow.Options{})
	rows, err := db.SQLDB().QueryContext(t.Context(), `EXPLAIN QUERY PLAN
SELECT t.account_id, t.kind, t.term, t.is_local, a.domain
FROM account_search_terms t JOIN accounts a ON a.id = t.account_id
WHERE t.term >= ? AND t.term < ?
ORDER BY t.term, t.account_id
LIMIT ?`, "al", "al\U0010FFFF", 100)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan, "\n")
	t.Logf("plan:\n%s", joined)
	if !strings.Contains(joined, "USING INDEX account_search_terms_term_account_id (term>? AND term<?)") || strings.Contains(joined, "TEMP B-TREE") {
		t.Errorf("plan =\n%s\nwant a range of account_search_terms_term_account_id without sorting", joined)
	}
}
