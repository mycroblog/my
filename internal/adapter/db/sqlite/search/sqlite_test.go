// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/searchtest"
)

func TestSearchRepositoryContract(t *testing.T) {
	t.Parallel()
	searchtest.SearchRepositoryContract(t, func(t *testing.T) searchtest.SearchHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return searchtest.SearchHarness{UnitOfWork: db, Repository: NewRepository(db)}
	})
}
