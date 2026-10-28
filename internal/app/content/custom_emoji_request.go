// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The requests of the holders of create_custom_emojis (ADR-0070 §2, §3, §4): a creator requests
// a new local emoji, or a new image of one they made, which a holder of manage_custom_emojis
// approves or rejects; and while they hold the permission, a creator changes the aliases, the
// category, and the place in the picker of the emoji they made, and deletes them. A request is
// not an emoji: the emoji is made when it is approved, so that no path of ADR-0051 (the picker,
// the texts, the federation, the reactions) shows an emoji that nobody approved.

// The errors of the requests.
var (
	// ErrNotCustomEmojiCreator: the user may not create custom emoji now (their roles do not
	// grant create_custom_emojis, or their account is not usable), so they cannot request one
	// nor change one they made (ADR-0070 §3).
	ErrNotCustomEmojiCreator = errors.New("content: the user may not create custom emoji")
	// ErrCustomEmojiRequestNotPending is what CustomEmojiRequestRepository.CloseCustomEmojiRequest
	// reports for a request that has an outcome.
	ErrCustomEmojiRequestNotPending = errors.New("content: the custom emoji request is not pending")
	// ErrCustomEmojiRequestPending is what CustomEmojiRequestRepository.InsertCustomEmojiRequest
	// reports for a request to replace the image of an emoji that has a pending one.
	ErrCustomEmojiRequestPending = errors.New("content: the custom emoji has a pending request")
)

// CustomEmojiRequestConflictReason says why a request cannot be decided or withdrawn.
type CustomEmojiRequestConflictReason string

// Reasons of a CustomEmojiRequestConflict.
const (
	// RequestConflictNotPending: the request has an outcome already.
	RequestConflictNotPending CustomEmojiRequestConflictReason = "not_pending"
	// RequestConflictRequesterCannotCreate: the requester no longer may create custom emoji.
	RequestConflictRequesterCannotCreate CustomEmojiRequestConflictReason = "requester_cannot_create"
	// RequestConflictShortcodeTaken: a local emoji has the shortcode of the request to create.
	RequestConflictShortcodeTaken CustomEmojiRequestConflictReason = "shortcode_taken"
)

// CustomEmojiRequestConflict reports a request that cannot be approved, rejected, or withdrawn
// as it is (a 409, ADR-0070 §2): the approver rejects it instead.
type CustomEmojiRequestConflict struct {
	Reason    CustomEmojiRequestConflictReason
	RequestID string
}

func (e *CustomEmojiRequestConflict) Error() string {
	return fmt.Sprintf("content: custom emoji request %s: %s", e.RequestID, e.Reason)
}

// CustomEmojiRequestRepository is the persistence port of the requests, part of
// CustomEmojiRepository (data-model §4.4). Lookups that find nothing return an error wrapping
// ErrNotFound.
type CustomEmojiRequestRepository interface {
	// InsertCustomEmojiRequest inserts a request with its pending row. A request to create fails
	// with ErrShortcodeTaken when a local emoji has its shortcode or a pending request reserves
	// it; a request to replace an image fails with ErrCustomEmojiRequestPending when the emoji
	// has a pending one.
	InsertCustomEmojiRequest(ctx context.Context, tx uow.Tx, r content.CustomEmojiRequest) error
	// CloseCustomEmojiRequest records the outcome of a pending request and deletes its pending
	// row; it fails with ErrCustomEmojiRequestNotPending for a request that is not pending.
	CloseCustomEmojiRequest(ctx context.Context, tx uow.Tx, o content.CustomEmojiRequestOutcome) error
	// PendingImageRequestOf returns, in tx, the pending request to replace the image of an
	// emoji, "" for none.
	PendingImageRequestOf(ctx context.Context, tx uow.Tx, emojiID string) (string, error)
	FindCustomEmojiRequest(ctx context.Context, id string) (content.CustomEmojiRequestRecord, error)
	// ListCustomEmojiRequests returns up to limit requests matching q, newest first.
	ListCustomEmojiRequests(ctx context.Context, q CustomEmojiRequestQuery, limit int) ([]content.CustomEmojiRequestRecord, error)
	// ListCustomEmojiRequestsByID returns, in ID order, the requests among ids that exist.
	ListCustomEmojiRequestsByID(ctx context.Context, ids []string) ([]content.CustomEmojiRequestRecord, error)
	// CountCustomEmojiUsage counts the local emoji that a user made and, apart, their pending
	// requests to create one (ADR-0070 §4).
	CountCustomEmojiUsage(ctx context.Context, userID string) (made, pending int, err error)
	// IsMediaOfPendingCustomEmojiRequest reports whether a pending request uses a media file.
	IsMediaOfPendingCustomEmojiRequest(ctx context.Context, mediaID string) (bool, error)
}

// CustomEmojiRequestQuery selects requests. Zero fields select every request.
type CustomEmojiRequestQuery struct {
	// RequesterUserID keeps the requests of one user.
	RequesterUserID string
	// State keeps the requests in one state.
	State content.CustomEmojiRequestState
	// Before is the ID of the last request of the previous page.
	Before string
}

// CustomEmojiCreators tells whether users may create custom emoji (the identity and account
// modules, through the composition root).
type CustomEmojiCreators interface {
	// MayCreate reports whether a user may create custom emoji now: their roles grant
	// create_custom_emojis and they may use the app (not suspended, not deleted).
	MayCreate(ctx context.Context, userID string) (bool, error)
	// AccountOf returns the local account of a user, "" when it has none.
	AccountOf(ctx context.Context, userID string) (string, error)
}

// CustomEmojiQuota is the limit of the custom emoji that a creator makes (ADR-0070 §4): the
// supporter plans set it (ADR-0072 §11). limited is false for no limit.
type CustomEmojiQuota interface {
	Limit(ctx context.Context, userID string) (limit int, limited bool, err error)
}

// NoCustomEmojiQuota limits nobody: a role that no plan manages, or a server whose supporter
// program is disabled, counts neither the emoji nor the pending requests.
type NoCustomEmojiQuota struct{}

// Limit implements CustomEmojiQuota.
func (NoCustomEmojiQuota) Limit(context.Context, string) (int, bool, error) { return 0, false, nil }

// PendingCustomEmojiRequestMedia returns the MediaReferrer that keeps the uploads of the pending
// requests from the cleanup of the unattached uploads; an upload goes back to unattached once its
// request ends.
func PendingCustomEmojiRequestMedia(repo CustomEmojiRequestRepository) MediaReferrer {
	return pendingRequestMedia{repo}
}

type pendingRequestMedia struct{ repo CustomEmojiRequestRepository }

func (p pendingRequestMedia) IsMediaReferenced(ctx context.Context, mediaID string) (bool, error) {
	return p.repo.IsMediaOfPendingCustomEmojiRequest(ctx, mediaID)
}

// Audited operations on the requests (ADR-0070 §8), named as the audit log names them.
const (
	CustomEmojiRequestSubmitted = "custom_emoji_request_submitted"
	CustomEmojiRequestWithdrawn = "custom_emoji_request_withdrawn"
	CustomEmojiRequestApproved  = "custom_emoji_request_approved"
	CustomEmojiRequestRejected  = "custom_emoji_request_rejected"
)

// EventCustomEmojiRequestRecorded is the event type of CustomEmojiRequestRecorded.
const EventCustomEmojiRequestRecorded = "content.custom_emoji_request_recorded"

// CustomEmojiRequestRecorded is emitted, in the transaction of the operation, when a request is
// submitted, withdrawn, approved, or rejected (not when it lapses). The moderation module writes
// its audit log entry from it, and the notification module notifies the holders of
// manage_custom_emojis of a submission and the requester of a decision (ADR-0070 §9).
type CustomEmojiRequestRecorded struct {
	// AuditID is the ID of the audit log entry, generated before the transaction.
	AuditID string
	// Action is one of CustomEmojiRequestSubmitted and the other operations.
	Action  string
	Request content.CustomEmojiRequest
	// Outcome is the outcome that the operation recorded, nil for a submission.
	Outcome *content.CustomEmojiRequestOutcome
	// RequesterAccountID is the local account of the requester, "" when it has none.
	RequesterAccountID string
	// ByUserID is who did it: the requester, the approver, or "" for the CLI.
	ByUserID string
	At       time.Time
}

// EventType implements uow.Event.
func (CustomEmojiRequestRecorded) EventType() string { return EventCustomEmojiRequestRecorded }

// CustomEmojiRequestView is a request with the image of its upload, nil when it cannot be shown
// (once the request ended, the upload is deleted with the unattached uploads).
type CustomEmojiRequestView struct {
	content.CustomEmojiRequestRecord
	View *CustomEmojiView
}

// SubmitCustomEmojiRequestInput describes a new request.
type SubmitCustomEmojiRequestInput struct {
	Kind content.CustomEmojiRequestKind
	// Shortcode is the shortcode of the new emoji of a request to create.
	Shortcode string
	// EmojiID is the emoji whose image a request to replace an image replaces.
	EmojiID string
	// MediaID is an upload of RequesterAccountID that finished processing.
	MediaID            string
	RequesterUserID    string
	RequesterAccountID string
}

// SubmitRequest records a request (ADR-0070 §2, §3), which notifies the holders of
// manage_custom_emojis and is audited. It fails with ErrNotCustomEmojiCreator when the user may
// not create custom emoji, with ErrNotFound for a request to replace the image of an emoji that
// is not a local one the user made, and with a *content.CustomEmojiError for a shortcode or an
// image that breaks the rules of ADR-0051, a shortcode that a local emoji has or a pending
// request reserves (content.EmojiShortcodeTaken), a request to create past the user's limit
// (content.EmojiLimitReached), and a second pending request to replace the image of an emoji
// (content.EmojiRequestPending).
func (s *CustomEmojiService) SubmitRequest(ctx context.Context, in SubmitCustomEmojiRequestInput) (CustomEmojiRequestView, error) {
	if err := s.requireCreator(ctx, in.RequesterUserID); err != nil {
		return CustomEmojiRequestView{}, err
	}
	req := content.CustomEmojiRequest{
		ID: s.d.IDs.NewID(), Kind: in.Kind, RequesterUserID: in.RequesterUserID, MediaID: in.MediaID, CreatedAt: s.d.Clock.Now(),
	}
	switch in.Kind {
	case content.RequestCreate:
		if err := s.checkNewShortcode(ctx, in); err != nil {
			return CustomEmojiRequestView{}, err
		}
		req.Shortcode = in.Shortcode
	case content.RequestImage:
		r, err := s.ownRecord(ctx, in.EmojiID, in.RequesterUserID)
		if err != nil {
			return CustomEmojiRequestView{}, err
		}
		req.EmojiID, req.Shortcode = r.Emoji.ID, r.Emoji.Shortcode
	default:
		return CustomEmojiRequestView{}, fmt.Errorf("content: unknown custom emoji request kind %q", in.Kind)
	}
	if err := s.checkImage(ctx, in.RequesterAccountID, in.MediaID); err != nil {
		return CustomEmojiRequestView{}, err
	}
	event := CustomEmojiRequestRecorded{
		AuditID: s.d.IDs.NewID(), Action: CustomEmojiRequestSubmitted, Request: req, RequesterAccountID: in.RequesterAccountID,
		ByUserID: in.RequesterUserID, At: req.CreatedAt,
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertCustomEmojiRequest(ctx, tx, req); err != nil {
			return err
		}
		return tx.Emit(ctx, event)
	})
	switch {
	case errors.Is(err, ErrShortcodeTaken):
		return CustomEmojiRequestView{}, &content.CustomEmojiError{Reason: content.EmojiShortcodeTaken, Value: req.Shortcode}
	case errors.Is(err, ErrCustomEmojiRequestPending):
		return CustomEmojiRequestView{}, &content.CustomEmojiError{Reason: content.EmojiRequestPending, Value: req.EmojiID}
	case err != nil:
		return CustomEmojiRequestView{}, fmt.Errorf("content: submit custom emoji request :%s:: %w", req.Shortcode, err)
	}
	return s.GetRequest(ctx, req.ID)
}

// checkNewShortcode checks the shortcode of a request to create and the limit of its requester.
func (s *CustomEmojiService) checkNewShortcode(ctx context.Context, in SubmitCustomEmojiRequestInput) error {
	if !content.ValidLocalShortcode(in.Shortcode) {
		return &content.CustomEmojiError{Reason: content.EmojiInvalidShortcode, Value: in.Shortcode}
	}
	limit, limited, err := s.d.Quota.Limit(ctx, in.RequesterUserID)
	if err != nil {
		return fmt.Errorf("content: the custom emoji limit of %s: %w", in.RequesterUserID, err)
	}
	if limited {
		made, pending, err := s.d.Repository.CountCustomEmojiUsage(ctx, in.RequesterUserID)
		if err != nil {
			return err
		}
		if made+pending >= limit {
			return &content.CustomEmojiError{Reason: content.EmojiLimitReached, Value: in.RequesterUserID}
		}
	}
	// A taken shortcode is refused before the image is checked; the insertion checks it again.
	taken, err := s.localShortcodeTaken(ctx, in.Shortcode)
	if err != nil {
		return err
	}
	if taken {
		return &content.CustomEmojiError{Reason: content.EmojiShortcodeTaken, Value: in.Shortcode}
	}
	return nil
}

// localShortcodeTaken reports whether a local emoji has shortcode.
func (s *CustomEmojiService) localShortcodeTaken(ctx context.Context, shortcode string) (bool, error) {
	taken, err := s.d.Repository.ListLocalCustomEmojis(ctx, []string{shortcode})
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(taken, func(t content.CustomEmojiRecord) bool { return t.Emoji.Shortcode == shortcode }), nil
}

// requireCreator fails with ErrNotCustomEmojiCreator unless userID may create custom emoji now.
func (s *CustomEmojiService) requireCreator(ctx context.Context, userID string) error {
	ok, err := s.mayCreate(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotCustomEmojiCreator
	}
	return nil
}

func (s *CustomEmojiService) mayCreate(ctx context.Context, userID string) (bool, error) {
	if s.d.Creators == nil || userID == "" {
		return false, nil
	}
	ok, err := s.d.Creators.MayCreate(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("content: whether %s may create custom emoji: %w", userID, err)
	}
	return ok, nil
}

// ownRecord returns the local emoji id that userID made, and fails with ErrNotFound for another
// emoji, so that a creator learns nothing of the emoji of others through these operations.
func (s *CustomEmojiService) ownRecord(ctx context.Context, id, userID string) (content.CustomEmojiRecord, error) {
	r, err := s.d.Repository.FindCustomEmojiRecord(ctx, id)
	if err != nil {
		return content.CustomEmojiRecord{}, err
	}
	if !r.Emoji.Local() || userID == "" || r.Emoji.CreatedByUserID != userID {
		return content.CustomEmojiRecord{}, fmt.Errorf("%w: custom emoji %s of another user", ErrNotFound, id)
	}
	return r, nil
}

// WithdrawRequest withdraws a pending request of userID, which is audited. It fails with
// ErrNotFound for an unknown request or one of another user, and with a
// *CustomEmojiRequestConflict for one that is not pending.
func (s *CustomEmojiService) WithdrawRequest(ctx context.Context, id, userID string) (CustomEmojiRequestView, error) {
	rec, err := s.d.Repository.FindCustomEmojiRequest(ctx, id)
	if err != nil {
		return CustomEmojiRequestView{}, err
	}
	if rec.Request.RequesterUserID != userID {
		return CustomEmojiRequestView{}, fmt.Errorf("%w: custom emoji request %s of another user", ErrNotFound, id)
	}
	o := content.CustomEmojiRequestOutcome{Outcome: content.RequestWithdrawn, DecidedByUserID: userID}
	if err := s.close(ctx, rec, o, CustomEmojiRequestWithdrawn, nil); err != nil {
		return CustomEmojiRequestView{}, err
	}
	return s.GetRequest(ctx, id)
}

// RejectRequest rejects a pending request with an optional reason that the requester sees, which
// is audited and notified to the requester. It fails with ErrNotFound for an unknown request, a
// *content.CustomEmojiError (content.EmojiInvalidReason) for a reason that breaks the rules, and
// a *CustomEmojiRequestConflict for a request that is not pending. byUserID is "" for the CLI.
func (s *CustomEmojiService) RejectRequest(ctx context.Context, id, reason, byUserID string) (CustomEmojiRequestView, error) {
	reason, err := content.NormalizeCustomEmojiRejectReason(reason)
	if err != nil {
		return CustomEmojiRequestView{}, err
	}
	rec, err := s.d.Repository.FindCustomEmojiRequest(ctx, id)
	if err != nil {
		return CustomEmojiRequestView{}, err
	}
	o := content.CustomEmojiRequestOutcome{Outcome: content.RequestRejected, DecidedByUserID: byUserID, Reason: reason}
	if err := s.close(ctx, rec, o, CustomEmojiRequestRejected, nil); err != nil {
		return CustomEmojiRequestView{}, err
	}
	return s.GetRequest(ctx, id)
}

// close records the outcome o of the pending request rec as the operation action, with the
// writes of then in the same transaction.
func (s *CustomEmojiService) close(ctx context.Context, rec content.CustomEmojiRequestRecord, o content.CustomEmojiRequestOutcome, action string,
	then func(tx uow.Tx) error,
) error {
	conflict := &CustomEmojiRequestConflict{Reason: RequestConflictNotPending, RequestID: rec.Request.ID}
	if rec.Outcome != nil {
		return conflict
	}
	account, err := s.requesterAccount(ctx, rec.Request.RequesterUserID)
	if err != nil {
		return err
	}
	o.ID, o.RequestID, o.DecidedAt = s.d.IDs.NewID(), rec.Request.ID, s.d.Clock.Now()
	event := CustomEmojiRequestRecorded{
		AuditID: s.d.IDs.NewID(), Action: action, Request: rec.Request, Outcome: &o, RequesterAccountID: account,
		ByUserID: o.DecidedByUserID, At: o.DecidedAt,
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.CloseCustomEmojiRequest(ctx, tx, o); err != nil {
			return err
		}
		if then != nil {
			if err := then(tx); err != nil {
				return err
			}
		}
		return tx.Emit(ctx, event)
	})
	switch {
	case errors.Is(err, ErrCustomEmojiRequestNotPending):
		return conflict
	case errors.Is(err, ErrShortcodeTaken):
		return &CustomEmojiRequestConflict{Reason: RequestConflictShortcodeTaken, RequestID: rec.Request.ID}
	case err != nil:
		return fmt.Errorf("content: %s %s: %w", action, rec.Request.ID, err)
	}
	return nil
}

func (s *CustomEmojiService) requesterAccount(ctx context.Context, userID string) (string, error) {
	if s.d.Creators == nil {
		return "", nil
	}
	id, err := s.d.Creators.AccountOf(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("content: the account of %s: %w", userID, err)
	}
	return id, nil
}

// ApproveRequest approves a pending request (ADR-0070 §2, §3): a request to create makes the
// local emoji, without a category, in the picker, and enabled, whose creator is the requester; a
// request to replace an image gives the emoji the new image. The image is a copy of the
// requester's upload as an upload of the instance actor, so that it outlives the requester's
// account; the requester's upload goes back to unattached. The approval and the emoji's operation
// are audited as byUserID's ("" for the CLI), and the requester is notified. It fails with
// ErrNotFound for an unknown request, a *CustomEmojiRequestConflict for a request that is not
// pending, whose requester no longer may create custom emoji, or whose shortcode a local emoji
// has, and a *content.CustomEmojiError for an image that no longer qualifies.
func (s *CustomEmojiService) ApproveRequest(ctx context.Context, id, byUserID string) (CustomEmojiRequestView, error) {
	rec, err := s.d.Repository.FindCustomEmojiRequest(ctx, id)
	if err != nil {
		return CustomEmojiRequestView{}, err
	}
	req := rec.Request
	if rec.Outcome != nil {
		return CustomEmojiRequestView{}, &CustomEmojiRequestConflict{Reason: RequestConflictNotPending, RequestID: id}
	}
	ok, err := s.mayCreate(ctx, req.RequesterUserID)
	if err != nil {
		return CustomEmojiRequestView{}, err
	}
	if !ok {
		return CustomEmojiRequestView{}, &CustomEmojiRequestConflict{Reason: RequestConflictRequesterCannotCreate, RequestID: id}
	}
	var target content.CustomEmojiRecord
	if req.Kind == content.RequestCreate {
		taken, err := s.localShortcodeTaken(ctx, req.Shortcode)
		if err != nil {
			return CustomEmojiRequestView{}, err
		}
		if taken {
			return CustomEmojiRequestView{}, &CustomEmojiRequestConflict{Reason: RequestConflictShortcodeTaken, RequestID: id}
		}
	} else if target, err = s.d.Repository.FindCustomEmojiRecord(ctx, req.EmojiID); errors.Is(err, ErrNotFound) {
		// The emoji was deleted, which lapsed the request.
		return CustomEmojiRequestView{}, &CustomEmojiRequestConflict{Reason: RequestConflictNotPending, RequestID: id}
	} else if err != nil {
		return CustomEmojiRequestView{}, err
	}
	mediaID, err := s.copyRequestImage(ctx, req.MediaID)
	if err != nil {
		return CustomEmojiRequestView{}, err
	}
	err = s.approve(ctx, rec, target, mediaID, byUserID)
	if err != nil {
		// Nothing of a failed approval stays: the copy goes with its blobs.
		if _, derr := s.d.Media.deleteUnattached(context.WithoutCancel(ctx), mediaID); derr != nil {
			err = errors.Join(err, derr)
		}
		return CustomEmojiRequestView{}, err
	}
	return s.GetRequest(ctx, id)
}

// approve records the approval of rec with the image mediaID: the new emoji, or the new image of
// target.
func (s *CustomEmojiService) approve(ctx context.Context, rec content.CustomEmojiRequestRecord, target content.CustomEmojiRecord, mediaID, byUserID string) error {
	req := rec.Request
	now := s.d.Clock.Now()
	e := target.Emoji
	action := CustomEmojiImageReplaced
	if req.Kind == content.RequestCreate {
		e = content.CustomEmoji{ID: s.d.IDs.NewID(), Shortcode: req.Shortcode, CreatedByUserID: req.RequesterUserID, CreatedAt: now}
		action = CustomEmojiCreated
	}
	img := content.CustomEmojiImage{ID: s.d.IDs.NewID(), EmojiID: e.ID, MediaID: mediaID, RevisedAt: now}
	administered := s.event(action, e, img.ID, byUserID, now)
	o := content.CustomEmojiRequestOutcome{Outcome: content.RequestApproved, DecidedByUserID: byUserID, EmojiID: e.ID}
	return s.close(ctx, rec, o, CustomEmojiRequestApproved, func(tx uow.Tx) error {
		if req.Kind == content.RequestCreate {
			if err := s.d.Repository.InsertCustomEmoji(ctx, tx, e); err != nil {
				return err
			}
		}
		if err := s.d.Repository.InsertCustomEmojiImage(ctx, tx, img); err != nil {
			return err
		}
		return tx.Emit(ctx, administered)
	})
}

// copyRequestImage copies the processed image of the upload mediaID of a request into a new
// upload of the instance actor with the emoji purpose, processed and checked as the image of a
// new emoji, and returns its ID.
func (s *CustomEmojiService) copyRequestImage(ctx context.Context, mediaID string) (string, error) {
	if s.d.InstanceAccount == nil {
		return "", errors.New("content: no instance actor to own the images of the approved custom emoji")
	}
	owner, err := s.d.InstanceAccount(ctx)
	if err != nil {
		return "", fmt.Errorf("content: the instance actor: %w", err)
	}
	m, err := s.d.Media.Media(ctx, mediaID)
	if errors.Is(err, ErrNotFound) || err == nil && (m.Upload == nil || !m.Ready()) {
		return "", &content.CustomEmojiError{Reason: content.EmojiMediaNotFound, Value: mediaID}
	}
	if err != nil {
		return "", err
	}
	// The processing result is copied: the original of a processed upload is gone.
	data, err := s.d.Media.readBlob(ctx, sourceVariant(*m.Result).BlobKey, s.d.Media.MaxUploadBytes())
	if err != nil {
		return "", &content.CustomEmojiError{Reason: content.EmojiMediaNotFound, Value: mediaID}
	}
	cp, err := s.d.Media.Upload(ctx, UploadInput{OwnerAccountID: owner, Content: bytes.NewReader(data), Purpose: content.PurposeEmoji})
	if err != nil {
		return "", fmt.Errorf("content: copy the image %s: %w", mediaID, err)
	}
	if res := s.d.Media.Process(ctx, cp.File.ID, content.PurposeEmoji); !res.IsDone() {
		err = fmt.Errorf("content: process the copy of the image %s: %v", mediaID, res)
	} else {
		err = s.checkImage(ctx, owner, cp.File.ID)
	}
	if err != nil {
		if _, derr := s.d.Media.deleteUnattached(context.WithoutCancel(ctx), cp.File.ID); derr != nil {
			err = errors.Join(err, derr)
		}
		return "", err
	}
	return cp.File.ID, nil
}

// GetRequest returns a request with its image. It fails with ErrNotFound for an unknown ID.
func (s *CustomEmojiService) GetRequest(ctx context.Context, id string) (CustomEmojiRequestView, error) {
	rec, err := s.d.Repository.FindCustomEmojiRequest(ctx, id)
	if err != nil {
		return CustomEmojiRequestView{}, err
	}
	out, err := s.presentRequests(ctx, []content.CustomEmojiRequestRecord{rec})
	if err != nil {
		return CustomEmojiRequestView{}, err
	}
	return out[0], nil
}

// ListRequests returns up to limit requests matching q, newest first, with their images.
func (s *CustomEmojiService) ListRequests(ctx context.Context, q CustomEmojiRequestQuery, limit int) ([]CustomEmojiRequestView, error) {
	recs, err := s.d.Repository.ListCustomEmojiRequests(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	return s.presentRequests(ctx, recs)
}

// RequestsByID returns, in ID order, the requests among ids that exist, with their images, in
// one lookup (the notifications).
func (s *CustomEmojiService) RequestsByID(ctx context.Context, ids []string) ([]CustomEmojiRequestView, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	recs, err := s.d.Repository.ListCustomEmojiRequestsByID(ctx, sortedSet(slices.Clone(ids)))
	if err != nil {
		return nil, err
	}
	return s.presentRequests(ctx, recs)
}

// ExistingRequestIDs returns, in ID order, the requests among ids that exist (the audit log).
func (s *CustomEmojiService) ExistingRequestIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	recs, err := s.d.Repository.ListCustomEmojiRequestsByID(ctx, sortedSet(slices.Clone(ids)))
	if err != nil {
		return nil, err
	}
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.Request.ID
	}
	return out, nil
}

// presentRequests presents the images of the uploads of recs as the images of local emoji.
func (s *CustomEmojiService) presentRequests(ctx context.Context, recs []content.CustomEmojiRequestRecord) ([]CustomEmojiRequestView, error) {
	current := make([]content.CurrentCustomEmoji, len(recs))
	for i, r := range recs {
		current[i] = content.CurrentCustomEmoji{
			Emoji: content.CustomEmoji{ID: r.Request.ID, Shortcode: r.Request.Shortcode},
			Image: content.CustomEmojiImage{MediaID: r.Request.MediaID},
		}
	}
	views, err := s.d.Media.customEmojiViews(ctx, s.d.Media.reads(), current)
	if err != nil {
		return nil, err
	}
	out := make([]CustomEmojiRequestView, len(recs))
	for i, r := range recs {
		out[i] = CustomEmojiRequestView{CustomEmojiRequestRecord: r}
		if v, ok := views[r.Request.ID]; ok {
			out[i].View = &v
		}
	}
	return out, nil
}

// CustomEmojiUsage is what a creator has used of their limit (ADR-0070 §4): the local emoji they
// made and their pending requests to create one, and the limit, when Limited. Pending is the part
// of Used that the pending requests take.
type CustomEmojiUsage struct {
	Used    int
	Pending int
	Limit   int
	Limited bool
}

// Usage returns what userID has used of their limit.
func (s *CustomEmojiService) Usage(ctx context.Context, userID string) (CustomEmojiUsage, error) {
	limit, limited, err := s.d.Quota.Limit(ctx, userID)
	if err != nil {
		return CustomEmojiUsage{}, fmt.Errorf("content: the custom emoji limit of %s: %w", userID, err)
	}
	made, pending, err := s.d.Repository.CountCustomEmojiUsage(ctx, userID)
	if err != nil {
		return CustomEmojiUsage{}, err
	}
	return CustomEmojiUsage{Used: made + pending, Pending: pending, Limit: limit, Limited: limited}, nil
}

// ListOwn returns up to limit local emoji that userID made, newest first, with their images.
func (s *CustomEmojiService) ListOwn(ctx context.Context, userID, before string, limit int) ([]AdminCustomEmoji, error) {
	if userID == "" {
		return nil, nil
	}
	return s.List(ctx, CustomEmojiQuery{Locality: LocalEmoji, CreatedBy: userID, Before: before}, limit)
}

// OwnCustomEmojiUpdate is what a creator changes of an emoji they made; nil fields keep their
// value. A creator neither enables nor disables an emoji (ADR-0070 §3).
type OwnCustomEmojiUpdate struct {
	VisibleInPicker *bool
	Category        *string
	Aliases         *[]string
}

// UpdateOwn changes an emoji that userID made, as Update does, audited as userID's. It fails with
// ErrNotFound for an emoji that is not a local one the user made, and with
// ErrNotCustomEmojiCreator when the user no longer may create custom emoji.
func (s *CustomEmojiService) UpdateOwn(ctx context.Context, id, userID string, u OwnCustomEmojiUpdate) (AdminCustomEmoji, error) {
	if _, err := s.ownRecord(ctx, id, userID); err != nil {
		return AdminCustomEmoji{}, err
	}
	if err := s.requireCreator(ctx, userID); err != nil {
		return AdminCustomEmoji{}, err
	}
	return s.Update(ctx, id, CustomEmojiUpdate{VisibleInPicker: u.VisibleInPicker, Category: u.Category, Aliases: u.Aliases, ByUserID: userID})
}

// DeleteOwn deletes an emoji that userID made, as Delete does, audited as userID's. It fails as
// UpdateOwn does.
func (s *CustomEmojiService) DeleteOwn(ctx context.Context, id, userID string) error {
	r, err := s.ownRecord(ctx, id, userID)
	if err != nil {
		return err
	}
	if err := s.requireCreator(ctx, userID); err != nil {
		return err
	}
	return s.delete(ctx, r, userID)
}
