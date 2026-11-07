// SPDX-License-Identifier: AGPL-3.0-only

// Package instance_test runs the instance flows (instancetest.InstanceFlows) once, on SQLite
// (ADR-0153 §1).
package instance_test

import (
	"testing"

	sqliteinstance "github.com/mycroblog/my/internal/adapter/db/sqlite/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/instancetest"
)

func TestInstanceFlows(t *testing.T) {
	t.Parallel()
	instancetest.InstanceFlows(t, func(t *testing.T, opts uow.Options) instancetest.InstanceHarness {
		db := flowtest.SQLite(t, opts)
		return instancetest.InstanceHarness{UnitOfWork: db, Instance: sqliteinstance.NewRepository(db)}
	})
}
