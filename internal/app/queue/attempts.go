// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"time"
	"unicode/utf8"
)

// MaxAttemptErrorBytes bounds the Error of an Attempt (ADR-0004 §6): longer errors keep their
// first bytes, cut at a UTF-8 boundary.
const MaxAttemptErrorBytes = 2000

// Attempt is the record of one run of a job that did not succeed (ADR-0004 §5): the runner
// records a retry and a dead job, never a success. It carries neither the payload nor the
// handler's logs; Error is the handler's error, which never holds an email address, an IP
// address, a token, or a post's text.
type Attempt struct {
	JobID   string
	JobType string
	// Attempt is the number of the run, from 1. A requeued dead job counts its runs again, so
	// one job can have two records of the same Attempt; FinishedAt tells them apart.
	Attempt int
	// Outcome is OutcomeRetry or OutcomeDead.
	Outcome string
	// Error is why the run failed, at most MaxAttemptErrorBytes.
	Error string
	// Worker names the runner that ran the job.
	Worker     string
	StartedAt  time.Time
	FinishedAt time.Time
}

// Position returns the position of a in the order of AttemptLog.ListAttempts.
func (a Attempt) Position() AttemptPosition {
	return AttemptPosition{FinishedAt: a.FinishedAt, Attempt: a.Attempt}
}

// AttemptPosition is a position in the order of AttemptLog.ListAttempts: the most recently
// finished first, and by descending Attempt among those that finished at the same time. The
// zero value is the position before the first record.
type AttemptPosition struct {
	FinishedAt time.Time
	Attempt    int
}

// AttemptLog keeps the records of the runs that did not succeed, for the queue dashboard
// (ADR-0004). It lives in the application database whatever the queue adapter, as dead jobs do.
type AttemptLog interface {
	// RecordAttempt stores a. Recording the same run again (same JobID, Attempt, and
	// FinishedAt) keeps one record.
	RecordAttempt(ctx context.Context, a Attempt) error
	// ListAttempts returns up to limit records of the job jobID, the most recently finished
	// first, starting after the position after (the zero value for the first); none when
	// limit <= 0.
	ListAttempts(ctx context.Context, jobID string, after AttemptPosition, limit int) ([]Attempt, error)
	// DeleteAttemptsBefore deletes at most limit records that finished before t, the oldest
	// first, and returns how many it deleted; none when limit <= 0. It is safe to repeat.
	DeleteAttemptsBefore(ctx context.Context, t time.Time, limit int) (int64, error)
}

// TruncateAttemptError returns s cut to at most MaxAttemptErrorBytes, at a UTF-8 boundary.
func TruncateAttemptError(s string) string {
	if len(s) <= MaxAttemptErrorBytes {
		return s
	}
	n := MaxAttemptErrorBytes
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
