// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitecontent "github.com/mycroblog/my/internal/adapter/db/sqlite/content"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	sqlitemoderation "github.com/mycroblog/my/internal/adapter/db/sqlite/moderation"
	sqlitenotification "github.com/mycroblog/my/internal/adapter/db/sqlite/notification"
	sqlitetimeline "github.com/mycroblog/my/internal/adapter/db/sqlite/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestRelayContract(t *testing.T) {
	t.Parallel()
	federationtest.RelayContract(t, func(t *testing.T, opts uow.Options) federationtest.RelayHarness {
		db, _ := newTestDB(t, "", opts)
		return federationtest.RelayHarness{UnitOfWork: db, Relays: NewRepository(db)}
	})
}

func TestFederationRepositoryContract(t *testing.T) {
	t.Parallel()
	federationtest.FederationRepositoryContract(t, func(t *testing.T, opts uow.Options) federationtest.FederationHarness {
		db, _ := newTestDB(t, "", opts)
		return federationtest.FederationHarness{UnitOfWork: db, Federation: NewRepository(db)}
	})
}

func TestInboxContract(t *testing.T) {
	t.Parallel()
	federationtest.InboxContract(t, func(t *testing.T, opts uow.Options) federationtest.InboxHarness {
		db, _ := newTestDB(t, "", opts)
		feds := NewRepository(db)
		return federationtest.InboxHarness{UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: sqliteaccount.NewRepository(db)}
	})
}

func TestInboxProcessingContract(t *testing.T) {
	t.Parallel()
	federationtest.InboxProcessingContract(t, func(t *testing.T, opts uow.Options) federationtest.InboxHarness {
		db, _ := newTestDB(t, "", opts)
		feds := NewRepository(db)
		return federationtest.InboxHarness{
			UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: sqliteaccount.NewRepository(db), Statuses: sqlitecontent.NewStatusRepository(db),
			Graph: sqlitegraph.NewRepository(db), Moderation: sqlitemoderation.NewRepository(db),
		}
	})
}

func TestRemoteStatusContract(t *testing.T) {
	t.Parallel()
	federationtest.RemoteStatusContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteStatusHarness {
		db, _ := newTestDB(t, "", opts)
		return federationtest.RemoteStatusHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
			Graph:          sqlitegraph.NewRepository(db),
			Statuses:       sqlitecontent.NewStatusRepository(db),
			Media:          sqlitecontent.NewMediaRepository(db),
			Timeline:       sqlitetimeline.NewRepository(db),
			Notifications:  sqlitenotification.NewRepository(db),
			Inbox:          NewRepository(db),
			Federation:     NewRepository(db),
			Instances:      NewRepository(db),
			Pins:           sqlitecontent.NewInteractionRepository(db),
			Quotes:         sqlitecontent.NewStatusRepository(db),
		}
	})
}

func TestRetentionContract(t *testing.T) {
	t.Parallel()
	federationtest.RetentionContract(t, func(t *testing.T, opts uow.Options) federationtest.RetentionHarness {
		db, _ := newTestDB(t, "", opts)
		return federationtest.RetentionHarness{
			RemoteStatusHarness: federationtest.RemoteStatusHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
				Graph:          sqlitegraph.NewRepository(db),
				Statuses:       sqlitecontent.NewStatusRepository(db),
				Media:          sqlitecontent.NewMediaRepository(db),
				Timeline:       sqlitetimeline.NewRepository(db),
				Notifications:  sqlitenotification.NewRepository(db),
				Inbox:          NewRepository(db),
			},
			Interactions: sqlitecontent.NewInteractionRepository(db),
			Federation:   NewRepository(db),
			Retention:    sqlitecontent.NewStatusRepository(db),
			Purges:       NewRepository(db),
			Instances:    NewRepository(db),
			Profiles:     sqliteaccount.NewRepository(db),
			Emojis:       sqlitecontent.NewCustomEmojiRepository(db),
		}
	})
}

func TestRemoteFollowContract(t *testing.T) {
	t.Parallel()
	federationtest.RemoteFollowContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteFollowHarness {
		db, _ := newTestDB(t, "", opts)
		feds := NewRepository(db)
		return federationtest.RemoteFollowHarness{
			FollowHarness: graphtest.FollowHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
				Graph:          sqlitegraph.NewRepository(db),
			},
			Notifications: sqlitenotification.NewRepository(db),
			Federation:    feds,
			Inbox:         feds,
		}
	})
}

func TestAccountMoveContract(t *testing.T) {
	t.Parallel()
	federationtest.AccountMoveContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteFollowHarness {
		db, _ := newTestDB(t, "", opts)
		feds := NewRepository(db)
		return federationtest.RemoteFollowHarness{
			FollowHarness: graphtest.FollowHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
				Graph:          sqlitegraph.NewRepository(db),
			},
			Notifications: sqlitenotification.NewRepository(db),
			Federation:    feds,
			Inbox:         feds,
		}
	})
}

func TestRemoteActorContract(t *testing.T) {
	t.Parallel()
	federationtest.RemoteActorContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteActorHarness {
		db, _ := newTestDB(t, "", opts)
		feds := NewRepository(db)
		return federationtest.RemoteActorHarness{UnitOfWork: db, Accounts: sqliteaccount.NewRepository(db), Federation: feds, Instances: feds}
	})
}

func TestRemoteInteractionContract(t *testing.T) {
	t.Parallel()
	federationtest.RemoteInteractionContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteInteractionHarness {
		db, _ := newTestDB(t, "", opts)
		feds := NewRepository(db)
		return federationtest.RemoteInteractionHarness{
			RemoteStatusHarness: federationtest.RemoteStatusHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
				Graph:          sqlitegraph.NewRepository(db),
				Statuses:       sqlitecontent.NewStatusRepository(db),
				Media:          sqlitecontent.NewMediaRepository(db),
				Timeline:       sqlitetimeline.NewRepository(db),
				Notifications:  sqlitenotification.NewRepository(db),
				Inbox:          feds,
				Quotes:         sqlitecontent.NewStatusRepository(db),
			},
			Interactions: sqlitecontent.NewInteractionRepository(db),
			Federation:   feds,
		}
	})
}
