// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// StatusRepository implements content.Repository on SQLite.
type StatusRepository struct {
	db *DB
}

var _ appcontent.Repository = (*StatusRepository)(nil)

// StatusRepository returns the content module's status repository on d.
func (d *DB) StatusRepository() *StatusRepository { return &StatusRepository{db: d} }

// InsertStatus implements content.Repository.
func (r *StatusRepository) InsertStatus(ctx context.Context, tx uow.Tx, s content.Status) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertStatus(ctx, sqlitegen.InsertStatusParams{
		ID:              s.ID,
		Uri:             s.URI,
		Url:             toNullString(s.URL),
		AuthorAccountID: s.AuthorAccountID,
		IsLocal:         boolToInt(s.IsLocal),
		Visibility:      string(s.Visibility),
		InReplyToUri:    toNullString(s.InReplyToURI),
		ConversationUri: toNullString(s.ConversationURI),
		PublishedAt:     toMillis(s.PublishedAt),
	})
	if isUniqueViolation(err, "statuses.uri") {
		return fmt.Errorf("insert status %s: %w", s.URI, appcontent.ErrStatusExists)
	}
	return wrap(err, "insert status")
}

// InsertLocalStatus implements content.Repository.
func (r *StatusRepository) InsertLocalStatus(ctx context.Context, tx uow.Tx, ls content.LocalStatus) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertLocalStatus(ctx, sqlitegen.InsertLocalStatusParams{
		StatusID:        ls.StatusID,
		AuthorAccountID: ls.AuthorAccountID,
		ClientID:        toNullString(ls.ClientID),
		IdempotencyKey:  toNullString(ls.IdempotencyKey),
	})
	if isUniqueViolation(err, "local_statuses.author_account_id, local_statuses.idempotency_key") {
		return fmt.Errorf("insert local status %s: %w", ls.StatusID, appcontent.ErrIdempotencyKeyUsed)
	}
	return wrap(err, "insert local status")
}

// InsertRevision implements content.Repository.
func (r *StatusRepository) InsertRevision(ctx context.Context, tx uow.Tx, rev content.Revision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertStatusRevision(ctx, sqlitegen.InsertStatusRevisionParams{
		ID:          rev.ID,
		StatusID:    rev.StatusID,
		Seq:         int64(rev.Seq),
		SpoilerText: rev.SpoilerText,
		Sensitive:   boolToInt(rev.Sensitive),
		Language:    rev.Language,
		RevisedAt:   toMillis(rev.RevisedAt),
	})
	if err != nil {
		return wrap(err, "insert status revision")
	}
	for _, b := range rev.Bodies {
		err := q.InsertStatusRevisionBody(ctx, sqlitegen.InsertStatusRevisionBodyParams{
			RevisionID: rev.ID, Language: b.Language, ContentHtml: b.ContentHTML,
		})
		if err != nil {
			return wrap(err, "insert status revision body")
		}
	}
	if rev.Source != nil {
		err := q.InsertStatusRevisionSource(ctx, sqlitegen.InsertStatusRevisionSourceParams{
			RevisionID: rev.ID, SourceText: rev.Source.Text, SourceFormat: string(rev.Source.Format),
		})
		if err != nil {
			return wrap(err, "insert status revision source")
		}
	}
	for _, a := range rev.Attachments {
		err := q.InsertStatusRevisionAttachment(ctx, sqlitegen.InsertStatusRevisionAttachmentParams{
			RevisionID: rev.ID, Position: int64(a.Position), MediaID: a.MediaID, Description: toNullString(a.Description),
		})
		if err != nil {
			return wrap(err, "insert status revision attachment")
		}
	}
	for _, id := range rev.MentionedAccountIDs {
		err := q.InsertStatusRevisionMention(ctx, sqlitegen.InsertStatusRevisionMentionParams{RevisionID: rev.ID, AccountID: id})
		if err != nil {
			return wrap(err, "insert status revision mention")
		}
	}
	for _, id := range rev.TagIDs {
		if err := q.InsertStatusRevisionTag(ctx, sqlitegen.InsertStatusRevisionTagParams{RevisionID: rev.ID, TagID: id}); err != nil {
			return wrap(err, "insert status revision tag")
		}
	}
	for _, id := range rev.EmojiIDs {
		if err := q.InsertStatusRevisionEmoji(ctx, sqlitegen.InsertStatusRevisionEmojiParams{RevisionID: rev.ID, EmojiID: id}); err != nil {
			return wrap(err, "insert status revision emoji")
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
	err = q.InsertStatusDeletion(ctx, sqlitegen.InsertStatusDeletionParams{
		StatusID:           d.StatusID,
		Uri:                d.URI,
		AuthorAccountID:    d.AuthorAccountID,
		Origin:             string(d.Origin),
		ModerationActionID: toNullString(d.ModerationActionID),
		DeletedAt:          toMillis(d.DeletedAt),
	})
	if isUniqueViolation(err, "status_deletions.status_id") || isUniqueViolation(err, "status_deletions.uri") {
		return fmt.Errorf("delete status %s: %w", d.StatusID, appcontent.ErrAlreadyDeleted)
	}
	return wrap(err, "insert status deletion")
}

// EnsureTag implements content.Repository.
func (r *StatusRepository) EnsureTag(ctx context.Context, tx uow.Tx, t content.Tag) (content.Tag, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.Tag{}, err
	}
	err = q.InsertTagIfAbsent(ctx, sqlitegen.InsertTagIfAbsentParams{
		ID: t.ID, Name: t.Name, NameNormalized: t.NameNormalized, CreatedAt: toMillis(t.CreatedAt),
	})
	if err != nil {
		return content.Tag{}, wrap(err, "insert tag")
	}
	row, err := q.GetTagByName(ctx, t.NameNormalized)
	if err != nil {
		return content.Tag{}, wrap(err, "get tag")
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
		return content.Head{}, notFound(err, appcontent.ErrNotFound, "head of "+statusID)
	}
	return headFrom(row), nil
}

// UpsertHead implements content.Repository.
func (r *StatusRepository) UpsertHead(ctx context.Context, tx uow.Tx, h content.Head) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertStatusHead(ctx, sqlitegen.UpsertStatusHeadParams{
		StatusID: h.StatusID, RevisionID: h.RevisionID, Seq: int64(h.Seq), LastRevisedAt: toMillis(h.LastRevisedAt),
	})
	return wrap(err, "upsert status head")
}

// ReplaceMentions implements content.Repository.
func (r *StatusRepository) ReplaceMentions(ctx context.Context, tx uow.Tx, statusID string, accountIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteStatusMentions(ctx, statusID); err != nil {
		return wrap(err, "delete status mentions")
	}
	for _, id := range accountIDs {
		if err := q.InsertStatusMention(ctx, sqlitegen.InsertStatusMentionParams{AccountID: id, StatusID: statusID}); err != nil {
			return wrap(err, "insert status mention")
		}
	}
	return nil
}

// IsDeleted implements content.Repository.
func (r *StatusRepository) IsDeleted(ctx context.Context, tx uow.Tx, statusID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	deleted, err := q.StatusIsDeleted(ctx, statusID)
	return deleted, wrap(err, "check status deletion")
}

// FindLiveStatusByURI implements content.Repository.
func (r *StatusRepository) FindLiveStatusByURI(ctx context.Context, tx uow.Tx, uri string) (content.Status, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.Status{}, err
	}
	row, err := q.GetLiveStatusByURI(ctx, uri)
	if err != nil {
		return content.Status{}, notFound(err, appcontent.ErrNotFound, "live status "+uri)
	}
	return statusFrom(row), nil
}

// ListLiveRepliesTo implements content.Repository.
func (r *StatusRepository) ListLiveRepliesTo(ctx context.Context, tx uow.Tx, uri string) ([]content.Status, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLiveRepliesTo(ctx, toNullString(uri))
	if err != nil {
		return nil, wrap(err, "list replies")
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
	return wrap(q.UpsertStatusReplyLink(ctx, sqlitegen.UpsertStatusReplyLinkParams(l)), "upsert reply link")
}

// ReplyLink implements content.Repository.
func (r *StatusRepository) ReplyLink(ctx context.Context, tx uow.Tx, statusID string) (content.ReplyLink, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.ReplyLink{}, err
	}
	row, err := q.GetStatusReplyLink(ctx, statusID)
	if err != nil {
		return content.ReplyLink{}, notFound(err, appcontent.ErrNotFound, "reply link of "+statusID)
	}
	return content.ReplyLink(row), nil
}

// InsertCounters implements content.Repository.
func (r *StatusRepository) InsertCounters(ctx context.Context, tx uow.Tx, c content.Counters) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertStatusCounters(ctx, sqlitegen.InsertStatusCountersParams(c)), "insert status counters")
}

// AddReplies implements content.Repository.
func (r *StatusRepository) AddReplies(ctx context.Context, tx uow.Tx, statusID string, delta int64) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.AddStatusReplies(ctx, sqlitegen.AddStatusRepliesParams{Delta: delta, StatusID: statusID}), "add replies")
}

// AddPost implements content.Repository.
func (r *StatusRepository) AddPost(ctx context.Context, tx uow.Tx, accountID string, publishedAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.AddAccountPost(ctx, sqlitegen.AddAccountPostParams{AccountID: accountID, LastStatusAt: toMillis(publishedAt)})
	return wrap(err, "add account post")
}

// RemovePost implements content.Repository.
func (r *StatusRepository) RemovePost(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	latest, err := q.LatestLivePublishedAt(ctx, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return wrap(q.DeleteAccountPostCounter(ctx, accountID), "delete account post counter")
	}
	if err != nil {
		return wrap(err, "latest status of "+accountID)
	}
	if err := q.RemoveAccountPost(ctx, sqlitegen.RemoveAccountPostParams{LastStatusAt: latest, AccountID: accountID}); err != nil {
		return wrap(err, "remove account post")
	}
	return wrap(q.DeleteEmptyAccountPostCounter(ctx, accountID), "delete empty account post counter")
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
			return wrap(err, step.what)
		}
	}
	return nil
}

func (r *StatusRepository) rebuild(ctx context.Context, tx uow.Tx, what string, steps ...func(*sqlitegen.Queries, context.Context) error) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if err := step(q, ctx); err != nil {
			return wrap(err, "rebuild "+what)
		}
	}
	return nil
}

// RebuildHeads implements content.Repository.
func (r *StatusRepository) RebuildHeads(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "status heads", (*sqlitegen.Queries).DeleteAllStatusHeads, (*sqlitegen.Queries).RebuildStatusHeads)
}

// RebuildMentions implements content.Repository.
func (r *StatusRepository) RebuildMentions(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "status mentions", (*sqlitegen.Queries).DeleteAllStatusMentions, (*sqlitegen.Queries).RebuildStatusMentions)
}

// RebuildReplyLinks implements content.Repository.
func (r *StatusRepository) RebuildReplyLinks(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "reply links", (*sqlitegen.Queries).DeleteAllStatusReplyLinks, (*sqlitegen.Queries).RebuildStatusReplyLinks)
}

// RebuildCounters implements content.Repository.
func (r *StatusRepository) RebuildCounters(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "status counters", (*sqlitegen.Queries).DeleteAllStatusCounters, (*sqlitegen.Queries).RebuildStatusCounters)
}

// RebuildPostCounters implements content.Repository.
func (r *StatusRepository) RebuildPostCounters(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "account post counters", (*sqlitegen.Queries).DeleteAllAccountPostCounters, (*sqlitegen.Queries).RebuildAccountPostCounters)
}

// ListHeads implements content.Repository.
func (r *StatusRepository) ListHeads(ctx context.Context) ([]content.Head, error) {
	rows, err := r.db.reader().ListStatusHeads(ctx)
	if err != nil {
		return nil, wrap(err, "list status heads")
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
		return nil, wrap(err, "list status mentions")
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
		return nil, wrap(err, "list reply links")
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
		return nil, wrap(err, "list status counters")
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
		return nil, wrap(err, "list account post counters")
	}
	out := make([]content.PostCounter, len(rows))
	for i, row := range rows {
		out[i] = postCounterFrom(row)
	}
	return out, nil
}

// FindStatus implements content.Repository.
func (r *StatusRepository) FindStatus(ctx context.Context, id string) (content.Status, error) {
	row, err := r.db.reader().GetStatus(ctx, id)
	if err != nil {
		return content.Status{}, notFound(err, appcontent.ErrNotFound, "status "+id)
	}
	return statusFrom(row), nil
}

// FindStatusByURI implements content.Repository.
func (r *StatusRepository) FindStatusByURI(ctx context.Context, uri string) (content.Status, error) {
	row, err := r.db.reader().GetStatusByURI(ctx, uri)
	if err != nil {
		return content.Status{}, notFound(err, appcontent.ErrNotFound, "status "+uri)
	}
	return statusFrom(row), nil
}

// FindLocalStatus implements content.Repository.
func (r *StatusRepository) FindLocalStatus(ctx context.Context, statusID string) (content.LocalStatus, error) {
	row, err := r.db.reader().GetLocalStatus(ctx, statusID)
	if err != nil {
		return content.LocalStatus{}, notFound(err, appcontent.ErrNotFound, "local status "+statusID)
	}
	return localStatusFrom(row), nil
}

// FindLocalStatusByIdempotencyKey implements content.Repository.
func (r *StatusRepository) FindLocalStatusByIdempotencyKey(ctx context.Context, authorAccountID, key string) (content.LocalStatus, error) {
	row, err := r.db.reader().GetLocalStatusByIdempotencyKey(ctx, sqlitegen.GetLocalStatusByIdempotencyKeyParams{
		AuthorAccountID: authorAccountID, IdempotencyKey: toNullString(key),
	})
	if err != nil {
		return content.LocalStatus{}, notFound(err, appcontent.ErrNotFound, "local status with idempotency key "+key)
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
		return nil, wrap(err, "list live status heads")
	}
	out := make([]appcontent.Post, len(rows))
	for i, row := range rows {
		out[i].Status = statusFrom(sqlitegen.Status{
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
func findRevision(ctx context.Context, q *sqlitegen.Queries, id string) (content.Revision, error) {
	row, err := q.GetStatusRevision(ctx, id)
	if err != nil {
		return content.Revision{}, notFound(err, appcontent.ErrNotFound, "status revision "+id)
	}
	rev := revisionFrom(row)
	bodies, err := q.ListStatusRevisionBodies(ctx, id)
	if err != nil {
		return content.Revision{}, wrap(err, "list status revision bodies")
	}
	for _, b := range bodies {
		rev.Bodies = append(rev.Bodies, content.Body{Language: b.Language, ContentHTML: b.ContentHtml})
	}
	src, err := q.GetStatusRevisionSource(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return content.Revision{}, wrap(err, "get status revision source")
	default:
		rev.Source = &content.Source{Text: src.SourceText, Format: content.SourceFormat(src.SourceFormat)}
	}
	attachments, err := q.ListStatusRevisionAttachments(ctx, id)
	if err != nil {
		return content.Revision{}, wrap(err, "list status revision attachments")
	}
	for _, a := range attachments {
		rev.Attachments = append(rev.Attachments, content.Attachment{
			Position: int(a.Position), MediaID: a.MediaID, Description: a.Description.String,
		})
	}
	if rev.MentionedAccountIDs, err = nonEmpty(q.ListStatusRevisionMentions(ctx, id)); err != nil {
		return content.Revision{}, wrap(err, "list status revision mentions")
	}
	if rev.TagIDs, err = nonEmpty(q.ListStatusRevisionTags(ctx, id)); err != nil {
		return content.Revision{}, wrap(err, "list status revision tags")
	}
	if rev.EmojiIDs, err = nonEmpty(q.ListStatusRevisionEmojis(ctx, id)); err != nil {
		return content.Revision{}, wrap(err, "list status revision emoji")
	}
	return rev, findRemoteRevisionParts(ctx, q, &rev)
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
		return content.Revision{}, notFound(err, appcontent.ErrNotFound, "head of "+statusID)
	}
	return r.FindRevision(ctx, head.RevisionID)
}

// ListRevisions implements content.Repository.
func (r *StatusRepository) ListRevisions(ctx context.Context, statusID string) ([]content.Revision, error) {
	rows, err := r.db.reader().ListStatusRevisions(ctx, statusID)
	if err != nil {
		return nil, wrap(err, "list status revisions")
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
		return content.Deletion{}, notFound(err, appcontent.ErrNotFound, "deletion of "+statusID)
	}
	return deletionFrom(row), nil
}

// FindDeletionByURI implements content.Repository.
func (r *StatusRepository) FindDeletionByURI(ctx context.Context, uri string) (content.Deletion, error) {
	row, err := r.db.reader().GetStatusDeletionByURI(ctx, uri)
	if err != nil {
		return content.Deletion{}, notFound(err, appcontent.ErrNotFound, "deletion of "+uri)
	}
	return deletionFrom(row), nil
}

// FindTagByName implements content.Repository.
func (r *StatusRepository) FindTagByName(ctx context.Context, nameNormalized string) (content.Tag, error) {
	row, err := r.db.reader().GetTagByName(ctx, nameNormalized)
	if err != nil {
		return content.Tag{}, notFound(err, appcontent.ErrNotFound, "tag "+nameNormalized)
	}
	return tagFrom(row), nil
}

// ListTagsByPrefix implements content.Repository.
func (r *StatusRepository) ListTagsByPrefix(ctx context.Context, low, high string, limit int) ([]content.Tag, error) {
	rows, err := r.db.reader().ListTagsByPrefix(ctx, sqlitegen.ListTagsByPrefixParams{Low: low, High: high, Lim: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list tags by prefix")
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
		return content.Tag{}, notFound(err, appcontent.ErrNotFound, "tag "+id)
	}
	return tagFrom(row), nil
}

// ListReplies implements content.Repository.
func (r *StatusRepository) ListReplies(ctx context.Context, uri string) ([]content.Status, error) {
	rows, err := r.db.reader().ListLiveRepliesTo(ctx, toNullString(uri))
	if err != nil {
		return nil, wrap(err, "list replies")
	}
	out := make([]content.Status, len(rows))
	for i, row := range rows {
		out[i] = statusFrom(row)
	}
	return out, nil
}

// ListLiveRemoteStatusIDs implements content.Repository.
func (r *StatusRepository) ListLiveRemoteStatusIDs(ctx context.Context, authorAccountID, before string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListLiveRemoteStatusIDsOfAuthor(ctx, sqlitegen.ListLiveRemoteStatusIDsOfAuthorParams{
		AuthorAccountID: authorAccountID, Before: before, Lim: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list remote statuses of "+authorAccountID)
	}
	return ids, nil
}

// FindCounters implements content.Repository.
func (r *StatusRepository) FindCounters(ctx context.Context, statusID string) (content.Counters, error) {
	row, err := r.db.reader().GetStatusCounters(ctx, statusID)
	if err != nil {
		return content.Counters{}, notFound(err, appcontent.ErrNotFound, "counters of "+statusID)
	}
	return content.Counters(row), nil
}

// FindPostCounter implements content.Repository.
func (r *StatusRepository) FindPostCounter(ctx context.Context, accountID string) (content.PostCounter, error) {
	row, err := r.db.reader().GetAccountPostCounter(ctx, accountID)
	if err != nil {
		return content.PostCounter{}, notFound(err, appcontent.ErrNotFound, "post counter of "+accountID)
	}
	return postCounterFrom(row), nil
}

func statusFrom(row sqlitegen.Status) content.Status {
	return content.Status{
		ID:              row.ID,
		URI:             row.Uri,
		URL:             row.Url.String,
		AuthorAccountID: row.AuthorAccountID,
		IsLocal:         row.IsLocal != 0,
		Visibility:      content.Visibility(row.Visibility),
		InReplyToURI:    row.InReplyToUri.String,
		ConversationURI: row.ConversationUri.String,
		PublishedAt:     fromMillis(row.PublishedAt),
	}
}

func localStatusFrom(row sqlitegen.LocalStatus) content.LocalStatus {
	return content.LocalStatus{
		StatusID:        row.StatusID,
		AuthorAccountID: row.AuthorAccountID,
		ClientID:        row.ClientID.String,
		IdempotencyKey:  row.IdempotencyKey.String,
	}
}

func revisionFrom(row sqlitegen.StatusRevision) content.Revision {
	return content.Revision{
		ID:          row.ID,
		StatusID:    row.StatusID,
		Seq:         int(row.Seq),
		SpoilerText: row.SpoilerText,
		Sensitive:   row.Sensitive != 0,
		Language:    row.Language,
		RevisedAt:   fromMillis(row.RevisedAt),
	}
}

func deletionFrom(row sqlitegen.StatusDeletion) content.Deletion {
	return content.Deletion{
		StatusID:           row.StatusID,
		URI:                row.Uri,
		AuthorAccountID:    row.AuthorAccountID,
		Origin:             content.DeletionOrigin(row.Origin),
		ModerationActionID: row.ModerationActionID.String,
		DeletedAt:          fromMillis(row.DeletedAt),
	}
}

func tagFrom(row sqlitegen.Tag) content.Tag {
	return content.Tag{ID: row.ID, Name: row.Name, NameNormalized: row.NameNormalized, CreatedAt: fromMillis(row.CreatedAt)}
}

func headFrom(row sqlitegen.StatusHead) content.Head {
	return content.Head{
		StatusID: row.StatusID, RevisionID: row.RevisionID, Seq: int(row.Seq), LastRevisedAt: fromMillis(row.LastRevisedAt),
	}
}

func postCounterFrom(row sqlitegen.AccountPostCounter) content.PostCounter {
	return content.PostCounter{
		AccountID: row.AccountID, StatusesCount: row.StatusesCount, LastStatusAt: fromMillis(row.LastStatusAt),
	}
}
