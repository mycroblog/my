// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestTracing checks the database spans of ADR-0063 §1: one per transaction attempt and one
// per query of the pool (pgx's QueryTracer), named after the sqlc query or the SQL keyword,
// with the database system and the query's name only.
func TestTracing(t *testing.T) {
	db := newTestDB(t, uow.Options{})
	tr := testkit.NewTracer()
	db.SetTracer(tr)
	ctx, parent := tr.Start(t.Context(), kernel.SpanSpec{Name: "POST /api/v1/statuses"}, kernel.Attrs{})
	if err := db.Tx(ctx, func(tx uow.Tx) error {
		q, err := db.queries(tx)
		if err != nil {
			return err
		}
		return insertProbe(ctx, q, testkit.NamedID("alice"))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.reader().CountLocalAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	parent.End("")

	txs := tr.Named("my.db.transaction")
	if len(txs) != 1 || txs[0].Attrs["db.system.name"] != "postgresql" || !txs[0].Ended || txs[0].ErrorType != "" {
		t.Errorf("transaction spans = %+v", txs)
	}
	for _, name := range []string{"InsertUser", "CountLocalAccounts"} {
		qs := tr.Named(name)
		if len(qs) != 1 || qs[0].Kind != kernel.SpanClient || qs[0].Attrs["db.operation.name"] != name || len(qs[0].Attrs) != 2 {
			t.Errorf("%s spans = %+v", name, qs)
		}
	}
	for _, s := range tr.Spans() {
		if !s.Ended {
			t.Errorf("span %s did not end", s.Name)
		}
	}
	if err := tr.Err(); err != nil {
		t.Error(err)
	}
}
