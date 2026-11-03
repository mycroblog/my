// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/federation"
)

// ObjectBoosts reads the boosts whose Announce object fetches serve; *appcontent.Interactions
// implements it.
type ObjectBoosts interface {
	FindLiveBoost(ctx context.Context, id string) (appcontent.Boost, error)
}

// Announce answers a GET of the id of an activity of this server for req (ADR-0144 §5): the
// Announce of a local boost that is public or unlisted and not retracted, built as the fan-out
// builds it. Any other activity, a booster that CanView hides or a ceiling restricts, a boost of a
// deleted status, and a local status that CanView hides from req fail with ErrNotFound alike.
func (o *Objects) Announce(ctx context.Context, req Requester, id string) (Object, error) {
	missing := fmt.Errorf("activity %s: %w", id, ErrNotFound)
	if o.d.Boosts == nil {
		return Object{}, missing
	}
	b, err := o.d.Boosts.FindLiveBoost(ctx, id)
	if errors.Is(err, appcontent.ErrNotFound) {
		return Object{}, missing
	}
	if err != nil {
		return Object{}, err
	}
	if !b.Federates() {
		return Object{}, missing
	}
	boosterLimits, err := o.d.Statuses.AuthorRestrictions(ctx, b.BoosterAccountID)
	if err != nil {
		return Object{}, err
	}
	vis := appcontent.EffectiveVisibility(b.Visibility, boosterLimits)
	if vis != appcontent.VisibilityPublic && vis != appcontent.VisibilityUnlisted {
		return Object{}, missing
	}
	st, rev, deleted, err := o.d.Statuses.ObjectStatus(ctx, b.StatusID)
	if errors.Is(err, appcontent.ErrNotFound) || (err == nil && deleted) {
		return Object{}, missing
	}
	if err != nil {
		return Object{}, err
	}
	var statusLimits appcontent.AuthorRestrictions
	if st.IsLocal {
		if statusLimits, err = o.d.Statuses.AuthorRestrictions(ctx, st.AuthorAccountID); err != nil {
			return Object{}, err
		}
	}
	// The boost is judged as a status of its booster. CanView in object fetches serves only this
	// server's statuses, so a remote boosted status is not judged again.
	boost := appcontent.Status{AuthorAccountID: b.BoosterAccountID, IsLocal: true, Visibility: b.Visibility}
	mayView := func(req Requester) (bool, error) {
		ok, err := o.canView(ctx, req, boost, appcontent.Revision{}, boosterLimits)
		if err != nil || !ok || !st.IsLocal {
			return ok, err
		}
		return o.canView(ctx, req, st, rev, statusLimits)
	}
	ok, err := mayView(req)
	if err != nil {
		return Object{}, err
	}
	if !ok {
		return Object{}, missing
	}
	out := Object{Public: req.Anonymous()}
	if !out.Public {
		if out.Public, err = mayView(Requester{}); err != nil {
			return Object{}, err
		}
	}
	booster, err := o.d.Accounts.FindByID(ctx, b.BoosterAccountID)
	if err != nil {
		return Object{}, fmt.Errorf("booster of %s: %w", id, err)
	}
	var mentioned []federation.IRI
	if st.AuthorAccountID != b.BoosterAccountID {
		switch author, err := o.d.Accounts.FindByID(ctx, st.AuthorAccountID); {
		case errors.Is(err, appaccount.ErrNotFound):
		case err != nil:
			return Object{}, err
		default:
			mentioned = append(mentioned, federation.IRI(author.Account.URI))
		}
	}
	out.Document, err = announceDocument(b, st, federation.IRI(booster.Account.URI), vis, mentioned).Marshal()
	return out, err
}
