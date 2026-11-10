// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds the boosts and favourites of remote accounts, which the federation module
// records from the Announce, Like, and Undo activities it receives
// (docs/architecture/federation.md §3.4). They emit the same events as the local commands, so the
// counters, the feed entries, and the notifications follow them; the federation module sends
// nothing for them (a remote boost does not federate, and a remote favourite is of a local status).

// RemoteBoostInput is the boost that an Announce of a remote account records.
type RemoteBoostInput struct {
	// URI is the IRI of the Announce.
	URI              string
	BoosterAccountID string
	StatusID         string
	// Visibility is what the addressing of the Announce gives; the boost has the narrower of it
	// and the status's visibility (data-model §4.4).
	Visibility content.Visibility
	// BoostedAt is the published time of the Announce; zero, or a time in the future, is now.
	BoostedAt time.Time
}

// RecordRemoteBoost records the boost of a remote account (an Announce, federation.md §3.4) and
// returns it. The account must be remote and able to see the status: a local status as an
// ActivityPub fetch by that account would see it (content.CanView in ContextObjectFetch). A
// local-visibility status, a followers-only status of someone else, a direct status, and an
// Announce addressed to no one fail with content.ErrNotBoostable, and a local status whose boost
// rules do not admit the account at once with ErrBoostNotAllowed (ADR-0155 §6): such a boost is
// not recorded, so it is neither counted nor notified. The boost's ID has the time of
// the Announce, its visibility is the narrower of the Announce's and the status's, and it never
// federates (content.Boost.Federates).
//
// Recording the same Announce again, or another one while the account boosts the status, returns
// the existing boost and emits nothing.
func (s *Interactions) RecordRemoteBoost(ctx context.Context, in RemoteBoostInput) (content.Boost, error) {
	st, err := s.remoteCountedStatus(ctx, in.BoosterAccountID, in.StatusID)
	if err != nil {
		return content.Boost{}, err
	}
	isAuthor := st.AuthorAccountID == in.BoosterAccountID
	vis := content.Narrower(in.Visibility, st.Visibility)
	switch {
	case st.Visibility == content.VisibilityLocal, !st.Visibility.Boostable(isAuthor):
		return content.Boost{}, fmt.Errorf("boost %s by %s: %w", st.ID, in.BoosterAccountID, content.ErrNotBoostable)
	case !vis.Valid(), vis == content.VisibilityDirect, vis == content.VisibilityLocal:
		return content.Boost{}, fmt.Errorf("boost %s addressed as %q: %w", st.ID, in.Visibility, content.ErrNotBoostable)
	}
	if st.IsLocal {
		if err := s.remoteMayBoost(ctx, in.BoosterAccountID, st); err != nil {
			return content.Boost{}, err
		}
	}
	at := s.remoteTime(in.BoostedAt)
	b := content.Boost{
		ID: s.d.IDs.NewIDAt(at), URI: in.URI, BoosterAccountID: in.BoosterAccountID, StatusID: st.ID,
		Visibility: vis, IsLocal: false, BoostedAt: at,
	}
	var out content.Boost
	repo := s.d.Interactions
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		out = content.Boost{}
		if err := s.ensureLive(ctx, tx, st.ID); err != nil {
			return err
		}
		edge, err := repo.BoostEdge(ctx, tx, in.BoosterAccountID, st.ID)
		switch {
		case err == nil:
			out, err = repo.Boost(ctx, tx, edge.BoostID)
			return err
		case !errors.Is(err, ErrNotFound):
			return err
		}
		if err := repo.InsertBoost(ctx, tx, b); err != nil {
			return err
		}
		out = b
		return tx.Emit(ctx, Boosted{Boost: b, Status: RefOf(st)})
	})
	if errors.Is(err, ErrBoostExists) {
		// The Announce is recorded already, live or retracted.
		return repo.FindBoostByURI(ctx, in.URI)
	}
	if err != nil {
		return content.Boost{}, err
	}
	return out, nil
}

// RetractRemoteBoost retracts the boost whose Announce is uri (an Undo Announce), on behalf of
// boosterAccountID. A boost that does not exist fails with ErrNotFound, and one of another
// account with ErrNotAuthor. Retracting a boost again changes nothing; so does retracting the
// boost of a deleted status, which has no live edge, except that its retraction is recorded.
func (s *Interactions) RetractRemoteBoost(ctx context.Context, uri, boosterAccountID string) error {
	b, err := s.d.Interactions.FindBoostByURI(ctx, uri)
	if err != nil {
		return err
	}
	if b.BoosterAccountID != boosterAccountID || b.IsLocal {
		return fmt.Errorf("boost %s: %w", uri, ErrNotAuthor)
	}
	st, err := s.d.Statuses.FindStatus(ctx, b.StatusID)
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	repo := s.d.Interactions
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		edge, err := repo.BoostEdge(ctx, tx, b.BoosterAccountID, b.StatusID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		live := err == nil && edge.BoostID == b.ID
		if !live {
			// Retracted already, or of a deleted status, whose boosts left the projections with it.
			switch _, err := repo.FindBoostRetractionTx(ctx, tx, b.ID); {
			case err == nil:
				return nil
			case !errors.Is(err, ErrNotFound):
				return err
			}
		}
		r := content.BoostRetraction{BoostID: b.ID, Origin: content.RetractedByRemote, RetractedAt: now}
		err = repo.InsertBoostRetraction(ctx, tx, r)
		if errors.Is(err, ErrBoostRetracted) {
			// A concurrent retraction won; a new attempt finds it.
			return uow.MarkRetryable(err)
		}
		if err != nil || !live {
			return err
		}
		return tx.Emit(ctx, BoostRetracted{Boost: b, Retraction: r, Status: RefOf(st)})
	})
}

// RemoteFavouriteInput is the favourite that a Like (or an emoji reaction) of a remote account
// records.
type RemoteFavouriteInput struct {
	// URI is the IRI of the Like.
	URI       string
	AccountID string
	StatusID  string
	// CreatedAt is the published time of the Like; zero, or a time in the future, is now.
	CreatedAt time.Time
}

// RecordRemoteFavourite records the favourite of a remote account (a Like, federation.md §3.4)
// and returns it. Only a local status takes remote favourites, and only one the account can see
// (content.CanView in ContextObjectFetch); anything else fails with ErrNotFound. An account under
// the interact restriction (FR-708) fails with ErrInteractionRestricted. Recording a
// favourite that the account already has, or a Like whose URI is recorded already, changes
// nothing and returns the existing favourite.
//
// Since remote accounts favourite only local statuses, the favourite of a remote status is always
// a local account's, the one whose Like the federation module sends.
func (s *Interactions) RecordRemoteFavourite(ctx context.Context, in RemoteFavouriteInput) (content.Favourite, error) {
	if err := s.checkRestrictions(ctx, in.AccountID, false); err != nil {
		return content.Favourite{}, err
	}
	st, err := s.remoteCountedStatus(ctx, in.AccountID, in.StatusID)
	if err != nil {
		return content.Favourite{}, err
	}
	if !st.IsLocal {
		return content.Favourite{}, fmt.Errorf("remote status %s: %w", st.ID, ErrNotFound)
	}
	at := s.remoteTime(in.CreatedAt)
	f := content.Favourite{ID: s.d.IDs.NewIDAt(at), AccountID: in.AccountID, StatusID: st.ID, URI: in.URI, CreatedAt: at}
	inserted := false
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		inserted = false
		if err := s.ensureLive(ctx, tx, st.ID); err != nil {
			return err
		}
		ok, err := s.d.Interactions.InsertFavourite(ctx, tx, f)
		if err != nil || !ok {
			return err
		}
		inserted = true
		return tx.Emit(ctx, Favourited{Favourite: f, Status: RefOf(st)})
	})
	switch {
	case errors.Is(err, ErrFavouriteURIUsed):
		return s.d.Interactions.FindFavouriteByURI(ctx, in.URI)
	case err != nil:
		return content.Favourite{}, err
	case inserted:
		return f, nil
	}
	return s.d.Interactions.FindFavourite(ctx, in.AccountID, st.ID)
}

// RemoveRemoteFavourite removes the favourite of a remote account that an Undo Like takes back:
// the favourite whose Like is uri, or, when no favourite has that URI, the account's favourite of
// statusID ("" for none). A favourite that does not exist fails with ErrNotFound, and one of
// another account with ErrNotAuthor.
func (s *Interactions) RemoveRemoteFavourite(ctx context.Context, uri, accountID, statusID string) error {
	f, err := s.d.Interactions.FindFavouriteByURI(ctx, uri)
	switch {
	case err == nil:
		if f.AccountID != accountID {
			return fmt.Errorf("favourite %s: %w", uri, ErrNotAuthor)
		}
		statusID = f.StatusID
	case !errors.Is(err, ErrNotFound):
		return err
	case statusID == "":
		return err
	}
	st, err := s.d.Statuses.FindStatus(ctx, statusID)
	if err != nil {
		return err
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		f, deleted, err := s.d.Interactions.DeleteFavourite(ctx, tx, accountID, statusID)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("favourite of %s by %s: %w", statusID, accountID, ErrNotFound)
		}
		return tx.Emit(ctx, Unfavourited{Favourite: f, Status: RefOf(st), RemovedAt: s.d.Clock.Now()})
	})
}

// remoteVisibleStatus returns statusID when accountID is a remote account that may see it. A
// remote status is visible to every remote account that knows of it: its own server decides who
// sees it. A local status is judged as an ActivityPub fetch by the account would be
// (content.CanView in ContextObjectFetch). A status that is missing, deleted, or hidden fails
// with ErrNotFound; an account that is missing or local with ErrActorUnavailable. See
// remoteCountedStatus for what the status of a deactivated author takes.
func (s *Interactions) remoteVisibleStatus(ctx context.Context, accountID, statusID string) (content.Status, error) {
	actor, err := s.d.Accounts.FindByID(ctx, accountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return content.Status{}, fmt.Errorf("account %s: %w", accountID, ErrActorUnavailable)
	}
	if err != nil {
		return content.Status{}, err
	}
	if actor.Account.IsLocal() {
		return content.Status{}, fmt.Errorf("account %s is local: %w", accountID, ErrActorUnavailable)
	}
	a := s.audience()
	st, err := a.live(ctx, statusID)
	if err != nil || !st.IsLocal {
		return st, err
	}
	ok, err := a.remoteVisible(ctx, accountID, st)
	if err != nil {
		return content.Status{}, err
	}
	if !ok {
		return content.Status{}, fmt.Errorf("status %s: %w", statusID, ErrNotFound)
	}
	return st, nil
}

// remoteCountedStatus is remoteVisibleStatus for a boost, a favourite, or a reaction, which remote
// servers that still show a post may send: it counts on a status that only its author's deactivation
// hides, so that the numbers are right when the account comes back, and the notification
// projector tells the deactivated author nothing (ADR-0089 §16).
func (s *Interactions) remoteCountedStatus(ctx context.Context, accountID, statusID string) (content.Status, error) {
	st, err := s.remoteVisibleStatus(ctx, accountID, statusID)
	if !errors.Is(err, ErrNotFound) {
		return st, err
	}
	a := s.audience()
	st, liveErr := a.live(ctx, statusID)
	if liveErr != nil {
		return content.Status{}, err
	}
	author, findErr := a.accounts.FindByID(ctx, st.AuthorAccountID)
	if findErr != nil || !author.Lifecycle.Deactivated() {
		return content.Status{}, err
	}
	switch ok, visErr := a.remoteVisibleAsActive(ctx, accountID, st); {
	case visErr != nil:
		return content.Status{}, visErr
	case !ok:
		return content.Status{}, err
	}
	return st, nil
}

// remoteVisibleAsActive is remoteVisible as though the author of st had not left.
func (a audience) remoteVisibleAsActive(ctx context.Context, accountID string, st content.Status) (bool, error) {
	r, err := a.restrictions.Restrictions(ctx, st.AuthorAccountID)
	if err != nil {
		return false, err
	}
	rel, err := a.relation(ctx, accountID, st)
	if err != nil {
		return false, err
	}
	viewer := content.Viewer{Kind: content.ViewerRemote, AccountID: accountID}
	facts := content.StatusFacts{AuthorAccountID: st.AuthorAccountID, AuthorIsLocal: st.IsLocal, Visibility: st.Visibility}
	return content.CanView(content.ContextObjectFetch, viewer, facts, r.Author, rel), nil
}

// remoteVisible reports whether the remote account accountID may see the local status st, as
// content.CanView decides for an ActivityPub fetch by that account.
func (a audience) remoteVisible(ctx context.Context, accountID string, st content.Status) (bool, error) {
	r, err := a.restrictions.Restrictions(ctx, st.AuthorAccountID)
	if err != nil {
		return false, err
	}
	restrictions := r.Author
	author, err := a.accounts.FindByID(ctx, st.AuthorAccountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		restrictions.ProfileView = true
	case err != nil:
		return false, err
	default:
		restrictions.ProfileView = restrictions.ProfileView || !author.Lifecycle.Active()
	}
	rel, err := a.relation(ctx, accountID, st)
	if err != nil {
		return false, err
	}
	viewer := content.Viewer{Kind: content.ViewerRemote, AccountID: accountID}
	facts := content.StatusFacts{AuthorAccountID: st.AuthorAccountID, AuthorIsLocal: st.IsLocal, Visibility: st.Visibility}
	return content.CanView(content.ContextObjectFetch, viewer, facts, restrictions, rel), nil
}

// relation returns what CanView needs of the relationship of accountID with the author of st.
func (a audience) relation(ctx context.Context, accountID string, st content.Status) (content.Relation, error) {
	var rel content.Relation
	r, err := a.relationships.Relationship(ctx, accountID, st.AuthorAccountID)
	if err != nil {
		return rel, err
	}
	rel.Blocking, rel.BlockedBy = r.Blocking, r.BlockedBy
	switch {
	case r.ActivelyFollowing():
		rel.Follow = content.FollowActive
	case r.RequestedFollow():
		rel.Follow = content.FollowPending
	}
	rel.Mentioned, err = a.interactions.Mentioned(ctx, st.ID, accountID)
	return rel, err
}

// remoteTime returns the time of a received activity: at, normalized, unless it is zero or in
// the future, which is now.
func (s *Interactions) remoteTime(at time.Time) time.Time {
	now := s.d.Clock.Now()
	at = kernel.Normalize(at)
	if at.IsZero() || at.After(now) {
		return now
	}
	return at
}

// remoteMayBoost fails with ErrBoostNotAllowed unless the boost rules of the local status st admit
// a boost by the remote account accountID, which may see it, at once. One that waits for the
// author's approval is not recorded either: this server does not hold one yet.
func (s *Interactions) remoteMayBoost(ctx context.Context, accountID string, st content.Status) error {
	decision, err := s.interactionReaders().mayInteract(ctx, content.InteractionAnnounce, accountID, false, nil, st)
	if err != nil {
		return err
	}
	if decision != content.InteractionAllowed {
		return fmt.Errorf("boost %s by %s: %w", st.ID, accountID, ErrBoostNotAllowed)
	}
	return nil
}
