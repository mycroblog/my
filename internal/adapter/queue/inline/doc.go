// SPDX-License-Identifier: AGPL-3.0-only

// Package inline implements the job queue ports in the memory of the enqueuing process, for
// tests and development (ADR-0004). Jobs do not survive the process.
//
// Queue implements queue.JobQueue, queue.Source for queue.PullRunner, queue.DeadJobs, and
// queue.JobInspector for the queue dashboard (ADR-0064), with
// the same lease, retry, dead-job, and ordering-key semantics as the database queue. After RunSynchronously,
// Enqueue runs the due jobs with the given runner before it returns, and RunUntil runs the jobs
// that become due later in the same way, never concurrently with the run of an Enqueue. Enqueue
// returns an error only when it enqueued no job: a failure of the run after the jobs were stored
// is left to the runner's log and the next run.
//
// Options.MaxDelay emulates a queue service with a delay limit (SQS allows 15 minutes): jobs
// further in the future become visible at the limit, and the runner re-enqueues them until
// their RunAt (the re-enqueue chain).
package inline
