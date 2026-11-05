// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/hook"
)

const (
	// JobPurgeDecisions removes the decisions past retention_hook_decisions; no payload.
	JobPurgeDecisions = "hook.purge_decisions"
	// PurgeInterval is the period of its schedule.
	PurgeInterval = time.Hour
	// PurgeBatch is how many decisions one transaction removes; purgeMaxBatches bounds one run.
	PurgeBatch      = 100
	purgeMaxBatches = 20
)

// The labels of the metrics.
const (
	labelHook    = "hook"
	labelMode    = "mode"
	labelOutcome = "outcome"
)

// Deps are the ports of the Service.
type Deps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Settings   Settings
	// Plugins holds the plugin of each configured hook. Without an entry a hook calls nothing.
	Plugins map[hook.Name]Plugin
	// Holds lists the held accounts; nil holds none.
	Holds HeldAccounts
	// Statuses tells which statuses exist; nil takes every status for existing.
	Statuses Statuses
	Clock    kernel.Clock
	IDs      kernel.IDGenerator
	// Meter records the metrics; nil records nothing.
	Meter kernel.Meter
	// Logger logs what no caller can act on; nil is slog.Default.
	Logger *slog.Logger
}

// OverrideEffect undoes what a decision did, inside the transaction of its cancellation.
type OverrideEffect func(ctx context.Context, tx uow.Tx, r hook.Record) error

// Service runs the decision hooks and implements Gate.
type Service struct {
	d        Deps
	breakers map[hook.Name]*breaker
	effects  map[hook.Name][]OverrideEffect

	decisions kernel.Counter
	failures  kernel.Counter
	latency   kernel.Histogram
}

var _ Gate = (*Service)(nil)

// NewService returns the Service on d.
func NewService(d Deps) *Service {
	if d.Meter == nil {
		d.Meter = kernel.NopMeter{}
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	s := &Service{
		d: d, breakers: make(map[hook.Name]*breaker, len(d.Plugins)), effects: map[hook.Name][]OverrideEffect{},
		decisions: d.Meter.Counter(kernel.Instrument{
			Name: "my.hook.decisions", Unit: "{decision}", Description: "Decisions recorded by the decision hooks, by hook, mode, and outcome.",
			LabelKeys: []string{labelHook, labelMode, labelOutcome},
		}),
		failures: d.Meter.Counter(kernel.Instrument{
			Name: "my.hook.failures", Unit: "{call}", Description: "Calls of a decision hook that failed or that its breaker skipped, by hook and kind of failure.",
			LabelKeys: []string{labelHook, labelOutcome},
		}),
		latency: d.Meter.Histogram(kernel.Instrument{
			Name: "my.hook.duration", Unit: "s", Description: "Duration of the calls of the decision hooks' plugins.",
			LabelKeys: []string{labelHook}, Buckets: kernel.DurationBuckets,
		}),
	}
	for name := range d.Plugins {
		s.breakers[name] = new(breaker)
	}
	return s
}

// RegisterOverrideEffect adds what cancelling a decision of the hook undoes. The concrete hooks add
// theirs: clearing a sensitive mark, putting a dropped activity back in the queue (ADR-0147 §9).
func (s *Service) RegisterOverrideEffect(n hook.Name, e OverrideEffect) {
	s.effects[n] = append(s.effects[n], e)
}

// RegisterJobs registers the retention job and its schedule.
func (s *Service) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPurgeDecisions, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := s.PurgeDecisions(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobPurgeDecisions, Every: PurgeInterval, Job: queue.NewJob{Type: JobPurgeDecisions}})
}

// Configured implements Gate.
func (s *Service) Configured(n hook.Name) bool {
	_, ok := s.d.Plugins[n]
	return ok
}

// Decide implements Gate.
func (s *Service) Decide(ctx context.Context, c Call) (Result, error) {
	plugin, ok := s.d.Plugins[c.Hook]
	if !ok {
		return Result{Hook: c.Hook, SubjectKey: c.SubjectKey, Effect: hook.Allow}, nil
	}
	mode, err := s.d.Settings.HookMode(ctx, c.Hook)
	if err != nil {
		return Result{}, fmt.Errorf("hook: the mode of %s: %w", c.Hook, err)
	}
	if mode == hook.ModeOff {
		return Result{Hook: c.Hook, SubjectKey: c.SubjectKey, Effect: hook.Allow}, nil
	}
	onFailure, err := s.d.Settings.HookOnFailure(ctx, c.Hook)
	if err != nil {
		return Result{}, fmt.Errorf("hook: the failure behaviour of %s: %w", c.Hook, err)
	}
	switch rec, err := s.d.Repository.RecordBySubject(ctx, c.Hook, c.SubjectKey); {
	case err == nil:
		return resultOf(rec, onFailure), nil
	case !errors.Is(err, ErrNotFound):
		return Result{}, fmt.Errorf("hook: read the decision of %s: %w", c.Hook, err)
	}

	answer, latency, err := s.call(ctx, plugin, c)
	if err != nil {
		return Result{}, err
	}
	if answer.Verdict.Failed() && mode == hook.ModeEnforce && onFailure == hook.RetryOnFailure {
		return Result{}, fmt.Errorf("%w: %s: %s", ErrRetryLater, c.Hook, answer.Verdict)
	}
	rec, err := s.record(ctx, c, mode, answer, latency)
	if err != nil {
		return Result{}, err
	}
	s.decisions.Add(ctx, 1, kernel.NewLabels(labelHook, string(c.Hook), labelMode, string(mode), labelOutcome, string(answer.Verdict)))
	return resultOf(rec, onFailure), nil
}

// call asks the plugin through the hook's breaker and returns its answer, a failure being the
// Verdict of an empty answer. It fails only when the caller's own context ended, which says nothing
// about the plugin: the job runs again and no decision is recorded.
func (s *Service) call(ctx context.Context, plugin Plugin, c Call) (Answer, time.Duration, error) {
	b := s.breakers[c.Hook]
	start := s.d.Clock.Now()
	if !b.allow(start) {
		s.failures.Add(ctx, 1, kernel.NewLabels(labelHook, string(c.Hook), labelOutcome, string(hook.FailedBreakerOpen)))
		return Answer{Verdict: hook.FailedBreakerOpen}, 0, nil
	}
	answer, err := plugin.Check(ctx, Request{Hook: c.Hook, SubjectKey: c.SubjectKey, Subject: c.Subject})
	end := s.d.Clock.Now()
	latency := max(end.Sub(start), 0)
	if ctxErr := ctx.Err(); ctxErr != nil {
		b.abandoned()
		return Answer{}, 0, fmt.Errorf("hook: call %s: %w", c.Hook, ctxErr)
	}
	s.latency.Record(ctx, latency.Seconds(), kernel.NewLabels(labelHook, string(c.Hook)))
	failure := failureOf(c.Hook, answer, err)
	if failure == "" {
		b.succeeded()
		return Answer{Verdict: answer.Verdict, Reason: answer.Reason, Note: hook.CleanNote(answer.Note)}, latency, nil
	}
	s.failures.Add(ctx, 1, kernel.NewLabels(labelHook, string(c.Hook), labelOutcome, string(failure)))
	if b.failed(end) {
		s.notifyFailing(ctx, c.Hook, end)
	}
	return Answer{Verdict: failure}, latency, nil
}

// failureOf returns the kind of failure of a call, "" for an answer the hook can act on. A verdict
// the hook does not take, such as a reject of media.check, is an invalid answer (ADR-0147 §2).
func failureOf(n hook.Name, a Answer, err error) hook.Outcome {
	switch {
	case errors.Is(err, ErrPluginTimeout):
		return hook.FailedTimeout
	case errors.Is(err, ErrPluginInvalid):
		return hook.FailedInvalid
	case err != nil:
		return hook.FailedError
	}
	spec, known := hook.SpecOf(n)
	if !known || !spec.Accepts(a.Verdict) || !a.Reason.Valid() {
		return hook.FailedInvalid
	}
	return ""
}

// notifyFailing tells the moderators. The call it follows goes on whatever happens here.
func (s *Service) notifyFailing(ctx context.Context, n hook.Name, at time.Time) {
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return tx.Emit(ctx, Failing{Hook: n, At: kernel.Normalize(at)})
	})
	if err != nil {
		s.d.Logger.LogAttrs(ctx, slog.LevelWarn, "hook: could not tell the moderators about a failing hook",
			slog.String(labelHook, string(n)), slog.Any("error", err))
	}
}

// record writes the decision in its own short transaction, so that a job that fails afterwards
// reads it when it runs again (ADR-0147 §6). When another run recorded one first, that one stands.
func (s *Service) record(ctx context.Context, c Call, mode hook.Mode, a Answer, latency time.Duration) (hook.Record, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	d := hook.Decision{
		ID: s.d.IDs.NewID(), Hook: c.Hook, SubjectKey: c.SubjectKey, AccountID: c.AccountID, Mode: mode,
		Outcome: a.Verdict, Reason: a.Reason, Note: a.Note, Latency: latency, DecidedAt: now,
	}
	if err := d.Validate(); err != nil {
		return hook.Record{}, err
	}
	inserted := false
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		inserted, err = s.d.Repository.InsertDecision(ctx, tx, d)
		return err
	})
	if err != nil {
		return hook.Record{}, fmt.Errorf("hook: record the decision of %s: %w", c.Hook, err)
	}
	if inserted {
		return hook.Record{Decision: d}, nil
	}
	return s.d.Repository.RecordBySubject(ctx, c.Hook, c.SubjectKey)
}

func resultOf(r hook.Record, onFailure hook.OnFailure) Result {
	d := r.Decision
	return Result{
		DecisionID: d.ID, Hook: d.Hook, SubjectKey: d.SubjectKey, Effect: r.Effect(onFailure), Reason: d.Reason, Note: d.Note,
	}
}

// LinkStatus implements Gate.
func (s *Service) LinkStatus(ctx context.Context, tx uow.Tx, decisionID, statusID string) error {
	return s.d.Repository.InsertDecisionStatus(ctx, tx, decisionID, statusID)
}

// Applied implements Gate.
func (s *Service) Applied(ctx context.Context, tx uow.Tx, r Result) error {
	if r.DecisionID == "" || !r.Effect.LeavesState() {
		return nil
	}
	return tx.Emit(ctx, DecisionApplied{
		DecisionID: r.DecisionID, Label: decisionLabel(r.Hook, r.SubjectKey), AuditID: s.d.IDs.NewID(), At: kernel.Normalize(s.d.Clock.Now()),
	})
}

// decisionLabel is the target label of a decision in the audit log.
func decisionLabel(n hook.Name, subjectKey string) string { return string(n) + " " + subjectKey }

// Override cancels a decision from the list. byUserID is the moderator, "" for the CLI.
func (s *Service) Override(ctx context.Context, id, byUserID string) (hook.Record, error) {
	var rec hook.Record
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		rec, err = s.d.Repository.RecordTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if rec.Override != nil {
			return ErrAlreadyOverridden
		}
		if !rec.Overridable() {
			return ErrNotOverridable
		}
		rec, err = s.OverrideTx(ctx, tx, rec, byUserID)
		return err
	})
	if err != nil {
		return hook.Record{}, err
	}
	return rec, nil
}

// OverrideTx cancels a decision inside tx, with the event of its audit log entry and what the hook
// registered to undo. The review of an appeal, which cancels a decision the list cannot, calls it.
func (s *Service) OverrideTx(ctx context.Context, tx uow.Tx, rec hook.Record, byUserID string) (hook.Record, error) {
	o := hook.Override{DecisionID: rec.Decision.ID, ByUserID: byUserID, At: kernel.Normalize(s.d.Clock.Now())}
	inserted, err := s.d.Repository.InsertOverride(ctx, tx, o)
	if err != nil {
		return hook.Record{}, err
	}
	if !inserted {
		return hook.Record{}, ErrAlreadyOverridden
	}
	rec.Override = &o
	for _, effect := range s.effects[rec.Decision.Hook] {
		if err := effect(ctx, tx, rec); err != nil {
			return hook.Record{}, err
		}
	}
	err = tx.Emit(ctx, DecisionOverridden{
		DecisionID: rec.Decision.ID, Label: decisionLabel(rec.Decision.Hook, rec.Decision.SubjectKey),
		ByUserID: byUserID, AuditID: s.d.IDs.NewID(), At: o.At,
	})
	if err != nil {
		return hook.Record{}, err
	}
	return rec, nil
}

// List returns the decisions that q selects, newest first.
func (s *Service) List(ctx context.Context, q ListQuery) ([]hook.Record, error) {
	return s.d.Repository.ListRecords(ctx, q)
}

// Status is what the admin list shows of a hook.
type Status struct {
	Name       hook.Name
	Configured bool
	Mode       hook.Mode
	OnFailure  hook.OnFailure
}

// Statuses returns every hook in documentation order.
func (s *Service) Statuses(ctx context.Context) ([]Status, error) {
	names := hook.Names()
	out := make([]Status, 0, len(names))
	for _, n := range names {
		mode, err := s.d.Settings.HookMode(ctx, n)
		if err != nil {
			return nil, err
		}
		onFailure, err := s.d.Settings.HookOnFailure(ctx, n)
		if err != nil {
			return nil, err
		}
		out = append(out, Status{Name: n, Configured: s.Configured(n), Mode: mode, OnFailure: onFailure})
	}
	return out, nil
}

// InUse returns the hooks that send data to a plugin, for the statement on the information page:
// those with a plugin and a mode other than off. A server with no plugin reads nothing.
func (s *Service) InUse(ctx context.Context) ([]hook.Name, error) {
	if len(s.d.Plugins) == 0 {
		return nil, nil
	}
	var out []hook.Name
	for _, n := range hook.Names() {
		if !s.Configured(n) {
			continue
		}
		mode, err := s.d.Settings.HookMode(ctx, n)
		if err != nil {
			return nil, err
		}
		if mode != hook.ModeOff {
			out = append(out, n)
		}
	}
	return out, nil
}

// Summary counts the decisions of the last period and returns when the period starts.
func (s *Service) Summary(ctx context.Context, period time.Duration) (time.Time, []SummaryRow, error) {
	since := s.d.Clock.Now().Add(-period)
	rows, err := s.d.Repository.Summarize(ctx, since)
	return since, rows, err
}

// PurgeDecisions removes the decisions past their retention that nothing keeps
// (hook.RetentionPolicy), and returns how many. Each batch is one short transaction, and what one
// run leaves the next continues.
func (s *Service) PurgeDecisions(ctx context.Context) (int64, error) {
	retention, appealWindow, err := s.d.Settings.HookRetention(ctx)
	if err != nil {
		return 0, err
	}
	policy := hook.RetentionPolicy{Held: map[string]bool{}}
	now := s.d.Clock.Now()
	policy.AppealsOpenSince = now.Add(-appealWindow)
	if s.d.Holds != nil {
		held, err := s.d.Holds.HeldAccountIDs(ctx)
		if err != nil {
			return 0, err
		}
		for _, id := range held {
			policy.Held[id] = true
		}
	}
	q := ExpiredQuery{Before: now.Add(-retention), Limit: PurgeBatch}
	var total int64
	for range purgeMaxBatches {
		expired, err := s.d.Repository.ListExpired(ctx, q)
		if err != nil {
			return total, err
		}
		if len(expired) == 0 {
			return total, nil
		}
		last := expired[len(expired)-1]
		q.AfterAt, q.AfterID = last.DecidedAt, last.ID
		ids, err := s.purgeable(ctx, policy, expired)
		if err != nil {
			return total, err
		}
		if len(ids) > 0 {
			err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.DeleteDecisions(ctx, tx, ids) })
			if err != nil {
				return total, err
			}
			total += int64(len(ids))
		}
		if len(expired) < q.Limit {
			break
		}
	}
	return total, nil
}

// purgeable returns the decisions of a batch that no rule keeps. A status with no row no longer
// counts: the rebuild of the projection drops it (ADR-0147 §8).
func (s *Service) purgeable(ctx context.Context, policy hook.RetentionPolicy, expired []hook.Expired) ([]string, error) {
	if s.d.Statuses != nil {
		var ids []string
		for _, e := range expired {
			ids = append(ids, e.StatusIDs...)
		}
		if len(ids) > 0 {
			existing, err := s.d.Statuses.ExistingStatusIDs(ctx, ids)
			if err != nil {
				return nil, err
			}
			present := make(map[string]bool, len(existing))
			for _, id := range existing {
				present[id] = true
			}
			for i := range expired {
				var left []string
				for _, id := range expired[i].StatusIDs {
					if present[id] {
						left = append(left, id)
					}
				}
				expired[i].StatusIDs = left
			}
		}
	}
	var out []string
	for _, e := range expired {
		if !policy.Keeps(e) {
			out = append(out, e.ID)
		}
	}
	return out, nil
}

// EraseAccountData implements appaccount.DataEraser: it removes the decisions of a deleted account.
// The erasure skips an account that a hold preserves, so a held account keeps them.
func (s *Service) EraseAccountData(ctx context.Context, target appaccount.ErasureTarget, _ string, limit int) (appaccount.ErasureStep, error) {
	var ids []string
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		ids, err = s.d.Repository.DecisionIDsOfAccount(ctx, tx, target.AccountID, limit)
		if err != nil || len(ids) == 0 {
			return err
		}
		return s.d.Repository.DeleteDecisions(ctx, tx, ids)
	})
	if err != nil {
		return appaccount.ErasureStep{}, err
	}
	return appaccount.ErasureStep{Done: len(ids) < limit}, nil
}
