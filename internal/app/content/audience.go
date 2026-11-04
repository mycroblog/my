// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/content"
)

// audience decides who may see one status, through content.CanView in ContextStatus, for the
// use cases that act on or present a single status (docs/architecture/visibility.md).
type audience struct {
	statuses      Repository
	interactions  InteractionRepository
	accounts      Accounts
	relationships Relationships
	restrictions  RestrictionPolicy
}

// live returns a status unless it is missing or deleted; both fail with ErrNotFound.
func (a audience) live(ctx context.Context, statusID string) (content.Status, error) {
	st, err := a.statuses.FindStatus(ctx, statusID)
	if err != nil {
		return content.Status{}, err
	}
	switch _, err := a.statuses.FindDeletion(ctx, statusID); {
	case err == nil:
		return content.Status{}, fmt.Errorf("status %s: %w", statusID, ErrNotFound)
	case !errors.Is(err, ErrNotFound):
		return content.Status{}, err
	}
	return st, nil
}

// visible reports whether viewer may see st and returns its author, the zero Actor when the
// author is unknown. The viewer is anonymous or a local account.
func (a audience) visible(ctx context.Context, viewer content.Viewer, st content.Status) (bool, appaccount.Actor, error) {
	r, err := a.restrictions.Restrictions(ctx, st.AuthorAccountID)
	if err != nil {
		return false, appaccount.Actor{}, err
	}
	restrictions := r.Author
	author, err := a.accounts.FindByID(ctx, st.AuthorAccountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		restrictions.ProfileView = true
	case err != nil:
		return false, appaccount.Actor{}, err
	default:
		// A suspended or deleted author is shown to no one else (data-model §5.1).
		restrictions.ProfileView = restrictions.ProfileView || !author.Lifecycle.Active()
	}

	var rel content.Relation
	if viewer.Kind == content.ViewerLocal && viewer.AccountID != "" && st.AuthorAccountID != viewer.AccountID {
		r, err := a.relationships.Relationship(ctx, viewer.AccountID, st.AuthorAccountID)
		if err != nil {
			return false, appaccount.Actor{}, err
		}
		rel.Blocking, rel.BlockedBy = r.Blocking, r.BlockedBy
		switch {
		case r.ActivelyFollowing():
			rel.Follow = content.FollowActive
		case r.RequestedFollow():
			rel.Follow = content.FollowPending
		}
		if rel.Mentioned, err = a.interactions.Mentioned(ctx, st.ID, viewer.AccountID); err != nil {
			return false, appaccount.Actor{}, err
		}
	}
	facts := content.StatusFacts{AuthorAccountID: st.AuthorAccountID, AuthorIsLocal: st.IsLocal, Visibility: st.Visibility}
	return content.CanView(content.ContextStatus, viewer, facts, restrictions, rel), author, nil
}

// reviewable reports whether the moderators, who review a report, may read st, which the report
// does not point at, and returns its author. They read it when reviewer may under the visibility st
// was posted with: no relation of a moderator counts, for or against, and the restrictions on its
// author, which moderators handle, do not hide it. An unknown author leaves nothing to present.
func (a audience) reviewable(ctx context.Context, st content.Status) (bool, appaccount.Actor, error) {
	author, err := a.accounts.FindByID(ctx, st.AuthorAccountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return false, appaccount.Actor{}, nil
	}
	if err != nil {
		return false, appaccount.Actor{}, err
	}
	facts := content.StatusFacts{AuthorAccountID: st.AuthorAccountID, AuthorIsLocal: st.IsLocal, Visibility: st.Visibility}
	return content.CanView(content.ContextStatus, reviewer, facts, content.AuthorRestrictions{}, content.Relation{}), author, nil
}

// visibleStatus returns statusID with its author when viewer may see it. A status that is
// missing, deleted, or hidden from the viewer fails with ErrNotFound alike, so that its
// existence does not leak.
func (a audience) visibleStatus(ctx context.Context, viewer content.Viewer, statusID string) (content.Status, appaccount.Actor, error) {
	st, err := a.live(ctx, statusID)
	if err != nil {
		return content.Status{}, appaccount.Actor{}, err
	}
	ok, author, err := a.visible(ctx, viewer, st)
	if err != nil {
		return content.Status{}, appaccount.Actor{}, err
	}
	if !ok {
		return content.Status{}, appaccount.Actor{}, fmt.Errorf("status %s: %w", statusID, ErrNotFound)
	}
	return st, author, nil
}
