// SPDX-License-Identifier: AGPL-3.0-only

// Package timeline_test runs the timeline flows (timelinetest.TimelineFlows) once, on SQLite
// (ADR-0153 §1).
package timeline_test

import (
	"testing"

	sqlitecontent "github.com/mycroblog/my/internal/adapter/db/sqlite/content"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqlitetimeline "github.com/mycroblog/my/internal/adapter/db/sqlite/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/timelinetest"
)

func TestTimelineFlows(t *testing.T) {
	t.Parallel()
	timelinetest.TimelineFlows(t, func(t *testing.T, opts uow.Options) timelinetest.TimelineHarness {
		db := flowtest.SQLite(t, opts)
		return timelinetest.TimelineHarness{
			UnitOfWork: db, Statuses: sqlitecontent.NewStatusRepository(db), Interactions: sqlitecontent.NewInteractionRepository(db),
			Graph: sqlitegraph.NewRepository(db), Timeline: sqlitetimeline.NewRepository(db),
		}
	})
}
