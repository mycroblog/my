// SPDX-License-Identifier: AGPL-3.0-only

package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/diagnostics"
)

const (
	// JobPruneCounts deletes the hourly counts older than diagnostics.CountRetention.
	JobPruneCounts = "diagnostics.prune_error_counts"
	// PruneInterval is the period of the prune's schedule.
	PruneInterval = 24 * time.Hour
)

// Deps are the ports of a Service.
type Deps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
}

// Service runs the use cases of the error groups.
type Service struct{ d Deps }

// NewService returns the Service on d.
func NewService(d Deps) *Service { return &Service{d: d} }

// RecordFirst records the first occurrence of ev's fingerprint in this process (ADR-0103 §4 (2)):
// in one short transaction, it inserts the group and emits GroupCreated when the fingerprint is
// new, or records the regressed event and emits GroupRegressed when the group is resolved and
// ev's version differs from the version it was resolved with. Then it adds the occurrence to the
// hourly count and makes it the sample, each in one statement outside the transaction. An event
// without a fingerprint records nothing.
func (s *Service) RecordFirst(ctx context.Context, ev kernel.ErrorEvent) error {
	if ev.Fingerprint == "" {
		return nil
	}
	if ev.Time.IsZero() {
		ev.Time = s.d.Clock.Now()
	}
	g := groupOf(ev)
	g.ID = s.d.IDs.NewID()
	eventID := s.d.IDs.NewID()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		created, err := s.d.Repository.InsertGroup(ctx, tx, g)
		if err != nil {
			return err
		}
		if created {
			return tx.Emit(ctx, GroupCreated{Group: g})
		}
		cur, err := s.d.Repository.CurrentByFingerprint(ctx, tx, g.Fingerprint)
		if err != nil {
			return err
		}
		if !diagnostics.Regresses(cur.Head.State, cur.Head.ResolvedVersion, g.FirstVersion) {
			return nil
		}
		e := Event{
			ID: eventID, GroupID: cur.Group.ID, Kind: diagnostics.EventRegressed,
			Actor: Actor{Kind: diagnostics.ActorSystem}, Version: g.FirstVersion, OccurredAt: ev.Time,
		}
		if err := s.d.Repository.InsertEvent(ctx, tx, e); err != nil {
			return err
		}
		return tx.Emit(ctx, GroupRegressed{Group: cur.Group, Event: e})
	})
	if err != nil {
		return fmt.Errorf("diagnostics: record the group of %s: %w", g.Fingerprint, err)
	}
	return errors.Join(
		s.d.Repository.AddCount(ctx, countOf(ev, 1)),
		s.d.Repository.PutSample(ctx, sampleOf(ev)),
	)
}

// AddOccurrences adds counts to the hourly counts and puts samples, each in one statement
// outside any transaction. It writes every one it can and returns the failures joined.
func (s *Service) AddOccurrences(ctx context.Context, counts []Count, samples []Sample) error {
	var errs []error
	for _, c := range counts {
		if err := s.d.Repository.AddCount(ctx, c); err != nil {
			errs = append(errs, err)
		}
	}
	for _, sm := range samples {
		if err := s.d.Repository.PutSample(ctx, sm); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Resolve marks a group resolved with the version of its last occurrence: a later occurrence of
// another version regresses it. Mute stops its notifications, and Unmute starts them again.
// Each returns the group's state; a change that changes nothing (resolving a resolved group,
// unmuting a group that is not muted) records nothing. They fail with ErrNotFound for an
// unknown group and ErrInvalidActor for an actor that is not a user with an ID or the CLI.
func (s *Service) Resolve(ctx context.Context, groupID string, by Actor) (Head, error) {
	return s.change(ctx, groupID, diagnostics.EventResolved, by)
}

// Mute marks a group muted; see Resolve.
func (s *Service) Mute(ctx context.Context, groupID string, by Actor) (Head, error) {
	return s.change(ctx, groupID, diagnostics.EventMuted, by)
}

// Unmute opens a muted group again; see Resolve.
func (s *Service) Unmute(ctx context.Context, groupID string, by Actor) (Head, error) {
	return s.change(ctx, groupID, diagnostics.EventUnmuted, by)
}

func (s *Service) change(ctx context.Context, groupID string, kind diagnostics.EventKind, by Actor) (Head, error) {
	switch {
	case by.Kind == diagnostics.ActorUser && by.UserID != "":
	case by.Kind == diagnostics.ActorCLI && by.UserID == "":
	default:
		return Head{}, ErrInvalidActor
	}
	now := s.d.Clock.Now()
	eventID := s.d.IDs.NewID()
	var head Head
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		cur, err := s.d.Repository.CurrentForUpdate(ctx, tx, groupID)
		if err != nil {
			return err
		}
		head = cur.Head
		next, changed := diagnostics.Next(cur.Head.State, kind)
		if !changed {
			return nil
		}
		e := Event{ID: eventID, GroupID: groupID, Kind: kind, Actor: by, OccurredAt: now}
		head = Head{GroupID: groupID, State: next, Since: now}
		if kind == diagnostics.EventResolved {
			e.Version, head.ResolvedVersion = cur.LastVersion, cur.LastVersion
		}
		if err := s.d.Repository.InsertEvent(ctx, tx, e); err != nil {
			return err
		}
		return tx.Emit(ctx, GroupStateChanged{Event: e, Head: head})
	})
	if err != nil {
		return Head{}, fmt.Errorf("diagnostics: %s the group %s: %w", kind, groupID, err)
	}
	return head, nil
}

// Group returns a group with its state; it fails with ErrNotFound for an unknown group.
func (s *Service) Group(ctx context.Context, id string) (Group, Head, error) {
	return s.d.Repository.FindGroup(ctx, id)
}

// Sample returns the latest occurrence of a group; it fails with ErrNotFound when none is
// recorded.
func (s *Service) Sample(ctx context.Context, groupID string) (Sample, error) {
	return s.d.Repository.FindSample(ctx, groupID)
}

// HourCounts returns the hourly counts of a group, oldest first.
func (s *Service) HourCounts(ctx context.Context, groupID string) ([]HourCount, error) {
	return s.d.Repository.HourCounts(ctx, groupID)
}

// Events returns the changes of a group's state, oldest first.
func (s *Service) Events(ctx context.Context, groupID string) ([]Event, error) {
	return s.d.Repository.Events(ctx, groupID)
}

// PruneCounts deletes the hourly counts of the hours older than diagnostics.CountRetention and
// returns how many it deleted. It is safe to repeat.
func (s *Service) PruneCounts(ctx context.Context) (int64, error) {
	n, err := s.d.Repository.DeleteCountsBefore(ctx, diagnostics.Hour(s.d.Clock.Now().Add(-diagnostics.CountRetention)))
	if err != nil {
		return n, fmt.Errorf("diagnostics: prune the error counts: %w", err)
	}
	return n, nil
}

// RegisterJobs registers the JobPruneCounts handler and its daily schedule.
func (s *Service) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPruneCounts, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := s.PruneCounts(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobPruneCounts, Every: PruneInterval, Job: queue.NewJob{Type: JobPruneCounts}})
}

// groupOf returns the group (without its ID) that ev's first occurrence makes.
func groupOf(ev kernel.ErrorEvent) Group {
	source, kind := ev.Source, ev.Kind
	if source != kernel.ErrorSourceWeb {
		source = kernel.ErrorSourceServer
	}
	if kind != kernel.ErrorKindPanic {
		kind = kernel.ErrorKindError
	}
	return Group{
		Fingerprint: ev.Fingerprint, Source: string(source), Kind: string(kind),
		ErrorType: diagnostics.CleanText(diagnostics.LastType(ev.Type)), Message: diagnostics.MessageHead(ev.Message),
		Where: diagnostics.CleanText(ev.Where), FirstVersion: diagnostics.CleanText(ev.Version), FirstSeenAt: ev.Time,
	}
}

func countOf(ev kernel.ErrorEvent, n int64) Count {
	return Count{
		Fingerprint: ev.Fingerprint, Hour: diagnostics.Hour(ev.Time), Occurrences: n,
		LastSeenAt: ev.Time, LastVersion: diagnostics.CleanText(ev.Version),
	}
}

func sampleOf(ev kernel.ErrorEvent) Sample {
	return Sample{
		Fingerprint: ev.Fingerprint, ErrorType: diagnostics.CleanText(ev.Type), Message: diagnostics.CleanText(ev.Message),
		Stack: diagnostics.CleanText(ev.Stack), RequestID: diagnostics.CleanText(ev.RequestID), TraceID: diagnostics.CleanText(ev.TraceID),
		Version: diagnostics.CleanText(ev.Version), Role: diagnostics.CleanText(ev.Role), OccurredAt: ev.Time,
	}
}
