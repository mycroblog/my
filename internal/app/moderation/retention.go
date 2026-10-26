// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"
	"fmt"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The retention of the moderation records (ADR-0038, data-model §4.3, §8): the hourly
// JobPurgeRecords erases the actions on accounts with their appeals, the reports, and the audit
// log entries past the retention periods of the instance settings, and the sign-up blocks
// moderation.SignupBlockRetention after they end (internal/domain/moderation retention.go says
// when each period starts), in bounded batches like the other purges (FR-803). An
// operator's hold on an account stops the erasure of its records, of its sign-in and
// registration origins (the identity module asks HeldAccountIDs), and of its statuses (the
// content module asks StatusRetained), until it is released.

const (
	// JobPurgeRecords erases the moderation records past their retention; no payload.
	JobPurgeRecords = "moderation.purge_records"
	// RecordPurgeInterval is the period of the purge's schedule.
	RecordPurgeInterval = time.Hour
	// AuditPurgeBatch is the number of audit log entries one statement deletes; RecordPurgeBatch
	// the number of actions or reports, with their children, one transaction deletes.
	// recordPurgeMaxBatches bounds the batches of one kind of record in one run, and the next
	// run continues.
	AuditPurgeBatch       = 1000
	RecordPurgeBatch      = 100
	recordPurgeMaxBatches = 20
)

// RetentionPeriods are how long the moderation records are kept (the instance settings
// retention_reports, retention_moderation_actions, and retention_audit_log).
type RetentionPeriods struct {
	Reports  time.Duration
	Actions  time.Duration
	AuditLog time.Duration
}

// DefaultRetentionPeriods are the periods of an instance that never set them.
var DefaultRetentionPeriods = RetentionPeriods{
	Reports: moderation.DefaultReportRetention, Actions: moderation.DefaultActionRetention, AuditLog: moderation.DefaultAuditLogRetention,
}

// RetentionSettings reads the retention periods; the composition root implements it on the
// instance module, so that a change applies at the next run.
type RetentionSettings interface {
	RetentionPeriods(ctx context.Context) (RetentionPeriods, error)
}

// ReportHistory is what the moderators did with a report: its assignments, resolutions, and
// reopenings, each oldest first.
type ReportHistory struct {
	Assignments []moderation.ReportAssignment
	Resolutions []moderation.ReportResolution
	Reopenings  []moderation.ReportReopening
}

// RetentionRepository is the persistence port of the retention of the moderation records and
// of the holds, part of Repository.
type RetentionRepository interface {
	InsertRetentionHold(ctx context.Context, tx uow.Tx, h moderation.RetentionHold) error
	// InsertRetentionHoldRelease fails with ErrAlreadyRecorded when the hold is already
	// released.
	InsertRetentionHoldRelease(ctx context.Context, tx uow.Tx, r moderation.RetentionHoldRelease) error
	// RetentionHold returns a hold with its release inside tx.
	RetentionHold(ctx context.Context, tx uow.Tx, id string) (moderation.RetentionHoldRecord, error)
	// ListRetentionHolds returns the holds of an account (of every account when accountID is
	// empty) with their releases, newest first.
	ListRetentionHolds(ctx context.Context, accountID string) ([]moderation.RetentionHoldRecord, error)
	// HeldAccountIDs returns the accounts that a hold in effect preserves, in ID order.
	HeldAccountIDs(ctx context.Context) ([]string, error)
	// StatusRetained reports inside tx whether a report or an action on statuses points at the
	// status, or a hold in effect preserves its author.
	StatusRetained(ctx context.Context, tx uow.Tx, statusID, authorAccountID string) (bool, error)
	// ListActionsToPurge returns up to limit actions on accounts, oldest first, that ended
	// before endedBefore (internal/domain/moderation retention.go) and whose target no hold preserves.
	ListActionsToPurge(ctx context.Context, tx uow.Tx, endedBefore time.Time, limit int) ([]string, error)
	// PurgeActions deletes the actions with their statuses, lifts, appeals, the appeals'
	// decisions and states, and their restriction rows.
	PurgeActions(ctx context.Context, tx uow.Tx, ids []string) error
	// ListReportsToPurge returns up to limit reports, oldest first, resolved before
	// resolvedBefore and still resolved, that no action follows from and whose target and
	// reporter no hold preserves.
	ListReportsToPurge(ctx context.Context, tx uow.Tx, resolvedBefore time.Time, limit int) ([]string, error)
	// PurgeReports deletes the reports with their statuses, rule citations, voice chat rooms,
	// assignments, resolutions, reopenings, notes, and states.
	PurgeReports(ctx context.Context, tx uow.Tx, ids []string) error
	// ReportedVoiceRooms returns which of roomIDs a report points at (report_voice_rooms).
	ReportedVoiceRooms(ctx context.Context, roomIDs []string) (map[string]bool, error)
	// ListSignupBlocksToPurge returns up to limit sign-up blocks, oldest first, that ended (were
	// lifted or expired, whichever came first) before endedBefore, except those made by a ban
	// of an account that a hold preserves.
	ListSignupBlocksToPurge(ctx context.Context, tx uow.Tx, endedBefore time.Time, limit int) ([]string, error)
	// PurgeSignupBlocks deletes the sign-up blocks with their lifts.
	PurgeSignupBlocks(ctx context.Context, tx uow.Tx, ids []string) error
	// DeleteAuditEntriesBefore deletes inside tx at most limit audit log entries recorded before
	// t, the oldest first, in one statement, except those on an account that a hold preserves
	// or on a report or an appeal of such an account, and returns how many it deleted.
	DeleteAuditEntriesBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// ReportHistory returns the assignments, resolutions, and reopenings of a report.
	ReportHistory(ctx context.Context, reportID string) (ReportHistory, error)
}

// PlaceRetentionHold records a hold on the erasure of an account's records after their retention
// (ADR-0038); byUserID is the operator, "" for the CLI. An account may have several holds, each
// released on its own.
func (s *Service) PlaceRetentionHold(ctx context.Context, accountID, reason, byUserID string) (moderation.RetentionHold, error) {
	switch _, err := s.d.Accounts.FindByID(ctx, accountID); {
	case errors.Is(err, appaccount.ErrNotFound):
		return moderation.RetentionHold{}, fmt.Errorf("%w: %s", ErrAccountNotFound, accountID)
	case err != nil:
		return moderation.RetentionHold{}, err
	}
	h := moderation.RetentionHold{
		ID: s.d.IDs.NewID(), AccountID: accountID, Reason: reason, CreatedByUserID: byUserID,
		CreatedAt: kernel.Normalize(s.d.Clock.Now()),
	}
	if err := h.Validate(); err != nil {
		return moderation.RetentionHold{}, err
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.InsertRetentionHold(ctx, tx, h) })
	if err != nil {
		return moderation.RetentionHold{}, err
	}
	return h, nil
}

// ReleaseRetentionHold releases a hold, after which the purges erase what it preserved at their
// next run. It reports whether it released the hold: releasing a released hold changes nothing.
// An unknown hold fails with ErrNotFound.
func (s *Service) ReleaseRetentionHold(ctx context.Context, holdID, byUserID string) (moderation.RetentionHoldRecord, bool, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	var (
		rec      moderation.RetentionHoldRecord
		released bool
	)
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		released = false
		if rec, err = s.d.Repository.RetentionHold(ctx, tx, holdID); err != nil {
			return err
		}
		if !rec.Active() {
			return nil
		}
		r := moderation.RetentionHoldRelease{HoldID: holdID, ReleasedByUserID: byUserID, ReleasedAt: now}
		switch err := s.d.Repository.InsertRetentionHoldRelease(ctx, tx, r); {
		case errors.Is(err, ErrAlreadyRecorded):
			// Released at the same time; read it back below.
		case err != nil:
			return err
		default:
			released = true
		}
		rec, err = s.d.Repository.RetentionHold(ctx, tx, holdID)
		return err
	})
	if err != nil {
		return moderation.RetentionHoldRecord{}, false, err
	}
	return rec, released, nil
}

// RetentionHolds lists the holds of an account (of every account when accountID is empty),
// released ones included, newest first.
func (s *Service) RetentionHolds(ctx context.Context, accountID string) ([]moderation.RetentionHoldRecord, error) {
	return s.d.Repository.ListRetentionHolds(ctx, accountID)
}

// ReportedVoiceRooms returns which of roomIDs a report points at: the voice module keeps the
// records of the participants of such a room until the reports are erased after their
// retention (ADR-0100 §13).
func (s *Service) ReportedVoiceRooms(ctx context.Context, roomIDs []string) (map[string]bool, error) {
	if len(roomIDs) == 0 {
		return map[string]bool{}, nil
	}
	return s.d.Repository.ReportedVoiceRooms(ctx, roomIDs)
}

// HeldAccountIDs returns the accounts that a hold in effect preserves; the purges of other
// modules keep their records.
func (s *Service) HeldAccountIDs(ctx context.Context) ([]string, error) {
	return s.d.Repository.HeldAccountIDs(ctx)
}

// StatusRetained reports inside tx whether the purge of statuses must keep a status: a report or
// an action on statuses points at it, which keeps the evidence for as long as the record that
// points at it, or a hold preserves its author (ADR-0038).
func (s *Service) StatusRetained(ctx context.Context, tx uow.Tx, statusID, authorAccountID string) (bool, error) {
	return s.d.Repository.StatusRetained(ctx, tx, statusID, authorAccountID)
}

// retentionPeriods returns the instance's retention periods.
func (s *Service) retentionPeriods(ctx context.Context) (RetentionPeriods, error) {
	if s.d.Retention == nil {
		return DefaultRetentionPeriods, nil
	}
	return s.d.Retention.RetentionPeriods(ctx)
}

// PurgedRecords counts what one run of PurgeRecords erased.
type PurgedRecords struct {
	Actions, Reports, SignupBlocks int
	AuditEntries                   int64
}

// PurgeRecords erases the moderation records past their retention: the actions with their
// appeals first, so that a report they followed from is erased in the same run, then the
// reports, then the sign-up blocks, then the audit log entries. Each batch is one short
// transaction, and one run deletes a bounded number of rows of each kind; the next run
// continues. It is safe to repeat.
func (s *Service) PurgeRecords(ctx context.Context) (PurgedRecords, error) {
	periods, err := s.retentionPeriods(ctx)
	if err != nil {
		return PurgedRecords{}, err
	}
	now := kernel.Normalize(s.d.Clock.Now())
	repo := s.d.Repository
	var out PurgedRecords
	purge := func(name string, before time.Time, list func(context.Context, uow.Tx, time.Time, int) ([]string, error), del func(context.Context, uow.Tx, []string) error) (int, error) {
		total := 0
		for range recordPurgeMaxBatches {
			var n int
			err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				ids, err := list(ctx, tx, before, RecordPurgeBatch)
				if err != nil || len(ids) == 0 {
					n = 0
					return err
				}
				n = len(ids)
				return del(ctx, tx, ids)
			})
			if err != nil {
				return total, fmt.Errorf("moderation: purge %s: %w", name, err)
			}
			total += n
			if n < RecordPurgeBatch {
				break
			}
		}
		return total, nil
	}
	if out.Actions, err = purge("actions", now.Add(-periods.Actions), repo.ListActionsToPurge, repo.PurgeActions); err != nil {
		return out, err
	}
	if out.Reports, err = purge("reports", now.Add(-periods.Reports), repo.ListReportsToPurge, repo.PurgeReports); err != nil {
		return out, err
	}
	if out.SignupBlocks, err = purge("sign-up blocks", now.Add(-moderation.SignupBlockRetention), repo.ListSignupBlocksToPurge, repo.PurgeSignupBlocks); err != nil {
		return out, err
	}
	before := now.Add(-periods.AuditLog)
	for range recordPurgeMaxBatches {
		var n int64
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, err = repo.DeleteAuditEntriesBefore(ctx, tx, before, AuditPurgeBatch)
			return err
		})
		if err != nil {
			return out, fmt.Errorf("moderation: purge audit log entries: %w", err)
		}
		out.AuditEntries += n
		if n < AuditPurgeBatch {
			break
		}
	}
	return out, nil
}

// registerPurgeJob registers the JobPurgeRecords handler and its hourly schedule.
func (s *Service) registerPurgeJob(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPurgeRecords, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := s.PurgeRecords(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobPurgeRecords, Every: RecordPurgeInterval, Job: queue.NewJob{Type: JobPurgeRecords}})
}
