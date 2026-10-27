// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"github.com/mycroblog/my/internal/app/diagnostics"
)

// setUpDiagnostics builds the error groups (ADR-0103 §4 (2), §7): the Service with its projector
// and the daily prune of the counts, and the Recorder that the ErrorReporter hands every report
// to. App.Close flushes the Recorder before the database closes.
func (a *App) setUpDiagnostics(repo diagnostics.Repository) {
	diagnostics.RegisterProjectors(a.Projectors, repo)
	a.Diagnostics = diagnostics.NewService(diagnostics.Deps{UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, IDs: a.IDs})
	a.Diagnostics.RegisterJobs(a.Handlers, a.Scheduler)
	a.ErrorReporter.AddSink(diagnostics.NewRecorder(diagnostics.RecorderOptions{
		Service: a.Diagnostics, Clock: a.Clock, Reporter: a.ErrorReporter,
	}))
}
