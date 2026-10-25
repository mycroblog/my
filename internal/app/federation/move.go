// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The Move of a remote account (federation.md §3.4, ADR-0089, FR-414). A Move is processed only
// when its actor moves itself to a target that names the actor among its aliases, the target has
// not moved itself, and neither account nor its domain is suspended; anything else is dropped
// with its reason in the log and in my.move.activities. A processed Move records the move in the
// account module, whose account.moved makes the graph module move the local followers.

// The outcomes of a received Move, the my.outcome label of my.move.activities.
const (
	// MoveOutcomeRecorded is a Move that passed the checks and was recorded.
	MoveOutcomeRecorded = "recorded"
	// MoveOutcomeDuplicate is a Move to a target that the account already moved to.
	MoveOutcomeDuplicate = "duplicate"
	// MoveOutcomeNotOwnActor is a Move whose object is not its actor (FEP-7628's pull mode, or
	// a Move of someone else).
	MoveOutcomeNotOwnActor = "not_own_actor"
	// MoveOutcomeSameAccount is a Move without a target or to its own actor.
	MoveOutcomeSameAccount = "same_account"
	// MoveOutcomeTargetUnavailable is a Move whose target could not be fetched, is not the
	// actor it names, or is deleted.
	MoveOutcomeTargetUnavailable = "target_unavailable"
	// MoveOutcomeAliasMissing is a Move to a target whose alsoKnownAs does not name the actor.
	MoveOutcomeAliasMissing = "alias_missing"
	// MoveOutcomeTargetMoved is a Move to a target that moved itself (chains are not followed).
	MoveOutcomeTargetMoved = "target_moved"
	// MoveOutcomeSuspended is a Move of or to an account that this server suspends, or whose
	// domain it suspends.
	MoveOutcomeSuspended = "suspended"
	// MoveOutcomeTooSoon is a Move to another target within account.RemoteMoveInterval after
	// the account's last move.
	MoveOutcomeTooSoon = "too_soon"
)

// MoveActivities counts the received Moves by what became of them (ADR-0089).
var MoveActivities = kernel.Instrument{
	Name: "my.move.activities", Unit: "{activity}", Description: "Move activities received, by whether they were recorded or why they were dropped (ADR-0089).",
	LabelKeys: []string{LabelOutcome},
}

// MoveTargets fetches the target of a Move from its origin, without a cache, and stores it;
// *RemoteActors implements it.
type MoveTargets interface {
	// RefreshActor fetches the actor at uri again, stores it, and returns it with the document
	// it was stored from.
	RefreshActor(ctx context.Context, uri string) (appaccount.Actor, *federation.Actor, error)
}

// MoveRecorder records the moves of remote accounts and reads the moves in force;
// *appaccount.Service implements it.
type MoveRecorder interface {
	FindByURI(ctx context.Context, uri string) (appaccount.Actor, error)
	RecordRemoteMove(ctx context.Context, in appaccount.RemoteMoveInput) (appaccount.Move, bool, error)
	MoveState(ctx context.Context, accountID string) (appaccount.MoveState, error)
}

// MoveLimits says whether moderation limits an account, which the target of its move does not
// inherit; *appmoderation.FederationPolicy implements it.
type MoveLimits interface {
	AccountLimited(ctx context.Context, accountID string) (bool, error)
}

// LocalAliases says whether a local account names a remote actor among its aliases, which lets
// that actor move to it (ADR-0089).
type LocalAliases interface {
	HasAlias(ctx context.Context, accountID, aliasURI string) (bool, error)
}

// MoveHandlerDeps are the ports of the handler of received Moves.
type MoveHandlerDeps struct {
	Targets  MoveTargets
	Accounts MoveRecorder
	// Domains and Suspensions are the suspensions of domains and accounts; nil suspends none.
	Domains     DomainPolicy
	Suspensions AccountPolicy
	// Limits tells the moved accounts that moderation limits; nil limits none.
	Limits MoveLimits
	// Aliases are the aliases of the local accounts; nil means that none has one, so no remote
	// account moves to a local one.
	Aliases LocalAliases
	// Host is this server's serving domain, which tells a local target.
	Host   string
	Meter  kernel.Meter
	Logger *slog.Logger
}

// RegisterMoveHandler registers the inbox handler of Move on p (federation.md §3.4).
func RegisterMoveHandler(p *InboxProcessor, d MoveHandlerDeps) {
	if d.Domains == nil {
		d.Domains = NoDomainRestrictions{}
	}
	if d.Suspensions == nil {
		d.Suspensions = NoAccountRestrictions{}
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	h := &moveHandler{d: d, moves: kernel.MeterOrNop(d.Meter).Counter(MoveActivities)}
	p.Handle(federation.ActivityMove, h.move)
}

type moveHandler struct {
	d     MoveHandlerDeps
	moves kernel.Counter
}

// drop counts and logs a Move that is not processed, and returns the error that completes it.
func (h *moveHandler) drop(ctx context.Context, a *InboxActivity, outcome string, cause error) error {
	h.moves.Add(ctx, 1, kernel.NewLabels(LabelOutcome, outcome))
	h.d.Logger.InfoContext(ctx, "move dropped",
		slog.String("activity_uri", string(a.Received.ActivityURI)), slog.String("actor", string(a.Activity.Actor)),
		slog.String("target", string(a.Activity.Target)), slog.String("reason", outcome), slog.Any("error", cause))
	return fmt.Errorf("%w: Move %s: %s", ErrDropActivity, a.Received.ActivityURI, outcome)
}

// move is the inbox handler of Move. The processor has checked that the signer is the actor
// (§3.2) and that the actor is a stored remote account that the federation mode receives from.
func (h *moveHandler) move(ctx context.Context, a *InboxActivity) error {
	act := a.Activity
	switch {
	case act.Object().ID != act.Actor:
		return h.drop(ctx, a, MoveOutcomeNotOwnActor, nil)
	case act.Target == "" || act.Target == act.Actor:
		return h.drop(ctx, a, MoveOutcomeSameAccount, nil)
	}
	source := a.Actor
	target, outcome, err := h.target(ctx, act)
	if err != nil || outcome != "" {
		if err != nil && outcome == "" {
			return err
		}
		return h.drop(ctx, a, outcome, err)
	}
	if target.Account.ID == source.Account.ID {
		return h.drop(ctx, a, MoveOutcomeSameAccount, nil)
	}
	switch suspended, err := h.suspended(ctx, source, target); {
	case err != nil:
		return err
	case suspended:
		return h.drop(ctx, a, MoveOutcomeSuspended, nil)
	}
	limited := false
	if h.d.Limits != nil {
		if limited, err = h.d.Limits.AccountLimited(ctx, source.Account.ID); err != nil {
			return err
		}
	}
	_, recorded, err := h.d.Accounts.RecordRemoteMove(ctx, appaccount.RemoteMoveInput{
		AccountID: source.Account.ID, TargetAccountID: target.Account.ID, ActivityURI: string(a.Received.ActivityURI),
		SourceLimited: limited,
	})
	switch {
	case errors.Is(err, appaccount.ErrMoveTooSoon):
		return h.drop(ctx, a, MoveOutcomeTooSoon, err)
	case err != nil:
		return err
	case !recorded:
		h.moves.Add(ctx, 1, kernel.NewLabels(LabelOutcome, MoveOutcomeDuplicate))
		return nil
	}
	h.moves.Add(ctx, 1, kernel.NewLabels(LabelOutcome, MoveOutcomeRecorded))
	return nil
}

// target returns the target of a Move that names the actor among its aliases and has not moved
// itself, or the outcome that drops the Move. A remote target is fetched again from its origin;
// a failure worth retrying is returned with no outcome.
func (h *moveHandler) target(ctx context.Context, act *federation.Activity) (appaccount.Actor, string, error) {
	if act.Target.Host() == h.d.Host {
		return h.localTarget(ctx, act)
	}
	stored, doc, err := h.d.Targets.RefreshActor(ctx, string(act.Target))
	switch {
	case err != nil && dropping(err):
		return appaccount.Actor{}, MoveOutcomeTargetUnavailable, err
	case err != nil:
		return appaccount.Actor{}, "", err
	case doc == nil || doc.ID != act.Target || !stored.Lifecycle.Active():
		return appaccount.Actor{}, MoveOutcomeTargetUnavailable, nil
	case !slices.Contains(doc.AlsoKnownAs, act.Actor):
		return appaccount.Actor{}, MoveOutcomeAliasMissing, nil
	case doc.MovedTo != "":
		return appaccount.Actor{}, MoveOutcomeTargetMoved, nil
	}
	return h.checkedTarget(ctx, stored)
}

// localTarget is target of a local account.
func (h *moveHandler) localTarget(ctx context.Context, act *federation.Activity) (appaccount.Actor, string, error) {
	t, err := h.d.Accounts.FindByURI(ctx, string(act.Target))
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return appaccount.Actor{}, MoveOutcomeTargetUnavailable, nil
	case err != nil:
		return appaccount.Actor{}, "", err
	case !t.Account.IsLocal() || t.Local == nil || !t.Lifecycle.Active():
		return appaccount.Actor{}, MoveOutcomeTargetUnavailable, nil
	}
	aliased := false
	if h.d.Aliases != nil {
		if aliased, err = h.d.Aliases.HasAlias(ctx, t.Account.ID, string(act.Actor)); err != nil {
			return appaccount.Actor{}, "", err
		}
	}
	if !aliased {
		return appaccount.Actor{}, MoveOutcomeAliasMissing, nil
	}
	return h.checkedTarget(ctx, t)
}

// checkedTarget returns t, or MoveOutcomeTargetMoved when t has a move in force here.
func (h *moveHandler) checkedTarget(ctx context.Context, t appaccount.Actor) (appaccount.Actor, string, error) {
	switch _, err := h.d.Accounts.MoveState(ctx, t.Account.ID); {
	case err == nil:
		return appaccount.Actor{}, MoveOutcomeTargetMoved, nil
	case !errors.Is(err, appaccount.ErrNotFound):
		return appaccount.Actor{}, "", err
	}
	return t, "", nil
}

// suspended reports whether this server suspends either account or its domain.
func (h *moveHandler) suspended(ctx context.Context, accounts ...appaccount.Actor) (bool, error) {
	for _, a := range accounts {
		if !a.Account.IsLocal() {
			s, err := h.d.Domains.FederationSuspended(ctx, a.Account.Domain)
			if err != nil || s {
				return s, err
			}
		}
		s, err := h.d.Suspensions.AccountFederationSuspended(ctx, a.Account.ID)
		if err != nil || s {
			return s, err
		}
	}
	return false, nil
}

// RefreshActor implements MoveTargets: it fetches the actor at uri again from its origin and
// stores it, as Refresh does, and returns the document it read too.
func (r *RemoteActors) RefreshActor(ctx context.Context, uri string) (appaccount.Actor, *federation.Actor, error) {
	actor, raw, err := r.fetchActor(ctx, uri)
	if err != nil {
		r.recordFailure(ctx, uri, err)
		return appaccount.Actor{}, nil, err
	}
	stored, err := r.store(ctx, actor, raw)
	if err != nil {
		return appaccount.Actor{}, nil, err
	}
	return stored, actor, nil
}
