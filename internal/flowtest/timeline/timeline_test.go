// SPDX-License-Identifier: AGPL-3.0-only

// Package timeline_test runs the timeline flows (testkit.TimelineFlows) once, on SQLite
// (ADR-0153 §1).
package timeline_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestTimelineFlows(t *testing.T) {
	t.Parallel()
	testkit.TimelineFlows(t, func(t *testing.T, opts uow.Options) testkit.TimelineHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.TimelineHarness{
			UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository(),
			Graph: db.GraphRepository(), Timeline: db.TimelineRepository(),
		}
	})
}
