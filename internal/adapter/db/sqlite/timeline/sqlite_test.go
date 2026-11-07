// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"testing"

	sqlitecontent "github.com/mycroblog/my/internal/adapter/db/sqlite/content"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/timelinetest"
)

func TestTimelineContract(t *testing.T) {
	t.Parallel()
	timelinetest.TimelineContract(t, func(t *testing.T, opts uow.Options) timelinetest.TimelineHarness {
		db, _ := newTestDB(t, "", opts)
		return timelinetest.TimelineHarness{
			UnitOfWork: db, Statuses: sqlitecontent.NewStatusRepository(db), Interactions: sqlitecontent.NewInteractionRepository(db),
			Graph: sqlitegraph.NewRepository(db), Timeline: NewRepository(db),
		}
	})
}
