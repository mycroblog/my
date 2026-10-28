// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The requests of the holders of create_custom_emojis (ADR-0070 §2, §7), part of
// CustomEmojiRepository.

// InsertCustomEmojiRequest implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) InsertCustomEmojiRequest(ctx context.Context, tx uow.Tx, req content.CustomEmojiRequest) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	pending := sqlitegen.InsertCustomEmojiPendingRequestParams{RequestID: req.ID, RequesterUserID: req.RequesterUserID}
	if req.Kind == content.RequestCreate {
		taken, err := q.LocalCustomEmojiShortcodeTaken(ctx, req.Shortcode)
		if err != nil {
			return wrap(err, "local custom emoji shortcode taken")
		}
		if taken != 0 {
			return appcontent.ErrShortcodeTaken
		}
		pending.ReservedShortcode = toNullString(req.Shortcode)
	} else {
		pending.EmojiID = toNullString(req.EmojiID)
	}
	err = q.InsertCustomEmojiRequest(ctx, sqlitegen.InsertCustomEmojiRequestParams{
		ID: req.ID, Kind: string(req.Kind), RequesterUserID: req.RequesterUserID, EmojiID: toNullString(req.EmojiID),
		Shortcode: req.Shortcode, MediaID: req.MediaID, CreatedAt: toMillis(req.CreatedAt),
	})
	if err != nil {
		return wrap(err, "insert custom emoji request")
	}
	err = q.InsertCustomEmojiPendingRequest(ctx, pending)
	switch {
	case isUniqueViolation(err, "custom_emoji_pending_requests.reserved_shortcode"):
		return appcontent.ErrShortcodeTaken
	case isUniqueViolation(err, "custom_emoji_pending_requests.emoji_id"):
		return appcontent.ErrCustomEmojiRequestPending
	}
	return wrap(err, "insert custom emoji pending request")
}

// CloseCustomEmojiRequest implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) CloseCustomEmojiRequest(ctx context.Context, tx uow.Tx, o content.CustomEmojiRequestOutcome) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.DeleteCustomEmojiPendingRequest(ctx, o.RequestID)
	if err != nil {
		return wrap(err, "delete custom emoji pending request")
	}
	if n == 0 {
		return appcontent.ErrCustomEmojiRequestNotPending
	}
	err = q.InsertCustomEmojiRequestOutcome(ctx, sqlitegen.InsertCustomEmojiRequestOutcomeParams{
		ID: o.ID, RequestID: o.RequestID, Outcome: string(o.Outcome), DecidedByUserID: toNullString(o.DecidedByUserID),
		Reason: toNullString(o.Reason), EmojiID: toNullString(o.EmojiID), DecidedAt: toMillis(o.DecidedAt),
	})
	if isUniqueViolation(err, "custom_emoji_request_outcomes.request_id") {
		return appcontent.ErrCustomEmojiRequestNotPending
	}
	return wrap(err, "insert custom emoji request outcome")
}

// PendingImageRequestOf implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) PendingImageRequestOf(ctx context.Context, tx uow.Tx, emojiID string) (string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetPendingImageRequestOfEmoji(ctx, emojiID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, wrap(err, "get pending image request")
}

// emojiRequestRow is a row of GetCustomEmojiRequest, ListCustomEmojiRequests, or
// ListCustomEmojiRequestsByID.
type emojiRequestRow struct {
	ID, Kind, RequesterUserID string
	EmojiID                   sql.NullString
	Shortcode, MediaID        string
	CreatedAt                 int64
	OutcomeID, Outcome        string
	DecidedByUserID, Reason   string
	OutcomeEmojiID            string
	DecidedAt                 int64
}

func emojiRequestFrom(row emojiRequestRow) content.CustomEmojiRequestRecord {
	out := content.CustomEmojiRequestRecord{Request: content.CustomEmojiRequest{
		ID: row.ID, Kind: content.CustomEmojiRequestKind(row.Kind), RequesterUserID: row.RequesterUserID, EmojiID: row.EmojiID.String,
		Shortcode: row.Shortcode, MediaID: row.MediaID, CreatedAt: fromMillis(row.CreatedAt),
	}}
	if row.OutcomeID != "" {
		out.Outcome = &content.CustomEmojiRequestOutcome{
			ID: row.OutcomeID, RequestID: row.ID, Outcome: content.CustomEmojiRequestOutcomeKind(row.Outcome),
			DecidedByUserID: row.DecidedByUserID, Reason: row.Reason, EmojiID: row.OutcomeEmojiID, DecidedAt: fromMillis(row.DecidedAt),
		}
	}
	return out
}

// FindCustomEmojiRequest implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) FindCustomEmojiRequest(ctx context.Context, id string) (content.CustomEmojiRequestRecord, error) {
	row, err := r.db.reader().GetCustomEmojiRequest(ctx, id)
	if err != nil {
		return content.CustomEmojiRequestRecord{}, notFound(err, appcontent.ErrNotFound, "custom emoji request "+id)
	}
	return emojiRequestFrom(emojiRequestRow(row)), nil
}

// ListCustomEmojiRequests implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) ListCustomEmojiRequests(ctx context.Context, q appcontent.CustomEmojiRequestQuery, limit int) ([]content.CustomEmojiRequestRecord, error) {
	rows, err := r.db.reader().ListCustomEmojiRequests(ctx, sqlitegen.ListCustomEmojiRequestsParams{
		RequesterUserID: q.RequesterUserID, State: string(q.State), BeforeID: q.Before, RowLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list custom emoji requests")
	}
	out := make([]content.CustomEmojiRequestRecord, len(rows))
	for i, row := range rows {
		out[i] = emojiRequestFrom(emojiRequestRow(row))
	}
	return out, nil
}

// ListCustomEmojiRequestsByID implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) ListCustomEmojiRequestsByID(ctx context.Context, ids []string) ([]content.CustomEmojiRequestRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := jsonList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListCustomEmojiRequestsByID(ctx, list)
	if err != nil {
		return nil, wrap(err, "list custom emoji requests by ID")
	}
	out := make([]content.CustomEmojiRequestRecord, len(rows))
	for i, row := range rows {
		out[i] = emojiRequestFrom(emojiRequestRow(row))
	}
	return out, nil
}

// CountCustomEmojiUsage implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) CountCustomEmojiUsage(ctx context.Context, userID string) (made, pending int, err error) {
	row, err := r.db.reader().CountCustomEmojiUsage(ctx, toNullString(userID))
	return int(row.Made), int(row.Pending), wrap(err, "count custom emoji usage")
}

// IsMediaOfPendingCustomEmojiRequest implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) IsMediaOfPendingCustomEmojiRequest(ctx context.Context, mediaID string) (bool, error) {
	n, err := r.db.reader().IsMediaOfPendingCustomEmojiRequest(ctx, mediaID)
	return n != 0, wrap(err, "is media of a pending custom emoji request")
}
