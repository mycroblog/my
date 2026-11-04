// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The instance rules (FR-705, data-model §4.9). A rule keeps its ID for life: revising its
// text or moving it records a new revision, and retiring it removes it from the rules in
// effect while reports that cite it still find it.
//
// Each change that records something records its audit log entry in the same transaction
// (ADR-0058); one that records nothing, such as moving a rule to its place, is not audited.

// RuleRepository is the persistence port of the instance rules. It is part of Repository.
type RuleRepository interface {
	InsertRule(ctx context.Context, tx uow.Tx, id string, createdAt time.Time) error
	// InsertRuleRevision inserts the revision.
	InsertRuleRevision(ctx context.Context, tx uow.Tx, r instance.RuleRevision) error
	// InsertRuleRetirement records the retirement, and reports false when the rule was
	// already retired (the earlier retirement stays).
	InsertRuleRetirement(ctx context.Context, tx uow.Tx, r instance.RuleRetirement) (bool, error)
	// Rule returns the rule with its latest revision and its retirement.
	Rule(ctx context.Context, tx uow.Tx, id string) (instance.Rule, error)
	// RuleHead returns the instance_rule_heads row of the rule.
	RuleHead(ctx context.Context, tx uow.Tx, ruleID string) (instance.RuleHead, error)
	UpsertRuleHead(ctx context.Context, tx uow.Tx, h instance.RuleHead) error
	DeleteRuleHead(ctx context.Context, tx uow.Tx, ruleID string) error
	DeleteAllRuleHeads(ctx context.Context, tx uow.Tx) error
	// ListRuleHeads returns the heads of the rules in effect, by position and then rule ID.
	ListRuleHeads(ctx context.Context, tx uow.Tx) ([]instance.RuleHead, error)
	// ListRuleRevisionHeads returns the rule, ID, and position of every rule revision, oldest
	// first, for rebuilding.
	ListRuleRevisionHeads(ctx context.Context, tx uow.Tx) ([]instance.RuleHead, error)
	// ListRetiredRuleIDs returns the IDs of the retired rules, for rebuilding.
	ListRetiredRuleIDs(ctx context.Context, tx uow.Tx) ([]string, error)

	// FindRule returns the rule with its latest revision and its retirement.
	FindRule(ctx context.Context, id string) (instance.Rule, error)
	// ListRules returns the rules in effect by position and then ID, followed, when
	// includeRetired is set, by the retired rules by the position of their last revision and
	// then ID.
	ListRules(ctx context.Context, includeRetired bool) ([]instance.Rule, error)
}

// Event types of the instance rules, for uow.Projectors.
const (
	EventRuleRevised = "instance.rule_revised"
	EventRuleRetired = "instance.rule_retired"
)

// RuleRevised is emitted when a rule revision is inserted.
type RuleRevised struct {
	Revision instance.RuleRevision
}

// EventType implements uow.Event.
func (RuleRevised) EventType() string { return EventRuleRevised }

// RuleRetired is emitted when a rule is retired.
type RuleRetired struct {
	Retirement instance.RuleRetirement
}

// EventType implements uow.Event.
func (RuleRetired) EventType() string { return EventRuleRetired }

func registerRuleProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventRuleRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		rev := e.(RuleRevised).Revision
		cur, err := repo.RuleHead(ctx, tx, rev.RuleID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if !rev.Supersedes(cur) {
			return nil
		}
		return repo.UpsertRuleHead(ctx, tx, instance.RuleHead{RuleID: rev.RuleID, RevisionID: rev.ID, Position: rev.Position})
	})
	r.Register(EventRuleRetired, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.DeleteRuleHead(ctx, tx, e.(RuleRetired).Retirement.RuleID)
	})
}

func rebuildRuleHeads(ctx context.Context, tx uow.Tx, repo Repository) error {
	if err := repo.DeleteAllRuleHeads(ctx, tx); err != nil {
		return err
	}
	revs, err := repo.ListRuleRevisionHeads(ctx, tx)
	if err != nil {
		return err
	}
	retired, err := repo.ListRetiredRuleIDs(ctx, tx)
	if err != nil {
		return err
	}
	heads := map[string]instance.RuleHead{}
	var order []string
	for _, r := range revs {
		if slices.Contains(retired, r.RuleID) {
			continue
		}
		h, seen := heads[r.RuleID]
		if !seen {
			order = append(order, r.RuleID)
		}
		if (instance.RuleRevision{ID: r.RevisionID}).Supersedes(h) {
			heads[r.RuleID] = r
		}
	}
	for _, id := range order {
		if err := repo.UpsertRuleHead(ctx, tx, heads[id]); err != nil {
			return err
		}
	}
	return nil
}

// AddRuleInput describes a new rule.
type AddRuleInput struct {
	Text string
	// Hint is an optional explanation of the rule; "" for none.
	Hint string
	// CreatedByUserID is the user adding the rule, or "" for the CLI.
	CreatedByUserID string
}

// AddRule records a new rule after the rules in effect. It fails with an error wrapping
// instance.ErrInvalidValue (a *instance.RuleTextError) for a bad text or hint and
// instance.ErrTooManyRules when instance.MaxRules rules are in effect.
func (s *Service) AddRule(ctx context.Context, in AddRuleInput) (instance.Rule, error) {
	if err := instance.ValidateRuleText(in.Text, in.Hint); err != nil {
		return instance.Rule{}, err
	}
	now := s.d.Clock.Now()
	rule := instance.Rule{ID: s.d.IDs.NewID(), CreatedAt: now}
	revID := s.d.IDs.NewID()
	audit := s.audit(appmoderation.AuditRuleAdded, appmoderation.AuditTargetRule, rule.ID, in.Text, revID, in.CreatedByUserID, now)
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		heads, err := s.d.Repository.ListRuleHeads(ctx, tx)
		if err != nil {
			return err
		}
		if len(heads) >= instance.MaxRules {
			return fmt.Errorf("add a rule: %d rules are in effect: %w", len(heads), instance.ErrTooManyRules)
		}
		position := 1
		if len(heads) > 0 {
			position = heads[len(heads)-1].Position + 1
		}
		rule.Revision = instance.RuleRevision{
			ID: revID, RuleID: rule.ID, Position: position, Text: in.Text, Hint: in.Hint, RevisedByUserID: in.CreatedByUserID, RevisedAt: now,
		}
		if err := s.d.Repository.InsertRule(ctx, tx, rule.ID, now); err != nil {
			return err
		}
		if err := s.d.Repository.InsertRuleRevision(ctx, tx, rule.Revision); err != nil {
			return err
		}
		if err := tx.Emit(ctx, RuleRevised{Revision: rule.Revision}); err != nil {
			return err
		}
		return tx.Emit(ctx, audit)
	})
	if err != nil {
		return instance.Rule{}, err
	}
	return rule, nil
}

// ReviseRuleInput describes a new text of a rule.
type ReviseRuleInput struct {
	RuleID string
	Text   string
	// Hint is the new hint; "" for none. KeepHint keeps the current hint instead.
	Hint     string
	KeepHint bool
	// RevisedByUserID is the user making the change, or "" for the CLI.
	RevisedByUserID string
}

// ReviseRule records a revision of the rule with a new text and hint, at its position; the
// rule keeps its ID. It fails with an error wrapping ErrNotFound for an unknown rule,
// instance.ErrRuleRetired for a retired one, and instance.ErrInvalidValue (a
// *instance.RuleTextError) for a bad text or hint.
func (s *Service) ReviseRule(ctx context.Context, in ReviseRuleInput) (instance.Rule, error) {
	if err := instance.ValidateRuleText(in.Text, in.Hint); err != nil {
		return instance.Rule{}, err
	}
	now := s.d.Clock.Now()
	revID := s.d.IDs.NewID()
	audit := s.audit(appmoderation.AuditRuleRevised, appmoderation.AuditTargetRule, in.RuleID, in.Text, revID, in.RevisedByUserID, now)
	var rule instance.Rule
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		if rule, err = s.d.Repository.Rule(ctx, tx, in.RuleID); err != nil {
			return err
		}
		if rule.Retired() {
			return fmt.Errorf("revise rule %s: %w", in.RuleID, instance.ErrRuleRetired)
		}
		hint := in.Hint
		if in.KeepHint {
			hint = rule.Revision.Hint
		}
		rule.Revision = instance.RuleRevision{
			ID: revID, RuleID: rule.ID, Position: rule.Revision.Position, Text: in.Text, Hint: hint, RevisedByUserID: in.RevisedByUserID, RevisedAt: now,
		}
		if err := s.d.Repository.InsertRuleRevision(ctx, tx, rule.Revision); err != nil {
			return err
		}
		if err := tx.Emit(ctx, RuleRevised{Revision: rule.Revision}); err != nil {
			return err
		}
		return tx.Emit(ctx, audit)
	})
	if err != nil {
		return instance.Rule{}, err
	}
	return rule, nil
}

// MoveRuleInput describes a new place of a rule among the rules in effect.
type MoveRuleInput struct {
	RuleID string
	// Position is the rule's new place, from 1; a place after the last rule moves it last.
	Position int
	// RevisedByUserID is the user making the change, or "" for the CLI.
	RevisedByUserID string
}

// MoveRule moves the rule to a place among the rules in effect and numbers them 1, 2, ... in
// their new order. Every rule whose position changes gets a revision with its text
// unchanged. It fails with an error wrapping ErrNotFound for an unknown rule,
// instance.ErrRuleRetired for a retired one, and instance.ErrInvalidValue for a position
// below 1.
func (s *Service) MoveRule(ctx context.Context, in MoveRuleInput) (instance.Rule, error) {
	if in.Position < 1 {
		return instance.Rule{}, fmt.Errorf("move rule %s: position %d is below 1: %w", in.RuleID, in.Position, instance.ErrInvalidValue)
	}
	now := s.d.Clock.Now()
	// The IDs stay the same across retries of the transaction.
	var ids []string
	id := func(i int) string {
		for len(ids) <= i {
			ids = append(ids, s.d.IDs.NewID())
		}
		return ids[i]
	}
	audit := s.audit(appmoderation.AuditRuleMoved, appmoderation.AuditTargetRule, in.RuleID, "", "", in.RevisedByUserID, now)
	var moved instance.Rule
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		source := ""
		var err error
		if moved, err = s.d.Repository.Rule(ctx, tx, in.RuleID); err != nil {
			return err
		}
		if moved.Retired() {
			return fmt.Errorf("move rule %s: %w", in.RuleID, instance.ErrRuleRetired)
		}
		heads, err := s.d.Repository.ListRuleHeads(ctx, tx)
		if err != nil {
			return err
		}
		heads = slices.DeleteFunc(heads, func(h instance.RuleHead) bool { return h.RuleID == in.RuleID })
		at := min(in.Position-1, len(heads))
		heads = slices.Insert(heads, at, instance.RuleHead{RuleID: in.RuleID, Position: moved.Revision.Position})
		n := 0
		for i, h := range heads {
			if h.Position == i+1 {
				continue
			}
			r := moved
			if h.RuleID != in.RuleID {
				if r, err = s.d.Repository.Rule(ctx, tx, h.RuleID); err != nil {
					return err
				}
			}
			r.Revision = instance.RuleRevision{
				ID: id(n), RuleID: r.ID, Position: i + 1, Text: r.Revision.Text, Hint: r.Revision.Hint, RevisedByUserID: in.RevisedByUserID, RevisedAt: now,
			}
			n++
			if err := s.d.Repository.InsertRuleRevision(ctx, tx, r.Revision); err != nil {
				return err
			}
			if err := tx.Emit(ctx, RuleRevised{Revision: r.Revision}); err != nil {
				return err
			}
			if r.ID == in.RuleID {
				moved, source = r, r.Revision.ID
			}
			if source == "" {
				source = r.Revision.ID
			}
		}
		if n == 0 {
			return nil // every rule is in place: nothing is recorded
		}
		// The entry's source is the moved rule's revision, or, when only the rules after it were
		// renumbered, the first revision recorded.
		e := withLabel(audit, moved.Revision.Text)
		e.Entry.SourceEventID = source
		return tx.Emit(ctx, e)
	})
	if err != nil {
		return instance.Rule{}, err
	}
	return moved, nil
}

// RetireRule retires the rule: it leaves the rules in effect, and reports that cite it still
// refer to it. Retiring a retired rule changes nothing. It fails with an error wrapping
// ErrNotFound for an unknown rule.
func (s *Service) RetireRule(ctx context.Context, ruleID, retiredByUserID string) (instance.Rule, error) {
	ret := instance.RuleRetirement{RuleID: ruleID, RetiredByUserID: retiredByUserID, RetiredAt: s.d.Clock.Now()}
	// The retirement's row is keyed by the rule's ID.
	audit := s.audit(appmoderation.AuditRuleRetired, appmoderation.AuditTargetRule, ruleID, "", ruleID, retiredByUserID, ret.RetiredAt)
	var rule instance.Rule
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		if rule, err = s.d.Repository.Rule(ctx, tx, ruleID); err != nil {
			return err
		}
		if rule.Retired() {
			return nil
		}
		inserted, err := s.d.Repository.InsertRuleRetirement(ctx, tx, ret)
		if err != nil || !inserted {
			return err
		}
		rule.Retirement = &ret
		if err := tx.Emit(ctx, RuleRetired{Retirement: ret}); err != nil {
			return err
		}
		return tx.Emit(ctx, withLabel(audit, rule.Revision.Text))
	})
	if err != nil {
		return instance.Rule{}, err
	}
	if rule.Retirement == nil {
		// Retired concurrently by someone else: read who did it.
		return s.d.Repository.FindRule(ctx, ruleID)
	}
	return rule, nil
}

// Rule returns a rule, retired or not, with its current revision; reports cite rules by ID.
// It fails with an error wrapping ErrNotFound for an unknown rule.
func (s *Service) Rule(ctx context.Context, id string) (instance.Rule, error) {
	return s.d.Repository.FindRule(ctx, id)
}

// RuleInEffect reports whether id is a rule in effect: one that exists and is not retired, which
// a new report may cite.
func (s *Service) RuleInEffect(ctx context.Context, id string) (bool, error) {
	r, err := s.d.Repository.FindRule(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !r.Retired(), nil
}

// Rules returns the rules in effect by position, followed by the retired ones when
// includeRetired is set.
func (s *Service) Rules(ctx context.Context, includeRetired bool) ([]instance.Rule, error) {
	return s.d.Repository.ListRules(ctx, includeRetired)
}

// withLabel returns e with the target label set, for an entry whose label is known only inside
// the transaction.
func withLabel(e appmoderation.AuditRecorded, label string) appmoderation.AuditRecorded {
	e.Entry.TargetLabel = label
	return e
}
