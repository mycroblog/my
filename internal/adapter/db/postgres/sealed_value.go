// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/postgres/accountgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/instancegen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/webhookgen"
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
	sealedInstanceSecrets       = "instance_secrets.sealed_secret"
	sealedAccountPrivateKeys    = "local_account_private_keys.private_key_enc"
	sealedLoginProviderSecrets  = "login_provider_secrets.sealed_secret"
	sealedTOTPEnrollmentSecrets = "totp_enrollments.secret_enc"
	sealedWebhookSecrets        = "webhook_secrets.sealed_secret"
)

// Columns implements rewrap.Store.
func (s *SealedValueStore) Columns() []string {
	return []string{sealedInstanceSecrets, sealedAccountPrivateKeys, sealedLoginProviderSecrets, sealedTOTPEnrollmentSecrets, sealedWebhookSecrets}
}

// List implements rewrap.Store.
func (s *SealedValueStore) List(ctx context.Context, column, after string, limit int) ([]rewrap.Value, error) {
	pageLimit, err := toInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	q := s.db.reader()
	var out []rewrap.Value
	switch column {
	case sealedAccountPrivateKeys:
		var rows []accountgen.ListSealedAccountPrivateKeysRow
		rows, err = q.ListSealedAccountPrivateKeys(ctx, accountgen.ListSealedAccountPrivateKeysParams{After: toNullString(after), PageLimit: pageLimit})
		for _, r := range rows {
			out = append(out, rewrap.Value{RowID: r.RowID, Sealed: r.Sealed})
		}
	case sealedLoginProviderSecrets:
		var rows []identitygen.ListSealedLoginProviderSecretsRow
		rows, err = q.ListSealedLoginProviderSecrets(ctx, identitygen.ListSealedLoginProviderSecretsParams{After: toNullString(after), PageLimit: pageLimit})
		for _, r := range rows {
			out = append(out, rewrap.Value{RowID: r.RowID, Sealed: r.Sealed})
		}
	case sealedTOTPEnrollmentSecrets:
		var rows []identitygen.ListSealedTOTPEnrollmentSecretsRow
		rows, err = q.ListSealedTOTPEnrollmentSecrets(ctx, identitygen.ListSealedTOTPEnrollmentSecretsParams{After: toNullString(after), PageLimit: pageLimit})
		for _, r := range rows {
			out = append(out, rewrap.Value{RowID: r.RowID, Sealed: r.Sealed})
		}
	case sealedInstanceSecrets:
		var rows []instancegen.ListSealedInstanceSecretsRow
		rows, err = q.ListSealedInstanceSecrets(ctx, instancegen.ListSealedInstanceSecretsParams{After: toNullString(after), PageLimit: pageLimit})
		for _, r := range rows {
			out = append(out, rewrap.Value{RowID: r.RowID, Sealed: r.Sealed})
		}
	case sealedWebhookSecrets:
		var rows []webhookgen.ListSealedWebhookSecretsRow
		rows, err = q.ListSealedWebhookSecrets(ctx, webhookgen.ListSealedWebhookSecretsParams{After: toNullString(after), PageLimit: pageLimit})
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
		n, err = q.ReplaceSealedAccountPrivateKeys(ctx, accountgen.ReplaceSealedAccountPrivateKeysParams{Sealed: sealed, RowID: rowID, OldSealed: old})
	case sealedLoginProviderSecrets:
		n, err = q.ReplaceSealedLoginProviderSecrets(ctx, identitygen.ReplaceSealedLoginProviderSecretsParams{Sealed: sealed, RowID: rowID, OldSealed: old})
	case sealedTOTPEnrollmentSecrets:
		n, err = q.ReplaceSealedTOTPEnrollmentSecrets(ctx, identitygen.ReplaceSealedTOTPEnrollmentSecretsParams{Sealed: sealed, RowID: rowID, OldSealed: old})
	case sealedInstanceSecrets:
		n, err = q.ReplaceSealedInstanceSecrets(ctx, instancegen.ReplaceSealedInstanceSecretsParams{Sealed: sealed, RowID: rowID, OldSealed: old})
	case sealedWebhookSecrets:
		n, err = q.ReplaceSealedWebhookSecrets(ctx, webhookgen.ReplaceSealedWebhookSecretsParams{Sealed: sealed, RowID: rowID, OldSealed: old})
	default:
		return false, fmt.Errorf("%w: %q", rewrap.ErrUnknownColumn, column)
	}
	if err != nil {
		return false, wrap(err, "replace sealed value of "+column)
	}
	return n > 0, nil
}
