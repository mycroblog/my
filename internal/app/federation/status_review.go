// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	apphook "github.com/mycroblog/my/internal/app/hook"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// HookSensitivity is what the content module records of a mark_sensitive decision;
// *appcontent.Service implements it.
type HookSensitivity interface {
	MarkSensitiveByHook(ctx context.Context, tx uow.Tx, statusID, decisionID string) error
	ClearSensitivityOfHookDecision(ctx context.Context, tx uow.Tx, decisionID string) error
}

// HookReports files the report of a flag decision; *appmoderation.Service implements it.
type HookReports interface {
	FileHookReport(ctx context.Context, tx uow.Tx, in appmoderation.HookReportInput) (bool, error)
}

// ReviewInstanceActor returns the instance actor, the reporter of a flag decision;
// *appaccount.Service implements it.
type ReviewInstanceActor interface {
	EnsureInstanceActor(ctx context.Context) (appaccount.Account, error)
}

// ReviewSettings reads the instance settings of the review of received posts at each use.
type ReviewSettings interface {
	// ReviewUnfollowedDirect reports whether the plugin of inbound.status also reads the direct posts
	// of authors no local account follows (ADR-0147 §10).
	ReviewUnfollowedDirect(ctx context.Context) (bool, error)
}

// StatusReview is the hook inbound.status on the Create and Update of a post (ADR-0147 §3): the
// plugin is asked outside the transaction, about a post in scope only, and the transaction that
// stores the post acts on the verdict. The zero value asks nothing.
type StatusReview struct {
	Gate        apphook.Gate
	Sensitivity HookSensitivity
	Reports     HookReports
	Instance    ReviewInstanceActor
	Settings    ReviewSettings
	// Received tells whether a dropped activity can be processed again.
	Received InboxRepository
}

// reviewActive reports whether inbound.status asks about anything. While it does not, the paths of
// a post read nothing more than they would without the hook.
func (h *StatusHandlers) reviewActive(ctx context.Context) (bool, error) {
	if h.d.Review.Gate == nil {
		return false, nil
	}
	return h.d.Review.Gate.Active(ctx, apphook.InboundStatus)
}

// statusVerdict is what the transaction that stores a post acts on.
type statusVerdict struct {
	apphook.Result
	// reporterAccountID is the instance actor when the verdict files a report: it is created on first
	// use, with keys and its own transaction, so it is settled before the transaction starts.
	reporterAccountID string
}

// allowed is the verdict on a post that the plugin is not asked about.
var allowed = statusVerdict{Result: apphook.Result{Effect: apphook.Allow}}

// review asks the plugin about a post that the caller found in scope. A direct post from an author
// no local account follows stays between the two unless the operator chose to send it (ADR-0147
// §10). What is sent is the remote post alone: nothing of a local post, not even the parent's text,
// goes out with it. It fails with an error that the job returns as it is, apphook.ErrRetryLater
// included.
func (h *StatusHandlers) review(ctx context.Context, a *InboxActivity, obj *federation.Object, in appcontent.RemoteStatusInput, m *postMentions) (statusVerdict, error) {
	followed, err := h.followed(ctx, a.Actor)
	if err != nil {
		return statusVerdict{}, err
	}
	if in.Visibility == appcontent.VisibilityDirect && !followed {
		send, err := h.d.Review.Settings.ReviewUnfollowedDirect(ctx)
		if err != nil {
			return statusVerdict{}, err
		}
		if !send {
			return allowed, nil
		}
	}
	res, err := h.d.Review.Gate.Decide(ctx, apphook.Call{
		Hook: apphook.InboundStatus, SubjectKey: string(a.Received.ActivityURI), AccountID: a.Actor.Account.ID,
		Subject: inboundStatusSubject(a, obj, in, m, followed),
	})
	if err != nil {
		return statusVerdict{}, err
	}
	v := statusVerdict{Result: res}
	if res.Effect == apphook.Flag {
		instance, err := h.d.Review.Instance.EnsureInstanceActor(ctx)
		if err != nil {
			return statusVerdict{}, err
		}
		v.reporterAccountID = instance.ID
	}
	return v, nil
}

func inboundStatusSubject(a *InboxActivity, obj *federation.Object, in appcontent.RemoteStatusInput, m *postMentions, followed bool) apphook.InboundStatusSubject {
	s := apphook.InboundStatusSubject{
		ActivityType: a.Activity.RawType, StatusURI: in.URI, StatusURL: in.URL, AuthorURI: string(a.Activity.Actor), AuthorDomain: in.AuthorDomain,
		Visibility: string(in.Visibility), AnyLocalFollower: followed, InReplyToURI: in.InReplyToURI, SpoilerText: in.SpoilerText,
		Sensitive: in.Sensitive, PublishedAt: obj.Published, MentionCount: len(in.MentionedAccountIDs), LocalMentionCount: m.localCount, Hashtags: in.Hashtags,
	}
	for _, b := range in.Bodies {
		s.Bodies = append(s.Bodies, apphook.InboundStatusBody{Language: b.Language, HTML: b.HTML})
	}
	for _, at := range in.Attachments {
		s.Attachments = append(s.Attachments, apphook.InboundStatusAttachment{Type: string(at.Type), URL: at.URL, ContentType: at.ContentType, Description: at.Description})
	}
	if in.Poll != nil {
		s.PollOptions = in.Poll.Options
	}
	return s
}

// reviewUpdate asks the plugin about a new revision of a stored post (ADR-0147 §3): only when the
// actor wrote the post, so a revision the transaction would refuse is not sent, and when the post is
// in scope, so that what only a relay carries reaches no plugin. A post out of scope stores its
// revision without a question.
func (h *StatusHandlers) reviewUpdate(ctx context.Context, a *InboxActivity, st appcontent.Status, obj *federation.Object, in appcontent.RemoteStatusInput, m *postMentions) (statusVerdict, error) {
	active, err := h.reviewActive(ctx)
	if err != nil || !active {
		return allowed, err
	}
	if st.IsLocal || st.AuthorAccountID != a.Actor.Account.ID {
		return allowed, nil
	}
	tombstoned, err := h.d.Statuses.Tombstoned(ctx, st.URI)
	if err != nil || tombstoned {
		return allowed, err
	}
	scoped, err := h.inScope(ctx, a.Actor, obj, m)
	if err != nil || !scoped {
		return allowed, err
	}
	return h.review(ctx, a, obj, in, m)
}

// reviewDropped is the error of a post that the verdict drops: the job completes, its ordering key
// goes on, and nothing of the post is stored.
func reviewDropped(obj *federation.Object) error {
	return fmt.Errorf("%w: %s is dropped by the hook %s", ErrDropActivity, obj.ID, apphook.InboundStatus)
}

// applyReview acts on the verdict inside the transaction that stored the post or its revision: the
// report of a flag, the mark of a mark_sensitive, and the link and the audit log entry of either.
func (h *StatusHandlers) applyReview(ctx context.Context, tx uow.Tx, v statusVerdict, statusID string, author appaccount.Account) error {
	res := v.Result
	switch res.Effect {
	case apphook.Flag:
		filed, err := h.d.Review.Reports.FileHookReport(ctx, tx, appmoderation.HookReportInput{
			ReporterAccountID: v.reporterAccountID, TargetAccountID: author.ID, StatusIDs: []string{statusID},
			DecisionID: res.DecisionID, Hook: string(res.Hook), ReasonCode: string(res.Reason), Comment: res.Note,
		})
		if err != nil || !filed {
			return err
		}
	case apphook.MarkSensitive:
		if err := h.d.Review.Sensitivity.MarkSensitiveByHook(ctx, tx, statusID, res.DecisionID); err != nil {
			return err
		}
	default:
		return nil
	}
	if err := h.d.Review.Gate.LinkStatus(ctx, tx, res.DecisionID, statusID); err != nil {
		return err
	}
	return h.d.Review.Gate.Applied(ctx, tx, res)
}

// UndoReview undoes a cancelled decision of inbound.status inside the transaction of its
// cancellation (ADR-0147 §9). A mark goes. A dropped activity is processed again while its document
// is kept, which reads the cancelled decision as allow and calls no plugin; past that nothing is
// left to bring back. A report stays: resolving it is the moderators' usual work.
func (h *StatusHandlers) UndoReview(ctx context.Context, tx uow.Tx, r apphook.Record) error {
	switch r.Decision.Outcome {
	case apphook.MarkSensitive:
		return h.d.Review.Sensitivity.ClearSensitivityOfHookDecision(ctx, tx, r.Decision.ID)
	case apphook.Drop:
		received, err := h.d.Review.Received.FindKeptReceivedActivityTx(ctx, tx, federation.IRI(r.Decision.SubjectKey))
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		job, err := InboxJob(received)
		if err != nil {
			return err
		}
		return h.d.Outbox.EnqueueTx(ctx, tx, job)
	}
	return nil
}
