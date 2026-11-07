// SPDX-License-Identifier: AGPL-3.0-only

package diagnostics

import (
	"testing"

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
