// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The decision hook local.status (ADR-0147 §2, §9): Create asks it about a new post of a local
// account, after the replay of an Idempotency-Key and before the transaction, and Edit about each
// edit, with the post and the new revision as the subject (hook.EditSubjectKey). The plugin's wait
// is short (the plugin's timeout, 1 second by default) and its failure never stops a post. By
// default only accounts created recently are sent, private and direct posts never are, and an
// account whose appeal a moderator overturned no longer is (the hook module checks that). A reject
// of an edit refuses that edit alone, which leaves the post as it was and cannot be appealed.

// ErrRejectedByReview reports a post that the hook local.status rejected while it enforces.
var ErrRejectedByReview = errors.New("content: the post was rejected by an automated review")

// RejectedByReviewError is the ErrRejectedByReview of one post, with the decision its author may
// appeal.
type RejectedByReviewError struct {
	DecisionID string
	// DecidedAt is when the decision was made; the appeal window runs from it.
	DecidedAt time.Time
}

func (e *RejectedByReviewError) Error() string {
	return fmt.Sprintf("%v: decision %s", ErrRejectedByReview, e.DecisionID)
}

// Unwrap returns ErrRejectedByReview.
func (e *RejectedByReviewError) Unwrap() error { return ErrRejectedByReview }

// StatusReview is what Create needs to ask local.status.
type StatusReview struct {
	// Hooks asks the hook; nil asks nothing.
	Hooks apphook.Gate
	// MaxAccountAge returns how old an account may be and still be sent, read at each post; 0
	// sends every account. nil sends every account.
	MaxAccountAge func(ctx context.Context) (time.Duration, error)
	// Reports files the report of a flag inside the post's transaction (ADR-0147 §7), with
	// InstanceActor as its reporter; nil for either files none, and the decision is only linked to
	// the post.
	Reports       HookReports
	InstanceActor func(ctx context.Context) (string, error)
}

// reviewVerdict is what Create carries into its transaction: the hook's Result and, for a flag
// that files a report, its reporter.
type reviewVerdict struct {
	apphook.Result
	reporterAccountID string
}

// reviewedPost is what local.status sends about a new post or an edit, and the key of its subject.
type reviewedPost struct {
	subjectKey string
	st         content.Status
	rev        content.Revision
	tagNames   []string
	quoteURI   string
	now        time.Time
}

// review asks local.status about the new post that p holds, and returns the Result to carry into
// the transaction; a Result with no decision when nothing is asked.
func (s *Service) review(ctx context.Context, author appaccount.Actor, p PreparedCreate) (apphook.Result, error) {
	post := reviewedPost{subjectKey: p.st.ID, st: p.st, rev: p.rev, now: p.now}
	for _, t := range p.tags {
		post.tagNames = append(post.tagNames, t.Name)
	}
	if p.quote != nil {
		post.quoteURI = p.quote.quoted.URI
	}
	return s.ask(ctx, author, post)
}

// reviewEdit asks local.status about an edit of st that stores rev, with the post as edited.
func (s *Service) reviewEdit(ctx context.Context, author appaccount.Actor, st content.Status, rev content.Revision, tags []content.Tag, detachQuote bool) (apphook.Result, error) {
	post := reviewedPost{subjectKey: apphook.EditSubjectKey(st.ID, rev.ID), st: st, rev: rev, now: s.d.Clock.Now()}
	for _, t := range tags {
		post.tagNames = append(post.tagNames, t.Name)
	}
	if !detachQuote && s.d.Quotes != nil {
		q, err := s.d.Quotes.FindQuote(ctx, st.ID)
		switch {
		case err == nil:
			post.quoteURI = q.QuotedURI
		case !errors.Is(err, ErrNotFound):
			return apphook.Result{}, err
		}
	}
	return s.ask(ctx, author, post)
}

// ask asks local.status about post when the post and its author are in its scope; a Result with no
// decision when nothing is asked.
func (s *Service) ask(ctx context.Context, author appaccount.Actor, post reviewedPost) (apphook.Result, error) {
	r := s.d.StatusReview
	none := apphook.Result{Hook: apphook.LocalStatus, SubjectKey: post.subjectKey, Effect: apphook.Allow}
	if r.Hooks == nil || !r.Hooks.Configured(apphook.LocalStatus) {
		return none, nil
	}
	switch post.st.Visibility {
	case content.VisibilityPrivate, content.VisibilityDirect:
		return none, nil
	}
	if r.MaxAccountAge != nil {
		age, err := r.MaxAccountAge(ctx)
		if err != nil {
			return apphook.Result{}, err
		}
		if age > 0 && post.now.Sub(author.Account.CreatedAt) > age {
			return none, nil
		}
	}
	return r.Hooks.Decide(ctx, apphook.Call{
		Hook: apphook.LocalStatus, SubjectKey: post.subjectKey, AccountID: author.Account.ID, Subject: localStatusSubject(author, post),
	})
}

// localStatusSubject is what local.status sends about the post.
func localStatusSubject(author appaccount.Actor, p reviewedPost) apphook.LocalStatusSubject {
	rev := p.rev
	out := apphook.LocalStatusSubject{
		StatusURI: p.st.URI, AuthorURI: author.Account.URI, AccountCreatedAt: author.Account.CreatedAt,
		Visibility: string(p.st.Visibility), InReplyToURI: p.st.InReplyToURI, QuoteURI: p.quoteURI, Language: rev.Language,
		SpoilerText: rev.SpoilerText, Sensitive: rev.Sensitive, MentionCount: len(rev.MentionedAccountIDs),
	}
	if author.Local != nil {
		out.AuthorUsername = author.Local.Username
	}
	if rev.Source != nil {
		out.Text = rev.Source.Text
	}
	if len(rev.Bodies) > 0 {
		out.HTML = rev.Bodies[0].ContentHTML
	}
	for _, a := range rev.Attachments {
		out.AttachmentDescriptions = append(out.AttachmentDescriptions, a.Description)
	}
	if rev.Poll != nil {
		for _, o := range rev.Poll.Options {
			out.PollOptions = append(out.PollOptions, o.Title)
		}
	}
	out.Hashtags = p.tagNames
	return out
}

// verdict returns what Create carries into its transaction, resolving the reporter of a flag that
// files a report outside it.
func (s *Service) verdict(ctx context.Context, r apphook.Result) (reviewVerdict, error) {
	v := reviewVerdict{Result: r}
	if r.DecisionID == "" || r.Effect != apphook.Flag || s.d.StatusReview.Reports == nil || s.d.StatusReview.InstanceActor == nil {
		return v, nil
	}
	var err error
	v.reporterAccountID, err = s.d.StatusReview.InstanceActor(ctx)
	return v, err
}

// reviewed acts inside the post's transaction on a flag in force: it files the report when a
// reporter is set, links the decision to the post, and records the effect in the audit log.
func (s *Service) reviewed(ctx context.Context, tx uow.Tx, v reviewVerdict, post Post) error {
	r := v.Result
	if r.DecisionID == "" || r.Effect != apphook.Flag {
		return nil
	}
	hooks := s.d.StatusReview.Hooks
	if err := hooks.LinkStatus(ctx, tx, r.DecisionID, post.Status.ID); err != nil {
		return err
	}
	if v.reporterAccountID == "" {
		return nil
	}
	filed, err := s.d.StatusReview.Reports.FileHookReport(ctx, tx, HookReport{
		ReporterAccountID: v.reporterAccountID, TargetAccountID: post.Status.AuthorAccountID, StatusIDs: []string{post.Status.ID},
		DecisionID: r.DecisionID, Hook: string(r.Hook), ReasonCode: string(r.Reason), Comment: r.Note,
	})
	if err != nil || !filed {
		return err
	}
	return hooks.Applied(ctx, tx, r)
}
