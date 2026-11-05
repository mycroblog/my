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
// account, after the replay of an Idempotency-Key and before the transaction. The plugin's wait is
// short (the plugin's timeout, 1 second by default) and its failure never stops a post. By default
// only accounts created recently are sent, private and direct posts never are, and an account
// whose appeal a moderator overturned no longer is (the hook module checks that).

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
	// Reports files the report of a flag inside the post's transaction (ADR-0147 §7); nil files
	// none, and the decision is only recorded.
	Reports HookReports
}

// HookReports files the report of a flag decision on a post, in the transaction that writes it.
type HookReports interface {
	FileHookReport(ctx context.Context, tx uow.Tx, r apphook.Result, authorAccountID, statusID string) error
}

// review asks local.status about the post that p holds, and returns the Result to carry into the
// transaction; a Result with no decision when nothing is asked.
func (s *Service) review(ctx context.Context, author appaccount.Actor, p PreparedCreate) (apphook.Result, error) {
	r := s.d.StatusReview
	none := apphook.Result{Hook: apphook.LocalStatus, SubjectKey: p.st.ID, Effect: apphook.Allow}
	if r.Hooks == nil || !r.Hooks.Configured(apphook.LocalStatus) {
		return none, nil
	}
	switch p.st.Visibility {
	case content.VisibilityPrivate, content.VisibilityDirect:
		return none, nil
	}
	if r.MaxAccountAge != nil {
		age, err := r.MaxAccountAge(ctx)
		if err != nil {
			return apphook.Result{}, err
		}
		if age > 0 && p.now.Sub(author.Account.CreatedAt) > age {
			return none, nil
		}
	}
	return r.Hooks.Decide(ctx, apphook.Call{
		Hook: apphook.LocalStatus, SubjectKey: p.st.ID, AccountID: author.Account.ID, Subject: localStatusSubject(author, p),
	})
}

// localStatusSubject is what local.status sends about the post.
func localStatusSubject(author appaccount.Actor, p PreparedCreate) apphook.LocalStatusSubject {
	rev := p.rev
	out := apphook.LocalStatusSubject{
		StatusURI: p.st.URI, AuthorURI: author.Account.URI, AccountCreatedAt: author.Account.CreatedAt,
		Visibility: string(p.st.Visibility), InReplyToURI: p.st.InReplyToURI, Language: rev.Language,
		SpoilerText: rev.SpoilerText, Sensitive: rev.Sensitive, MentionCount: len(rev.MentionedAccountIDs),
	}
	if author.Local != nil {
		out.AuthorUsername = author.Local.Username
	}
	if p.quote != nil {
		out.QuoteURI = p.quote.quoted.URI
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
	for _, t := range p.tags {
		out.Hashtags = append(out.Hashtags, t.Name)
	}
	return out
}

// reviewed acts inside the post's transaction on a flag in force: it links the decision to the
// post, and files the report when a reporter is set, which the audit log records.
func (s *Service) reviewed(ctx context.Context, tx uow.Tx, r apphook.Result, post Post) error {
	if r.DecisionID == "" || r.Effect != apphook.Flag {
		return nil
	}
	hooks := s.d.StatusReview.Hooks
	if err := hooks.LinkStatus(ctx, tx, r.DecisionID, post.Status.ID); err != nil {
		return err
	}
	if s.d.StatusReview.Reports == nil {
		return nil
	}
	if err := s.d.StatusReview.Reports.FileHookReport(ctx, tx, r, post.Status.AuthorAccountID, post.Status.ID); err != nil {
		return err
	}
	return hooks.Applied(ctx, tx, r)
}
