// SPDX-License-Identifier: AGPL-3.0-only

package translation

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/translationtest"
)

func TestTranslationRepositoryContract(t *testing.T) {
	t.Parallel()
	translationtest.TranslationRepositoryContract(t, func(t *testing.T) translationtest.TranslationHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return translationtest.TranslationHarness{UnitOfWork: db, Repository: NewRepository(db)}
	})
}
