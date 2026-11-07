// SPDX-License-Identifier: AGPL-3.0-only

package rewrap

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/rewraptest"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
	"github.com/mycroblog/my/schema"
)

// sealedValueInserts insert a row holding a sealed value, by column; the other columns hold any
// valid values.
var sealedValueInserts = map[string]string{ //nolint:gosec // G101: SQL statements, not credentials
	"instance_secrets.sealed_secret":             `INSERT INTO instance_secrets (id, name, sealed_secret, created_at) VALUES (?1, ?1, ?2, 0)`,
	"local_account_private_keys.private_key_enc": `INSERT INTO local_account_private_keys (account_key_id, private_key_enc) VALUES (?1, ?2)`,
	"login_provider_secrets.sealed_secret":       `INSERT INTO login_provider_secrets (id, provider_id, sealed_secret, created_at) VALUES (?1, ?1, ?2, 0)`,
	"totp_enrollments.secret_enc":                `INSERT INTO totp_enrollments (id, user_id, secret_enc, enrolled_at) VALUES (?1, ?1, ?2, 0)`,
	"webhook_secrets.sealed_secret":              `INSERT INTO webhook_secrets (id, endpoint_id, sealed_secret, created_at) VALUES (?1, ?1, ?2, 0)`,
}

func TestSealedValueStoreContract(t *testing.T) {
	t.Parallel()
	rewraptest.SealedValueStoreContract(t, func(t *testing.T) rewraptest.SealedValueHarness {
		db, err := sqlite.Open(t.Context(), sqlitetest.NewFile(t), uow.Options{})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return rewraptest.SealedValueHarness{
			UnitOfWork: db, Store: NewSealedValueStore(db), Schema: schema.SQLite,
			Insert: func(t *testing.T, column, rowID string, sealed []byte) {
				t.Helper()
				if _, err := db.SQLDB().ExecContext(t.Context(), sealedValueInserts[column], rowID, sealed); err != nil {
					t.Fatalf("insert into %s: %v", column, err)
				}
			},
		}
	})
}
