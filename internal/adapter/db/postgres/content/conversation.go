// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// EnsureConversation implements content.Repository.
func (r *StatusRepository) EnsureConversation(ctx context.Context, tx uow.Tx, c content.Conversation) (content.Conversation, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.Conversation{}, err
	}
	err = q.InsertConversationIfAbsent(ctx, contentgen.InsertConversationIfAbsentParams{ID: c.ID, Uri: c.URI, IsLocal: c.IsLocal})
	if err != nil {
		return content.Conversation{}, postgres.Wrap(err, "insert conversation")
	}
	row, err := q.GetConversationByURI(ctx, c.URI)
	if err != nil {
		return content.Conversation{}, postgres.Wrap(err, "read conversation")
	}
	return conversationFrom(row), nil
}

// FindConversationTx implements content.Repository.
func (r *StatusRepository) FindConversationTx(ctx context.Context, tx uow.Tx, id string) (content.Conversation, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.Conversation{}, err
	}
	row, err := q.GetConversation(ctx, id)
	if err != nil {
		return content.Conversation{}, postgres.NotFound(err, appcontent.ErrNotFound, "conversation "+id)
	}
	return conversationFrom(row), nil
}

// ConversationMergeTargetTx implements content.Repository.
func (r *StatusRepository) ConversationMergeTargetTx(ctx context.Context, tx uow.Tx, id string) (string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return "", err
	}
	into, err := q.GetConversationMergeTarget(ctx, id)
	if err != nil {
		return "", postgres.NotFound(err, appcontent.ErrNotFound, "merge of conversation "+id)
	}
	return into, nil
}

// StatusConversationsTx implements content.Repository.
func (r *StatusRepository) StatusConversationsTx(ctx context.Context, tx uow.Tx, statusIDs []string) (map[string]content.Conversation, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	return statusConversations(ctx, q, statusIDs)
}

// StatusConversations implements content.Repository.
func (r *StatusRepository) StatusConversations(ctx context.Context, statusIDs []string) (map[string]content.Conversation, error) {
	return statusConversations(ctx, r.db.reader(), statusIDs)
}

func statusConversations(ctx context.Context, q *contentgen.Queries, statusIDs []string) (map[string]content.Conversation, error) {
	out := make(map[string]content.Conversation, len(statusIDs))
	if len(statusIDs) == 0 {
		return out, nil
	}
	rows, err := q.ListStatusConversations(ctx, statusIDs)
	if err != nil {
		return nil, postgres.Wrap(err, "list status conversations")
	}
	for _, row := range rows {
		out[row.StatusID] = content.Conversation{ID: row.ID, URI: row.Uri, IsLocal: row.IsLocal}
	}
	return out, nil
}

// InsertStatusConversation implements content.Repository.
func (r *StatusRepository) InsertStatusConversation(ctx context.Context, tx uow.Tx, statusID, conversationID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertStatusConversation(ctx, contentgen.InsertStatusConversationParams{StatusID: statusID, ConversationID: conversationID})
	return postgres.Wrap(err, "insert status conversation")
}

// MergeConversation implements content.Repository.
func (r *StatusRepository) MergeConversation(ctx context.Context, tx uow.Tx, m content.ConversationMerge) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertConversationMerge(ctx, contentgen.InsertConversationMergeParams{
		MergedConversationID: m.MergedConversationID, IntoConversationID: m.IntoConversationID,
		StatusID: m.StatusID, MergedAt: m.MergedAt,
	})
	if err != nil {
		return postgres.Wrap(err, "insert conversation merge")
	}
	err = q.MoveStatusConversations(ctx, contentgen.MoveStatusConversationsParams{
		IntoConversationID: m.IntoConversationID, MergedConversationID: m.MergedConversationID,
	})
	return postgres.Wrap(err, "move status conversations")
}

// FindConversation implements content.Repository.
func (r *StatusRepository) FindConversation(ctx context.Context, id string) (content.Conversation, error) {
	row, err := r.db.reader().GetConversation(ctx, id)
	if err != nil {
		return content.Conversation{}, postgres.NotFound(err, appcontent.ErrNotFound, "conversation "+id)
	}
	return conversationFrom(row), nil
}

// FindConversationByURI implements content.Repository.
func (r *StatusRepository) FindConversationByURI(ctx context.Context, uri string) (content.Conversation, error) {
	row, err := r.db.reader().GetConversationByURI(ctx, uri)
	if err != nil {
		return content.Conversation{}, postgres.NotFound(err, appcontent.ErrNotFound, "conversation "+uri)
	}
	return conversationFrom(row), nil
}

// FindConversationMerge implements content.Repository.
func (r *StatusRepository) FindConversationMerge(ctx context.Context, id string) (content.ConversationMerge, error) {
	row, err := r.db.reader().GetConversationMerge(ctx, id)
	if err != nil {
		return content.ConversationMerge{}, postgres.NotFound(err, appcontent.ErrNotFound, "merge of conversation "+id)
	}
	return content.ConversationMerge{
		MergedConversationID: row.MergedConversationID, IntoConversationID: row.IntoConversationID,
		StatusID: row.StatusID, MergedAt: row.MergedAt,
	}, nil
}

// ListLiveConversationStatuses implements content.Repository.
func (r *StatusRepository) ListLiveConversationStatuses(ctx context.Context, conversationID, viewerAccountID, after string, limit int) ([]content.Status, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListLiveConversationStatuses(ctx, contentgen.ListLiveConversationStatusesParams{
		ConversationID: conversationID, ViewerAccountID: postgres.ToNullString(viewerAccountID), After: postgres.ToNullString(after), Lim: lim,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list conversation statuses")
	}
	out := make([]content.Status, len(rows))
	for i, row := range rows {
		out[i] = statusFrom(row)
	}
	return out, nil
}

// LocalStatusInConversationOf implements content.Repository.
func (r *StatusRepository) LocalStatusInConversationOf(ctx context.Context, statusID string) (bool, error) {
	ok, err := r.db.reader().ExistsLocalStatusInConversationOf(ctx, statusID)
	return ok, postgres.Wrap(err, "find local status in conversation")
}

func conversationFrom(row contentgen.Conversation) content.Conversation {
	return content.Conversation{ID: row.ID, URI: row.Uri, IsLocal: row.IsLocal}
}
