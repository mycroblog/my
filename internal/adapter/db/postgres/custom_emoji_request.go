// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
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
	pending := postgresgen.InsertCustomEmojiPendingRequestParams{RequestID: req.ID, RequesterUserID: req.RequesterUserID}
	if req.Kind == content.RequestCreate {
		taken, err := q.LocalCustomEmojiShortcodeTaken(ctx, req.Shortcode)
		if err != nil {
			return wrap(err, "local custom emoji shortcode taken")
		}
		if taken {
			return appcontent.ErrShortcodeTaken
		}
		pending.ReservedShortcode = toNullString(req.Shortcode)
	} else {
		pending.EmojiID = toNullString(req.EmojiID)
	}
	err = q.InsertCustomEmojiRequest(ctx, postgresgen.InsertCustomEmojiRequestParams{
		ID: req.ID, Kind: string(req.Kind), RequesterUserID: req.RequesterUserID, EmojiID: toNullString(req.EmojiID),
		Shortcode: req.Shortcode, MediaID: req.MediaID, CreatedAt: utc(req.CreatedAt),
	})
	if err != nil {
		return wrap(err, "insert custom emoji request")
	}
	err = q.InsertCustomEmojiPendingRequest(ctx, pending)
	switch {
	case isUniqueViolation(err, "custom_emoji_pending_requests_reserved_shortcode"):
		return appcontent.ErrShortcodeTaken
	case isUniqueViolation(err, "custom_emoji_pending_requests_emoji_id"):
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
	err = q.InsertCustomEmojiRequestOutcome(ctx, postgresgen.InsertCustomEmojiRequestOutcomeParams{
		ID: o.ID, RequestID: o.RequestID, Outcome: string(o.Outcome), DecidedByUserID: toNullString(o.DecidedByUserID),
		Reason: toNullString(o.Reason), EmojiID: toNullString(o.EmojiID), DecidedAt: utc(o.DecidedAt),
	})
	if isUniqueViolation(err, "custom_emoji_request_outcomes_request_id") {
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
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, wrap(err, "get pending image request")
}

// emojiRequestRow is a row of GetCustomEmojiRequest, ListCustomEmojiRequests, or
// ListCustomEmojiRequestsByID.
type emojiRequestRow struct {
	ID, Kind, RequesterUserID string
	EmojiID                   *string
	Shortcode, MediaID        string
	CreatedAt                 time.Time
	OutcomeID, Outcome        string
	DecidedByUserID, Reason   string
	OutcomeEmojiID            string
	DecidedAt                 time.Time
}

func emojiRequestFrom(row emojiRequestRow) content.CustomEmojiRequestRecord {
	out := content.CustomEmojiRequestRecord{Request: content.CustomEmojiRequest{
		ID: row.ID, Kind: content.CustomEmojiRequestKind(row.Kind), RequesterUserID: row.RequesterUserID, EmojiID: fromNullString(row.EmojiID),
		Shortcode: row.Shortcode, MediaID: row.MediaID, CreatedAt: utc(row.CreatedAt),
	}}
	if row.OutcomeID != "" {
		out.Outcome = &content.CustomEmojiRequestOutcome{
			ID: row.OutcomeID, RequestID: row.ID, Outcome: content.CustomEmojiRequestOutcomeKind(row.Outcome),
			DecidedByUserID: row.DecidedByUserID, Reason: row.Reason, EmojiID: row.OutcomeEmojiID, DecidedAt: utc(row.DecidedAt),
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
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListCustomEmojiRequests(ctx, postgresgen.ListCustomEmojiRequestsParams{
		RequesterUserID: toNullString(q.RequesterUserID), State: string(q.State), BeforeID: toNullString(q.Before), RowLimit: n,
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
	rows, err := r.db.reader().ListCustomEmojiRequestsByID(ctx, ids)
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
func (r *CustomEmojiRepository) CountCustomEmojiUsage(ctx context.Context, userID string, since, until time.Time) (approved, pending int, err error) {
	row, err := r.db.reader().CountCustomEmojiUsage(ctx, postgresgen.CountCustomEmojiUsageParams{UserID: userID, Since: since, Until: until})
	return int(row.Approved), int(row.Pending), wrap(err, "count custom emoji usage")
}

// IsMediaOfPendingCustomEmojiRequest implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) IsMediaOfPendingCustomEmojiRequest(ctx context.Context, mediaID string) (bool, error) {
	pending, err := r.db.reader().IsMediaOfPendingCustomEmojiRequest(ctx, mediaID)
	return pending, wrap(err, "is media of a pending custom emoji request")
}
