// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
)

// JobState is the state of a job on the queue dashboard (ADR-0004 §7), derived from the job's
// current values: no column records it.
type JobState string

// The states of a job.
const (
	// JobStateQueued is a job that never ran and no worker holds: it waits for its RunAt or a
	// worker.
	JobStateQueued JobState = "queued"
	// JobStateRunning is a job whose lease holds.
	JobStateRunning JobState = "running"
	// JobStateRetrying is a job that ran at least once and waits to run again.
	JobStateRetrying JobState = "retrying"
	// JobStateDead is a dead job (DeadJobs).
	JobStateDead JobState = "dead"
)

// PendingStates are the states of the jobs not yet completed, in the order the dashboard lists
// them.
var PendingStates = []JobState{JobStateQueued, JobStateRunning, JobStateRetrying}

// Valid reports whether s is one of the states.
func (s JobState) Valid() bool {
	switch s {
	case JobStateQueued, JobStateRunning, JobStateRetrying, JobStateDead:
		return true
	}
	return false
}

// PendingJob is a job not yet completed, as JobInspector lists it.
type PendingJob struct {
	ID             string
	Type           string
	Payload        json.RawMessage
	IdempotencyKey string
	OrderingKey    string
	State          JobState
	// Attempt is the number of runs so far, the one in progress included.
	Attempt int
	// RunAt is when the job is due.
	RunAt time.Time
	// LastError is why the previous run did not finish, or empty.
	LastError string
	// Worker and LockedUntil are the lease of a running job; empty and zero otherwise.
	Worker      string
	LockedUntil time.Time
	CreatedAt   time.Time
}

// Position returns the position of j in the order of JobInspector.ListJobs.
func (j PendingJob) Position() JobPosition { return JobPosition{RunAt: j.RunAt, ID: j.ID} }

// JobPosition is a position in the order of JobInspector.ListJobs: the earliest RunAt first, and
// by ascending ID among the jobs due at the same time. The zero value is the position before the
// first job.
type JobPosition struct {
	RunAt time.Time
	ID    string
}

// JobFilter selects jobs: by state (empty for every pending state) and by type (empty for every
// type).
type JobFilter struct {
	State JobState
	Type  string
}

// JobCount is the number of jobs of one type in one state.
type JobCount struct {
	State JobState
	Type  string
	Count int64
}

// JobSummary is the state of the jobs of a queue at one time, for the dashboard.
type JobSummary struct {
	// Counts holds the jobs of every state, dead jobs included, by state and type, sorted by
	// state then type; states and types without jobs are left out.
	Counts []JobCount
	// OldestReadyAt is the earliest RunAt among the due jobs that no worker holds; zero when
	// none is.
	OldestReadyAt time.Time
	// OldestReadyByType is the same for each type with such a job (ADR-0004 §17).
	OldestReadyByType map[string]time.Time
}

// JobInspector reads the jobs a queue holds, for the dashboard (ADR-0004 §7). The database queue
// and the inline queue implement it. Cloud Tasks only signals: its jobs are in the
// database queue (ADR-0004 §9), so the jobs are listed with those queues too.
type JobInspector interface {
	// ListJobs returns up to limit pending jobs that match f at now, the earliest RunAt first,
	// starting after the position after; none when limit <= 0 or f.State is JobStateDead (list
	// dead jobs with DeadJobs).
	ListJobs(ctx context.Context, now time.Time, f JobFilter, after JobPosition, limit int) ([]PendingJob, error)
	// SummarizeJobs counts the jobs by state and type at now.
	SummarizeJobs(ctx context.Context, now time.Time) (JobSummary, error)
}

// StateAt returns the state at now of a pending job with the given attempt count and lease.
func StateAt(now time.Time, attempt int, lockedUntil time.Time) JobState {
	switch {
	case lockedUntil.After(now):
		return JobStateRunning
	case attempt == 0:
		return JobStateQueued
	default:
		return JobStateRetrying
	}
}

// SortJobCounts sorts c in the order of JobSummary.Counts: by state (queued, running,
// retrying, dead), then by type.
func SortJobCounts(c []JobCount) []JobCount {
	rank := func(s JobState) int {
		switch s {
		case JobStateQueued:
			return 0
		case JobStateRunning:
			return 1
		case JobStateRetrying:
			return 2
		}
		return 3
	}
	slices.SortFunc(c, func(a, b JobCount) int {
		if d := rank(a.State) - rank(b.State); d != 0 {
			return d
		}
		return strings.Compare(a.Type, b.Type)
	})
	return c
}
