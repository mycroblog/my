// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"database/sql"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/instancetest"
)

func TestInstanceRepositoryContract(t *testing.T) {
	t.Parallel()
	instancetest.InstanceRepositoryContract(t, func(t *testing.T, opts uow.Options) instancetest.InstanceHarness {
		db, _ := newTestDB(t, "", opts)
		return instancetest.InstanceHarness{UnitOfWork: db, Instance: NewRepository(db), InsertLegacyTheme: insertLegacyTheme(db)}
	})
}

// insertLegacyTheme writes a theme revision as the releases before the token set stored it.
func insertLegacyTheme(db *sqlite.DB) func(ctx context.Context, rev instancetest.LegacyThemeRevision) error {
	return func(ctx context.Context, rev instancetest.LegacyThemeRevision) error {
		return db.Tx(ctx, func(tx uow.Tx) error {
			stx, err := db.SQLTx(tx)
			if err != nil {
				return err
			}
			by := sql.NullString{String: rev.RevisedByUserID, Valid: rev.RevisedByUserID != ""}
			if _, err := stx.ExecContext(ctx, "INSERT INTO instance_theme_revisions (id, custom_css, revised_by_user_id, revised_at) VALUES (?, ?, ?, ?)",
				rev.ID, rev.CustomCSS, by, sqlite.ToMillis(rev.RevisedAt)); err != nil {
				return err
			}
			for _, r := range rev.Rows {
				if _, err := stx.ExecContext(ctx, "INSERT INTO instance_theme_tokens (revision_id, token, scheme, value) VALUES (?, ?, ?, ?)",
					rev.ID, r.Token, string(r.Scheme), r.Value); err != nil {
					return err
				}
			}
			_, err = stx.ExecContext(ctx, "INSERT INTO instance_theme (id, revision_id) VALUES (1, ?) ON CONFLICT (id) DO UPDATE SET revision_id = excluded.revision_id", rev.ID)
			return err
		})
	}
}
