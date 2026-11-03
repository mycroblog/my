// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// ErrActorUnavailable reports an acting account that does not exist, is not local, or is not
// active: interactions are commands of local accounts, and federated ones go through the
// federation module.
var ErrActorUnavailable = errors.New("content: the acting account does not exist, is not local, or is not active")

// InteractionDeps are the ports and settings an Interactions service uses.
type InteractionDeps struct {
	UnitOfWork    uow.UnitOfWork
	Statuses      Repository
	Interactions  InteractionRepository
	Accounts      Accounts
	Relationships Relationships
	// Restrictions are the authors' moderation restrictions; nil means NoRestrictions.
	Restrictions RestrictionPolicy
	// RecordRemoteEmojis records the custom emoji that remote reactions use, as those of remote
	// posts (Service.RecordRemoteEmojis); nil refuses those reactions.
	RecordRemoteEmojis func(ctx context.Context, tx uow.Tx, ownerAccountID, domain string, emojis []RemoteEmojiInput) ([]string, error)
	// EmojiTags reads the enabled custom emoji that local accounts react with, with their images
	// (MediaService.EmojiTags); nil refuses the reactions with custom emoji.
	EmojiTags func(ctx context.Context, ids []string) ([]EmojiTag, error)
	// Quotes stores the quotes and the approvals this server gives (ADR-0069); nil answers
	// every QuoteRequest with ErrNotFound.
	Quotes QuoteRepository
	Clock  kernel.Clock
	IDs    kernel.IDGenerator
	// Host is the serving domain of the Announce and Like activity URIs (ADR-0018).
	Host string
}

// Interactions runs the use cases of local accounts acting on statuses (FR-107, FR-113, FR-116):
// boosting, favouriting, reacting with an emoji, bookmarking, and pinning, and undoing each. Every command is safe to
// repeat: doing what is already done, or undoing what is not, changes nothing and emits no
// event, so the counters never move twice.
//
// An account may act only on a status it can see: each command except the undoing ones checks
// the status with content.CanView in ContextStatus, and a status the account cannot see, or a
// deleted one, fails with ErrNotFound, the same as a missing one, so that its existence does
// not leak (docs/architecture/visibility.md).
type Interactions struct {
	d InteractionDeps
}

// NewInteractions returns an Interactions service on d.
func NewInteractions(d InteractionDeps) *Interactions {
	if d.Restrictions == nil {
		d.Restrictions = NoRestrictions{}
	}
	return &Interactions{d: d}
}

// Boost makes accountID boost statusID and returns the live boost. Boosting a status that the
// account already boosts returns the existing boost. The boost copies the status's visibility;
// a boost of a local status never federates (content.Boost.Federates, ADR-0017). A status whose
// visibility does not allow the account to boost it fails with content.ErrNotBoostable, and an
// account whose restrictions forbid posting or interacting with ErrPostingRestricted or
// ErrInteractionRestricted.
func (s *Interactions) Boost(ctx context.Context, accountID, statusID string) (content.Boost, error) {
	if err := s.checkRestrictions(ctx, accountID, true); err != nil {
		return content.Boost{}, err
	}
	st, err := s.visibleStatus(ctx, accountID, statusID)
	if err != nil {
		return content.Boost{}, err
	}
	if !st.Visibility.Boostable(st.AuthorAccountID == accountID) {
		return content.Boost{}, fmt.Errorf("boost %s: %w", statusID, content.ErrNotBoostable)
	}
	id := s.d.IDs.NewID()
	b := content.Boost{
		ID: id, URI: content.ActivityURI(s.d.Host, id), BoosterAccountID: accountID, StatusID: st.ID,
		Visibility: st.Visibility, IsLocal: true, BoostedAt: s.d.Clock.Now(),
	}
	var out content.Boost
	repo := s.d.Interactions
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		out = content.Boost{}
		if err := s.ensureLive(ctx, tx, st.ID); err != nil {
			return err
		}
		edge, err := repo.BoostEdge(ctx, tx, accountID, st.ID)
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
	if err != nil {
		return content.Boost{}, err
	}
	return out, nil
}

// Unboost retracts accountID's live boost of statusID. Without one it changes nothing.
func (s *Interactions) Unboost(ctx context.Context, accountID, statusID string) error {
	st, err := s.d.Statuses.FindStatus(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	repo := s.d.Interactions
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		edge, err := repo.BoostEdge(ctx, tx, accountID, statusID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		b, err := repo.Boost(ctx, tx, edge.BoostID)
		if err != nil {
			return err
		}
		r := content.BoostRetraction{BoostID: b.ID, Origin: content.RetractedByBooster, RetractedAt: now}
		err = repo.InsertBoostRetraction(ctx, tx, r)
		if errors.Is(err, ErrBoostRetracted) {
			// A concurrent Unboost won; a new attempt finds the edge gone.
			return uow.MarkRetryable(err)
		}
		if err != nil {
			return err
		}
		return tx.Emit(ctx, BoostRetracted{Boost: b, Retraction: r, Status: RefOf(st)})
	})
}

// Favourite makes accountID favourite statusID and returns the favourite. Favouriting a status
// that the account already favourites returns the existing favourite. The favourite takes back
// the account's reaction to the status, with which it shares the Like slot (ADR-0052 §1). An
// account whose restrictions forbid interacting fails with ErrInteractionRestricted.
func (s *Interactions) Favourite(ctx context.Context, accountID, statusID string) (content.Favourite, error) {
	if err := s.checkRestrictions(ctx, accountID, false); err != nil {
		return content.Favourite{}, err
	}
	st, err := s.visibleStatus(ctx, accountID, statusID)
	if err != nil {
		return content.Favourite{}, err
	}
	taken, err := s.slotEmojis(ctx, accountID, st)
	if err != nil {
		return content.Favourite{}, err
	}
	id := s.d.IDs.NewID()
	f := content.Favourite{ID: id, AccountID: accountID, StatusID: st.ID, URI: content.ActivityURI(s.d.Host, id), CreatedAt: s.d.Clock.Now()}
	inserted := false
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		inserted = false
		if err := s.ensureLive(ctx, tx, st.ID); err != nil {
			return err
		}
		// The favourite fills the Like slot (local_reaction.go): it takes back a reaction.
		if err := s.takeBackReactions(ctx, tx, accountID, st, "", taken, s.d.Clock.Now()); err != nil {
			return err
		}
		ok, err := s.d.Interactions.InsertFavourite(ctx, tx, f)
		if err != nil || !ok {
			return err
		}
		inserted = true
		return tx.Emit(ctx, Favourited{Favourite: f, Status: RefOf(st)})
	})
	if err != nil {
		return content.Favourite{}, err
	}
	if inserted {
		return f, nil
	}
	return s.d.Interactions.FindFavourite(ctx, accountID, st.ID)
}

// Unfavourite removes accountID's favourite of statusID. Without one it changes nothing.
func (s *Interactions) Unfavourite(ctx context.Context, accountID, statusID string) error {
	st, err := s.d.Statuses.FindStatus(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		f, deleted, err := s.d.Interactions.DeleteFavourite(ctx, tx, accountID, statusID)
		if err != nil || !deleted {
			return err
		}
		return tx.Emit(ctx, Unfavourited{Favourite: f, Status: RefOf(st), RemovedAt: s.d.Clock.Now()})
	})
}

// Bookmark makes accountID bookmark statusID (FR-113). Bookmarking a status that the account
// already bookmarks changes nothing.
func (s *Interactions) Bookmark(ctx context.Context, accountID, statusID string) error {
	st, err := s.visibleStatus(ctx, accountID, statusID)
	if err != nil {
		return err
	}
	b := content.Bookmark{ID: s.d.IDs.NewID(), AccountID: accountID, StatusID: st.ID, CreatedAt: s.d.Clock.Now()}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.ensureLive(ctx, tx, st.ID); err != nil {
			return err
		}
		_, err := s.d.Interactions.InsertBookmark(ctx, tx, b)
		return err
	})
}

// Unbookmark removes accountID's bookmark of statusID. Without one it changes nothing.
func (s *Interactions) Unbookmark(ctx context.Context, accountID, statusID string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := s.d.Interactions.DeleteBookmark(ctx, tx, accountID, statusID)
		return err
	})
}

// Pin pins accountID's own statusID to its profile. Pinning a status that is already pinned
// changes nothing. Someone else's status fails with ErrNotAuthor, a direct one with
// content.ErrNotPinnable, and a pin beyond content.MaxPins with content.ErrPinLimit.
func (s *Interactions) Pin(ctx context.Context, accountID, statusID string) error {
	st, err := s.visibleStatus(ctx, accountID, statusID)
	if err != nil {
		return err
	}
	if st.AuthorAccountID != accountID {
		return fmt.Errorf("pin %s: %w", statusID, ErrNotAuthor)
	}
	if !st.Visibility.Pinnable() {
		return fmt.Errorf("pin %s: %w", statusID, content.ErrNotPinnable)
	}
	p := content.Pin{ID: s.d.IDs.NewID(), AccountID: accountID, StatusID: st.ID, CreatedAt: s.d.Clock.Now()}
	repo := s.d.Interactions
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.ensureLive(ctx, tx, st.ID); err != nil {
			return err
		}
		// A round that inserts nothing although the status is not pinned and a slot is free lost
		// that slot to a concurrent pin, which has committed by then; each later round sees it.
		// Only concurrent unpins that keep freeing slots exhaust the rounds; then the transaction
		// runs again.
		for range content.MaxPins {
			inserted, err := repo.InsertPin(ctx, tx, p)
			if err != nil || inserted {
				return err
			}
			pinned, err := repo.HasPin(ctx, tx, accountID, st.ID)
			if err != nil || pinned {
				return err
			}
			n, err := repo.CountPins(ctx, tx, accountID)
			if err != nil {
				return err
			}
			if n >= content.MaxPins {
				return fmt.Errorf("pin %s: %w", statusID, content.ErrPinLimit)
			}
		}
		return uow.MarkRetryable(fmt.Errorf("pin %s: free slots taken concurrently", statusID))
	})
}

// Unpin unpins accountID's statusID. Without a pin it changes nothing.
func (s *Interactions) Unpin(ctx context.Context, accountID, statusID string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := s.d.Interactions.DeletePin(ctx, tx, accountID, statusID)
		return err
	})
}

// ReplaceRemotePins makes the pins of the remote account accountID the statuses statusIDs, the
// newest pin first: the mirror of the account's featured collection, which the federation
// module reads when it fetches or refreshes the account (ADR-0044). Only the first
// content.MaxPins statuses that are the account's own, pinnable, and not deleted are kept; the
// others are skipped. Pins that statusIDs leaves out are removed. A local account fails with
// ErrActorUnavailable: its pins are its own commands (Pin, Unpin). Replacing the pins with the
// ones the account has changes nothing.
func (s *Interactions) ReplaceRemotePins(ctx context.Context, accountID string, statusIDs []string) error {
	a, err := s.d.Accounts.FindByID(ctx, accountID)
	if errors.Is(err, appaccount.ErrNotFound) || (err == nil && a.Account.IsLocal()) {
		return fmt.Errorf("account %s: %w", accountID, ErrActorUnavailable)
	}
	if err != nil {
		return err
	}
	var pinned []string
	for _, id := range statusIDs {
		if len(pinned) == content.MaxPins {
			break
		}
		st, err := s.d.Statuses.FindStatus(ctx, id)
		switch {
		case errors.Is(err, ErrNotFound):
			continue
		case err != nil:
			return err
		case st.AuthorAccountID != accountID || !st.Visibility.Pinnable() || slices.Contains(pinned, st.ID):
			continue
		}
		pinned = append(pinned, st.ID)
	}
	current, err := s.d.Interactions.ListPins(ctx, accountID)
	if err != nil {
		return err
	}
	if slices.EqualFunc(current, pinned, func(p content.Pin, id string) bool { return p.StatusID == id }) {
		return nil
	}
	// IDs order the pins newest first, so the last status gets the first (smallest) ID.
	pins := make([]content.Pin, len(pinned))
	now := s.d.Clock.Now()
	for i := len(pinned) - 1; i >= 0; i-- {
		pins[i] = content.Pin{ID: s.d.IDs.NewID(), AccountID: accountID, StatusID: pinned[i], CreatedAt: now}
	}
	repo := s.d.Interactions
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := repo.DeletePinsOf(ctx, tx, accountID); err != nil {
			return err
		}
		for _, p := range pins {
			deleted, err := s.d.Statuses.IsDeleted(ctx, tx, p.StatusID)
			if err != nil {
				return err
			}
			if deleted {
				continue
			}
			inserted, err := repo.InsertPin(ctx, tx, p)
			if err != nil {
				return err
			}
			if !inserted {
				// A concurrent replacement took a slot; the transaction runs again.
				return uow.MarkRetryable(fmt.Errorf("pin %s of %s: slot taken concurrently", p.StatusID, accountID))
			}
		}
		return nil
	})
}

// visibleStatus returns statusID when accountID is a local, active account that may see it
// (content.CanView in ContextStatus). A status that is missing, deleted, or hidden from the
// account fails with ErrNotFound.
func (s *Interactions) visibleStatus(ctx context.Context, accountID, statusID string) (content.Status, error) {
	viewer, err := s.d.Accounts.FindByID(ctx, accountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return content.Status{}, fmt.Errorf("account %s: %w", accountID, ErrActorUnavailable)
	}
	if err != nil {
		return content.Status{}, err
	}
	if !viewer.Account.IsLocal() || !viewer.Lifecycle.Active() {
		return content.Status{}, fmt.Errorf("account %s: %w", accountID, ErrActorUnavailable)
	}
	st, _, err := s.audience().visibleStatus(ctx, content.Viewer{Kind: content.ViewerLocal, AccountID: accountID}, statusID)
	return st, err
}

// checkRestrictions fails with ErrInteractionRestricted when the account may not interact and,
// for a boost, which is also a post, with ErrPostingRestricted when it may not post
// (data-model §5.2).
func (s *Interactions) checkRestrictions(ctx context.Context, accountID string, boost bool) error {
	r, err := s.d.Restrictions.Restrictions(ctx, accountID)
	switch {
	case err != nil:
		return err
	case boost && !r.CanPost():
		return fmt.Errorf("account %s: %w", accountID, ErrPostingRestricted)
	case !r.CanInteract():
		return fmt.Errorf("account %s: %w", accountID, ErrInteractionRestricted)
	}
	return nil
}

func (s *Interactions) audience() audience {
	return audience{
		statuses: s.d.Statuses, interactions: s.d.Interactions, accounts: s.d.Accounts,
		relationships: s.d.Relationships, restrictions: s.d.Restrictions,
	}
}

// ensureLive fails with ErrNotFound when the status was deleted after visibleStatus read it.
func (s *Interactions) ensureLive(ctx context.Context, tx uow.Tx, statusID string) error {
	deleted, err := s.d.Statuses.IsDeleted(ctx, tx, statusID)
	if err != nil {
		return err
	}
	if deleted {
		return fmt.Errorf("status %s: %w", statusID, ErrNotFound)
	}
	return nil
}

// Counters returns the replies, boosts, and favourites counts of a status that is not deleted;
// a deleted or missing status fails with ErrNotFound.
func (s *Interactions) Counters(ctx context.Context, statusID string) (content.Counters, error) {
	return s.d.Statuses.FindCounters(ctx, statusID)
}
