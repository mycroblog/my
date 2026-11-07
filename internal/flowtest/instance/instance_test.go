// SPDX-License-Identifier: AGPL-3.0-only

// Package instance_test runs the instance flows (testkit.InstanceFlows) once, on SQLite
// (ADR-0153 §1).
package instance_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestInstanceFlows(t *testing.T) {
	t.Parallel()
	testkit.InstanceFlows(t, func(t *testing.T, opts uow.Options) testkit.InstanceHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.InstanceHarness{UnitOfWork: db, Instance: db.InstanceRepository()}
	})
}
