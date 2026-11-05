// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// MediaReview is the hook media.check (ADR-0147 §2): the plugin is asked about an upload when it is
// processed, before its result is ready, and the answer acts where the file is used. A reject
// leaves the file failed, with nothing of it kept. A flag of an avatar, a header, or an emoji files
// its report when the file is processed; a post acts on a flag and a mark_sensitive of its
// attachments in the transaction that stores it. The zero value asks nothing.
type MediaReview struct {
	Gate apphook.Gate
	// Reports files the report of a flag decision.
	Reports HookReports
	// InstanceActor returns the ID of the instance actor, the reporter of a flag decision.
	InstanceActor func(ctx context.Context) (string, error)
}

// HookReports files the report of a flag decision of a hook inside the caller's transaction. The
// composition root implements it on the moderation module, which reads this module.
type HookReports interface {
	FileHookReport(ctx context.Context, tx uow.Tx, in HookReport) (bool, error)
}

// HookReport is the report of a flag decision: one per decision, with the instance actor as the
// reporter (ADR-0147 §7).
type HookReport struct {
	ReporterAccountID string
	TargetAccountID   string
	// StatusIDs are the target's posts the decision is about; none for an avatar, a header, or an emoji.
	StatusIDs  []string
	DecisionID string
	Hook       string
	ReasonCode string
	// Comment is the plugin's note, "" for none.
	Comment string
}

// MaxReviewImage bounds the image media.check sends. The smallest variant is far below it; one
// that is not goes without its image, so that the call stays within the message limit of a hook.
const MaxReviewImage = 512 << 10

// mediaVerdict is what a decision of media.check does where the file is used.
type mediaVerdict struct {
	apphook.Result
	// accountID is the owner of the file, the target of a report.
	accountID string
	// reporterAccountID is the instance actor when the verdict files a report: it is created on
	// first use, with keys and its own transaction, so it is settled before the transaction starts.
	reporterAccountID string
}

// active reports whether media.check asks about anything. While it does not, processing reads
// nothing more than it would without the hook.
func (r *MediaReview) active(ctx context.Context) (bool, error) {
	if r == nil || r.Gate == nil {
		return false, nil
	}
	return r.Gate.Active(ctx, apphook.MediaCheck)
}

// review asks media.check about an upload whose processing ended ready, before its variants are
// stored: the first processing asks the plugin, and processing it for another purpose reads the
// decision recorded then. It sends the smallest still variant, never the file as received. It fails
// with an error that the job returns as it is, apphook.ErrRetryLater included.
func (s *MediaService) review(ctx context.Context, first bool, result content.MediaProcessingResult, variants []sizedVariant) (mediaVerdict, error) {
	allow := mediaVerdict{Result: apphook.Result{Effect: apphook.Allow}}
	r := s.d.Review
	if active, err := r.active(ctx); err != nil || !active {
		return allow, err
	}
	file, err := s.d.Repository.FindMediaFile(ctx, result.MediaID)
	if err != nil {
		return allow, err
	}
	var res apphook.Result
	if first {
		res, err = r.Gate.Decide(ctx, apphook.Call{
			Hook: apphook.MediaCheck, SubjectKey: result.MediaID, AccountID: file.OwnerAccountID, Subject: mediaSubject(result, variants),
		})
	} else {
		res, err = r.Gate.Recorded(ctx, apphook.MediaCheck, result.MediaID)
	}
	if err != nil {
		return allow, err
	}
	v := mediaVerdict{Result: res, accountID: file.OwnerAccountID}
	if !first && res.Effect == apphook.Reject {
		// The file was let through when it was first processed; a decision cannot take it back.
		v.Effect = apphook.Allow
	}
	if v.Effect == apphook.Flag && result.Purpose != content.PurposeAttachment {
		if v.reporterAccountID, err = r.InstanceActor(ctx); err != nil {
			return allow, fmt.Errorf("content: the instance actor: %w", err)
		}
	}
	return v, nil
}

// mediaSubject is what media.check sends of a processed upload.
func mediaSubject(result content.MediaProcessingResult, variants []sizedVariant) apphook.MediaSubject {
	s := apphook.MediaSubject{
		Purpose: string(result.Purpose), MediaType: string(result.MediaType), Width: result.Width, Height: result.Height,
		Duration: time.Duration(result.DurationMS) * time.Millisecond, Animated: result.MediaType == content.MediaTypeVideo,
	}
	for _, sv := range variants {
		switch {
		case sv.motion == content.MotionAnimated && result.MediaType == content.MediaTypeImage:
			s.Animated = true
		case sv.motion == content.MotionStill && sv.size == content.SizeSmall && len(sv.v.Data) <= MaxReviewImage:
			s.Image, s.ImageContentType, s.ImageWidth, s.ImageHeight = sv.v.Data, sv.v.ContentType, sv.v.Width, sv.v.Height
		}
	}
	return s
}

// applyReview acts inside the transaction that records the result: a flag of an avatar, a header,
// or an emoji files its report there, as no post will. A mark_sensitive of those is only recorded:
// there is no post to mark.
func (s *MediaService) applyReview(ctx context.Context, tx uow.Tx, v mediaVerdict, purpose content.MediaPurpose) error {
	if v.Effect != apphook.Flag || purpose == content.PurposeAttachment {
		return nil
	}
	filed, err := s.d.Review.Reports.FileHookReport(ctx, tx, HookReport{
		ReporterAccountID: v.reporterAccountID, TargetAccountID: v.accountID,
		DecisionID: v.DecisionID, Hook: string(v.Hook), ReasonCode: string(v.Reason), Comment: v.Note,
	})
	if err != nil || !filed {
		return err
	}
	return s.d.Review.Gate.Applied(ctx, tx, v.Result)
}

// rejectedResult is what a reject of media.check leaves of a processing result: a failure, with
// no variant, the same as content the processor refuses.
func (s *MediaService) rejectedResult(ctx context.Context, result content.MediaProcessingResult) content.MediaProcessingResult {
	s.d.Logger.InfoContext(ctx, "media rejected by the hook media.check", slog.String("media_id", result.MediaID))
	return content.MediaProcessingResult{
		ID: result.ID, MediaID: result.MediaID, Purpose: result.Purpose, MediaType: content.MediaTypeUnknown,
		Outcome: content.MediaFailed, ProcessedAt: result.ProcessedAt,
	}
}

// attachmentVerdicts reads the decisions of media.check on the files a post attaches, leaving out
// those in kept, which an earlier revision of the post attached and acted on. It reads nothing while
// the hook has no plugin. It runs before the transaction.
func (s *Service) attachmentVerdicts(ctx context.Context, authorID string, attachments []content.Attachment, kept []content.Attachment) ([]mediaVerdict, error) {
	r := s.d.MediaReview
	if r == nil || r.Gate == nil || len(attachments) == 0 || !r.Gate.Configured(apphook.MediaCheck) {
		return nil, nil
	}
	var out []mediaVerdict
	reporter := ""
	for _, a := range attachments {
		if attachedIn(kept, a.MediaID) {
			continue
		}
		res, err := r.Gate.Recorded(ctx, apphook.MediaCheck, a.MediaID)
		if err != nil {
			return nil, err
		}
		v := mediaVerdict{Result: res, accountID: authorID}
		switch res.Effect {
		case apphook.Flag:
			if reporter == "" {
				if reporter, err = r.InstanceActor(ctx); err != nil {
					return nil, fmt.Errorf("content: the instance actor: %w", err)
				}
			}
			v.reporterAccountID = reporter
		case apphook.MarkSensitive:
		default:
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

func attachedIn(attachments []content.Attachment, mediaID string) bool {
	for _, a := range attachments {
		if a.MediaID == mediaID {
			return true
		}
	}
	return false
}

// applyAttachmentVerdicts acts inside the transaction that stores a post on the decisions of its
// attachments: the report of a flag and the mark of a mark_sensitive (ADR-0147 §8), with the link
// and the audit log entry of either.
func (s *Service) applyAttachmentVerdicts(ctx context.Context, tx uow.Tx, statusID string, verdicts []mediaVerdict) error {
	for _, v := range verdicts {
		gate := s.d.MediaReview.Gate
		switch v.Effect {
		case apphook.Flag:
			filed, err := s.d.MediaReview.Reports.FileHookReport(ctx, tx, HookReport{
				ReporterAccountID: v.reporterAccountID, TargetAccountID: v.accountID, StatusIDs: []string{statusID},
				DecisionID: v.DecisionID, Hook: string(v.Hook), ReasonCode: string(v.Reason), Comment: v.Note,
			})
			if err != nil {
				return err
			}
			if !filed {
				continue
			}
		case apphook.MarkSensitive:
			if err := s.MarkSensitiveByHook(ctx, tx, statusID, v.DecisionID); err != nil {
				return err
			}
		default:
			continue
		}
		if err := gate.LinkStatus(ctx, tx, v.DecisionID, statusID); err != nil {
			return err
		}
		if err := gate.Applied(ctx, tx, v.Result); err != nil {
			return err
		}
	}
	return nil
}

// UndoMediaReview clears the mark of a cancelled mark_sensitive decision of media.check inside the
// transaction of its cancellation (ADR-0147 §9). A report stays, and a rejected file does not come
// back: nothing of it was kept.
func (s *Service) UndoMediaReview(ctx context.Context, tx uow.Tx, r apphook.Record) error {
	if r.Decision.Outcome != apphook.MarkSensitive {
		return nil
	}
	return s.ClearSensitivityOfHookDecision(ctx, tx, r.Decision.ID)
}
