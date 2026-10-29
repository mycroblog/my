// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/schema"
)

// sealedValueInserts insert a row holding a sealed value, by column; the other columns hold any
// valid values.
var sealedValueInserts = map[string]string{ //nolint:gosec // G101: SQL statements, not credentials
	sealedInstanceSecrets:       `INSERT INTO instance_secrets (id, name, sealed_secret, created_at) VALUES (?1, ?1, ?2, 0)`,
	sealedAccountPrivateKeys:    `INSERT INTO local_account_private_keys (account_key_id, private_key_enc) VALUES (?1, ?2)`,
	sealedLoginProviderSecrets:  `INSERT INTO login_provider_secrets (id, provider_id, sealed_secret, created_at) VALUES (?1, ?1, ?2, 0)`,
	sealedTOTPEnrollmentSecrets: `INSERT INTO totp_enrollments (id, user_id, secret_enc, enrolled_at) VALUES (?1, ?1, ?2, 0)`,
	sealedWebhookSecrets:        `INSERT INTO webhook_secrets (id, endpoint_id, sealed_secret, created_at) VALUES (?1, ?1, ?2, 0)`,
}

func TestSealedValueStoreContract(t *testing.T) {
	testkit.SealedValueStoreContract(t, func(t *testing.T) testkit.SealedValueHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return testkit.SealedValueHarness{
			UnitOfWork: db, Store: db.SealedValueStore(), Schema: schema.SQLite,
			Insert: func(t *testing.T, column, rowID string, sealed []byte) {
				t.Helper()
				if _, err := db.db.ExecContext(t.Context(), sealedValueInserts[column], rowID, sealed); err != nil {
					t.Fatalf("insert into %s: %v", column, err)
				}
			},
		}
	})
}
