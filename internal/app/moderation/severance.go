// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The follows that suspensions cut (FR-717, ADR-0036, data-model §4.3). The suspension of a
// remote account ends every follow between it and the local accounts, in both directions, in
// the transaction of the action, and its server gets a Reject of each of its follows and an
// Undo of each local follow of it. The suspension of a domain ends every follow between the
// local accounts and the accounts of the domain and its subdomains without telling them, in a
// job (JobSeverDomain) that takes one local account per transaction. For each local account
// that lost an active follow, one relationship_severances row of the action records how many,
// and RelationshipsSevered tells the notification module. Lifting the suspension restores
// nothing, and processing the same action again records and notifies nothing more.

// Severance is moderation.Severance, the follows that one local account lost.
type Severance = moderation.Severance

// SeveranceRecord is moderation.SeveranceRecord.
type SeveranceRecord = moderation.SeveranceRecord

// EventRelationshipsSevered follows each new relationship_severances row, in its transaction.
const EventRelationshipsSevered = "moderation.relationships_severed"

// RelationshipsSevered carries the follows that one local account lost to a suspension.
type RelationshipsSevered struct{ Severance Severance }

// EventType implements uow.Event.
func (RelationshipsSevered) EventType() string { return EventRelationshipsSevered }

// SeveranceRepository is the part of Repository that records severances.
type SeveranceRepository interface {
	// InsertSeverance inserts a severance and reports whether it did: a severance of an account
	// whose action already has one for it inserts nothing.
	InsertSeverance(ctx context.Context, tx uow.Tx, s moderation.Severance) (bool, error)
	// FindSeverance returns a severance of accountID with what its action suspended; one of
	// another account is ErrNotFound.
	FindSeverance(ctx context.Context, accountID, id string) (moderation.SeveranceRecord, error)
}

// Follows is the part of the graph module's public API that cuts follows. *appgraph.Service
// implements it.
type Follows interface {
	Counterparts(ctx context.Context, tx uow.Tx, accountID string) ([]string, error)
	SeverByModeration(ctx context.Context, tx uow.Tx, accountID string, others []string, announce bool, now time.Time) (appgraph.Severed, error)
}

// Directory is the part of the account module's public API that finds the accounts that a
// domain suspension covers. *appaccount.Service implements it.
type Directory interface {
	LocalAccountIDs(ctx context.Context, after string, limit int) ([]string, error)
	AccountsOnDomain(ctx context.Context, tx uow.Tx, domain string, ids []string) ([]string, error)
}

const (
	// JobSeverDomain cuts the follows of the local accounts with a suspended domain; its payload
	// is severDomainPayload.
	JobSeverDomain = "moderation.sever_domain"
)

type severDomainPayload struct {
	queue.FanOutStep
	DomainActionID string `json:"domain_action_id"`
}

// severs reports whether the Service cuts follows: Deps has the graph module.
func (s *Service) severs() bool { return s.d.Follows != nil }

// enqueueDomainSeverance enqueues JobSeverDomain in tx for a domain suspension a, when the
// Service cuts follows and has the account directory and an outbox.
func (s *Service) enqueueDomainSeverance(ctx context.Context, tx uow.Tx, a moderation.DomainAction) error {
	if a.Kind != moderation.DomainSuspend || !s.severs() || s.d.Directory == nil || s.d.Outbox == nil {
		return nil
	}
	payload, err := json.Marshal(severDomainPayload{DomainActionID: a.ID})
	if err != nil {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobSeverDomain, Payload: payload, IdempotencyKey: JobSeverDomain + ":" + a.ID})
}

// severAccountIn cuts, inside the transaction of action a, the follows between its remote target
// and the local accounts, telling the target's server, and records a severance for each local
// account that lost an active follow.
func (s *Service) severAccountIn(ctx context.Context, tx uow.Tx, a moderation.Action, now time.Time) error {
	locals, err := s.d.Follows.Counterparts(ctx, tx, a.TargetAccountID)
	if err != nil {
		return err
	}
	for _, local := range locals {
		severed, err := s.d.Follows.SeverByModeration(ctx, tx, local, []string{a.TargetAccountID}, true, now)
		if err != nil {
			return err
		}
		if _, err := s.recordSeverance(ctx, tx, Severance{ActionID: a.ID, AccountID: local}, severed, now); err != nil {
			return err
		}
	}
	return nil
}

// recordSeverance inserts sv with the counts of severed, unless the account lost no active
// follow, and emits RelationshipsSevered when it is new; created reports a new one.
func (s *Service) recordSeverance(ctx context.Context, tx uow.Tx, sv Severance, severed appgraph.Severed, now time.Time) (created bool, err error) {
	if !severed.Lost() {
		return false, nil
	}
	sv.ID, sv.FollowersLost, sv.FollowingLost, sv.OccurredAt = s.d.IDs.NewID(), severed.Followers, severed.Following, now
	if err := sv.Validate(); err != nil {
		return false, err
	}
	created, err = s.d.Repository.InsertSeverance(ctx, tx, sv)
	if err != nil || !created {
		return false, err
	}
	return true, tx.Emit(ctx, RelationshipsSevered{Severance: sv})
}

// SeverDomain cuts the follows between the local accounts and the accounts of the domain that the
// suspension domainActionID covers, its subdomains included, without telling them, and records a
// severance for each local account that lost an active follow; it returns how many local accounts it
// recorded. It runs one short transaction per local account, so running it again after a failure
// changes nothing it did: the follows that it ended stay ended and the severances that it recorded
// are not recorded again. An action that is not a domain suspension cuts nothing; one that has been
// lifted meanwhile still cuts its follows, as its issue did.
func (s *Service) SeverDomain(ctx context.Context, domainActionID string) (int, error) {
	recorded := 0
	err := s.severDomain(ctx, domainActionID, &recorded, func(fan queue.FanOut) error {
		return fan.Walk(ctx, queue.KeyRange{})
	})
	return recorded, err
}

// severDomain does what SeverDomain says through run, which walks some or all of the local accounts,
// adding the local accounts it recorded to recorded.
func (s *Service) severDomain(ctx context.Context, domainActionID string, recorded *int, run func(queue.FanOut) error) error {
	if !s.severs() || s.d.Directory == nil {
		return fmt.Errorf("%w: severing follows without the graph and account modules", ErrUnsupportedAction)
	}
	rec, err := s.d.Repository.FindDomainActionRecord(ctx, domainActionID)
	if err != nil {
		return err
	}
	a := rec.Action
	if a.Kind != moderation.DomainSuspend {
		return nil
	}
	fan := queue.FanOut{
		Size: s.d.FanOutSize,
		Page: queue.PageByLastKey(s.d.Directory.LocalAccountIDs),
		Handle: func(ctx context.Context, locals []string) error {
			for _, local := range locals {
				var created bool
				err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
					var err error
					created, err = s.severDomainOf(ctx, tx, a, local)
					return err
				})
				if err != nil {
					return err
				}
				if created {
					*recorded++
				}
			}
			return nil
		},
		Enqueue: func(ctx context.Context, steps []queue.FanOutStep) error {
			jobs := make([]queue.NewJob, 0, len(steps))
			for _, step := range steps {
				payload, err := json.Marshal(severDomainPayload{FanOutStep: step, DomainActionID: a.ID})
				if err != nil {
					return err
				}
				jobs = append(jobs, queue.NewJob{Type: JobSeverDomain, Payload: payload, IdempotencyKey: queue.StepKey(JobSeverDomain+":"+a.ID, step)})
			}
			return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Outbox.EnqueueTx(ctx, tx, jobs...) })
		},
	}
	return run(fan)
}

// severDomainOf cuts, inside tx, the follows between the local account and the accounts of the
// suspended domain of a, and records its severance; created reports a new one.
func (s *Service) severDomainOf(ctx context.Context, tx uow.Tx, a moderation.DomainAction, local string) (created bool, err error) {
	others, err := s.d.Follows.Counterparts(ctx, tx, local)
	if err != nil || len(others) == 0 {
		return false, err
	}
	if others, err = s.d.Directory.AccountsOnDomain(ctx, tx, a.Domain, others); err != nil || len(others) == 0 {
		return false, err
	}
	now := kernel.Normalize(s.d.Clock.Now())
	severed, err := s.d.Follows.SeverByModeration(ctx, tx, local, others, false, now)
	if err != nil {
		return false, err
	}
	return s.recordSeverance(ctx, tx, Severance{DomainActionID: a.ID, AccountID: local}, severed, now)
}

// handleSeverDomain runs JobSeverDomain.
func (s *Service) handleSeverDomain(ctx context.Context, j queue.Job) queue.Result {
	var p severDomainPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.DomainActionID == "" {
		return queue.Fail(fmt.Errorf("moderation: payload of %s: %q", JobSeverDomain, j.Payload))
	}
	recorded := 0
	err := s.severDomain(ctx, p.DomainActionID, &recorded, func(fan queue.FanOut) error {
		return fan.Run(ctx, p.FanOutStep)
	})
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrUnsupportedAction):
		return queue.Fail(err)
	case err != nil:
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// SeveranceOf returns a severance of accountID with what its action suspended, for the
// relationships_severed notification; one of another account fails with ErrNotFound.
func (s *Service) SeveranceOf(ctx context.Context, accountID, id string) (SeveranceRecord, error) {
	return s.d.Repository.FindSeverance(ctx, accountID, id)
}
