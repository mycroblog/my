// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"testing"

	postgrescontent "github.com/mycroblog/my/internal/adapter/db/postgres/content"
	postgresgraph "github.com/mycroblog/my/internal/adapter/db/postgres/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/timelinetest"
)

func TestTimelineContract(t *testing.T) {
	timelinetest.TimelineContract(t, func(t *testing.T, opts uow.Options) timelinetest.TimelineHarness {
		db := newTestDB(t, opts)
		return timelinetest.TimelineHarness{
			UnitOfWork: db, Statuses: postgrescontent.NewStatusRepository(db), Interactions: postgrescontent.NewInteractionRepository(db),
			Graph: postgresgraph.NewRepository(db), Timeline: NewRepository(db),
		}
	})
}
