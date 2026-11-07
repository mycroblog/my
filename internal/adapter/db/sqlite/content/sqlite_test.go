// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	sqliteinstance "github.com/mycroblog/my/internal/adapter/db/sqlite/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/contenttest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestMediaRepositoryContract(t *testing.T) {
	t.Parallel()
	contenttest.MediaRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.MediaHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.MediaHarness{UnitOfWork: db, Media: NewMediaRepository(db), Statuses: NewStatusRepository(db)}
	})
}

func TestLinkCardRepositoryContract(t *testing.T) {
	t.Parallel()
	contenttest.LinkCardRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.LinkCardHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.LinkCardHarness{UnitOfWork: db, Cards: NewStatusRepository(db), Media: NewMediaRepository(db)}
	})
}

func TestCustomEmojiRepositoryContract(t *testing.T) {
	t.Parallel()
	contenttest.CustomEmojiRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.CustomEmojiHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.CustomEmojiHarness{
			UnitOfWork: db, Media: NewMediaRepository(db), Statuses: NewStatusRepository(db), Emojis: NewCustomEmojiRepository(db),
		}
	})
}

func TestStatusRepositoryContract(t *testing.T) {
	t.Parallel()
	contenttest.StatusRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.StatusHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.StatusHarness{UnitOfWork: db, Statuses: NewStatusRepository(db)}
	})
}

func TestInteractionRepositoryContract(t *testing.T) {
	t.Parallel()
	contenttest.InteractionRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.InteractionHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.InteractionHarness{UnitOfWork: db, Statuses: NewStatusRepository(db), Interactions: NewInteractionRepository(db)}
	})
}

func TestAutoDeleteContract(t *testing.T) {
	t.Parallel()
	contenttest.AutoDeleteContract(t, func(t *testing.T, opts uow.Options) contenttest.AutoDeleteHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.AutoDeleteHarness{
			InteractionHarness: contenttest.InteractionHarness{UnitOfWork: db, Statuses: NewStatusRepository(db), Interactions: NewInteractionRepository(db)},
			AutoDeletes:        NewStatusRepository(db),
		}
	})
}

func TestPostingContract(t *testing.T) {
	t.Parallel()
	contenttest.PostingContract(t, func(t *testing.T, opts uow.Options) contenttest.PostingHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.PostingHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
			Graph:          sqlitegraph.NewRepository(db),
			Instance:       sqliteinstance.NewRepository(db),
			Statuses:       NewStatusRepository(db),
			Media:          NewMediaRepository(db),
		}
	})
}
