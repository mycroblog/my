// SPDX-License-Identifier: AGPL-3.0-only

// Package diagnostics holds the use cases of the error groups (ADR-0103 §4 (2), §7): the
// Recorder that the ErrorReporter hands every report to, which groups the reports by their
// fingerprint in the database, the resolve, mute, and unmute of a group, the regression of a
// resolved group, and the daily prune of the hourly counts. The adapters of Repository live in
// internal/adapter/db/*; their contract test suite is testkit.DiagnosticsRepositoryContract.
package diagnostics

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/diagnostics"
)

// ErrNotFound reports an unknown error group.
var ErrNotFound = errors.New("diagnostics: not found")

// ErrInvalidActor reports an actor that cannot change a group's state: a user without an ID,
// another kind with one, or the system.
var ErrInvalidActor = errors.New("diagnostics: invalid actor")

// Group is an error group (error_groups): what its first occurrence told about the error.
type Group struct {
	ID          string
	Fingerprint string
	// Source is server or web, and Kind panic or error (kernel.ErrorSource, kernel.ErrorKind).
	Source string
	Kind   string
	// ErrorType is the last type of the chain, and Message the head of the scrubbed message
	// (diagnostics.MessageHead).
	ErrorType    string
	Message      string
	Where        string
	FirstVersion string
	// Unauthenticated is true when the first occurrence was a web report sent without a
	// session (kernel.ErrorEvent.Unauthenticated): the group notifies nobody until a report
	// with a session confirms it.
	Unauthenticated bool
	FirstSeenAt     time.Time
}

// Head is the current state of a group (error_group_states).
type Head struct {
	GroupID string
	State   diagnostics.State
	// ResolvedVersion is the version of the last occurrence when the group was resolved, ""
	// unless State is resolved.
	ResolvedVersion string
	// Since is when the state began.
	Since time.Time
}

// Current is a group with its state and the version of its last occurrence (its first version
// when no sample is recorded), read in the transaction of a change.
type Current struct {
	Group       Group
	Head        Head
	LastVersion string
}

// Actor is who changes a group's state: a user with UserID, or the CLI.
type Actor struct {
	Kind   diagnostics.ActorKind
	UserID string
}

// Event is a change of a group's state (error_group_events).
type Event struct {
	ID         string
	GroupID    string
	Kind       diagnostics.EventKind
	Actor      Actor
	Version    string
	OccurredAt time.Time
}

// Count is a number of occurrences of the group of Fingerprint in the hour Hour, the last of
// them at LastSeenAt with LastVersion: what one write adds to error_group_counts.
type Count struct {
	Fingerprint string
	Hour        time.Time
	Occurrences int64
	LastSeenAt  time.Time
	LastVersion string
}

// HourCount is the row of error_group_counts of one hour of a group.
type HourCount struct {
	Hour        time.Time
	Occurrences int64
	LastSeenAt  time.Time
	LastVersion string
}

// Sample is the latest occurrence of the group of Fingerprint (error_group_samples), scrubbed.
// RequestID and TraceID are "" for none.
type Sample struct {
	Fingerprint string
	ErrorType   string
	Message     string
	Stack       string
	RequestID   string
	TraceID     string
	Version     string
	Role        string
	OccurredAt  time.Time
}

// Repository is the persistence port of the diagnostics module.
type Repository interface {
	// InsertGroup inserts g and reports whether it did: a group of the same fingerprint that
	// exists already, or that a concurrent transaction inserts, writes nothing.
	InsertGroup(ctx context.Context, tx uow.Tx, g Group) (bool, error)
	// CurrentByFingerprint and CurrentForUpdate return a group with its state in tx, locking the
	// state on the databases that lock rows; they fail with ErrNotFound for an unknown group.
	CurrentByFingerprint(ctx context.Context, tx uow.Tx, fingerprint string) (Current, error)
	CurrentForUpdate(ctx context.Context, tx uow.Tx, groupID string) (Current, error)
	InsertEvent(ctx context.Context, tx uow.Tx, e Event) error
	// InsertConfirmation records the first occurrence with a session of an unauthenticated
	// group and reports whether it did: a group confirmed already writes nothing.
	InsertConfirmation(ctx context.Context, tx uow.Tx, groupID string, at time.Time) (bool, error)
	// UpsertHead sets the state of a group (the projector of the module's events).
	UpsertHead(ctx context.Context, tx uow.Tx, h Head) error

	// AddCount adds c to its group's count of its hour in one statement outside any
	// transaction, so that concurrent writers each add theirs; a fingerprint without a group
	// writes nothing.
	AddCount(ctx context.Context, c Count) error
	// PutSample makes s the sample of its group in one statement outside any transaction, unless
	// the sample there occurred later; a fingerprint without a group writes nothing.
	PutSample(ctx context.Context, s Sample) error
	// DeleteCountsBefore deletes the hourly counts of the hours that start before before, in one
	// statement outside any transaction, and returns how many it deleted.
	DeleteCountsBefore(ctx context.Context, before time.Time) (int64, error)

	// FindGroup returns a group with its state; it fails with ErrNotFound for an unknown group.
	FindGroup(ctx context.Context, id string) (Group, Head, error)
	// FindSample returns the sample of a group; it fails with ErrNotFound when there is none.
	FindSample(ctx context.Context, groupID string) (Sample, error)
	// HourCounts returns the hourly counts of a group, oldest first.
	HourCounts(ctx context.Context, groupID string) ([]HourCount, error)
	// Events returns the events of a group, oldest first.
	Events(ctx context.Context, groupID string) ([]Event, error)

	// RebuildHeads empties error_group_states and derives it from the groups and the newest
	// event of each (data-model §9).
	RebuildHeads(ctx context.Context, tx uow.Tx) error
}
