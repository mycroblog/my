// SPDX-License-Identifier: AGPL-3.0-only

package diagnostics

import (
	"testing"

	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/diagnosticstest"
)

func TestDiagnosticsRepositoryContract(t *testing.T) {
	t.Parallel()
	diagnosticstest.DiagnosticsRepositoryContract(t, func(t *testing.T, opts uow.Options) diagnosticstest.DiagnosticsHarness {
		db, _ := newTestDB(t, "", opts)
		return diagnosticstest.DiagnosticsHarness{UnitOfWork: db, Diagnostics: NewRepository(db)}
	})
}

func TestUXRepositoryContract(t *testing.T) {
	t.Parallel()
	diagnosticstest.UXRepositoryContract(t, func(t *testing.T) appdiagnostics.UXRepository {
		db, _ := newTestDB(t, "", uow.Options{})
		return NewRepository(db)
	})
}
