// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"context"

	"github.com/mycroblog/my/internal/app/queue"
)

// The jobs of the search module (ADR-0149 §7). A job carries IDs only, never text.
const (
	// JobSyncStatus brings the document of one status in line with the database. Its payload
	// names the status.
	JobSyncStatus = "search.sync_status"
	// JobSyncAuthor brings the documents of one author in line with the database. Its payload
	// names the account.
	JobSyncAuthor = "search.sync_author"
	// JobSyncDomain enqueues JobSyncAuthor for each author of a domain. Its payload names the
	// domain.
	JobSyncDomain = "search.sync_domain"
	// JobNarrow removes the documents of a visibility that search_visibilities no longer
	// includes. Its payload names the visibility.
	JobNarrow = "search.narrow"
	// JobRebuild refills the index from the database in batches. Its payload is a cursor.
	JobRebuild = "search.rebuild"
)

// RegisterJobs registers the handlers of the jobs, with or without a backend: a type with no
// handler ends in a dead job, and worker_job_types refuses to start when a type it names does not
// exist (ADR-0149 §7). No code enqueues these jobs yet, so the handlers succeed doing nothing.
func (s *Service) RegisterJobs(reg *queue.JobHandlerRegistry) {
	doNothing := func(context.Context, queue.Job) queue.Result { return queue.Done() }
	reg.Register(JobSyncStatus, doNothing)
	reg.Register(JobSyncAuthor, doNothing)
	reg.Register(JobSyncDomain, doNothing)
	reg.Register(JobNarrow, doNothing)
	reg.Register(JobRebuild, doNothing)
}
