// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"testing"

	postgresaccount "github.com/mycroblog/my/internal/adapter/db/postgres/account"
	postgresgraph "github.com/mycroblog/my/internal/adapter/db/postgres/graph"
	postgresidentity "github.com/mycroblog/my/internal/adapter/db/postgres/identity"
	postgresinstance "github.com/mycroblog/my/internal/adapter/db/postgres/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/contenttest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestMediaRepositoryContract(t *testing.T) {
	contenttest.MediaRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.MediaHarness {
		db := newTestDB(t, opts)
		return contenttest.MediaHarness{UnitOfWork: db, Media: NewMediaRepository(db), Statuses: NewStatusRepository(db)}
	})
}

func TestLinkCardRepositoryContract(t *testing.T) {
	contenttest.LinkCardRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.LinkCardHarness {
		db := newTestDB(t, opts)
		return contenttest.LinkCardHarness{UnitOfWork: db, Cards: NewStatusRepository(db), Media: NewMediaRepository(db)}
	})
}

func TestCustomEmojiRepositoryContract(t *testing.T) {
	contenttest.CustomEmojiRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.CustomEmojiHarness {
		db := newTestDB(t, opts)
		return contenttest.CustomEmojiHarness{
			UnitOfWork: db, Media: NewMediaRepository(db), Statuses: NewStatusRepository(db), Emojis: NewCustomEmojiRepository(db),
		}
	})
}

func TestStatusRepositoryContract(t *testing.T) {
	contenttest.StatusRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.StatusHarness {
		db := newTestDB(t, opts)
		return contenttest.StatusHarness{UnitOfWork: db, Statuses: NewStatusRepository(db)}
	})
}

func TestInteractionRepositoryContract(t *testing.T) {
	contenttest.InteractionRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.InteractionHarness {
		db := newTestDB(t, opts)
		return contenttest.InteractionHarness{UnitOfWork: db, Statuses: NewStatusRepository(db), Interactions: NewInteractionRepository(db)}
	})
}

func TestAutoDeleteContract(t *testing.T) {
	contenttest.AutoDeleteContract(t, func(t *testing.T, opts uow.Options) contenttest.AutoDeleteHarness {
		db := newTestDB(t, opts)
		return contenttest.AutoDeleteHarness{
			InteractionHarness: contenttest.InteractionHarness{UnitOfWork: db, Statuses: NewStatusRepository(db), Interactions: NewInteractionRepository(db)},
			AutoDeletes:        NewStatusRepository(db),
		}
	})
}

func TestPostingContract(t *testing.T) {
	contenttest.PostingContract(t, func(t *testing.T, opts uow.Options) contenttest.PostingHarness {
		db := newTestDB(t, opts)
		return contenttest.PostingHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: postgresidentity.NewRepository(db), Accounts: postgresaccount.NewRepository(db)},
			Graph:          postgresgraph.NewRepository(db),
			Instance:       postgresinstance.NewRepository(db),
			Statuses:       NewStatusRepository(db),
			Media:          NewMediaRepository(db),
		}
	})
}
