// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
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

// visibleToLocal reports, by account, whether each of the local accounts of viewerIDs may see
// statusID, as visibleStatus decides for one viewer, reading the status, its author, and the
// author's restrictions once and the viewers' relations with the author together when
// Relationships can. A missing or deleted status is visible to none.
func (a audience) visibleToLocal(ctx context.Context, statusID string, viewerIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(viewerIDs))
	st, err := a.live(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	r, err := a.restrictions.Restrictions(ctx, st.AuthorAccountID)
	if err != nil {
		return nil, err
	}
	restrictions := r.Author
	author, err := a.accounts.FindByID(ctx, st.AuthorAccountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		restrictions.ProfileView = true
	case err != nil:
		return nil, err
	default:
		restrictions.ProfileView = restrictions.ProfileView || !author.Lifecycle.Active()
	}
	var others []string
	for _, id := range viewerIDs {
		if id != "" && id != st.AuthorAccountID && !slices.Contains(others, id) {
			others = append(others, id)
		}
	}
	rels := map[string]content.Relation{}
	if len(others) > 0 {
		mentioned, err := a.interactions.MentionedAmong(ctx, st.ID, others)
		if err != nil {
			return nil, err
		}
		if b, ok := a.relationships.(relationshipBatch); ok {
			// The author's relationships with the viewers, seen from the viewers' side.
			rs, err := b.Relationships(ctx, st.AuthorAccountID, others)
			if err != nil {
				return nil, err
			}
			for _, id := range others {
				m := rs[id]
				rels[id] = relationFrom(appgraph.Relationship{Following: m.FollowedBy, Blocking: m.BlockedBy, BlockedBy: m.Blocking}, mentioned[id])
			}
		} else {
			for _, id := range others {
				rel, err := a.relationships.Relationship(ctx, id, st.AuthorAccountID)
				if err != nil {
					return nil, err
				}
				rels[id] = relationFrom(rel, mentioned[id])
			}
		}
	}
	facts := content.StatusFacts{AuthorAccountID: st.AuthorAccountID, AuthorIsLocal: st.IsLocal, Visibility: st.Visibility}
	for _, id := range viewerIDs {
		viewer := content.Viewer{Kind: content.ViewerLocal, AccountID: id}
		out[id] = content.CanView(content.ContextStatus, viewer, facts, restrictions, rels[id])
	}
	return out, nil
}

// relationFrom is the relation of a viewer with an author, from the viewer's relationship with the
// author and whether the status mentions the viewer.
func relationFrom(r appgraph.Relationship, mentioned bool) content.Relation {
	rel := content.Relation{Blocking: r.Blocking, BlockedBy: r.BlockedBy, Mentioned: mentioned}
	switch {
	case r.ActivelyFollowing():
		rel.Follow = content.FollowActive
	case r.RequestedFollow():
		rel.Follow = content.FollowPending
	}
	return rel
}
