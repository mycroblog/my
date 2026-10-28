// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// RemoteMediaRecorder records the media files that a received document declares;
// *MediaService implements it.
type RemoteMediaRecorder interface {
	RecordRemoteMedia(ctx context.Context, tx uow.Tx, in RemoteMediaInput) (content.RemoteMedia, error)
}

// ErrInvalidRemoteStatus reports a remote status that cannot be stored: no URI or author, or a
// visibility that a remote status cannot have.
var ErrInvalidRemoteStatus = errors.New("content: the remote status is not valid")

// RemoteStatusInput is a remote post as the federation module read it from a Create or an
// Update (docs/architecture/federation.md §4): the facts of the status and the content of one
// revision. The HTML is as received; the content module sanitizes it.
type RemoteStatusInput struct {
	URI             string
	URL             string // "" when the post has no separate HTML page
	AuthorAccountID string
	// AuthorDomain is the canonical host of the author, the domain of its custom emoji.
	AuthorDomain string
	// Visibility comes from the addressing; it is never local (ADR-0017).
	Visibility   content.Visibility
	InReplyToURI string
	PublishedAt  time.Time
	// RevisedAt is the AP updated time, or PublishedAt when there is none.
	RevisedAt time.Time
	// ActivityURI is the Create or Update that carried the post, "" for a fetch.
	ActivityURI string
	// Bodies are the HTML per language in document order; the first is the default language.
	Bodies      []RemoteBody
	SpoilerText string // plain text
	Sensitive   bool
	// MentionedAccountIDs are the accounts that the post mentions or addresses.
	MentionedAccountIDs []string
	// Hashtags are the names of the Hashtag tags, without '#'.
	Hashtags    []string
	Emojis      []RemoteEmojiInput
	Attachments []RemoteAttachmentInput
	// OmittedAttachments is the number of media attachments that the post has after
	// Attachments and that were left out; the attachments of Attachments after the first
	// MaxAttachments are left out and counted too.
	OmittedAttachments int
	Poll               *RemotePollInput
	// Quote is the post that the post quotes, nil for none (ADR-0069). It is recorded with the
	// status; a revision without one takes the quote away, and one with another is ignored.
	Quote *RemoteQuoteInput
	// QuotePolicy is who may quote the post, nil when it declares nobody (ADR-0069 §3); every
	// Update replaces it.
	QuotePolicy *RemoteQuotePolicyInput
}

// RemoteBody is the HTML of a remote post in one language (a canonical BCP 47 tag or "und").
type RemoteBody struct {
	Language string
	HTML     string
}

// RemoteEmojiInput is a custom emoji that a remote post uses.
type RemoteEmojiInput struct {
	URI       string // the id of the Emoji object, "" when it has none
	Shortcode string // without colons
	ImageURL  string
}

// RemoteAttachmentInput is a media attachment as the remote post declares it.
type RemoteAttachmentInput struct {
	// Type is the AS2 type: Document, Image, Video, or Audio.
	Type        string
	URL         string
	ContentType string // the AS2 mediaType, "" when undeclared
	Description string // as received; stored as plain text
	Blurhash    string
	Width       int
	Height      int
	// Focus is the focalPoint (x, y), nil when undeclared.
	Focus *[2]float64
}

// RemotePollInput is the poll of a remote Question.
type RemotePollInput struct {
	Multiple bool
	ClosesAt *time.Time
	// Options are the titles of the choices, as received; Votes their counts.
	Options     []string
	Votes       []int64
	VotersCount *int64
}

// IngestRemoteStatus stores a remote post inside tx (federation.md §3.4): the status with an ID
// whose time is its published time (#71), and its first revision with the bodies sanitized, the
// mentions, hashtags, custom emoji, the attachments' declared values, and the poll with its
// counts. It emits StatusCreated, so that the projections, the timelines, and the notifications
// see the post. The caller decides that the post is in scope (ADR-0034) and is not tombstoned.
//
// A URI that a status already has fails with ErrStatusExists; the transaction must then be
// abandoned (the status is already there).
func (s *Service) IngestRemoteStatus(ctx context.Context, tx uow.Tx, in RemoteStatusInput) (Post, error) {
	if err := s.checkRemote(in); err != nil {
		return Post{}, err
	}
	now := s.d.Clock.Now()
	published := kernel.Normalize(in.PublishedAt)
	if published.IsZero() || published.After(now) {
		published = now
	}
	st := content.Status{
		ID:              s.d.IDs.NewIDAt(published),
		URI:             in.URI,
		URL:             in.URL,
		AuthorAccountID: in.AuthorAccountID,
		Visibility:      in.Visibility,
		InReplyToURI:    in.InReplyToURI,
		PublishedAt:     published,
	}
	st.ConversationURI = st.URI
	if in.InReplyToURI != "" {
		st.ConversationURI = ""
		parent, err := s.d.Repository.FindLiveStatusByURI(ctx, tx, in.InReplyToURI)
		switch {
		case errors.Is(err, ErrNotFound):
			// The thread is unknown until the parent arrives.
		case err != nil:
			return Post{}, err
		default:
			st.ConversationURI = parent.ConversationURI
			if st.ConversationURI == "" {
				st.ConversationURI = parent.URI
			}
		}
	}
	if err := s.d.Repository.InsertStatus(ctx, tx, st); err != nil {
		return Post{}, err
	}
	revisedAt := remoteRevisedAt(in.RevisedAt, published, now)
	rev, err := s.remoteRevision(ctx, tx, in, st.ID, 1, revisedAt)
	if err != nil {
		return Post{}, err
	}
	if err := s.d.Repository.InsertRevision(ctx, tx, rev); err != nil {
		return Post{}, err
	}
	if err := tx.Emit(ctx, StatusCreated{Status: st, Revision: rev}); err != nil {
		return Post{}, err
	}
	if err := s.recordQuote(ctx, tx, st, in.Quote); err != nil {
		return Post{}, err
	}
	if in.QuotePolicy != nil {
		if err := s.recordRemoteQuotePolicy(ctx, tx, st, in.QuotePolicy); err != nil {
			return Post{}, err
		}
	}
	if err := s.observePoll(ctx, tx, rev.Poll, in.Poll, now); err != nil {
		return Post{}, err
	}
	return Post{Status: st, Revision: rev}, nil
}

// ReviseRemoteStatus records a new revision of a remote post inside tx (federation.md §3.3,
// §3.4): only when its RevisedAt is later than the current revision's, so that an Update that
// arrives late or twice makes no revision. A revision without a quote takes the status's quote
// away (ADR-0069 §3). The counts of the current poll are recorded either
// way, when they are higher. It reports whether a revision was made. A status that does not
// exist or is deleted fails with ErrNotFound; one of another author with ErrNotAuthor.
func (s *Service) ReviseRemoteStatus(ctx context.Context, tx uow.Tx, in RemoteStatusInput) (bool, error) {
	if err := s.checkRemote(in); err != nil {
		return false, err
	}
	st, err := s.d.Repository.FindLiveStatusByURI(ctx, tx, in.URI)
	if err != nil {
		return false, err
	}
	if st.IsLocal || st.AuthorAccountID != in.AuthorAccountID {
		return false, fmt.Errorf("status %s: %w", in.URI, ErrNotAuthor)
	}
	head, err := s.d.Repository.Head(ctx, tx, st.ID)
	if err != nil {
		return false, err
	}
	now := s.d.Clock.Now()
	if err := s.recordRemoteQuotePolicy(ctx, tx, st, in.QuotePolicy); err != nil {
		return false, err
	}
	revisedAt := remoteRevisedAt(in.RevisedAt, st.PublishedAt, now)
	if !revisedAt.After(head.LastRevisedAt) {
		current, err := s.d.Repository.FindRevision(ctx, head.RevisionID)
		if err != nil {
			return false, err
		}
		return false, s.observePoll(ctx, tx, current.Poll, in.Poll, now)
	}
	rev, err := s.remoteRevision(ctx, tx, in, st.ID, head.Seq+1, revisedAt)
	if err != nil {
		return false, err
	}
	if err := s.d.Repository.InsertRevision(ctx, tx, rev); err != nil {
		return false, err
	}
	if err := tx.Emit(ctx, StatusRevised{Status: st, Revision: rev}); err != nil {
		return false, err
	}
	if in.Quote == nil {
		if err := s.detachQuote(ctx, tx, st); err != nil {
			return false, err
		}
	}
	return true, s.observePoll(ctx, tx, rev.Poll, in.Poll, now)
}

// DeleteRemoteStatus records inside tx that the author of a remote post deleted it (a Delete,
// federation.md §3.4): the deletion (origin remote), which stays as the tombstone of the URI, and
// the removal from every projection. A status that is already deleted is left as it is. A URI
// that no status has fails with ErrNotFound; a status of another author, or a local one, with
// ErrNotAuthor. A concurrent deletion fails with ErrAlreadyDeleted, which abandons tx.
func (s *Service) DeleteRemoteStatus(ctx context.Context, tx uow.Tx, uri, authorAccountID string) error {
	st, err := s.d.Repository.FindLiveStatusByURI(ctx, tx, uri)
	if errors.Is(err, ErrNotFound) {
		// A deletion by retention is not the author's: the Delete still leaves its tombstone.
		if d, derr := s.d.Repository.FindDeletionByURI(ctx, uri); derr == nil && d.Tombstone() {
			return nil // deleted before
		} else if derr != nil && !errors.Is(derr, ErrNotFound) {
			return derr
		}
		return err
	}
	if err != nil {
		return err
	}
	if st.IsLocal || st.AuthorAccountID != authorAccountID {
		return fmt.Errorf("status %s: %w", uri, ErrNotAuthor)
	}
	d := content.Deletion{
		StatusID: st.ID, URI: st.URI, AuthorAccountID: st.AuthorAccountID, Origin: content.DeletedByRemote, DeletedAt: s.d.Clock.Now(),
	}
	if err := s.d.Repository.InsertDeletion(ctx, tx, d); err != nil {
		return err
	}
	return tx.Emit(ctx, StatusDeleted{Deletion: d, Status: st})
}

// LiveRemoteStatusIDs returns up to limit remote statuses of an author that are not deleted,
// newest first, by ID, whose ID is below before ("" for the first page): the statuses that an
// operator refetches from their origin (federation.md §6.4).
func (s *Service) LiveRemoteStatusIDs(ctx context.Context, authorAccountID, before string, limit int) ([]string, error) {
	return s.d.Repository.ListLiveRemoteStatusIDs(ctx, authorAccountID, before, limit)
}

// FindStatusByURI returns the status with a URI, deleted or not. It fails with ErrNotFound when
// there is none.
func (s *Service) FindStatusByURI(ctx context.Context, uri string) (Status, error) {
	return s.d.Repository.FindStatusByURI(ctx, uri)
}

// Tombstoned reports whether a deletion keeps a URI from being ingested again
// (content.Deletion.Tombstone).
func (s *Service) Tombstoned(ctx context.Context, uri string) (bool, error) {
	d, err := s.d.Repository.FindDeletionByURI(ctx, uri)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return d.Tombstone(), nil
}

// LocalStatusInConversation reports whether a local status that is not deleted belongs to the
// conversation conversationURI ("" is no conversation): a reply into it is in the scope of remote
// content (ADR-0034).
func (s *Service) LocalStatusInConversation(ctx context.Context, conversationURI string) (bool, error) {
	return s.d.Repository.LocalStatusInConversation(ctx, conversationURI)
}

func (s *Service) checkRemote(in RemoteStatusInput) error {
	switch {
	case s.d.Sanitizer == nil:
		return errors.New("content: remote statuses need an HTML sanitizer")
	case in.URI == "" || in.AuthorAccountID == "":
		return fmt.Errorf("%w: no URI or author", ErrInvalidRemoteStatus)
	case !in.Visibility.Valid() || in.Visibility == content.VisibilityLocal:
		return fmt.Errorf("%w: visibility %q", ErrInvalidRemoteStatus, in.Visibility)
	}
	return nil
}

// remoteRevisedAt is the time of a remote revision: its updated time, or published, never in
// the future.
func remoteRevisedAt(revisedAt, published, now time.Time) time.Time {
	t := kernel.Normalize(revisedAt)
	if t.IsZero() || t.Before(published) {
		t = published
	}
	if t.After(now) {
		t = now
	}
	return t
}

// remoteRevision builds the revision of a remote post, recording its custom emoji, hashtags, and
// attachments in tx.
func (s *Service) remoteRevision(ctx context.Context, tx uow.Tx, in RemoteStatusInput, statusID string, seq int, at time.Time) (content.Revision, error) {
	san := s.d.Sanitizer
	rev := content.Revision{
		ID: s.d.IDs.NewID(), StatusID: statusID, Seq: seq,
		SpoilerText: strings.TrimSpace(in.SpoilerText), Sensitive: in.Sensitive, Language: UndeterminedLanguage, RevisedAt: at,
		Remote: &content.RemoteRevisionSource{ActivityURI: in.ActivityURI, HTMLPolicyVersion: san.PolicyVersion()},
	}
	for _, b := range in.Bodies {
		lang := b.Language
		if lang == "" {
			lang = UndeterminedLanguage
		}
		if slices.ContainsFunc(rev.Bodies, func(x content.Body) bool { return x.Language == lang }) {
			continue
		}
		if len(rev.Bodies) == 0 {
			rev.Language = lang
		}
		rev.Bodies = append(rev.Bodies, content.Body{Language: lang, ContentHTML: sanitizeOptional(san.SanitizeHTML, b.HTML)})
	}
	if len(rev.Bodies) == 0 {
		rev.Bodies = []content.Body{{Language: UndeterminedLanguage}}
	}
	slices.SortFunc(rev.Bodies, func(a, b content.Body) int { return strings.Compare(a.Language, b.Language) })

	rev.MentionedAccountIDs = sortedSet(in.MentionedAccountIDs)
	var tags []content.Tag
	for _, h := range in.Hashtags {
		name := strings.TrimSpace(h)
		if name == "" || strings.ContainsFunc(name, isSpace) || slices.ContainsFunc(tags, func(t content.Tag) bool { return t.NameNormalized == NormalizeTag(name) }) {
			continue
		}
		tags = append(tags, content.Tag{ID: s.d.IDs.NewID(), Name: name, NameNormalized: NormalizeTag(name), CreatedAt: at})
	}
	var err error
	if rev.TagIDs, err = ensureTags(ctx, tx, s.d.Repository, tags); err != nil {
		return content.Revision{}, err
	}
	if rev.EmojiIDs, err = s.RecordRemoteEmojis(ctx, tx, in.AuthorAccountID, in.AuthorDomain, in.Emojis); err != nil {
		return content.Revision{}, err
	}
	if rev.Attachments, rev.Remote.OmittedAttachments, err = s.remoteAttachments(ctx, tx, in); err != nil {
		return content.Revision{}, err
	}
	if p := in.Poll; p != nil && len(p.Options) > 0 {
		poll := &content.Poll{ID: s.d.IDs.NewID(), Multiple: p.Multiple, ClosesAt: p.ClosesAt}
		for i, title := range p.Options[:min(len(p.Options), content.MaxPollOptions)] {
			poll.Options = append(poll.Options, content.PollOption{Position: i, Title: strings.TrimSpace(san.PlainText(title))})
		}
		rev.Poll = poll
	}
	return rev, nil
}

// RecordRemoteEmojis records inside tx the custom emoji that a remote document of an account
// uses (a post, or the profile of an actor, ADR-0051 §3), each with its image, and returns
// their IDs, sorted. domain is the canonical host of the
// account, the domain of its emoji: an emoji is identified by its shortcode and domain, and the
// id of its Emoji object is recorded as its URI only when it has that host, so that no server
// can take the row of another's emoji. A new image URL of a known emoji becomes its new image,
// recorded as a remote media file of the account that sent it; the URL of the current image
// changes nothing, whichever account sends it.
func (s *Service) RecordRemoteEmojis(ctx context.Context, tx uow.Tx, ownerAccountID, domain string, emojis []RemoteEmojiInput) ([]string, error) {
	if len(emojis) == 0 {
		return nil, nil
	}
	if s.d.RemoteMedia == nil {
		return nil, errors.New("content: remote emoji need a remote media recorder")
	}
	var ids []string
	now := s.d.Clock.Now()
	for _, e := range emojis {
		if e.Shortcode == "" || e.ImageURL == "" {
			continue
		}
		uri := e.URI
		if !hasHost(uri, domain) {
			uri = ""
		}
		emoji, err := s.d.Repository.EnsureCustomEmoji(ctx, tx, content.CustomEmoji{
			ID: s.d.IDs.NewID(), Shortcode: e.Shortcode, Domain: domain, URI: uri, CreatedAt: now,
		})
		if err != nil {
			return nil, err
		}
		// The image is replaced only when its URL changes: the same image in the documents of
		// other accounts adds no revision and no media file of theirs.
		current, err := s.d.Repository.CurrentCustomEmojiImageURL(ctx, tx, emoji.ID)
		switch {
		case errors.Is(err, ErrNotFound), err == nil && current != e.ImageURL:
			m, err := s.d.RemoteMedia.RecordRemoteMedia(ctx, tx, RemoteMediaInput{
				OwnerAccountID: ownerAccountID, RemoteURL: e.ImageURL, DeclaredMediaType: content.MediaTypeImage, Purpose: content.PurposeEmoji,
			})
			if err != nil {
				return nil, err
			}
			err = s.d.Repository.InsertCustomEmojiImage(ctx, tx, content.CustomEmojiImage{
				ID: s.d.IDs.NewID(), EmojiID: emoji.ID, MediaID: m.MediaID, RevisedAt: now,
			})
			if err != nil {
				return nil, err
			}
		case err != nil:
			return nil, err
		}
		ids = append(ids, emoji.ID)
	}
	return sortedSet(ids), nil
}

// hasHost reports whether uri is an absolute URL whose host is domain.
func hasHost(uri, domain string) bool {
	u, err := url.Parse(uri)
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, domain)
}

// remoteAttachments records the first MaxAttachments attachments of a remote post as remote
// media files of the author, with their declared values (federation.md §9), and returns them
// with the number of the attachments left out.
func (s *Service) remoteAttachments(ctx context.Context, tx uow.Tx, in RemoteStatusInput) ([]content.Attachment, int, error) {
	omitted := max(in.OmittedAttachments, 0)
	if len(in.Attachments) == 0 {
		return nil, omitted, nil
	}
	if s.d.RemoteMedia == nil {
		return nil, 0, errors.New("content: remote statuses need a remote media recorder")
	}
	var out []content.Attachment
	for _, a := range in.Attachments {
		if a.URL == "" {
			continue
		}
		if len(out) == MaxAttachments {
			omitted++
			continue
		}
		description := truncateRunes(strings.TrimSpace(s.d.Sanitizer.PlainText(a.Description)), MaxMediaDescription)
		var focus *content.MediaFocus
		if a.Focus != nil {
			focus = &content.MediaFocus{X: clampFocus(a.Focus[0]), Y: clampFocus(a.Focus[1])}
		}
		m, err := s.d.RemoteMedia.RecordRemoteMedia(ctx, tx, RemoteMediaInput{
			OwnerAccountID: in.AuthorAccountID, RemoteURL: a.URL, DeclaredContentType: a.ContentType, DeclaredMediaType: content.DeclaredMediaType(a.Type, a.ContentType),
			Purpose: content.PurposeAttachment,
			Width:   max(a.Width, 0), Height: max(a.Height, 0), Blurhash: a.Blurhash, Focus: focus, Description: description,
		})
		if err != nil {
			return nil, 0, err
		}
		if slices.ContainsFunc(out, func(o content.Attachment) bool { return o.MediaID == m.MediaID }) {
			continue
		}
		out = append(out, content.Attachment{Position: len(out), MediaID: m.MediaID, Description: description})
	}
	return out, omitted, nil
}

// observePoll records the counts of a remote poll when they supersede the newest observation
// (content.PollTallyObservation.Supersedes) and emits PollTallyObserved.
func (s *Service) observePoll(ctx context.Context, tx uow.Tx, poll *content.Poll, in *RemotePollInput, now time.Time) error {
	if poll == nil || in == nil || len(in.Votes) < len(poll.Options) {
		return nil
	}
	o := content.PollTallyObservation{PollID: poll.ID, VotersCount: in.VotersCount, ObservedAt: now}
	for _, v := range in.Votes[:len(poll.Options)] {
		o.Votes = append(o.Votes, max(v, 0))
	}
	if o.VotersCount != nil && *o.VotersCount < 0 {
		o.VotersCount = nil
	}
	prev, err := s.d.Repository.LatestPollObservation(ctx, tx, poll.ID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if !o.Supersedes(prev) {
		return nil
	}
	o.ID = s.d.IDs.NewID()
	if err := s.d.Repository.InsertPollObservation(ctx, tx, o); err != nil {
		return err
	}
	return tx.Emit(ctx, PollTallyObserved{Observation: o})
}

// PollTallies returns the current counts of a poll, by position; a poll without counts has none.
func (s *Service) PollTallies(ctx context.Context, pollID string) ([]content.PollTally, error) {
	return s.d.Repository.FindPollTallies(ctx, pollID)
}

// CustomEmoji returns a custom emoji with its current image.
func (s *Service) CustomEmoji(ctx context.Context, id string) (content.CustomEmoji, content.CustomEmojiImage, error) {
	e, err := s.d.Repository.FindCustomEmoji(ctx, id)
	if err != nil {
		return content.CustomEmoji{}, content.CustomEmojiImage{}, err
	}
	img, err := s.d.Repository.FindCustomEmojiImage(ctx, id)
	return e, img, err
}

func sanitizeOptional(sanitize func(string) string, html string) string {
	if strings.TrimSpace(html) == "" {
		return ""
	}
	return sanitize(html)
}

func sortedSet(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '　' }

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func clampFocus(v float64) float64 { return min(max(v, -1), 1) }
