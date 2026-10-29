// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/rewrap"
	appwebhook "github.com/mycroblog/my/internal/app/webhook"
	domidentity "github.com/mycroblog/my/internal/domain/identity"
)

// sealedColumns are the columns of the modules that hold values sealed with the Sealer, each
// with the AAD that its module binds a value to. The store of the database lists the same
// columns, and its contract checks them against the schema, so rewrap.NewService panics when a
// module adds a sealed column without adding it here.
var sealedColumns = []rewrap.Column{
	{Name: "instance_secrets.sealed_secret", AAD: appinstance.SecretAAD},
	{Name: "local_account_private_keys.private_key_enc", AAD: appaccount.PrivateKeyAAD},
	{Name: domidentity.LoginProviderSecretColumn, AAD: appidentity.LoginProviderSecretAAD},
	{Name: domidentity.TOTPSecretColumn, AAD: appidentity.TOTPSecretAAD},
	{Name: "webhook_secrets.sealed_secret", AAD: appwebhook.SecretAAD},
}

// setUpRewrap builds the service of `my kek`, which re-wraps the stored sealed values under the
// current KEK after a rotation.
func (a *App) setUpRewrap(store rewrap.Store) {
	a.Rewrap = rewrap.NewService(rewrap.Options{
		UnitOfWork: a.UnitOfWork, Store: store, Sealer: a.Sealer, Columns: sealedColumns,
	})
}
