// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// StatusRepository implements content.Repository on PostgreSQL.
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
	err = q.InsertStatus(ctx, postgresgen.InsertStatusParams{
		ID:              s.ID,
		Uri:             s.URI,
		Url:             toNullString(s.URL),
		AuthorAccountID: s.AuthorAccountID,
		IsLocal:         s.IsLocal,
		Visibility:      string(s.Visibility),
		InReplyToUri:    toNullString(s.InReplyToURI),
		ConversationUri: toNullString(s.ConversationURI),
		PublishedAt:     s.PublishedAt,
	})
	if isUniqueViolation(err, "statuses_uri") {
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
	err = q.InsertLocalStatus(ctx, postgresgen.InsertLocalStatusParams{
		StatusID:        ls.StatusID,
		AuthorAccountID: ls.AuthorAccountID,
		ClientID:        toNullString(ls.ClientID),
		IdempotencyKey:  toNullString(ls.IdempotencyKey),
	})
	if isUniqueViolation(err, "local_statuses_author_account_id_idempotency_key") {
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
	seq, err := toInt32(rev.Seq, "status revision seq")
	if err != nil {
		return err
	}
	err = q.InsertStatusRevision(ctx, postgresgen.InsertStatusRevisionParams{
		ID:          rev.ID,
		StatusID:    rev.StatusID,
		Seq:         seq,
		SpoilerText: rev.SpoilerText,
		Sensitive:   rev.Sensitive,
		Language:    rev.Language,
		RevisedAt:   rev.RevisedAt,
	})
	if isUniqueViolation(err, "status_revisions_status_id_seq") {
		// A concurrent revision took this seq after the command read the head at READ COMMITTED.
		// A new attempt reads the new head and takes the next seq.
		return uow.MarkRetryable(wrap(err, "insert status revision"))
	}
	if err != nil {
		return wrap(err, "insert status revision")
	}
	for _, b := range rev.Bodies {
		err := q.InsertStatusRevisionBody(ctx, postgresgen.InsertStatusRevisionBodyParams{
			RevisionID: rev.ID, Language: b.Language, ContentHtml: b.ContentHTML,
		})
		if err != nil {
			return wrap(err, "insert status revision body")
		}
	}
	if rev.Source != nil {
		err := q.InsertStatusRevisionSource(ctx, postgresgen.InsertStatusRevisionSourceParams{
			RevisionID: rev.ID, SourceText: rev.Source.Text, SourceFormat: string(rev.Source.Format),
		})
		if err != nil {
			return wrap(err, "insert status revision source")
		}
	}
	for _, a := range rev.Attachments {
		pos, err := toInt32(a.Position, "attachment position")
		if err != nil {
			return err
		}
		err = q.InsertStatusRevisionAttachment(ctx, postgresgen.InsertStatusRevisionAttachmentParams{
			RevisionID: rev.ID, Position: pos, MediaID: a.MediaID, Description: toNullString(a.Description),
		})
		if err != nil {
			return wrap(err, "insert status revision attachment")
		}
	}
	for _, id := range rev.MentionedAccountIDs {
		err := q.InsertStatusRevisionMention(ctx, postgresgen.InsertStatusRevisionMentionParams{RevisionID: rev.ID, AccountID: id})
		if err != nil {
			return wrap(err, "insert status revision mention")
		}
	}
	for _, id := range rev.TagIDs {
		if err := q.InsertStatusRevisionTag(ctx, postgresgen.InsertStatusRevisionTagParams{RevisionID: rev.ID, TagID: id}); err != nil {
			return wrap(err, "insert status revision tag")
		}
	}
	for _, id := range rev.EmojiIDs {
		if err := q.InsertStatusRevisionEmoji(ctx, postgresgen.InsertStatusRevisionEmojiParams{RevisionID: rev.ID, EmojiID: id}); err != nil {
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
	err = q.InsertStatusDeletion(ctx, postgresgen.InsertStatusDeletionParams{
		StatusID:           d.StatusID,
		Uri:                d.URI,
		AuthorAccountID:    d.AuthorAccountID,
		Origin:             string(d.Origin),
		ModerationActionID: toNullString(d.ModerationActionID),
		DeletedAt:          d.DeletedAt,
	})
	if isUniqueViolation(err, "status_deletions_pkey") || isUniqueViolation(err, "status_deletions_tombstone_uri") {
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
	err = q.InsertTagIfAbsent(ctx, postgresgen.InsertTagIfAbsentParams{
		ID: t.ID, Name: t.Name, NameNormalized: t.NameNormalized, CreatedAt: t.CreatedAt,
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
	seq, err := toInt32(h.Seq, "status head seq")
	if err != nil {
		return err
	}
	err = q.UpsertStatusHead(ctx, postgresgen.UpsertStatusHeadParams{
		StatusID: h.StatusID, RevisionID: h.RevisionID, Seq: seq, LastRevisedAt: h.LastRevisedAt,
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
		if err := q.InsertStatusMention(ctx, postgresgen.InsertStatusMentionParams{AccountID: id, StatusID: statusID}); err != nil {
			return wrap(err, "insert status mention")
		}
	}
	return nil
}

// LockStatusKeys implements content.Repository with transaction-scoped advisory locks.
func (r *StatusRepository) LockStatusKeys(ctx context.Context, tx uow.Tx, keys []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := q.LockStatusKey(ctx, "content:"+k); err != nil {
			return wrap(err, "lock status key")
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
	return wrap(q.UpsertStatusReplyLink(ctx, postgresgen.UpsertStatusReplyLinkParams(l)), "upsert reply link")
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
	return wrap(q.InsertStatusCounters(ctx, postgresgen.InsertStatusCountersParams(c)), "insert status counters")
}

// AddReplies implements content.Repository.
func (r *StatusRepository) AddReplies(ctx context.Context, tx uow.Tx, statusID string, delta int64) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.AddStatusReplies(ctx, postgresgen.AddStatusRepliesParams{Delta: delta, StatusID: statusID}), "add replies")
}

// AddPost implements content.Repository.
func (r *StatusRepository) AddPost(ctx context.Context, tx uow.Tx, accountID string, publishedAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.AddAccountPost(ctx, postgresgen.AddAccountPostParams{AccountID: accountID, LastStatusAt: publishedAt})
	return wrap(err, "add account post")
}

// RemovePost implements content.Repository.
func (r *StatusRepository) RemovePost(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	latest, err := q.LatestLivePublishedAt(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return wrap(q.DeleteAccountPostCounter(ctx, accountID), "delete account post counter")
	}
	if err != nil {
		return wrap(err, "latest status of "+accountID)
	}
	if err := q.RemoveAccountPost(ctx, postgresgen.RemoveAccountPostParams{LastStatusAt: latest, AccountID: accountID}); err != nil {
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

func (r *StatusRepository) rebuild(ctx context.Context, tx uow.Tx, what string, steps ...func(*postgresgen.Queries, context.Context) error) error {
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
	return r.rebuild(ctx, tx, "status heads", (*postgresgen.Queries).DeleteAllStatusHeads, (*postgresgen.Queries).RebuildStatusHeads)
}

// RebuildMentions implements content.Repository.
func (r *StatusRepository) RebuildMentions(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "status mentions", (*postgresgen.Queries).DeleteAllStatusMentions, (*postgresgen.Queries).RebuildStatusMentions)
}

// RebuildReplyLinks implements content.Repository.
func (r *StatusRepository) RebuildReplyLinks(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "reply links", (*postgresgen.Queries).DeleteAllStatusReplyLinks, (*postgresgen.Queries).RebuildStatusReplyLinks)
}

// RebuildCounters implements content.Repository.
func (r *StatusRepository) RebuildCounters(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "status counters", (*postgresgen.Queries).DeleteAllStatusCounters, (*postgresgen.Queries).RebuildStatusCounters)
}

// RebuildPostCounters implements content.Repository.
func (r *StatusRepository) RebuildPostCounters(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "account post counters", (*postgresgen.Queries).DeleteAllAccountPostCounters, (*postgresgen.Queries).RebuildAccountPostCounters)
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
	row, err := r.db.reader().GetLocalStatusByIdempotencyKey(ctx, postgresgen.GetLocalStatusByIdempotencyKeyParams{
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
		out[i].Status = statusFrom(postgresgen.Status{
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
func findRevision(ctx context.Context, q *postgresgen.Queries, id string) (content.Revision, error) {
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
	case errors.Is(err, pgx.ErrNoRows):
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
			Position: int(a.Position), MediaID: a.MediaID, Description: fromNullString(a.Description),
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
	n, err := toInt32(limit, "tag search limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListTagsByPrefix(ctx, postgresgen.ListTagsByPrefixParams{Low: low, High: high, Lim: n})
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
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListLiveRemoteStatusIDsOfAuthor(ctx, postgresgen.ListLiveRemoteStatusIDsOfAuthorParams{
		AuthorAccountID: authorAccountID, Before: toNullString(before), Lim: lim,
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

func statusFrom(row postgresgen.Status) content.Status {
	return content.Status{
		ID:              row.ID,
		URI:             row.Uri,
		URL:             fromNullString(row.Url),
		AuthorAccountID: row.AuthorAccountID,
		IsLocal:         row.IsLocal,
		Visibility:      content.Visibility(row.Visibility),
		InReplyToURI:    fromNullString(row.InReplyToUri),
		ConversationURI: fromNullString(row.ConversationUri),
		PublishedAt:     utc(row.PublishedAt),
	}
}

func localStatusFrom(row postgresgen.LocalStatus) content.LocalStatus {
	return content.LocalStatus{
		StatusID:        row.StatusID,
		AuthorAccountID: row.AuthorAccountID,
		ClientID:        fromNullString(row.ClientID),
		IdempotencyKey:  fromNullString(row.IdempotencyKey),
	}
}

func revisionFrom(row postgresgen.StatusRevision) content.Revision {
	return content.Revision{
		ID:          row.ID,
		StatusID:    row.StatusID,
		Seq:         int(row.Seq),
		SpoilerText: row.SpoilerText,
		Sensitive:   row.Sensitive,
		Language:    row.Language,
		RevisedAt:   utc(row.RevisedAt),
	}
}

func deletionFrom(row postgresgen.StatusDeletion) content.Deletion {
	return content.Deletion{
		StatusID:           row.StatusID,
		URI:                row.Uri,
		AuthorAccountID:    row.AuthorAccountID,
		Origin:             content.DeletionOrigin(row.Origin),
		ModerationActionID: fromNullString(row.ModerationActionID),
		DeletedAt:          utc(row.DeletedAt),
	}
}

func tagFrom(row postgresgen.Tag) content.Tag {
	return content.Tag{ID: row.ID, Name: row.Name, NameNormalized: row.NameNormalized, CreatedAt: utc(row.CreatedAt)}
}

func headFrom(row postgresgen.StatusHead) content.Head {
	return content.Head{
		StatusID: row.StatusID, RevisionID: row.RevisionID, Seq: int(row.Seq), LastRevisedAt: utc(row.LastRevisedAt),
	}
}

func postCounterFrom(row postgresgen.AccountPostCounter) content.PostCounter {
	return content.PostCounter{
		AccountID: row.AccountID, StatusesCount: row.StatusesCount, LastStatusAt: utc(row.LastStatusAt),
	}
}
