// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/domain/federation"
)

// NoteParts resolves what a revision refers to (its media, mentioned accounts, and hashtags) for
// the Note of a status. *appcontent.Reader implements it.
type NoteParts interface {
	Parts(ctx context.Context, rev appcontent.Revision) (appcontent.RevisionParts, error)
}

// NoteQuotes reads what the Note of a status says of quotes (ADR-0069 §3): its quote, who may
// quote it, and the page of the quoted status that the fallback links. *appcontent.Service
// implements it.
type NoteQuotes interface {
	FindQuote(ctx context.Context, statusID string) (appcontent.Quote, appcontent.QuoteState, error)
	FindQuotePolicy(ctx context.Context, statusID string) (appcontent.StatusQuotePolicy, error)
	QuotedPageURL(ctx context.Context, uri string) (string, error)
}

// Relationships reads the relationship of an account with another; *appgraph.Service
// implements it.
type Relationships interface {
	Relationship(ctx context.Context, accountID, targetID string) (appgraph.Relationship, error)
}

// notes builds the Note of a local status, the one place that the fan-out (the object of Create
// and Update) and object fetches (GET of the status's IRI) build it (federation.md §7.4).
type notes struct {
	parts NoteParts
	graph Relationships
	// quotes reads the quotes of statuses; nil leaves quotes and interactionPolicy out.
	quotes NoteQuotes
	// host serves the hashtag pages; accountDomain is the domain of local handles.
	host, accountDomain string
}

// note returns the Note of st as of rev by its local author, addressed by the effective
// visibility vis. A zero updated leaves updated out.
//
// The Note of a local-visibility status leaves out of tag the remote accounts that do not
// actively follow the author at the time it is built (ADR-0035): a server that gets the status
// for a follower would otherwise let a mentioned account there see it. The mention links in the
// content stay. Its addressing has no mentioned accounts anyway (ADR-0017).
func (n notes) note(ctx context.Context, st appcontent.Status, rev appcontent.Revision, author appaccount.Actor, vis appcontent.Visibility, updated time.Time) (federation.NoteDocument, error) {
	parts, err := n.parts.Parts(ctx, rev)
	if err != nil {
		return federation.NoteDocument{}, err
	}
	if vis == appcontent.VisibilityLocal {
		kept := parts.Mentions[:0:0]
		for _, m := range parts.Mentions {
			if !m.Account.IsLocal() {
				r, err := n.graph.Relationship(ctx, m.Account.ID, st.AuthorAccountID)
				if err != nil {
					return federation.NoteDocument{}, err
				}
				if !r.ActivelyFollowing() {
					continue
				}
			}
			kept = append(kept, m)
		}
		parts.Mentions = kept
	}
	note := statusNote(st, rev, author, vis, parts, updated, n.host, n.accountDomain)
	if n.quotes == nil {
		return note, nil
	}
	return n.withQuotes(ctx, note, st, author, vis)
}

// withQuotes adds to the Note of the local status st its quote, unless the author took it away,
// and its interactionPolicy (ADR-0069 §3): its quote policy for a public or unlisted status,
// the author alone for the others, which remote servers see as followers-only.
func (n notes) withQuotes(ctx context.Context, note federation.NoteDocument, st appcontent.Status, author appaccount.Actor, vis appcontent.Visibility) (federation.NoteDocument, error) {
	switch q, state, err := n.quotes.FindQuote(ctx, st.ID); {
	case errors.Is(err, appcontent.ErrNotFound):
	case err != nil:
		return federation.NoteDocument{}, err
	case state.Approval != appcontent.QuoteApprovalDetached:
		page, err := n.quotes.QuotedPageURL(ctx, q.QuotedURI)
		if err != nil {
			return federation.NoteDocument{}, err
		}
		auth := ""
		if state.Approval == appcontent.QuoteApprovalAccepted {
			auth = state.AuthorizationURI
		}
		note = note.WithQuote(federation.IRI(q.QuotedURI), federation.IRI(auth), page)
	}
	actor := federation.IRI(author.Account.URI)
	approved := []federation.IRI{actor}
	if vis == appcontent.VisibilityPublic || vis == appcontent.VisibilityUnlisted {
		policy := appcontent.QuotePolicyPublic
		switch p, err := n.quotes.FindQuotePolicy(ctx, st.ID); {
		case err == nil:
			policy = p.Policy
		case !errors.Is(err, appcontent.ErrNotFound):
			return federation.NoteDocument{}, err
		}
		switch policy {
		case appcontent.QuotePolicyPublic:
			approved = []federation.IRI{federation.PublicIRI}
		case appcontent.QuotePolicyFollowers:
			approved = []federation.IRI{federation.FollowersOf(actor)}
		}
	}
	note.InteractionPolicy = &federation.InteractionPolicy{CanQuote: federation.QuoteApprovalPolicy{AutomaticApproval: approved}}
	return note, nil
}

// statusNote returns the Note of a local status st as of rev by author, addressed by the
// effective visibility vis to its followers and the mentioned accounts of parts, with the body of
// rev: content per language, content warning, attachments, and the mentions, hashtags, and custom
// emoji of parts as tag (federation.md §7.4, ADR-0051 §3). Its url is the status page under the author's current username
// (ADR-0053). A zero updated leaves updated out.
func statusNote(st appcontent.Status, rev appcontent.Revision, author appaccount.Actor, vis appcontent.Visibility, parts appcontent.RevisionParts, updated time.Time, host, accountDomain string) federation.NoteDocument {
	mentioned := make([]federation.IRI, 0, len(parts.Mentions))
	for _, m := range parts.Mentions {
		mentioned = append(mentioned, federation.IRI(m.Account.URI))
	}
	actor := federation.IRI(author.Account.URI)
	aud := federation.Address(federation.Visibility(vis), federation.FollowersOf(actor), mentioned)
	note := federation.NoteDocument{
		ObjectDocument: federation.ObjectDocument{
			ID: federation.IRI(st.URI), Type: federation.ObjectNote,
			Published: federation.FormatTime(st.PublishedAt), URL: appcontent.StatusPageURL(host, st, author),
			AttributedTo: actor, To: aud.To, Cc: aud.Cc,
		},
		Summary: rev.SpoilerText,
		// A content warning hides the whole status, as Mastodon marks it.
		Sensitive:  rev.Sensitive || rev.SpoilerText != "",
		Attachment: []federation.AttachmentDocument{},
		Tag:        []federation.TagDocument{},
	}
	if st.InReplyToURI != "" {
		r := federation.IRI(st.InReplyToURI)
		note.InReplyTo = &r
	}
	if !updated.IsZero() {
		note.Updated = federation.FormatTime(updated)
	}
	for i, b := range rev.Bodies {
		if i == 0 || b.Language == rev.Language {
			note.Content = b.ContentHTML
		}
		if b.Language != "" && b.Language != "und" {
			if note.ContentMap == nil {
				note.ContentMap = map[string]string{}
			}
			note.ContentMap[b.Language] = b.ContentHTML
		}
	}
	for _, m := range parts.Attachments {
		note.Attachment = append(note.Attachment, attachmentOf(m))
	}
	for _, m := range parts.Mentions {
		note.Tag = append(note.Tag, federation.TagDocument{
			Type: federation.TagMention, Href: m.Account.URI, Name: handleOf(m, accountDomain),
		})
	}
	for _, t := range parts.Tags {
		note.Tag = append(note.Tag, federation.TagDocument{
			Type: federation.TagHashtag, Href: "https://" + host + "/tags/" + url.PathEscape(t.Name), Name: "#" + t.Name,
		})
	}
	for _, e := range parts.Emojis {
		note.Tag = append(note.Tag, EmojiTagOf(e, host))
	}
	if p := parts.Poll; p != nil {
		note = note.AsQuestion(questionPoll(p))
	}
	return note
}

// questionPoll returns the poll of a Question from the poll as an anonymous viewer sees it: the
// counts of each choice are 0 while they are hidden (content.Poll.TotalsHidden), and the poll
// is closed once it has ended (FR-112).
func questionPoll(v *appcontent.PollView) federation.QuestionPoll {
	q := federation.QuestionPoll{Multiple: v.Poll.Multiple, Closed: v.Closed, VotersCount: v.VotersCount}
	if v.Poll.ClosesAt != nil {
		q.EndTime = *v.Poll.ClosesAt
	}
	for i, o := range v.Poll.Options {
		q.Options = append(q.Options, o.Title)
		var votes int64
		if !v.TotalsHidden && i < len(v.Votes) {
			votes = v.Votes[i]
		}
		q.Votes = append(q.Votes, votes)
	}
	return q
}

// attachmentOf returns the attachment document of a processed media file: the URL, type, and
// size of its large variant, moving for an animation (ADR-0054 §5), or of the video or audio
// file with its poster's large variant as icon and its length as duration; its alternative
// text, blurhash, and focal point.
func attachmentOf(m appcontent.MediaView) federation.AttachmentDocument {
	large := m.Image.Primary()
	a := federation.AttachmentDocument{
		Type: federation.AttachmentType, MediaType: large.ContentType, URL: large.URL, Name: m.Description,
		Width: large.Width, Height: large.Height,
	}
	if r := m.Media.Result; r != nil {
		a.Blurhash = r.Blurhash
		if a.Width == 0 || a.Height == 0 {
			a.Width, a.Height = r.Width, r.Height
		}
		if r.DurationMS > 0 {
			a.Duration = federation.FormatDuration(time.Duration(r.DurationMS) * time.Millisecond)
		}
	}
	if m.Image.Media != nil && m.Image.HasStill() {
		poster := m.Image.Still.Large
		a.Icon = &federation.ImageDocument{Type: federation.ImageType, MediaType: poster.ContentType, URL: poster.URL}
	}
	if f := m.Media.Focus; f != nil && (f.X != 0 || f.Y != 0) {
		a.FocalPoint = []float64{f.X, f.Y}
	}
	return a
}

// handleOf returns the handle of a mentioned account, @user@domain, with local accounts on
// accountDomain; "" when the account has no handle.
func handleOf(a appaccount.Actor, accountDomain string) string {
	switch {
	case a.Handle != nil && a.Handle.Domain != "":
		return "@" + a.Handle.Username + "@" + a.Handle.Domain
	case a.Handle != nil:
		return "@" + a.Handle.Username + "@" + accountDomain
	case a.Local != nil:
		return "@" + a.Local.Username + "@" + accountDomain
	}
	return ""
}

// document builds the body of act (federation.md §7.4): the activity envelope with its context
// and addressing around the object. The object of a Create or Update is the status's Note as of
// the revision the activity presents. The Delete of a status, the Like and Undo Like of a
// favourite, and the Create of a vote have no document here: their projectors store them with
// the event (deleteDocument, likeDocument, registerPollVoteProjectors), so document returns nil.
func (f *FanOut) document(ctx context.Context, act federation.OutboundActivity, s subject) ([]byte, error) {
	if act.ObjectKind == federation.OutboundBlock {
		return f.blockDocument(ctx, act, s)
	}
	if act.ObjectKind == federation.OutboundReport {
		return f.flagDocument(ctx, act, s)
	}
	if act.ObjectKind == federation.OutboundFollow {
		return f.followDocument(ctx, s)
	}
	if act.ObjectKind == federation.OutboundAccount {
		images, err := LocalActorImages(ctx, f.d.Images, s.actor)
		if err != nil {
			return nil, err
		}
		if images.Emojis, err = LocalActorEmojis(ctx, f.d.EmojiTags, f.d.Host, s.actor); err != nil {
			return nil, err
		}
		if images.AlsoKnownAs, err = LocalActorAliases(ctx, f.d.Aliases, s.actor); err != nil {
			return nil, err
		}
		return accountDocument(f.d.Host, act, s.actor, images, s.suspended)
	}
	if act.ObjectKind == federation.OutboundFavourite || act.ObjectKind == federation.OutboundPollVote ||
		act.ObjectKind == federation.OutboundRelay || act.ObjectKind == federation.OutboundQuote || act.ObjectKind == federation.OutboundRefusal ||
		(act.ObjectKind == federation.OutboundStatus && act.Type == federation.ActivityDelete) {
		return nil, nil
	}
	author, err := f.d.Accounts.FindByID(ctx, act.ActorAccountID)
	if err != nil {
		return nil, fmt.Errorf("actor %s: %w", act.ActorAccountID, err)
	}
	actor := federation.IRI(author.Account.URI)

	var doc federation.ActivityDocument
	switch act.ObjectKind {
	case federation.OutboundStatus:
		published, updated := s.status.PublishedAt, time.Time{}
		if act.Type == federation.ActivityUpdate {
			// The Update of the counts of a poll that was never edited has no updated, so that
			// the receivers do not mark the status as edited (Mastodon takes it as an implicit
			// update of the poll).
			published, updated = s.revision.RevisedAt, updatedOf(s.revision)
		}
		note, err := f.notes.note(ctx, s.status, s.revision, author, s.visibility, updated)
		if err != nil {
			return nil, err
		}
		doc = federation.NewActivityDocument(act.ActivityURI, act.Type, actor, published, federation.Audience{To: note.To, Cc: note.Cc}, note)
	case federation.OutboundBoost:
		mentioned, err := f.actorIRIs(ctx, s.mentioned)
		if err != nil {
			return nil, err
		}
		aud := federation.Address(federation.Visibility(s.visibility), federation.FollowersOf(actor), mentioned)
		announce := federation.NewActivityDocument(federation.IRI(s.boost.URI), federation.ActivityAnnounce, actor,
			s.boost.BoostedAt, aud, federation.IRI(s.status.URI))
		doc = announce
		if act.Type == federation.ActivityUndo {
			doc = federation.NewActivityDocument(act.ActivityURI, act.Type, actor, act.CreatedAt, aud, announce.Embedded())
		}
	default:
		return nil, fmt.Errorf("%w: %s of a %s", federation.ErrInvalidOutbound, act.Type, act.ObjectKind)
	}
	return doc.Marshal()
}

// accountDocument builds the Update or the Delete of a local actor, addressed to the public as
// Mastodon addresses them. The Update carries the actor's document as it is when the fan-out
// starts, the same document a GET of the actor returns; a later revision sends an Update of its
// own. The document of a suspended actor is emptied and marked suspended (federation.md §11.1).
// The Delete names the actor by its id.
func accountDocument(host string, act federation.OutboundActivity, a appaccount.Actor, images ActorImages, suspended bool) ([]byte, error) {
	actor := federation.IRI(a.Account.URI)
	aud := federation.Audience{To: []federation.IRI{federation.PublicIRI}}
	var object any = actor
	if act.Type == federation.ActivityUpdate {
		doc := LocalActorDocument(host, a, "", images)
		if suspended {
			doc = doc.Suspend()
		}
		object = doc.Embedded()
	}
	doc := federation.NewActivityDocument(act.ActivityURI, act.Type, actor, act.CreatedAt, aud, object)
	if act.Type == federation.ActivityUpdate {
		// The embedded actor has no context of its own; the Update carries the actor's.
		doc.Context = federation.ActorLDContext()
	}
	return doc.Marshal()
}

// deleteDocument returns the Delete of a local status st, built from the deletion event alone so
// that it does not depend on rows that the purge removes: a Tombstone of the status's IRI,
// addressed by the status's own visibility to the author's followers (federation.md §7.1). It
// names no mentioned account: a Delete tells only that the status is gone, and goes to the
// inboxes of the Create.
func deleteDocument(act federation.OutboundActivity, host string, st appcontent.Status) ([]byte, error) {
	actor := federation.IRI(appaccount.ActorURI(host, st.AuthorAccountID))
	uri := st.URI
	if uri == "" {
		uri = appcontent.StatusURI(host, st.ID)
	}
	aud := federation.Address(federation.Visibility(st.Visibility), federation.FollowersOf(actor), nil)
	return federation.NewActivityDocument(act.ActivityURI, act.Type, actor, act.CreatedAt, aud, federation.TombstoneOf(federation.IRI(uri))).Marshal()
}

// actorIRI returns the ActivityPub id of an account.
func (f *FanOut) actorIRI(ctx context.Context, accountID string) (federation.IRI, error) {
	a, err := f.d.Accounts.FindByID(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("actor %s: %w", accountID, err)
	}
	return federation.IRI(a.Account.URI), nil
}

// actorIRIs returns the ids of the accounts that exist among ids, in order.
func (f *FanOut) actorIRIs(ctx context.Context, ids []string) ([]federation.IRI, error) {
	var out []federation.IRI
	for _, id := range ids {
		iri, err := f.actorIRI(ctx, id)
		if errors.Is(err, appaccount.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, iri)
	}
	return out, nil
}
