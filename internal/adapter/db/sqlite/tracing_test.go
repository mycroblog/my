// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestTracing checks the database spans of ADR-0063 §1: one per transaction attempt and one
// per query, named after the sqlc query, with the database system and the query's name only.
func TestTracing(t *testing.T) {
	db, _ := newTestDB(t, "", uow.Options{})
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
	if len(txs) != 1 || txs[0].Attrs["db.system.name"] != "sqlite" || !txs[0].Ended || txs[0].ErrorType != "" {
		t.Errorf("transaction spans = %+v", txs)
	}
	for _, name := range []string{"InsertUser", "CountLocalAccounts"} {
		qs := tr.Named(name)
		if len(qs) != 1 || qs[0].Kind != kernel.SpanClient || qs[0].Attrs["db.operation.name"] != name || len(qs[0].Attrs) != 2 {
			t.Errorf("%s spans = %+v", name, qs)
		}
	}
	if err := tr.Err(); err != nil {
		t.Error(err)
	}
}
