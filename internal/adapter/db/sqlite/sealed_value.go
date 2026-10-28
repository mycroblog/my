// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/app/rewrap"
	"github.com/mycroblog/my/internal/app/uow"
)

// SealedValueStore implements rewrap.Store on the sealed columns of the modules.
type SealedValueStore struct{ db *DB }

var _ rewrap.Store = (*SealedValueStore)(nil)

// SealedValueStore returns the store of the sealed values that `my kek rewrap` re-wraps.
func (d *DB) SealedValueStore() *SealedValueStore { return &SealedValueStore{db: d} }

// The sealed columns, as "table.column".
const (
	sealedAccountPrivateKeys    = "local_account_private_keys.private_key_enc"
	sealedLoginProviderSecrets  = "login_provider_secrets.sealed_secret"
	sealedTOTPEnrollmentSecrets = "totp_enrollments.secret_enc"
	sealedWebhookSecrets        = "webhook_secrets.sealed_secret"
)

// Columns implements rewrap.Store.
func (s *SealedValueStore) Columns() []string {
	return []string{sealedAccountPrivateKeys, sealedLoginProviderSecrets, sealedTOTPEnrollmentSecrets, sealedWebhookSecrets}
}

// List implements rewrap.Store.
func (s *SealedValueStore) List(ctx context.Context, column, after string, limit int) ([]rewrap.Value, error) {
	q := s.db.reader()
	var out []rewrap.Value
	var err error
	switch column {
	case sealedAccountPrivateKeys:
		var rows []sqlitegen.ListSealedAccountPrivateKeysRow
		rows, err = q.ListSealedAccountPrivateKeys(ctx, sqlitegen.ListSealedAccountPrivateKeysParams{After: after, PageLimit: int64(limit)})
		for _, r := range rows {
			out = append(out, rewrap.Value{RowID: r.RowID, Sealed: r.Sealed})
		}
	case sealedLoginProviderSecrets:
		var rows []sqlitegen.ListSealedLoginProviderSecretsRow
		rows, err = q.ListSealedLoginProviderSecrets(ctx, sqlitegen.ListSealedLoginProviderSecretsParams{After: after, PageLimit: int64(limit)})
		for _, r := range rows {
			out = append(out, rewrap.Value{RowID: r.RowID, Sealed: r.Sealed})
		}
	case sealedTOTPEnrollmentSecrets:
		var rows []sqlitegen.ListSealedTOTPEnrollmentSecretsRow
		rows, err = q.ListSealedTOTPEnrollmentSecrets(ctx, sqlitegen.ListSealedTOTPEnrollmentSecretsParams{After: after, PageLimit: int64(limit)})
		for _, r := range rows {
			out = append(out, rewrap.Value{RowID: r.RowID, Sealed: r.Sealed})
		}
	case sealedWebhookSecrets:
		var rows []sqlitegen.ListSealedWebhookSecretsRow
		rows, err = q.ListSealedWebhookSecrets(ctx, sqlitegen.ListSealedWebhookSecretsParams{After: after, PageLimit: int64(limit)})
		for _, r := range rows {
			out = append(out, rewrap.Value{RowID: r.RowID, Sealed: r.Sealed})
		}
	default:
		return nil, fmt.Errorf("%w: %q", rewrap.ErrUnknownColumn, column)
	}
	return out, wrap(err, "list sealed values of "+column)
}

// Replace implements rewrap.Store.
func (s *SealedValueStore) Replace(ctx context.Context, tx uow.Tx, column, rowID string, old, sealed []byte) (bool, error) {
	q, err := s.db.queries(tx)
	if err != nil {
		return false, err
	}
	var n int64
	switch column {
	case sealedAccountPrivateKeys:
		n, err = q.ReplaceSealedAccountPrivateKeys(ctx, sqlitegen.ReplaceSealedAccountPrivateKeysParams{Sealed: sealed, RowID: rowID, OldSealed: old})
	case sealedLoginProviderSecrets:
		n, err = q.ReplaceSealedLoginProviderSecrets(ctx, sqlitegen.ReplaceSealedLoginProviderSecretsParams{Sealed: sealed, RowID: rowID, OldSealed: old})
	case sealedTOTPEnrollmentSecrets:
		n, err = q.ReplaceSealedTOTPEnrollmentSecrets(ctx, sqlitegen.ReplaceSealedTOTPEnrollmentSecretsParams{Sealed: sealed, RowID: rowID, OldSealed: old})
	case sealedWebhookSecrets:
		n, err = q.ReplaceSealedWebhookSecrets(ctx, sqlitegen.ReplaceSealedWebhookSecretsParams{Sealed: sealed, RowID: rowID, OldSealed: old})
	default:
		return false, fmt.Errorf("%w: %q", rewrap.ErrUnknownColumn, column)
	}
	if err != nil {
		return false, wrap(err, "replace sealed value of "+column)
	}
	return n > 0, nil
}
