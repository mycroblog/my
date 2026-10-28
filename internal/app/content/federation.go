// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/content"
)

// The part of this module's public API that the federation module reads to deliver local
// statuses and boosts (docs/architecture/federation.md §7): the statuses, revisions, and boosts
// that outbound activities refer to, and what the visibility policy needs about their authors.
type (
	// Status is content.Status.
	Status = content.Status
	// Revision is content.Revision.
	Revision = content.Revision
	// Boost is content.Boost.
	Boost = content.Boost
	// Favourite is content.Favourite.
	Favourite = content.Favourite
	// Tag is content.Tag, which the timeline module's prefix search of hashtags reads
	// (ADR-0092 §1).
	Tag = content.Tag
)

// ErrNotBoostable is content.ErrNotBoostable: a status the account may not boost.
var ErrNotBoostable = content.ErrNotBoostable

// The values of the visibility policy's types that delivery and ActivityPub fetches use.
const (
	ViewerRemote       = content.ViewerRemote
	ContextDelivery    = content.ContextDelivery
	ContextObjectFetch = content.ContextObjectFetch
	FollowPending      = content.FollowPending
)

// StatusURI returns the ActivityPub id of a local status on host (ADR-0018).
func StatusURI(host, statusID string) string { return content.StatusURI(host, statusID) }

// StatusPageURL returns the HTML page of st by author (ADR-0053): of a local status, the page
// under the author's current username, derived on read; of a remote status, the url it arrived
// with. It is "" when there is none.
func StatusPageURL(host string, st Status, author appaccount.Actor) string {
	if st.IsLocal && author.Local == nil {
		return ""
	}
	username := ""
	if author.Local != nil {
		username = author.Local.Username
	}
	return st.PageURL(host, username)
}

// ActivityURI returns the IRI of an activity of this instance other than Create (ADR-0018).
func ActivityURI(host, id string) string { return content.ActivityURI(host, id) }

// FindStatus returns a status, deleted or not. It fails with ErrNotFound when there is none.
func (s *Service) FindStatus(ctx context.Context, id string) (Status, error) {
	return s.d.Repository.FindStatus(ctx, id)
}

// LiveRevision returns a status that is not deleted with one of its revisions, the revision an
// outbound Create or Update presents. A missing or deleted status, and a revision of another
// status, fail with ErrNotFound.
// An empty revisionID reads the current revision: the Update of a status whose quote or quote
// policy changed without a new revision (ADR-0069 §3).
func (s *Service) LiveRevision(ctx context.Context, statusID, revisionID string) (Status, Revision, error) {
	st, err := s.d.Repository.FindStatus(ctx, statusID)
	if err != nil {
		return Status{}, Revision{}, err
	}
	switch _, err := s.d.Repository.FindDeletion(ctx, statusID); {
	case err == nil:
		return Status{}, Revision{}, fmt.Errorf("status %s: %w", statusID, ErrNotFound)
	case !errors.Is(err, ErrNotFound):
		return Status{}, Revision{}, err
	}
	var rev Revision
	if revisionID == "" {
		rev, err = s.d.Repository.FindCurrentRevision(ctx, statusID)
	} else {
		rev, err = s.d.Repository.FindRevision(ctx, revisionID)
	}
	if err != nil {
		return Status{}, Revision{}, err
	}
	if rev.StatusID != statusID {
		return Status{}, Revision{}, fmt.Errorf("revision %s of status %s: %w", revisionID, statusID, ErrNotFound)
	}
	return st, rev, nil
}

// AuthorRestrictions returns the restrictions of an author that bear on who sees its statuses,
// as CanView takes them: the moderation restrictions in effect, and profile_view for an author
// that is missing, suspended, or deleted (data-model §5.1).
func (s *Service) AuthorRestrictions(ctx context.Context, accountID string) (AuthorRestrictions, error) {
	r, err := s.d.Restrictions.Restrictions(ctx, accountID)
	if err != nil {
		return AuthorRestrictions{}, err
	}
	out := r.Author
	author, err := s.d.Accounts.FindByID(ctx, accountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		out.ProfileView = true
	case err != nil:
		return AuthorRestrictions{}, err
	default:
		out.ProfileView = out.ProfileView || !author.Lifecycle.Active()
	}
	return out, nil
}

// FindBoost returns a boost, retracted or not. It fails with ErrNotFound when there is none.
func (i *Interactions) FindBoost(ctx context.Context, id string) (Boost, error) {
	return i.d.Interactions.FindBoost(ctx, id)
}

// EffectiveVisibility is content.EffectiveVisibility: the visibility a status is judged and
// addressed by, min(original, the author's ceiling) (data-model §5.4).
func EffectiveVisibility(original Visibility, author AuthorRestrictions) Visibility {
	return content.EffectiveVisibility(original, author)
}

// ObjectStatus returns a status with the revision its ActivityPub object presents, and whether
// it is deleted (federation.md §10). The revision of a live status is its current one; that of
// a deleted status is its last one, which still tells whom the status mentioned, and is the zero
// Revision once the purge has removed it. It fails with ErrNotFound when there is no status.
func (s *Service) ObjectStatus(ctx context.Context, id string) (Status, Revision, bool, error) {
	st, err := s.d.Repository.FindStatus(ctx, id)
	if err != nil {
		return Status{}, Revision{}, false, err
	}
	switch _, err := s.d.Repository.FindDeletion(ctx, id); {
	case errors.Is(err, ErrNotFound):
		rev, err := s.d.Repository.FindCurrentRevision(ctx, id)
		if err != nil {
			return Status{}, Revision{}, false, err
		}
		return st, rev, false, nil
	case err != nil:
		return Status{}, Revision{}, false, err
	}
	revs, err := s.d.Repository.ListRevisions(ctx, id)
	if err != nil || len(revs) == 0 {
		return st, Revision{}, true, err
	}
	rev, err := s.d.Repository.FindRevision(ctx, revs[len(revs)-1].ID)
	if errors.Is(err, ErrNotFound) {
		return st, Revision{}, true, nil
	}
	return st, rev, true, err
}

// PostCount returns the number of statuses of an account that are not deleted, as its outbox
// reports it.
func (s *Service) PostCount(ctx context.Context, accountID string) (int64, error) {
	c, err := s.d.Repository.FindPostCounter(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	return c.StatusesCount, err
}

// LocalPostCount returns the posts of the local accounts as NodeInfo's usage.localPosts reports
// them, counted as Mastodon counts its local posts: the statuses that are not deleted and not
// direct, and the boosts that are not retracted, of statuses that are not deleted.
func (s *Service) LocalPostCount(ctx context.Context) (int64, error) {
	return s.d.Repository.LocalPostsTotal(ctx)
}

// DailyLocalPublicPosts counts the public posts of local accounts that are not deleted in each
// of the days periods of 24 hours before to, oldest first, for the counts of the landing page
// (GET /instance/overview). Only public posts count: the other visibilities are not for every
// visitor (ADR-0017), and boosts are not posts of their own.
func (s *Service) DailyLocalPublicPosts(ctx context.Context, to time.Time, days int) ([]int, error) {
	return s.d.Repository.DailyLocalPublicPosts(ctx, to, days)
}

// ListRecentLocalPosters returns up to limit local accounts with a public or local status that
// is not deleted, published at since or later, the one that posted last first: the recently
// active accounts of the suggestions (FR-308).
func (s *Service) ListRecentLocalPosters(ctx context.Context, since time.Time, limit int) ([]string, error) {
	return s.d.Repository.ListRecentLocalPosters(ctx, since, limit)
}
