// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/content"
)

// ReviewedStatus is a status as the moderators review it as the evidence of a report.
type ReviewedStatus struct {
	// StatusView is the status as it was last; its Relation is nil. A deleted status has left the
	// projections, so its Counters are zero and it has no Reactions.
	StatusView
	// DeletedAt is when the status was deleted, the zero time when it is not.
	DeletedAt time.Time
}

// Review returns a status for the moderators who handle a report that points at it, whatever
// its visibility and also when it is deleted (data-model §4.3, ADR-0038: the evidence of a
// report stays until the report's retention ends). It does not go through content.CanView: it is
// not a view of any viewer, and only the moderation module calls it, for the statuses of a
// report (report_statuses), on behalf of a holder of the manage_reports permission. A status
// that is gone fails with ErrNotFound.
func (r *Reader) Review(ctx context.Context, statusID string) (ReviewedStatus, error) {
	st, err := r.d.Statuses.FindStatus(ctx, statusID)
	if err != nil {
		return ReviewedStatus{}, err
	}
	var out ReviewedStatus
	switch d, err := r.d.Statuses.FindDeletion(ctx, statusID); {
	case err == nil:
		out.DeletedAt = d.DeletedAt
	case !errors.Is(err, ErrNotFound):
		return ReviewedStatus{}, err
	}
	author, err := r.d.Accounts.FindByID(ctx, st.AuthorAccountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return ReviewedStatus{}, errors.Join(ErrNotFound, err)
	}
	if err != nil {
		return ReviewedStatus{}, err
	}
	rev, err := r.lastRevision(ctx, st.ID)
	if err != nil {
		return ReviewedStatus{}, err
	}
	out.StatusView = StatusView{Status: st, Revision: rev, Author: author, Counters: content.Counters{StatusID: st.ID}}
	if st.InReplyToURI != "" {
		switch parent, err := r.d.Statuses.FindStatusByURI(ctx, st.InReplyToURI); {
		case errors.Is(err, ErrNotFound):
		case err != nil:
			return ReviewedStatus{}, err
		default:
			out.InReplyTo = &parent
		}
	}
	parts, err := r.Parts(ctx, rev)
	if err != nil {
		return ReviewedStatus{}, err
	}
	out.Attachments, out.Mentions, out.Tags = parts.Attachments, parts.Mentions, parts.Tags
	if len(out.Attachments) > 0 && !out.Revision.Sensitive {
		if out.Revision.Sensitive, err = r.forcedSensitive(ctx, st); err != nil {
			return ReviewedStatus{}, err
		}
	}
	switch c, err := r.d.Statuses.FindCounters(ctx, st.ID); {
	case err == nil:
		out.Counters = c
	case !errors.Is(err, ErrNotFound):
		return ReviewedStatus{}, err
	}
	// The moderator has no relation to the status, so nothing says that they reacted.
	if out.Reactions, err = r.reactions(ctx, st, ""); err != nil {
		return ReviewedStatus{}, err
	}
	return out, nil
}

// lastRevision returns the current revision of a status, or for a deleted status, which has no
// current revision, its last one.
func (r *Reader) lastRevision(ctx context.Context, statusID string) (content.Revision, error) {
	rev, err := r.d.Statuses.FindCurrentRevision(ctx, statusID)
	if !errors.Is(err, ErrNotFound) {
		return rev, err
	}
	revs, err := r.d.Statuses.ListRevisions(ctx, statusID)
	if err != nil {
		return content.Revision{}, err
	}
	if len(revs) == 0 {
		return content.Revision{}, ErrNotFound
	}
	return r.d.Statuses.FindRevision(ctx, revs[len(revs)-1].ID)
}
