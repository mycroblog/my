// SPDX-License-Identifier: AGPL-3.0-only

// Package dbupgrade runs the data upgrade steps of `my db upgrade` and records them in the
// database (ADR-0087 §2, §4). A schema change that rewrites existing rows adds the new tables or
// columns in one release (the expand) and rewrites the rows with a step: Go code with a key, run
// once per database after `schema apply`, idempotent, in small transactions (ADR-0003). A later
// minor release drops what the step read (the contract) and names the step as a prerequisite:
// `my db upgrade --check`, run before `schema apply`, stops when the database has not recorded
// it and names the release to upgrade through first.
//
// The binary keeps registering a step after its contract, with Run set to nil: there is nothing
// left to rewrite, since the server writes only the new side, but a new database, or one that
// upgraded across the step, still records it. Readiness then requires every registered step, and
// every prerequisite is a registered step.
//
// The steps run in a one-off job that the operator starts, never when `my serve` starts, so that
// instances that start together do not run them side by side.
package dbupgrade

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
)

// Step is a data upgrade step.
type Step struct {
	// Key identifies the step in the record; it never changes once released.
	Key string
	// Description tells the operator what the step does.
	Description string
	// Run rewrites the rows through u, in transactions that each change a bounded number of rows
	// (ADR-0003) and that hold no external I/O. It must be safe to run again after it failed or
	// completed. Nil means that the step has nothing left to run (its contract has shipped).
	Run func(ctx context.Context, u uow.UnitOfWork) error
	// Projections name the projections (`my projection list`) to rebuild after Run.
	Projections []string
}

// Prerequisite is a step that the contract of this release assumes the database ran.
type Prerequisite struct {
	// Key is the key of a registered step.
	Key string
	// Via is the release whose `my db upgrade` runs the step, which a database that lacks it
	// upgrades through first, for example "v0.3.2".
	Via string
}

// Run is a recorded step.
type Run struct {
	Key         string
	CompletedAt time.Time
}

// Repository records the steps that completed. The database adapters implement it on the
// table data_upgrade_runs and pass testkit.DataUpgradeRepositoryContract.
type Repository interface {
	// Runs returns the recorded steps, ordered by key. It returns ErrNoRecord when the database
	// lacks the table of the record, and ErrEmptyDatabase when it holds no table at all.
	Runs(ctx context.Context) ([]Run, error)
	// RecordRun records inside tx that the step key completed at at. Recording a step again keeps
	// the first record.
	RecordRun(ctx context.Context, tx uow.Tx, key string, at time.Time) error
}

var (
	// ErrNoRecord is returned by Repository.Runs for a database whose schema predates the record.
	ErrNoRecord = errors.New("dbupgrade: the database has no record of data upgrades")
	// ErrEmptyDatabase is returned by Repository.Runs for a database that holds no table.
	ErrEmptyDatabase = errors.New("dbupgrade: the database holds no table")
	// ErrNotUpgraded is returned by Ready while registered steps are not recorded.
	ErrNotUpgraded = errors.New("dbupgrade: data upgrade steps have not run; run my db upgrade")
)

// Options configure a Service.
type Options struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Clock      kernel.Clock
	// Steps are the registered steps, in the order that Upgrade runs them.
	Steps []Step
	// Prerequisites are the steps that the contract of this release assumes.
	Prerequisites []Prerequisite
	// RebuildProjection rebuilds the projection of a name of Step.Projections.
	RebuildProjection func(ctx context.Context, name string) error
}

// Service runs, checks, and reports the data upgrade steps.
type Service struct {
	u       uow.UnitOfWork
	repo    Repository
	clock   kernel.Clock
	steps   []Step
	prereqs []Prerequisite
	rebuild func(ctx context.Context, name string) error
}

// NewService returns the service of the steps in opts. It panics on a step without a key, a key
// that two steps share, a step that names projections without RebuildProjection, and a
// prerequisite that is not a registered step or names no release.
func NewService(opts Options) *Service {
	keys := map[string]bool{}
	for _, s := range opts.Steps {
		if s.Key == "" {
			panic("dbupgrade: a step needs a key")
		}
		if keys[s.Key] {
			panic(fmt.Sprintf("dbupgrade: step %q is registered twice", s.Key))
		}
		keys[s.Key] = true
		if len(s.Projections) > 0 && opts.RebuildProjection == nil {
			panic(fmt.Sprintf("dbupgrade: step %q rebuilds projections without RebuildProjection", s.Key))
		}
	}
	for _, p := range opts.Prerequisites {
		if !keys[p.Key] {
			panic(fmt.Sprintf("dbupgrade: prerequisite %q is not a registered step", p.Key))
		}
		if p.Via == "" {
			panic(fmt.Sprintf("dbupgrade: prerequisite %q names no release", p.Key))
		}
	}
	return &Service{
		u: opts.UnitOfWork, repo: opts.Repository, clock: opts.Clock,
		steps: slices.Clone(opts.Steps), prereqs: slices.Clone(opts.Prerequisites), rebuild: opts.RebuildProjection,
	}
}

// Steps returns the registered steps, in the order that Upgrade runs them.
func (s *Service) Steps() []Step { return slices.Clone(s.steps) }

// recorded returns the keys of the recorded steps.
func (s *Service) recorded(ctx context.Context) (map[string]bool, error) {
	runs, err := s.repo.Runs(ctx)
	if err != nil {
		return nil, err
	}
	done := make(map[string]bool, len(runs))
	for _, r := range runs {
		done[r.Key] = true
	}
	return done, nil
}

// Pending returns the registered steps that the database has not recorded, in order.
func (s *Service) Pending(ctx context.Context) ([]Step, error) {
	if len(s.steps) == 0 {
		return nil, nil
	}
	done, err := s.recorded(ctx)
	if err != nil {
		return nil, err
	}
	var pending []Step
	for _, st := range s.steps {
		if !done[st.Key] {
			pending = append(pending, st)
		}
	}
	return pending, nil
}

// Upgrade runs the pending steps in order and records each one after it completes: its Run, then
// the rebuild of its projections. It calls started, when not nil, before each step. It stops at
// the first step that fails, which runs again the next time, and returns the steps that it
// completed.
func (s *Service) Upgrade(ctx context.Context, started func(Step)) ([]Step, error) {
	pending, err := s.Pending(ctx)
	if err != nil {
		return nil, err
	}
	var completed []Step
	for _, st := range pending {
		if started != nil {
			started(st)
		}
		if err := s.runStep(ctx, st); err != nil {
			return completed, fmt.Errorf("dbupgrade: step %s: %w", st.Key, err)
		}
		completed = append(completed, st)
	}
	return completed, nil
}

func (s *Service) runStep(ctx context.Context, st Step) error {
	if st.Run != nil {
		if err := st.Run(ctx, s.u); err != nil {
			return err
		}
	}
	for _, name := range st.Projections {
		if err := s.rebuild(ctx, name); err != nil {
			return fmt.Errorf("rebuild projection %s: %w", name, err)
		}
	}
	return s.u.Tx(ctx, func(tx uow.Tx) error {
		return s.repo.RecordRun(ctx, tx, st.Key, s.clock.Now())
	})
}

// CheckResult is the result of Check.
type CheckResult struct {
	// NewDatabase reports that the database holds no table yet: there is no data to carry over.
	NewDatabase bool
	// Missing are the prerequisites that the database has not recorded, in order.
	Missing []Prerequisite
}

// Check reports the prerequisites that the database has not recorded. A database that holds no
// table is new and lacks none; one whose schema predates the record lacks every prerequisite.
func (s *Service) Check(ctx context.Context) (CheckResult, error) {
	if len(s.prereqs) == 0 {
		return CheckResult{}, nil
	}
	done, err := s.recorded(ctx)
	switch {
	case errors.Is(err, ErrEmptyDatabase):
		return CheckResult{NewDatabase: true}, nil
	case errors.Is(err, ErrNoRecord):
		done = map[string]bool{}
	case err != nil:
		return CheckResult{}, err
	}
	var r CheckResult
	for _, p := range s.prereqs {
		if !done[p.Key] {
			r.Missing = append(r.Missing, p)
		}
	}
	return r, nil
}

// Ready reports whether the database recorded every registered step. The error wraps
// ErrNotUpgraded and names the steps that are missing.
func (s *Service) Ready(ctx context.Context) error {
	pending, err := s.Pending(ctx)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	keys := make([]string, len(pending))
	for i, st := range pending {
		keys[i] = st.Key
	}
	return fmt.Errorf("%w (missing %s)", ErrNotUpgraded, strings.Join(keys, ", "))
}
