// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"testing"

	postgresaccount "github.com/mycroblog/my/internal/adapter/db/postgres/account"
	postgrescontent "github.com/mycroblog/my/internal/adapter/db/postgres/content"
	postgresgraph "github.com/mycroblog/my/internal/adapter/db/postgres/graph"
	postgresidentity "github.com/mycroblog/my/internal/adapter/db/postgres/identity"
	postgresmoderation "github.com/mycroblog/my/internal/adapter/db/postgres/moderation"
	postgresnotification "github.com/mycroblog/my/internal/adapter/db/postgres/notification"
	postgrestimeline "github.com/mycroblog/my/internal/adapter/db/postgres/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestRelayContract(t *testing.T) {
	federationtest.RelayContract(t, func(t *testing.T, opts uow.Options) federationtest.RelayHarness {
		db := newTestDB(t, opts)
		return federationtest.RelayHarness{UnitOfWork: db, Relays: NewRepository(db)}
	})
}

func TestFederationRepositoryContract(t *testing.T) {
	federationtest.FederationRepositoryContract(t, func(t *testing.T, opts uow.Options) federationtest.FederationHarness {
		db := newTestDB(t, opts)
		return federationtest.FederationHarness{UnitOfWork: db, Federation: NewRepository(db)}
	})
}

func TestInboxContract(t *testing.T) {
	federationtest.InboxContract(t, func(t *testing.T, opts uow.Options) federationtest.InboxHarness {
		db := newTestDB(t, opts)
		feds := NewRepository(db)
		return federationtest.InboxHarness{UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: postgresaccount.NewRepository(db)}
	})
}

func TestInboxProcessingContract(t *testing.T) {
	federationtest.InboxProcessingContract(t, func(t *testing.T, opts uow.Options) federationtest.InboxHarness {
		db := newTestDB(t, opts)
		feds := NewRepository(db)
		return federationtest.InboxHarness{
			UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: postgresaccount.NewRepository(db), Statuses: postgrescontent.NewStatusRepository(db),
			Graph: postgresgraph.NewRepository(db), Moderation: postgresmoderation.NewRepository(db),
		}
	})
}

func TestRemoteStatusContract(t *testing.T) {
	federationtest.RemoteStatusContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteStatusHarness {
		db := newTestDB(t, opts)
		return federationtest.RemoteStatusHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: postgresidentity.NewRepository(db), Accounts: postgresaccount.NewRepository(db)},
			Graph:          postgresgraph.NewRepository(db),
			Statuses:       postgrescontent.NewStatusRepository(db),
			Media:          postgrescontent.NewMediaRepository(db),
			Timeline:       postgrestimeline.NewRepository(db),
			Notifications:  postgresnotification.NewRepository(db),
			Inbox:          NewRepository(db),
			Federation:     NewRepository(db),
			Instances:      NewRepository(db),
			Pins:           postgrescontent.NewInteractionRepository(db),
			Quotes:         postgrescontent.NewStatusRepository(db),
		}
	})
}

func TestRetentionContract(t *testing.T) {
	federationtest.RetentionContract(t, func(t *testing.T, opts uow.Options) federationtest.RetentionHarness {
		db := newTestDB(t, opts)
		return federationtest.RetentionHarness{
			RemoteStatusHarness: federationtest.RemoteStatusHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: postgresidentity.NewRepository(db), Accounts: postgresaccount.NewRepository(db)},
				Graph:          postgresgraph.NewRepository(db),
				Statuses:       postgrescontent.NewStatusRepository(db),
				Media:          postgrescontent.NewMediaRepository(db),
				Timeline:       postgrestimeline.NewRepository(db),
				Notifications:  postgresnotification.NewRepository(db),
				Inbox:          NewRepository(db),
			},
			Interactions: postgrescontent.NewInteractionRepository(db),
			Federation:   NewRepository(db),
			Retention:    postgrescontent.NewStatusRepository(db),
			Purges:       NewRepository(db),
			Instances:    NewRepository(db),
			Profiles:     postgresaccount.NewRepository(db),
			Emojis:       postgrescontent.NewCustomEmojiRepository(db),
		}
	})
}

func TestRemoteFollowContract(t *testing.T) {
	federationtest.RemoteFollowContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteFollowHarness {
		db := newTestDB(t, opts)
		feds := NewRepository(db)
		return federationtest.RemoteFollowHarness{
			FollowHarness: graphtest.FollowHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: postgresidentity.NewRepository(db), Accounts: postgresaccount.NewRepository(db)},
				Graph:          postgresgraph.NewRepository(db),
			},
			Notifications: postgresnotification.NewRepository(db),
			Federation:    feds,
			Inbox:         feds,
		}
	})
}

func TestAccountMoveContract(t *testing.T) {
	federationtest.AccountMoveContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteFollowHarness {
		db := newTestDB(t, opts)
		feds := NewRepository(db)
		return federationtest.RemoteFollowHarness{
			FollowHarness: graphtest.FollowHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: postgresidentity.NewRepository(db), Accounts: postgresaccount.NewRepository(db)},
				Graph:          postgresgraph.NewRepository(db),
			},
			Notifications: postgresnotification.NewRepository(db),
			Federation:    feds,
			Inbox:         feds,
		}
	})
}

func TestRemoteActorContract(t *testing.T) {
	federationtest.RemoteActorContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteActorHarness {
		db := newTestDB(t, opts)
		feds := NewRepository(db)
		return federationtest.RemoteActorHarness{UnitOfWork: db, Accounts: postgresaccount.NewRepository(db), Federation: feds, Instances: feds}
	})
}

func TestRemoteInteractionContract(t *testing.T) {
	federationtest.RemoteInteractionContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteInteractionHarness {
		db := newTestDB(t, opts)
		feds := NewRepository(db)
		return federationtest.RemoteInteractionHarness{
			RemoteStatusHarness: federationtest.RemoteStatusHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: postgresidentity.NewRepository(db), Accounts: postgresaccount.NewRepository(db)},
				Graph:          postgresgraph.NewRepository(db),
				Statuses:       postgrescontent.NewStatusRepository(db),
				Media:          postgrescontent.NewMediaRepository(db),
				Timeline:       postgrestimeline.NewRepository(db),
				Notifications:  postgresnotification.NewRepository(db),
				Inbox:          feds,
				Quotes:         postgrescontent.NewStatusRepository(db),
			},
			Interactions: postgrescontent.NewInteractionRepository(db),
			Federation:   feds,
		}
	})
}
