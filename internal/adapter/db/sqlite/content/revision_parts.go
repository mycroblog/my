// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/domain/content"
)

// FindCurrentRevisions implements content.Repository: the rows of every revision and its parts
// with one query per part, built as findRevision builds one (revisionRows).
func (r *StatusRepository) FindCurrentRevisions(ctx context.Context, statusIDs []string) (map[string]content.Revision, error) {
	out := make(map[string]content.Revision, len(statusIDs))
	if len(statusIDs) == 0 {
		return out, nil
	}
	q := r.db.reader()
	ids, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCurrentStatusRevisions(ctx, ids)
	if err != nil {
		return nil, sqlite.Wrap(err, "list current status revisions")
	}
	if len(rows) == 0 {
		return out, nil
	}
	byRevision := make(map[string]*revisionRows, len(rows))
	revIDs := make([]string, len(rows))
	for i, row := range rows {
		byRevision[row.ID] = &revisionRows{row: row}
		revIDs[i] = row.ID
	}
	revs, err := sqlite.JSONList(revIDs)
	if err != nil {
		return nil, err
	}
	bodies, err := q.ListStatusRevisionBodiesOf(ctx, revs)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status revision bodies")
	}
	for _, b := range bodies {
		byRevision[b.RevisionID].bodies = append(byRevision[b.RevisionID].bodies, b)
	}
	sources, err := q.ListStatusRevisionSourcesOf(ctx, revs)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status revision sources")
	}
	for _, src := range sources {
		byRevision[src.RevisionID].source = &src
	}
	attachments, err := q.ListStatusRevisionAttachmentsOf(ctx, revs)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status revision attachments")
	}
	for _, a := range attachments {
		byRevision[a.RevisionID].attachments = append(byRevision[a.RevisionID].attachments, a)
	}
	mentions, err := q.ListStatusRevisionMentionsOf(ctx, revs)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status revision mentions")
	}
	for _, m := range mentions {
		byRevision[m.RevisionID].mentions = append(byRevision[m.RevisionID].mentions, m.AccountID)
	}
	tags, err := q.ListStatusRevisionTagsOf(ctx, revs)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status revision tags")
	}
	for _, t := range tags {
		byRevision[t.RevisionID].tags = append(byRevision[t.RevisionID].tags, t.TagID)
	}
	emojis, err := q.ListStatusRevisionEmojisOf(ctx, revs)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status revision emoji")
	}
	for _, e := range emojis {
		byRevision[e.RevisionID].emojis = append(byRevision[e.RevisionID].emojis, e.EmojiID)
	}
	polls, err := q.ListPollsOfRevisions(ctx, revs)
	if err != nil {
		return nil, sqlite.Wrap(err, "list polls")
	}
	if len(polls) > 0 {
		pollOf := make(map[string]*revisionRows, len(polls))
		pollIDs := make([]string, len(polls))
		for i, p := range polls {
			rr := byRevision[p.RevisionID]
			rr.poll = &p
			pollOf[p.ID] = rr
			pollIDs[i] = p.ID
		}
		pids, err := sqlite.JSONList(pollIDs)
		if err != nil {
			return nil, err
		}
		options, err := q.ListPollOptionsOf(ctx, pids)
		if err != nil {
			return nil, sqlite.Wrap(err, "list poll options")
		}
		for _, o := range options {
			pollOf[o.PollID].options = append(pollOf[o.PollID].options, o)
		}
	}
	remotes, err := q.ListRemoteStatusRevisionsOf(ctx, revs)
	if err != nil {
		return nil, sqlite.Wrap(err, "list remote status revisions")
	}
	for _, rm := range remotes {
		byRevision[rm.RevisionID].remote = &rm
	}
	for _, row := range rows {
		out[row.StatusID] = byRevision[row.ID].revision()
	}
	return out, nil
}
