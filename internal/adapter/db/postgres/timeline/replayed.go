// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/timelinegen"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

// InsertReplayed implements timeline.Repository, one statement for each kind of row.
func (r *Repository) InsertReplayed(ctx context.Context, tx uow.Tx, entries []apptimeline.Entry, recipients []apptimeline.DirectRecipient,
	tags []apptimeline.TagEntry, usage map[string]int64,
) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		var p timelinegen.InsertManyFeedEntriesParams
		for _, e := range entries {
			seq, err := postgres.ToInt32(e.RevisionSeq, "revision seq")
			if err != nil {
				return err
			}
			p.Ids, p.Kinds, p.StatusIds = append(p.Ids, e.ID), append(p.Kinds, string(e.Kind)), append(p.StatusIds, e.StatusID)
			p.AuthorAccountIds, p.AuthorDomains = append(p.AuthorAccountIds, e.AuthorAccountID), append(p.AuthorDomains, e.AuthorDomain)
			p.OriginalAuthorAccountIds = append(p.OriginalAuthorAccountIds, e.OriginalAuthorAccountID)
			p.OriginalAuthorDomains = append(p.OriginalAuthorDomains, e.OriginalAuthorDomain)
			p.Visibilities, p.IsLocals, p.Languages = append(p.Visibilities, string(e.Visibility)), append(p.IsLocals, e.IsLocal), append(p.Languages, e.Language)
			p.RevisionSeqs, p.IsReplies = append(p.RevisionSeqs, seq), append(p.IsReplies, e.IsReply)
		}
		if err := q.InsertManyFeedEntries(ctx, p); err != nil {
			return postgres.Wrap(err, "insert feed entries")
		}
	}
	if len(recipients) > 0 {
		var p timelinegen.InsertManyFeedDirectRecipientsParams
		for _, d := range recipients {
			p.EntryIds, p.RecipientAccountIds = append(p.EntryIds, d.EntryID), append(p.RecipientAccountIds, d.RecipientAccountID)
		}
		if err := q.InsertManyFeedDirectRecipients(ctx, p); err != nil {
			return postgres.Wrap(err, "insert feed direct recipients")
		}
	}
	if len(tags) > 0 {
		var p timelinegen.InsertManyFeedTagEntriesParams
		for _, t := range tags {
			p.TagIds, p.EntryIds, p.AuthorAccountIds = append(p.TagIds, t.TagID), append(p.EntryIds, t.EntryID), append(p.AuthorAccountIds, t.AuthorAccountID)
			p.Visibilities, p.IsLocals, p.RevisedAts = append(p.Visibilities, string(t.Visibility)), append(p.IsLocals, t.IsLocal), append(p.RevisedAts, postgres.UTC(t.RevisedAt))
		}
		if err := q.InsertManyFeedTagEntries(ctx, p); err != nil {
			return postgres.Wrap(err, "insert feed tag entries")
		}
	}
	return applyTagUsage(ctx, q, usage)
}
