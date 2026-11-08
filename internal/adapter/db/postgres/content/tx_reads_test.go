// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
)

// TestTxReadsUseTheTransactionsConnection runs the reads in a transaction on a pool of one
// connection, which the transaction holds: a read that took another connection would wait for the
// deadline and fail.
func TestTxReadsUseTheTransactionsConnection(t *testing.T) {
	dbURL, err := url.Parse(pgtest.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	q := dbURL.Query()
	q.Set("pool_max_conns", "1")
	dbURL.RawQuery = q.Encode()
	db, err := postgres.Open(t.Context(), dbURL.String(), uow.Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(db.Close)

	statuses := NewStatusRepository(db)
	interactions := NewInteractionRepository(db)
	id := testkit.NamedID("nothing")
	reads := map[string]func(ctx context.Context, tx uow.Tx) error{
		"FindCustomEmojiByShortcodeTx": func(ctx context.Context, tx uow.Tx) error {
			_, err := statuses.FindCustomEmojiByShortcodeTx(ctx, tx, "nothing", "example.com")
			return err
		},
		"FindRevisionTx": func(ctx context.Context, tx uow.Tx) error { _, err := statuses.FindRevisionTx(ctx, tx, id); return err },
		"FindDeletionByURITx": func(ctx context.Context, tx uow.Tx) error {
			_, err := statuses.FindDeletionByURITx(ctx, tx, "https://example.com/s")
			return err
		},
		"FindStatusByURITx": func(ctx context.Context, tx uow.Tx) error {
			_, err := statuses.FindStatusByURITx(ctx, tx, "https://example.com/s")
			return err
		},
		"FindBoostRetractionTx": func(ctx context.Context, tx uow.Tx) error {
			_, err := interactions.FindBoostRetractionTx(ctx, tx, id)
			return err
		},
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			err := db.Tx(ctx, func(tx uow.Tx) error { return read(ctx, tx) })
			if err != nil && !errors.Is(err, appcontent.ErrNotFound) {
				t.Errorf("%s: %v", name, err)
			}
		})
	}
}
