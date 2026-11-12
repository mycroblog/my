// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/diagnostics"
)

// setUpDiagnostics builds the error groups (ADR-0103 §4 (2), §7): the Service with its projector
// and the daily prune of the counts, and the Recorder that the ErrorReporter hands every report
// to; and the UX measurements of the web client (ADR-0167 §3) with the daily prune of their
// counts. App.Close flushes the Recorder and the UX measurements before the database closes.
func (a *App) setUpDiagnostics(repo diagnostics.Repository, ux diagnostics.UXRepository) {
	diagnostics.RegisterProjectors(a.Projectors, repo)
	a.Diagnostics = diagnostics.NewService(diagnostics.Deps{UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, IDs: a.IDs})
	a.Diagnostics.RegisterJobs(a.Handlers, a.Scheduler)
	a.ErrorReporter.AddSink(diagnostics.NewRecorder(diagnostics.RecorderOptions{
		Service: a.Diagnostics, Clock: a.Clock, Reporter: a.ErrorReporter,
	}))
	a.UXMeasurements = diagnostics.NewUX(diagnostics.UXOptions{Repository: ux, Clock: a.Clock, Meter: a.Meter, Reporter: a.ErrorReporter})
	a.UXMeasurements.RegisterJobs(a.Handlers, a.Scheduler)
}

// closeUXMeasurements writes the UX measurements counted in memory while the database is still
// open.
func (a *App) closeUXMeasurements() {
	if a.UXMeasurements == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), errorReporterCloseTimeout)
	defer cancel()
	_ = a.UXMeasurements.Close(ctx) // the counts of the last minute are lost when the database is down
}
