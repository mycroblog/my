// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
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
	pending := contentgen.InsertCustomEmojiPendingRequestParams{RequestID: req.ID, RequesterUserID: req.RequesterUserID}
	if req.Kind == content.RequestCreate {
		taken, err := q.LocalCustomEmojiShortcodeTaken(ctx, req.Shortcode)
		if err != nil {
			return sqlite.Wrap(err, "local custom emoji shortcode taken")
		}
		if taken != 0 {
			return appcontent.ErrShortcodeTaken
		}
		pending.ReservedShortcode = sqlite.ToNullString(req.Shortcode)
	} else {
		pending.EmojiID = sqlite.ToNullString(req.EmojiID)
	}
	err = q.InsertCustomEmojiRequest(ctx, contentgen.InsertCustomEmojiRequestParams{
		ID: req.ID, Kind: string(req.Kind), RequesterUserID: sqlite.ToNullString(req.RequesterUserID), EmojiID: sqlite.ToNullString(req.EmojiID),
		Shortcode: req.Shortcode, MediaID: req.MediaID, CreatedAt: sqlite.ToMillis(req.CreatedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert custom emoji request")
	}
	err = q.InsertCustomEmojiPendingRequest(ctx, pending)
	switch {
	case sqlite.IsUniqueViolation(err, "custom_emoji_pending_requests.reserved_shortcode"):
		return appcontent.ErrShortcodeTaken
	case sqlite.IsUniqueViolation(err, "custom_emoji_pending_requests.emoji_id"):
		return appcontent.ErrCustomEmojiRequestPending
	}
	return sqlite.Wrap(err, "insert custom emoji pending request")
}

// CloseCustomEmojiRequest implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) CloseCustomEmojiRequest(ctx context.Context, tx uow.Tx, o content.CustomEmojiRequestOutcome) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.DeleteCustomEmojiPendingRequest(ctx, o.RequestID)
	if err != nil {
		return sqlite.Wrap(err, "delete custom emoji pending request")
	}
	if n == 0 {
		return appcontent.ErrCustomEmojiRequestNotPending
	}
	err = q.InsertCustomEmojiRequestOutcome(ctx, contentgen.InsertCustomEmojiRequestOutcomeParams{
		ID: o.ID, RequestID: o.RequestID, Outcome: string(o.Outcome), DecidedByUserID: sqlite.ToNullString(o.DecidedByUserID),
		Reason: sqlite.ToNullString(o.Reason), EmojiID: sqlite.ToNullString(o.EmojiID), DecidedAt: sqlite.ToMillis(o.DecidedAt),
	})
	if sqlite.IsUniqueViolation(err, "custom_emoji_request_outcomes.request_id") {
		return appcontent.ErrCustomEmojiRequestNotPending
	}
	return sqlite.Wrap(err, "insert custom emoji request outcome")
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
	return id, sqlite.Wrap(err, "get pending image request")
}

// emojiRequestRow is a row of GetCustomEmojiRequest, ListCustomEmojiRequests, or
// ListCustomEmojiRequestsByID.
type emojiRequestRow struct {
	ID, Kind                 string
	RequesterUserID, EmojiID sql.NullString
	Shortcode, MediaID       string
	CreatedAt                int64
	OutcomeID, Outcome       string
	DecidedByUserID, Reason  string
	OutcomeEmojiID           string
	DecidedAt                int64
}

func emojiRequestFrom(row emojiRequestRow) content.CustomEmojiRequestRecord {
	out := content.CustomEmojiRequestRecord{Request: content.CustomEmojiRequest{
		ID: row.ID, Kind: content.CustomEmojiRequestKind(row.Kind), RequesterUserID: row.RequesterUserID.String, EmojiID: row.EmojiID.String,
		Shortcode: row.Shortcode, MediaID: row.MediaID, CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}}
	if row.OutcomeID != "" {
		out.Outcome = &content.CustomEmojiRequestOutcome{
			ID: row.OutcomeID, RequestID: row.ID, Outcome: content.CustomEmojiRequestOutcomeKind(row.Outcome),
			DecidedByUserID: row.DecidedByUserID, Reason: row.Reason, EmojiID: row.OutcomeEmojiID, DecidedAt: sqlite.FromMillis(row.DecidedAt),
		}
	}
	return out
}

// FindCustomEmojiRequest implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) FindCustomEmojiRequest(ctx context.Context, id string) (content.CustomEmojiRequestRecord, error) {
	row, err := r.db.reader().GetCustomEmojiRequest(ctx, id)
	if err != nil {
		return content.CustomEmojiRequestRecord{}, sqlite.NotFound(err, appcontent.ErrNotFound, "custom emoji request "+id)
	}
	return emojiRequestFrom(emojiRequestRow(row)), nil
}

// ListCustomEmojiRequests implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) ListCustomEmojiRequests(ctx context.Context, q appcontent.CustomEmojiRequestQuery, limit int) ([]content.CustomEmojiRequestRecord, error) {
	rows, err := r.db.reader().ListCustomEmojiRequests(ctx, contentgen.ListCustomEmojiRequestsParams{
		RequesterUserID: q.RequesterUserID, State: string(q.State), BeforeID: q.Before, RowLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list custom emoji requests")
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
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListCustomEmojiRequestsByID(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list custom emoji requests by ID")
	}
	out := make([]content.CustomEmojiRequestRecord, len(rows))
	for i, row := range rows {
		out[i] = emojiRequestFrom(emojiRequestRow(row))
	}
	return out, nil
}

// CountCustomEmojiUsage implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) CountCustomEmojiUsage(ctx context.Context, userID string, since, until time.Time) (approved, pending int, err error) {
	row, err := r.db.reader().CountCustomEmojiUsage(ctx, contentgen.CountCustomEmojiUsageParams{UserID: sqlite.ToNullString(userID), Since: sqlite.ToMillis(since), Until: sqlite.ToMillis(until)})
	return int(row.Approved), int(row.Pending), sqlite.Wrap(err, "count custom emoji usage")
}

// IsMediaOfPendingCustomEmojiRequest implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) IsMediaOfPendingCustomEmojiRequest(ctx context.Context, mediaID string) (bool, error) {
	n, err := r.db.reader().IsMediaOfPendingCustomEmojiRequest(ctx, mediaID)
	return n != 0, sqlite.Wrap(err, "is media of a pending custom emoji request")
}

// ListPendingCustomEmojiRequestsOfUser implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) ListPendingCustomEmojiRequestsOfUser(ctx context.Context, tx uow.Tx, userID string) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListPendingCustomEmojiRequestsOfUser(ctx, userID)
	return ids, sqlite.Wrap(err, "list pending custom emoji requests")
}

// BlankCustomEmojiRequester implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) BlankCustomEmojiRequester(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.BlankCustomEmojiRequester(ctx, sqlite.ToNullString(userID)), "blank custom emoji requester")
}

// RebuildCustomEmojiPendingRequests implements content.CustomEmojiRequestRepository.
func (r *CustomEmojiRepository) RebuildCustomEmojiPendingRequests(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteAllCustomEmojiPendingRequests(ctx); err != nil {
		return sqlite.Wrap(err, "delete custom emoji pending requests")
	}
	return sqlite.Wrap(q.RebuildCustomEmojiPendingRequests(ctx), "rebuild custom emoji pending requests")
}
