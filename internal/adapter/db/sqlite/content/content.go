// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// StatusRepository implements content.Repository on SQLite.
type StatusRepository struct {
	db *moduleDB
}

var _ appcontent.Repository = (*StatusRepository)(nil)

// NewStatusRepository returns the content module's status repository on d.
func NewStatusRepository(d *sqlite.DB) *StatusRepository { return &StatusRepository{db: &moduleDB{d}} }

// InsertStatus implements content.Repository.
func (r *StatusRepository) InsertStatus(ctx context.Context, tx uow.Tx, s content.Status) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertStatus(ctx, contentgen.InsertStatusParams{
		ID:              s.ID,
		Uri:             s.URI,
		Url:             sqlite.ToNullString(s.URL),
		AuthorAccountID: s.AuthorAccountID,
		IsLocal:         sqlite.BoolToInt(s.IsLocal),
		Visibility:      string(s.Visibility),
		InReplyToUri:    sqlite.ToNullString(s.InReplyToURI),
		ConversationUri: sqlite.ToNullString(s.ConversationURI),
		PublishedAt:     sqlite.ToMillis(s.PublishedAt),
	})
	if sqlite.IsUniqueViolation(err, "statuses.uri") {
		return fmt.Errorf("insert status %s: %w", s.URI, appcontent.ErrStatusExists)
	}
	return sqlite.Wrap(err, "insert status")
}

// InsertLocalStatus implements content.Repository.
func (r *StatusRepository) InsertLocalStatus(ctx context.Context, tx uow.Tx, ls content.LocalStatus) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertLocalStatus(ctx, contentgen.InsertLocalStatusParams{
		StatusID:        ls.StatusID,
		AuthorAccountID: ls.AuthorAccountID,
		OauthClientID:   sqlite.ToNullString(ls.ClientID),
		IdempotencyKey:  sqlite.ToNullString(ls.IdempotencyKey),
	})
	if sqlite.IsUniqueViolation(err, "local_statuses.author_account_id, local_statuses.idempotency_key") {
		return fmt.Errorf("insert local status %s: %w", ls.StatusID, appcontent.ErrIdempotencyKeyUsed)
	}
	return sqlite.Wrap(err, "insert local status")
}

// InsertRevision implements content.Repository.
func (r *StatusRepository) InsertRevision(ctx context.Context, tx uow.Tx, rev content.Revision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertStatusRevision(ctx, contentgen.InsertStatusRevisionParams{
		ID:          rev.ID,
		StatusID:    rev.StatusID,
		Seq:         int64(rev.Seq),
		SpoilerText: rev.SpoilerText,
		Sensitive:   sqlite.BoolToInt(rev.Sensitive),
		Language:    rev.Language,
		RevisedAt:   sqlite.ToMillis(rev.RevisedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert status revision")
	}
	for _, b := range rev.Bodies {
		err := q.InsertStatusRevisionBody(ctx, contentgen.InsertStatusRevisionBodyParams{
			RevisionID: rev.ID, Language: b.Language, ContentHtml: b.ContentHTML,
		})
		if err != nil {
			return sqlite.Wrap(err, "insert status revision body")
		}
	}
	if rev.Source != nil {
		err := q.InsertStatusRevisionSource(ctx, contentgen.InsertStatusRevisionSourceParams{
			RevisionID: rev.ID, SourceText: rev.Source.Text, SourceFormat: string(rev.Source.Format),
		})
		if err != nil {
			return sqlite.Wrap(err, "insert status revision source")
		}
	}
	for _, a := range rev.Attachments {
		err := q.InsertStatusRevisionAttachment(ctx, contentgen.InsertStatusRevisionAttachmentParams{
			RevisionID: rev.ID, Position: int64(a.Position), MediaID: a.MediaID, Description: sqlite.ToNullString(a.Description),
		})
		if err != nil {
			return sqlite.Wrap(err, "insert status revision attachment")
		}
	}
	for _, id := range rev.MentionedAccountIDs {
		err := q.InsertStatusRevisionMention(ctx, contentgen.InsertStatusRevisionMentionParams{RevisionID: rev.ID, AccountID: id})
		if err != nil {
			return sqlite.Wrap(err, "insert status revision mention")
		}
	}
	for _, id := range rev.TagIDs {
		if err := q.InsertStatusRevisionTag(ctx, contentgen.InsertStatusRevisionTagParams{RevisionID: rev.ID, TagID: id}); err != nil {
			return sqlite.Wrap(err, "insert status revision tag")
		}
	}
	for _, id := range rev.EmojiIDs {
		if err := q.InsertStatusRevisionEmoji(ctx, contentgen.InsertStatusRevisionEmojiParams{RevisionID: rev.ID, EmojiID: id}); err != nil {
			return sqlite.Wrap(err, "insert status revision emoji")
		}
	}
	return insertRemoteRevisionParts(ctx, q, rev)
}

// InsertDeletion implements content.Repository.
func (r *StatusRepository) InsertDeletion(ctx context.Context, tx uow.Tx, d content.Deletion) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertStatusDeletion(ctx, contentgen.InsertStatusDeletionParams{
		StatusID:           d.StatusID,
		Uri:                d.URI,
		AuthorAccountID:    d.AuthorAccountID,
		Origin:             string(d.Origin),
		ModerationActionID: sqlite.ToNullString(d.ModerationActionID),
		DeletedAt:          sqlite.ToMillis(d.DeletedAt),
	})
	if sqlite.IsUniqueViolation(err, "status_deletions.status_id") || sqlite.IsUniqueViolation(err, "status_deletions.uri") {
		return fmt.Errorf("delete status %s: %w", d.StatusID, appcontent.ErrAlreadyDeleted)
	}
	return sqlite.Wrap(err, "insert status deletion")
}

// EnsureTag implements content.Repository.
func (r *StatusRepository) EnsureTag(ctx context.Context, tx uow.Tx, t content.Tag) (content.Tag, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.Tag{}, err
	}
	err = q.InsertTagIfAbsent(ctx, contentgen.InsertTagIfAbsentParams{
		ID: t.ID, Name: t.Name, NameNormalized: t.NameNormalized, CreatedAt: sqlite.ToMillis(t.CreatedAt),
	})
	if err != nil {
		return content.Tag{}, sqlite.Wrap(err, "insert tag")
	}
	row, err := q.GetTagByName(ctx, t.NameNormalized)
	if err != nil {
		return content.Tag{}, sqlite.Wrap(err, "get tag")
	}
	return tagFrom(row), nil
}

// Head implements content.Repository.
func (r *StatusRepository) Head(ctx context.Context, tx uow.Tx, statusID string) (content.Head, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.Head{}, err
	}
	row, err := q.GetStatusHead(ctx, statusID)
	if err != nil {
		return content.Head{}, sqlite.NotFound(err, appcontent.ErrNotFound, "head of "+statusID)
	}
	return headFrom(row), nil
}

// UpsertHead implements content.Repository.
func (r *StatusRepository) UpsertHead(ctx context.Context, tx uow.Tx, h content.Head) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertStatusHead(ctx, contentgen.UpsertStatusHeadParams{
		StatusID: h.StatusID, RevisionID: h.RevisionID, Seq: int64(h.Seq), LastRevisedAt: sqlite.ToMillis(h.LastRevisedAt),
	})
	return sqlite.Wrap(err, "upsert status head")
}

// ReplaceMentions implements content.Repository.
func (r *StatusRepository) ReplaceMentions(ctx context.Context, tx uow.Tx, statusID string, accountIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteStatusMentions(ctx, statusID); err != nil {
		return sqlite.Wrap(err, "delete status mentions")
	}
	for _, id := range accountIDs {
		if err := q.InsertStatusMention(ctx, contentgen.InsertStatusMentionParams{AccountID: id, StatusID: statusID}); err != nil {
			return sqlite.Wrap(err, "insert status mention")
		}
	}
	return nil
}

// LockStatusKeys implements content.Repository. It does nothing: write transactions begin with
// BEGIN IMMEDIATE and so already run one at a time.
func (r *StatusRepository) LockStatusKeys(context.Context, uow.Tx, []string) error { return nil }

// IsDeleted implements content.Repository.
func (r *StatusRepository) IsDeleted(ctx context.Context, tx uow.Tx, statusID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	deleted, err := q.StatusIsDeleted(ctx, statusID)
	return deleted, sqlite.Wrap(err, "check status deletion")
}

// FindLiveStatusByURI implements content.Repository.
func (r *StatusRepository) FindLiveStatusByURI(ctx context.Context, tx uow.Tx, uri string) (content.Status, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.Status{}, err
	}
	row, err := q.GetLiveStatusByURI(ctx, uri)
	if err != nil {
		return content.Status{}, sqlite.NotFound(err, appcontent.ErrNotFound, "live status "+uri)
	}
	return statusFrom(row), nil
}

// ListLiveRepliesTo implements content.Repository.
func (r *StatusRepository) ListLiveRepliesTo(ctx context.Context, tx uow.Tx, uri string) ([]content.Status, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLiveRepliesTo(ctx, sqlite.ToNullString(uri))
	if err != nil {
		return nil, sqlite.Wrap(err, "list replies")
	}
	out := make([]content.Status, len(rows))
	for i, row := range rows {
		out[i] = statusFrom(row)
	}
	return out, nil
}

// UpsertReplyLink implements content.Repository.
func (r *StatusRepository) UpsertReplyLink(ctx context.Context, tx uow.Tx, l content.ReplyLink) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertStatusReplyLink(ctx, contentgen.UpsertStatusReplyLinkParams(l)), "upsert reply link")
}

// ReplyLink implements content.Repository.
func (r *StatusRepository) ReplyLink(ctx context.Context, tx uow.Tx, statusID string) (content.ReplyLink, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.ReplyLink{}, err
	}
	row, err := q.GetStatusReplyLink(ctx, statusID)
	if err != nil {
		return content.ReplyLink{}, sqlite.NotFound(err, appcontent.ErrNotFound, "reply link of "+statusID)
	}
	return content.ReplyLink(row), nil
}

// InsertCounters implements content.Repository.
func (r *StatusRepository) InsertCounters(ctx context.Context, tx uow.Tx, c content.Counters) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertStatusCounters(ctx, contentgen.InsertStatusCountersParams(c)), "insert status counters")
}

// AddReplies implements content.Repository.
func (r *StatusRepository) AddReplies(ctx context.Context, tx uow.Tx, statusID string, delta int64) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.AddStatusReplies(ctx, contentgen.AddStatusRepliesParams{Delta: delta, StatusID: statusID}), "add replies")
}

// AddPost implements content.Repository.
func (r *StatusRepository) AddPost(ctx context.Context, tx uow.Tx, accountID string, publishedAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.AddAccountPost(ctx, contentgen.AddAccountPostParams{AccountID: accountID, LastStatusAt: sqlite.ToMillis(publishedAt)})
	return sqlite.Wrap(err, "add account post")
}

// RemovePost implements content.Repository.
func (r *StatusRepository) RemovePost(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	latest, err := q.LatestLivePublishedAt(ctx, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return sqlite.Wrap(q.DeleteAccountPostCounter(ctx, accountID), "delete account post counter")
	}
	if err != nil {
		return sqlite.Wrap(err, "latest status of "+accountID)
	}
	if err := q.RemoveAccountPost(ctx, contentgen.RemoveAccountPostParams{LastStatusAt: latest, AccountID: accountID}); err != nil {
		return sqlite.Wrap(err, "remove account post")
	}
	return sqlite.Wrap(q.DeleteEmptyAccountPostCounter(ctx, accountID), "delete empty account post counter")
}

// DeleteStatusProjections implements content.Repository.
func (r *StatusRepository) DeleteStatusProjections(ctx context.Context, tx uow.Tx, statusID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, step := range []struct {
		what string
		fn   func(context.Context, string) error
	}{
		{"delete status head", q.DeleteStatusHead},
		{"delete status mentions", q.DeleteStatusMentions},
		{"delete status counters", q.DeleteStatusCounters},
		{"delete reply link", q.DeleteStatusReplyLink},
		{"delete reply links to parent", q.DeleteStatusReplyLinksToParent},
	} {
		if err := step.fn(ctx, statusID); err != nil {
			return sqlite.Wrap(err, step.what)
		}
	}
	return nil
}

func (r *StatusRepository) rebuild(ctx context.Context, tx uow.Tx, what string, steps ...func(*contentgen.Queries, context.Context) error) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if err := step(q, ctx); err != nil {
			return sqlite.Wrap(err, "rebuild "+what)
		}
	}
	return nil
}

// RebuildHeads implements content.Repository.
func (r *StatusRepository) RebuildHeads(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "status heads", (*contentgen.Queries).DeleteAllStatusHeads, (*contentgen.Queries).RebuildStatusHeads)
}

// RebuildMentions implements content.Repository.
func (r *StatusRepository) RebuildMentions(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "status mentions", (*contentgen.Queries).DeleteAllStatusMentions, (*contentgen.Queries).RebuildStatusMentions)
}

// RebuildReplyLinks implements content.Repository.
func (r *StatusRepository) RebuildReplyLinks(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "reply links", (*contentgen.Queries).DeleteAllStatusReplyLinks, (*contentgen.Queries).RebuildStatusReplyLinks)
}

// RebuildCounters implements content.Repository.
func (r *StatusRepository) RebuildCounters(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "status counters", (*contentgen.Queries).DeleteAllStatusCounters, (*contentgen.Queries).RebuildStatusCounters)
}

// RebuildPostCounters implements content.Repository.
func (r *StatusRepository) RebuildPostCounters(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "account post counters", (*contentgen.Queries).DeleteAllAccountPostCounters, (*contentgen.Queries).RebuildAccountPostCounters)
}

// ListHeads implements content.Repository.
func (r *StatusRepository) ListHeads(ctx context.Context) ([]content.Head, error) {
	rows, err := r.db.reader().ListStatusHeads(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status heads")
	}
	out := make([]content.Head, len(rows))
	for i, row := range rows {
		out[i] = headFrom(row)
	}
	return out, nil
}

// ListMentions implements content.Repository.
func (r *StatusRepository) ListMentions(ctx context.Context) ([]content.Mention, error) {
	rows, err := r.db.reader().ListStatusMentions(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status mentions")
	}
	out := make([]content.Mention, len(rows))
	for i, row := range rows {
		out[i] = content.Mention(row)
	}
	return out, nil
}

// ListReplyLinks implements content.Repository.
func (r *StatusRepository) ListReplyLinks(ctx context.Context) ([]content.ReplyLink, error) {
	rows, err := r.db.reader().ListStatusReplyLinks(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list reply links")
	}
	out := make([]content.ReplyLink, len(rows))
	for i, row := range rows {
		out[i] = content.ReplyLink(row)
	}
	return out, nil
}

// ListCounters implements content.Repository.
func (r *StatusRepository) ListCounters(ctx context.Context) ([]content.Counters, error) {
	rows, err := r.db.reader().ListStatusCounters(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status counters")
	}
	out := make([]content.Counters, len(rows))
	for i, row := range rows {
		out[i] = content.Counters(row)
	}
	return out, nil
}

// ListPostCounters implements content.Repository.
func (r *StatusRepository) ListPostCounters(ctx context.Context) ([]content.PostCounter, error) {
	rows, err := r.db.reader().ListAccountPostCounters(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list account post counters")
	}
	out := make([]content.PostCounter, len(rows))
	for i, row := range rows {
		out[i] = postCounterFrom(row)
	}
	return out, nil
}

// ListSearchableStatusIDs implements content.Repository.
func (r *StatusRepository) ListSearchableStatusIDs(ctx context.Context, after string, limit int) ([]string, error) {
	out, err := r.db.reader().ListSearchableStatusIDs(ctx, contentgen.ListSearchableStatusIDsParams{After: after, PageLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list searchable statuses")
	}
	return out, nil
}

// ListAuthorSearchableStatusIDs implements content.Repository.
func (r *StatusRepository) ListAuthorSearchableStatusIDs(ctx context.Context, authorAccountID, after string, limit int) ([]string, error) {
	out, err := r.db.reader().ListAuthorSearchableStatusIDs(ctx, contentgen.ListAuthorSearchableStatusIDsParams{
		AuthorAccountID: authorAccountID, After: after, PageLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list searchable statuses of "+authorAccountID)
	}
	return out, nil
}

// FindStatus implements content.Repository.
func (r *StatusRepository) FindStatus(ctx context.Context, id string) (content.Status, error) {
	row, err := r.db.reader().GetStatus(ctx, id)
	if err != nil {
		return content.Status{}, sqlite.NotFound(err, appcontent.ErrNotFound, "status "+id)
	}
	return statusFrom(row), nil
}

// FindStatuses implements content.Repository.
func (r *StatusRepository) FindStatuses(ctx context.Context, ids []string) (map[string]content.Status, error) {
	out := make(map[string]content.Status, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListStatusesByIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list statuses")
	}
	for _, row := range rows {
		out[row.ID] = statusFrom(row)
	}
	return out, nil
}

// FindStatusByURI implements content.Repository.
func (r *StatusRepository) FindStatusByURI(ctx context.Context, uri string) (content.Status, error) {
	row, err := r.db.reader().GetStatusByURI(ctx, uri)
	if err != nil {
		return content.Status{}, sqlite.NotFound(err, appcontent.ErrNotFound, "status "+uri)
	}
	return statusFrom(row), nil
}

// FindLocalStatus implements content.Repository.
func (r *StatusRepository) FindLocalStatus(ctx context.Context, statusID string) (content.LocalStatus, error) {
	row, err := r.db.reader().GetLocalStatus(ctx, statusID)
	if err != nil {
		return content.LocalStatus{}, sqlite.NotFound(err, appcontent.ErrNotFound, "local status "+statusID)
	}
	return localStatusFrom(row), nil
}

// FindLocalStatusByIdempotencyKey implements content.Repository.
func (r *StatusRepository) FindLocalStatusByIdempotencyKey(ctx context.Context, authorAccountID, key string) (content.LocalStatus, error) {
	row, err := r.db.reader().GetLocalStatusByIdempotencyKey(ctx, contentgen.GetLocalStatusByIdempotencyKeyParams{
		AuthorAccountID: authorAccountID, IdempotencyKey: sqlite.ToNullString(key),
	})
	if err != nil {
		return content.LocalStatus{}, sqlite.NotFound(err, appcontent.ErrNotFound, "local status with idempotency key "+key)
	}
	return localStatusFrom(row), nil
}

// FindRevision implements content.Repository.
func (r *StatusRepository) FindRevision(ctx context.Context, id string) (content.Revision, error) {
	return findRevision(ctx, r.db.reader(), id)
}

// ListCurrentPosts implements content.Repository.
func (r *StatusRepository) ListCurrentPosts(ctx context.Context, tx uow.Tx) ([]appcontent.Post, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLiveStatusHeads(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list live status heads")
	}
	out := make([]appcontent.Post, len(rows))
	for i, row := range rows {
		out[i].Status = statusFrom(contentgen.Status{
			ID: row.ID, Uri: row.Uri, Url: row.Url, AuthorAccountID: row.AuthorAccountID, IsLocal: row.IsLocal,
			Visibility: row.Visibility, InReplyToUri: row.InReplyToUri, ConversationUri: row.ConversationUri,
			PublishedAt: row.PublishedAt,
		})
		if out[i].Revision, err = findRevision(ctx, q, row.RevisionID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// findRevision returns a revision with its children.
func findRevision(ctx context.Context, q *contentgen.Queries, id string) (content.Revision, error) {
	row, err := q.GetStatusRevision(ctx, id)
	if err != nil {
		return content.Revision{}, sqlite.NotFound(err, appcontent.ErrNotFound, "status revision "+id)
	}
	rr := revisionRows{row: row}
	if rr.bodies, err = q.ListStatusRevisionBodies(ctx, id); err != nil {
		return content.Revision{}, sqlite.Wrap(err, "list status revision bodies")
	}
	src, err := q.GetStatusRevisionSource(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return content.Revision{}, sqlite.Wrap(err, "get status revision source")
	default:
		rr.source = &src
	}
	if rr.attachments, err = q.ListStatusRevisionAttachments(ctx, id); err != nil {
		return content.Revision{}, sqlite.Wrap(err, "list status revision attachments")
	}
	if rr.mentions, err = q.ListStatusRevisionMentions(ctx, id); err != nil {
		return content.Revision{}, sqlite.Wrap(err, "list status revision mentions")
	}
	if rr.tags, err = q.ListStatusRevisionTags(ctx, id); err != nil {
		return content.Revision{}, sqlite.Wrap(err, "list status revision tags")
	}
	if rr.emojis, err = q.ListStatusRevisionEmojis(ctx, id); err != nil {
		return content.Revision{}, sqlite.Wrap(err, "list status revision emoji")
	}
	poll, err := q.GetPollByRevision(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return content.Revision{}, sqlite.Wrap(err, "get poll")
	default:
		rr.poll = &poll
		if rr.options, err = q.ListPollOptions(ctx, poll.ID); err != nil {
			return content.Revision{}, sqlite.Wrap(err, "list poll options")
		}
	}
	remote, err := q.GetRemoteStatusRevision(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return content.Revision{}, sqlite.Wrap(err, "get remote status revision")
	default:
		rr.remote = &remote
	}
	return rr.revision(), nil
}

// revisionRows are the rows of a revision and its parts, read for one revision (findRevision) or
// for many together (FindCurrentRevisions); revision builds the revision from them alike.
type revisionRows struct {
	row                    contentgen.StatusRevision
	bodies                 []contentgen.StatusRevisionBody
	source                 *contentgen.StatusRevisionSource
	attachments            []contentgen.StatusRevisionAttachment
	mentions, tags, emojis []string
	poll                   *contentgen.Poll
	options                []contentgen.PollOption
	remote                 *contentgen.RemoteStatusRevision
}

func (rr revisionRows) revision() content.Revision {
	rev := revisionFrom(rr.row)
	for _, b := range rr.bodies {
		rev.Bodies = append(rev.Bodies, content.Body{Language: b.Language, ContentHTML: b.ContentHtml})
	}
	if src := rr.source; src != nil {
		rev.Source = &content.Source{Text: src.SourceText, Format: content.SourceFormat(src.SourceFormat)}
	}
	for _, a := range rr.attachments {
		rev.Attachments = append(rev.Attachments, content.Attachment{
			Position: int(a.Position), MediaID: a.MediaID, Description: a.Description.String,
		})
	}
	rev.MentionedAccountIDs, _ = nonEmpty(rr.mentions, nil)
	rev.TagIDs, _ = nonEmpty(rr.tags, nil)
	rev.EmojiIDs, _ = nonEmpty(rr.emojis, nil)
	if rr.poll != nil {
		p := &content.Poll{ID: rr.poll.ID, Multiple: rr.poll.Multiple != 0, HideTotals: rr.poll.HideTotals != 0, ClosesAt: sqlite.FromNullMillis(rr.poll.ClosesAt)}
		for _, o := range rr.options {
			p.Options = append(p.Options, content.PollOption{Position: int(o.Position), Title: o.Title})
		}
		rev.Poll = p
	}
	if rr.remote != nil {
		rev.Remote = &content.RemoteRevisionSource{ActivityURI: rr.remote.ActivityUri.String, HTMLPolicyVersion: int(rr.remote.HtmlPolicyVersion),
			OmittedAttachments: int(rr.remote.OmittedAttachmentCount),
		}
	}
	return rev
}

// nonEmpty turns an empty list into nil, as the domain leaves absent children nil.
func nonEmpty(ids []string, err error) ([]string, error) {
	if len(ids) == 0 {
		return nil, err
	}
	return ids, err
}

// FindCurrentRevision implements content.Repository.
func (r *StatusRepository) FindCurrentRevision(ctx context.Context, statusID string) (content.Revision, error) {
	head, err := r.db.reader().GetStatusHead(ctx, statusID)
	if err != nil {
		return content.Revision{}, sqlite.NotFound(err, appcontent.ErrNotFound, "head of "+statusID)
	}
	return r.FindRevision(ctx, head.RevisionID)
}

// ListRevisions implements content.Repository.
func (r *StatusRepository) ListRevisions(ctx context.Context, statusID string) ([]content.Revision, error) {
	rows, err := r.db.reader().ListStatusRevisions(ctx, statusID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status revisions")
	}
	out := make([]content.Revision, len(rows))
	for i, row := range rows {
		out[i] = revisionFrom(row)
	}
	return out, nil
}

// FindDeletion implements content.Repository.
func (r *StatusRepository) FindDeletion(ctx context.Context, statusID string) (content.Deletion, error) {
	row, err := r.db.reader().GetStatusDeletion(ctx, statusID)
	if err != nil {
		return content.Deletion{}, sqlite.NotFound(err, appcontent.ErrNotFound, "deletion of "+statusID)
	}
	return deletionFrom(row), nil
}

// FindDeletionByURI implements content.Repository.
func (r *StatusRepository) FindDeletionByURI(ctx context.Context, uri string) (content.Deletion, error) {
	row, err := r.db.reader().GetStatusDeletionByURI(ctx, uri)
	if err != nil {
		return content.Deletion{}, sqlite.NotFound(err, appcontent.ErrNotFound, "deletion of "+uri)
	}
	return deletionFrom(row), nil
}

// FindTagByName implements content.Repository.
func (r *StatusRepository) FindTagByName(ctx context.Context, nameNormalized string) (content.Tag, error) {
	row, err := r.db.reader().GetTagByName(ctx, nameNormalized)
	if err != nil {
		return content.Tag{}, sqlite.NotFound(err, appcontent.ErrNotFound, "tag "+nameNormalized)
	}
	return tagFrom(row), nil
}

// ListTagsByPrefix implements content.Repository.
func (r *StatusRepository) ListTagsByPrefix(ctx context.Context, low, high string, limit int) ([]content.Tag, error) {
	rows, err := r.db.reader().ListTagsByPrefix(ctx, contentgen.ListTagsByPrefixParams{Low: low, High: high, Lim: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list tags by prefix")
	}
	out := make([]content.Tag, 0, len(rows))
	for _, row := range rows {
		out = append(out, tagFrom(row))
	}
	return out, nil
}

// FindTag implements content.Repository.
func (r *StatusRepository) FindTag(ctx context.Context, id string) (content.Tag, error) {
	row, err := r.db.reader().GetTag(ctx, id)
	if err != nil {
		return content.Tag{}, sqlite.NotFound(err, appcontent.ErrNotFound, "tag "+id)
	}
	return tagFrom(row), nil
}

// ListReplies implements content.Repository.
func (r *StatusRepository) ListReplies(ctx context.Context, uri string) ([]content.Status, error) {
	rows, err := r.db.reader().ListLiveRepliesTo(ctx, sqlite.ToNullString(uri))
	if err != nil {
		return nil, sqlite.Wrap(err, "list replies")
	}
	out := make([]content.Status, len(rows))
	for i, row := range rows {
		out[i] = statusFrom(row)
	}
	return out, nil
}

// ListLiveRemoteStatusIDs implements content.Repository.
func (r *StatusRepository) ListLiveRemoteStatusIDs(ctx context.Context, authorAccountID, before string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListLiveRemoteStatusIDsOfAuthor(ctx, contentgen.ListLiveRemoteStatusIDsOfAuthorParams{
		AuthorAccountID: authorAccountID, Before: before, Lim: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list remote statuses of "+authorAccountID)
	}
	return ids, nil
}

// FindCounters implements content.Repository.
func (r *StatusRepository) FindCounters(ctx context.Context, statusID string) (content.Counters, error) {
	row, err := r.db.reader().GetStatusCounters(ctx, statusID)
	if err != nil {
		return content.Counters{}, sqlite.NotFound(err, appcontent.ErrNotFound, "counters of "+statusID)
	}
	return content.Counters(row), nil
}

// FindPostCounter implements content.Repository.
func (r *StatusRepository) FindPostCounter(ctx context.Context, accountID string) (content.PostCounter, error) {
	row, err := r.db.reader().GetAccountPostCounter(ctx, accountID)
	if err != nil {
		return content.PostCounter{}, sqlite.NotFound(err, appcontent.ErrNotFound, "post counter of "+accountID)
	}
	return postCounterFrom(row), nil
}

func statusFrom(row contentgen.Status) content.Status {
	return content.Status{
		ID:              row.ID,
		URI:             row.Uri,
		URL:             row.Url.String,
		AuthorAccountID: row.AuthorAccountID,
		IsLocal:         row.IsLocal != 0,
		Visibility:      content.Visibility(row.Visibility),
		InReplyToURI:    row.InReplyToUri.String,
		ConversationURI: row.ConversationUri.String,
		PublishedAt:     sqlite.FromMillis(row.PublishedAt),
	}
}

func localStatusFrom(row contentgen.LocalStatus) content.LocalStatus {
	return content.LocalStatus{
		StatusID:        row.StatusID,
		AuthorAccountID: row.AuthorAccountID,
		ClientID:        row.OauthClientID.String,
		IdempotencyKey:  row.IdempotencyKey.String,
	}
}

func revisionFrom(row contentgen.StatusRevision) content.Revision {
	return content.Revision{
		ID:          row.ID,
		StatusID:    row.StatusID,
		Seq:         int(row.Seq),
		SpoilerText: row.SpoilerText,
		Sensitive:   row.Sensitive != 0,
		Language:    row.Language,
		RevisedAt:   sqlite.FromMillis(row.RevisedAt),
	}
}

func deletionFrom(row contentgen.StatusDeletion) content.Deletion {
	return content.Deletion{
		StatusID:           row.StatusID,
		URI:                row.Uri,
		AuthorAccountID:    row.AuthorAccountID,
		Origin:             content.DeletionOrigin(row.Origin),
		ModerationActionID: row.ModerationActionID.String,
		DeletedAt:          sqlite.FromMillis(row.DeletedAt),
	}
}

func tagFrom(row contentgen.Tag) content.Tag {
	return content.Tag{ID: row.ID, Name: row.Name, NameNormalized: row.NameNormalized, CreatedAt: sqlite.FromMillis(row.CreatedAt)}
}

func headFrom(row contentgen.StatusHead) content.Head {
	return content.Head{
		StatusID: row.StatusID, RevisionID: row.RevisionID, Seq: int(row.Seq), LastRevisedAt: sqlite.FromMillis(row.LastRevisedAt),
	}
}

func postCounterFrom(row contentgen.AccountPostCounter) content.PostCounter {
	return content.PostCounter{
		AccountID: row.AccountID, StatusesCount: row.StatusesCount, LastStatusAt: sqlite.FromMillis(row.LastStatusAt),
	}
}
