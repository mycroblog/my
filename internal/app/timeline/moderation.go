// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// Accounts reads the lifecycle of the accounts that moderation restricts. *appaccount.Service
// implements it.
type Accounts interface {
	FindByIDTx(ctx context.Context, tx uow.Tx, id string) (appaccount.Actor, error)
}

// ModeratedEffects reads, in the transaction, the restrictions that moderation has in effect on an
// account. A reactivation falls back to them: an action in effect stays when the account comes
// back (ADR-0089 §14). *appmoderation.AccountPolicy implements it.
type ModeratedEffects interface {
	AccountEffectsTx(ctx context.Context, tx uow.Tx, accountID string) ([]appmoderation.Effect, error)
}

// feedRestrictions are the moderation restrictions that the timelines apply (data-model §4.6).
var feedRestrictions = []timeline.Restriction{
	timeline.RestrictProfileView, timeline.RestrictVisibilityCeilingPrivate, timeline.RestrictPublicDiscovery,
}

// RegisterModerationProjectors subscribes feed_author_restrictions and feed_domain_restrictions to
// the moderation module's events: each event carries the restrictions in effect on one account
// or domain, and the projectors replace the timeline restrictions of that account or domain
// with those among them that bear on timelines, each with the latest end of the actions that
// impose it. A deleted or a deactivated account keeps the profile_view that its lifecycle gave it.
func RegisterModerationProjectors(r *uow.Projectors, repo Repository, accounts Accounts) {
	r.Register(appmoderation.EventAccountRestrictionsChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appmoderation.AccountRestrictionsChanged)
		effects := ev.Effects
		a, err := accounts.FindByIDTx(ctx, tx, ev.AccountID)
		switch {
		case errors.Is(err, appaccount.ErrNotFound):
		case err != nil:
			return err
		case a.Deleted(), a.Lifecycle.Deactivated():
			// Deletion is final (data-model §8), and a deactivation lasts until the reactivation
			// (ADR-0089 §15), whatever moderation does meanwhile.
			effects = lifecycleHidden(effects)
		}
		return replaceRestrictions(effects, func(r timeline.Restriction, until *time.Time) error {
			return repo.UpsertAuthorRestriction(ctx, tx, AuthorRestriction{AccountID: ev.AccountID, Restriction: r, EffectiveUntil: until})
		}, func(r timeline.Restriction) error {
			return repo.DeleteAuthorRestriction(ctx, tx, ev.AccountID, r)
		})
	})
	r.Register(appmoderation.EventDomainRestrictionsChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appmoderation.DomainRestrictionsChanged)
		return replaceRestrictions(ev.Effects, func(r timeline.Restriction, until *time.Time) error {
			return repo.UpsertDomainRestriction(ctx, tx, DomainRestriction{Domain: ev.Domain, Restriction: r, EffectiveUntil: until})
		}, func(r timeline.Restriction) error {
			return repo.DeleteDomainRestriction(ctx, tx, ev.Domain, r)
		})
	})
}

// RebuildRestrictions empties feed_author_restrictions and feed_domain_restrictions and derives
// them again, in one transaction, by applying the projectors of RegisterModerationProjectors and
// of the deletions and deactivations of accounts to the events that sources replay (data-model
// §9): the restrictions in effect that moderation replays, and the deletions and the deactivations
// in force that account replays.
//
// The commands replace the restrictions of an account or a domain only after an action or a
// lift, so a row whose end has passed stays until the next one; it restricts nothing (the reads
// compare its end with the time). The rebuild keeps the rows that the tables hold whose end has
// passed at the time of clock, unless it derives a row for the same restriction.
func RebuildRestrictions(ctx context.Context, u uow.UnitOfWork, repo Repository, accounts Accounts, moderated ModeratedEffects, clock kernel.Clock, sources ...Source) error {
	projectors := &uow.Projectors{}
	RegisterModerationProjectors(projectors, repo, accounts)
	registerLifecycleProjector(projectors, repo, moderated)
	err := u.Tx(ctx, func(tx uow.Tx) error {
		now := kernel.Normalize(clock.Now())
		authors, domains, err := repo.ListRestrictions(ctx, tx)
		if err != nil {
			return err
		}
		if err := repo.DeleteRestrictions(ctx, tx); err != nil {
			return err
		}
		for _, src := range sources {
			if err := src(ctx, tx, func(e uow.Event) error { return projectors.Project(ctx, tx, e) }); err != nil {
				return err
			}
		}
		derivedAuthors, derivedDomains, err := repo.ListRestrictions(ctx, tx)
		if err != nil {
			return err
		}
		for _, r := range authors {
			if !ended(r.EffectiveUntil, now) || slices.ContainsFunc(derivedAuthors, func(d AuthorRestriction) bool {
				return d.AccountID == r.AccountID && d.Restriction == r.Restriction
			}) {
				continue
			}
			if err := repo.UpsertAuthorRestriction(ctx, tx, r); err != nil {
				return err
			}
		}
		for _, r := range domains {
			if !ended(r.EffectiveUntil, now) || slices.ContainsFunc(derivedDomains, func(d DomainRestriction) bool {
				return d.Domain == r.Domain && d.Restriction == r.Restriction
			}) {
				continue
			}
			if err := repo.UpsertDomainRestriction(ctx, tx, r); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("rebuild the feed restrictions: %w", err)
	}
	return nil
}

// lifecycleHidden returns effects with the profile_view of an account whose lifecycle hides it,
// which no action ends.
func lifecycleHidden(effects []appmoderation.Effect) []appmoderation.Effect {
	return append(slices.Clone(effects), appmoderation.Effect{Restriction: appmoderation.RestrictProfileView})
}

// ended reports whether a restriction that lasts until until has ended at now.
func ended(until *time.Time, now time.Time) bool { return until != nil && !until.After(now) }

// replaceRestrictions upserts the timeline restrictions among effects and deletes the others.
func replaceRestrictions(effects []appmoderation.Effect, upsert func(timeline.Restriction, *time.Time) error, remove func(timeline.Restriction) error) error {
	for _, r := range feedRestrictions {
		var (
			found bool
			until *time.Time
		)
		for _, e := range effects {
			if string(e.Restriction) != string(r) {
				continue
			}
			if !found || (until != nil && (e.Until == nil || e.Until.After(*until))) {
				until = e.Until
			}
			found = true
		}
		var err error
		if found {
			err = upsert(r, until)
		} else {
			err = remove(r)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
