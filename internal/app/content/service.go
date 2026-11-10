// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/i18n"
)

var (
	// ErrAccountUnavailable reports an author that does not exist or is not active.
	ErrAccountUnavailable = errors.New("content: the account does not exist or is not active")
	// ErrRemoteAccount reports a remote author: these use cases cover local statuses only, and
	// remote statuses arrive through the federation module.
	ErrRemoteAccount = errors.New("content: the account is not local")
	// ErrPostingRestricted reports an author whose restrictions forbid posting and editing.
	ErrPostingRestricted = errors.New("content: the account may not post")
	// ErrInteractionRestricted reports an account whose restrictions forbid favouriting,
	// boosting, and replying.
	ErrInteractionRestricted = errors.New("content: the account may not interact")
	// ErrNotAuthor reports an edit or deletion of another account's status.
	ErrNotAuthor = errors.New("content: the status belongs to another account")
	// ErrReplyTargetNotFound reports a reply to a status that does not exist, is deleted, or
	// that the author may not see.
	ErrReplyTargetNotFound = errors.New("content: the status replied to is not found")
	// ErrEmptyText reports a status without text.
	ErrEmptyText = errors.New("content: the text is empty")
	// ErrTextTooLong reports a text and content warning longer than max_status_length.
	ErrTextTooLong = errors.New("content: the text is too long")
	// ErrInvalidText reports text that is not valid UTF-8 or contains a NUL character.
	ErrInvalidText = errors.New("content: the text is not valid")
	// ErrInvalidLanguage reports a language that is not a well-formed BCP 47 tag.
	ErrInvalidLanguage = errors.New("content: the language is not a valid BCP 47 tag")
	// ErrTooManyMedia reports more than MaxAttachments media files, or one file attached twice.
	ErrTooManyMedia = errors.New("content: too many media files")
	// ErrMediaNotFound reports an attachment that names no upload of the author: a file that
	// does not exist and another account's file fail alike.
	ErrMediaNotFound = errors.New("content: the media file does not exist or belongs to another account")
	// ErrMediaNotReady reports an attachment whose media file is still being processed or
	// failed processing.
	ErrMediaNotReady = errors.New("content: the media file is not processed")
	// ErrInvalidMediaDescription reports an alt text longer than MaxMediaDescription, or one
	// that is not valid UTF-8 or contains a NUL character.
	ErrInvalidMediaDescription = errors.New("content: the media description is not valid")
	// ErrMissingMediaDescription reports an attachment without an alt text while the instance
	// requires one (the require_media_description setting, FR-105).
	ErrMissingMediaDescription = errors.New("content: the media file has no description")
	// ErrPollWithMedia reports a post with both a poll and media attachments (FR-112).
	ErrPollWithMedia = errors.New("content: a post with a poll attaches no media")
	// ErrLocalVisibilityUnavailable reports a new local post on a one-person server, where local
	// means no more than private (ADR-0152 §5).
	ErrLocalVisibilityUnavailable = errors.New("content: a one-person server posts nothing new as local")
	// ErrPollNotEditable reports an edit of a status with a poll: the votes belong to the poll
	// of the revision they were cast in (FR-112).
	ErrPollNotEditable = errors.New("content: a status with a poll cannot be edited")
)

// TextTooLongError is the ErrTextTooLong of one post, with the limit in effect.
type TextTooLongError struct {
	// Length is the CountCharacters of the post, Max the max_status_length setting.
	Length, Max int
}

func (e *TextTooLongError) Error() string {
	return fmt.Sprintf("%v: %d characters, at most %d", ErrTextTooLong, e.Length, e.Max)
}

// Unwrap returns ErrTextTooLong.
func (e *TextTooLongError) Unwrap() error { return ErrTextTooLong }

// Limits of the media attached to a local status (FR-105).
const (
	// MaxAttachments is the number of media files one status may attach.
	MaxAttachments = 4
	// MaxMediaDescription is the number of characters of an alt text.
	MaxMediaDescription = 1500
)

// UndeterminedLanguage is the language of a status whose language is unknown (BCP 47 "und").
const UndeterminedLanguage = "und"

// Accounts is the part of the account module's public API that the content module reads: the
// author's locality and lifecycle state, and the accounts that a text mentions.
// *appaccount.Service implements it.
type Accounts interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
	// FindByIDs returns the accounts of ids that exist, by ID, as FindByID returns one, with one
	// query per part of an actor whatever the number of ids.
	FindByIDs(ctx context.Context, ids []string) (map[string]appaccount.Actor, error)
	FindByHandle(ctx context.Context, username, domain string) (appaccount.Actor, error)
}

// RemoteMentions finds the remote accounts that local text mentions (federation.md §6.2). The
// federation module's RemoteActors resolves handles and the account module reads profile pages;
// the platform joins them.
type RemoteMentions interface {
	// ResolveHandle returns the remote account with the handle username@domain: the stored one,
	// or the one that WebFinger leads to, fetched and stored. It does network I/O.
	ResolveHandle(ctx context.Context, username, domain string) (appaccount.Actor, error)
	// RemoteProfileURL returns the profile page (the actor's url) of a remote account, "" when
	// its document has none.
	RemoteProfileURL(ctx context.Context, accountID string) (string, error)
}

// Bounds of resolving the unknown remote handles of one text (federation.md §6.2): handles past
// MaxResolvedMentions, and those still unresolved after MentionResolveTimeout, stay text.
const (
	MaxResolvedMentions   = 5
	MentionResolveTimeout = 10 * time.Second
)

// Relationships is the part of the graph module's public API that the content module reads:
// the follows and blocks between two accounts. *appgraph.Service implements it.
type Relationships interface {
	Relationship(ctx context.Context, accountID, targetID string) (appgraph.Relationship, error)
}

// Deps are the ports and settings a Service uses.
type Deps struct {
	UnitOfWork    uow.UnitOfWork
	Repository    Repository
	Accounts      Accounts
	Relationships Relationships
	Restrictions  RestrictionPolicy
	// Media reads the media files that posts attach; nil accepts no attachments.
	Media MediaRepository
	// RemoteMentions resolves the remote handles that a text mentions; nil leaves every remote
	// handle text.
	RemoteMentions RemoteMentions
	// Sanitizer cleans the HTML of remote statuses (ADR-0019 §2); remote statuses need it.
	Sanitizer kernel.HTMLSanitizer
	// RemoteMedia records the attachments and custom emoji images of remote statuses.
	RemoteMedia RemoteMediaRecorder
	// Quotes stores the quotes of statuses (ADR-0069); nil records none.
	Quotes QuoteRepository
	// MaxStatusLength returns the current max_status_length instance setting (FR-101); it is
	// read per command, so that changes apply at once.
	MaxStatusLength func(ctx context.Context) (int, error)
	// RequireMediaDescription returns the current require_media_description instance setting
	// (FR-105), read per command; nil requires no alt text.
	RequireMediaDescription func(ctx context.Context) (bool, error)
	// SingleUser returns the current single_user instance setting (ADR-0152 §5), read per command:
	// a one-person server posts nothing new as local. Nil leaves it off.
	SingleUser func(ctx context.Context) (bool, error)
	// DefaultLanguage returns the default post language of a user (FR-110), used when a post
	// names none; nil or UndeterminedLanguage leaves the language undetermined.
	DefaultLanguage func(ctx context.Context, userID string) (string, error)
	// DefaultInteractionPolicy returns who may reply to, boost, and quote the new statuses of a
	// user (ADR-0155 §5), used for the kinds a status names no policy of; nil means
	// content.DefaultInteractionSettings.
	DefaultInteractionPolicy func(ctx context.Context, userID string) (content.InteractionSettings, error)
	// LocalEmojis returns the enabled local custom emoji that texts use as :shortcode:
	// ((*CustomEmojiService).LocalEmojiIDs, ADR-0051 §3); nil records no custom emoji.
	LocalEmojis func(ctx context.Context, texts ...string) ([]string, error)
	// StatusReview asks the decision hook local.status about new posts (ADR-0147); its zero
	// value asks nothing.
	StatusReview StatusReview
	// MediaReview acts on the decisions of the hook media.check on the files a post attaches; nil
	// acts on none.
	MediaReview *MediaReview
	Clock       kernel.Clock
	IDs         kernel.IDGenerator
	// Host is the serving domain of the status URIs and pages (ADR-0018).
	Host string
	// AccountDomain is the domain of local handles; @user@AccountDomain and @user@Host mention a
	// local account like @user. "" means Host.
	AccountDomain string
}

// Service runs the content module's use cases for local statuses: posting, editing, and
// deleting them.
type Service struct {
	d Deps
}

// NewService returns a Service on d.
func NewService(d Deps) *Service {
	if d.Restrictions == nil {
		d.Restrictions = NoRestrictions{}
	}
	if d.AccountDomain == "" {
		d.AccountDomain = d.Host
	}
	return &Service{d: d}
}

// Post is a status with its current revision.
type Post struct {
	Status   content.Status
	Revision content.Revision
	// ParentStored reports, for a status just stored, that the status it replies to was stored
	// before it (its conversation is the parent's); false for one that replies to nothing and on
	// every read.
	ParentStored bool
}

// TextInput is the content of a post or an edit.
type TextInput struct {
	// Text is the plain text the author typed (FR-103). It must not be blank.
	Text string
	// SpoilerText is the content warning, "" for none (FR-104).
	SpoilerText string
	// Sensitive marks the media as sensitive (FR-104).
	Sensitive bool
	// Language is a BCP 47 tag; "" means the author's default post language (FR-110).
	Language string
	// Media are the media files to attach, in order (FR-105): at most MaxAttachments, each
	// an upload of the author that finished processing.
	Media []MediaInput
}

// MediaInput is a media file to attach.
type MediaInput struct {
	MediaID string
	// Description is the alt text, "" for none.
	Description string
}

// CreateInput describes a new local status.
type CreateInput struct {
	TextInput
	AuthorAccountID string
	// Visibility is the requested visibility; the author's visibility ceiling may narrow it.
	Visibility content.Visibility
	// InReplyToID is the ID of the status replied to, "" for none (FR-106).
	InReplyToID string
	// Poll is the poll of the status, nil for none (FR-112). A status with a poll attaches no
	// media.
	Poll *content.PollInput
	// ClientID is the OAuth client that posts, "" when none is recorded.
	ClientID string
	// IdempotencyKey is the client API's Idempotency-Key, "" for none. A key that the author
	// already used returns the status posted with it.
	IdempotencyKey string
	// QuotedStatusID is the ID of the status quoted, "" for none (FR-114).
	QuotedStatusID string
	// InteractionPolicy is who may reply to, boost, and quote the status; the kinds it leaves out
	// take the author's defaults (ADR-0155 §5).
	InteractionPolicy InteractionPolicyChange
}

// Create posts a local status. The text becomes HTML with mentions of active accounts (local
// ones, and remote ones resolved before the transaction), hashtags, and URLs linked; attached media must be the author's own processed uploads
// (ErrMediaNotFound, ErrMediaNotReady); the visibility is capped by the author's visibility ceiling
// (data-model §5.4) and stored as capped. A reply must target a status the author may see.
// A poll (FR-112) must keep the limits of content.NewPoll (content.ErrInvalidPoll) and comes
// without media (ErrPollWithMedia); it is posted with a tally observation of no votes. A quote
// must name a status the author may see (ErrQuoteTargetNotFound) and may quote
// (ErrQuoteNotAllowed, ADR-0069 §3); the status gets the interaction policy asked for, with the
// author's defaults for the kinds it leaves out (ErrInvalidInteractionPolicy).
//
// Retrying with an IdempotencyKey that the author already used returns the status posted with it
// and changes nothing; when that status is deleted, it fails with ErrAlreadyDeleted. A key that
// is not valid fails with content.ErrInvalidIdempotencyKey.
//
// A new post then goes to the decision hook local.status, outside the transaction (status_review.go);
// a reject in force fails with a *RejectedByReviewError and writes nothing. On a one-person server
// a local post fails with ErrLocalVisibilityUnavailable (ADR-0152 §5).
func (s *Service) Create(ctx context.Context, in CreateInput) (Post, error) {
	if err := s.checkNewVisibility(ctx, in.Visibility); err != nil {
		return Post{}, err
	}
	if err := content.ValidateIdempotencyKey(in.IdempotencyKey); err != nil {
		return Post{}, err
	}
	author, err := s.localActive(ctx, in.AuthorAccountID)
	if err != nil {
		return Post{}, err
	}
	if in.IdempotencyKey != "" {
		if post, err := s.replay(ctx, in.AuthorAccountID, in.IdempotencyKey); !errors.Is(err, ErrNotFound) {
			return post, err
		}
	}
	p, err := s.prepareCreate(ctx, author, in)
	if err != nil {
		return Post{}, err
	}
	review, err := s.review(ctx, author, p)
	if err != nil {
		return Post{}, err
	}
	if review.Effect == apphook.Reject {
		return Post{}, &RejectedByReviewError{DecisionID: review.DecisionID, DecidedAt: review.DecidedAt}
	}
	verdict, err := s.verdict(ctx, review)
	if err != nil {
		return Post{}, err
	}
	var post Post
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		if post, err = s.CreateTx(ctx, tx, p); err != nil {
			return err
		}
		return s.reviewed(ctx, tx, verdict, post)
	})
	if errors.Is(err, ErrIdempotencyKeyUsed) {
		// A concurrent request with the same key won.
		return s.replay(ctx, in.AuthorAccountID, in.IdempotencyKey)
	}
	if err != nil {
		return Post{}, err
	}
	return post, nil
}

// checkNewVisibility fails with an error wrapping content.ErrInvalidVisibility for a visibility
// that is not one, and with ErrLocalVisibilityUnavailable for local on a one-person server
// (ADR-0152 §5).
func (s *Service) checkNewVisibility(ctx context.Context, v content.Visibility) error {
	if !v.Valid() {
		return fmt.Errorf("%w: %q", content.ErrInvalidVisibility, v)
	}
	if v != content.VisibilityLocal {
		return nil
	}
	on, err := s.singleUser(ctx)
	if err != nil {
		return err
	}
	if on {
		return ErrLocalVisibilityUnavailable
	}
	return nil
}

// singleUser reports whether the server is a one-person server (ADR-0152).
func (s *Service) singleUser(ctx context.Context) (bool, error) {
	if s.d.SingleUser == nil {
		return false, nil
	}
	on, err := s.d.SingleUser(ctx)
	if err != nil {
		return false, fmt.Errorf("content: instance single_user: %w", err)
	}
	return on, nil
}

// PreparedCreate is a new local status that PrepareCreate checked and built and CreateTx writes.
type PreparedCreate struct {
	st          content.Status
	local       content.LocalStatus
	rev         content.Revision
	tags        []content.Tag
	interaction content.InteractionSettings
	quote       *localQuote
	tally       *content.PollTallyObservation
	// media are the decisions of media.check on the attachments that CreateTx acts on.
	media []mediaVerdict
	now   time.Time
}

// PrepareCreate checks and builds a new local status as Create does, without writing it, so
// that another module writes it with CreateTx inside its own transaction: the start post of a
// voice chat room (ADR-0100 §9). It takes no Idempotency-Key (content.ErrInvalidIdempotencyKey).
func (s *Service) PrepareCreate(ctx context.Context, in CreateInput) (PreparedCreate, error) {
	if err := s.checkNewVisibility(ctx, in.Visibility); err != nil {
		return PreparedCreate{}, err
	}
	if in.IdempotencyKey != "" {
		return PreparedCreate{}, content.ErrInvalidIdempotencyKey
	}
	author, err := s.localActive(ctx, in.AuthorAccountID)
	if err != nil {
		return PreparedCreate{}, err
	}
	return s.prepareCreate(ctx, author, in)
}

// CreateTx writes inside tx the status that PrepareCreate or Create prepared, emitting
// StatusCreated, and returns it.
func (s *Service) CreateTx(ctx context.Context, tx uow.Tx, p PreparedCreate) (Post, error) {
	repo := s.d.Repository
	ins, err := s.insertStatus(ctx, tx, p.st, "")
	if err != nil {
		return Post{}, err
	}
	if err := repo.InsertLocalStatus(ctx, tx, p.local); err != nil {
		return Post{}, err
	}
	rev := p.rev
	if rev.TagIDs, err = ensureTags(ctx, tx, repo, p.tags); err != nil {
		return Post{}, err
	}
	if err := repo.InsertRevision(ctx, tx, rev); err != nil {
		return Post{}, err
	}
	if err := tx.Emit(ctx, StatusCreated{Status: p.st, Revision: rev, ConversationID: ins.ConversationID}); err != nil {
		return Post{}, err
	}
	if err := s.applyAttachmentVerdicts(ctx, tx, p.st.ID, p.media); err != nil {
		return Post{}, err
	}
	if s.d.Quotes != nil {
		if err := s.d.Quotes.ReplaceInteractionPolicy(ctx, tx, p.st.ID, nil, p.interaction.Policy()); err != nil {
			return Post{}, err
		}
	}
	if p.quote != nil {
		if err := s.recordLocalQuote(ctx, tx, p.st, *p.quote); err != nil {
			return Post{}, err
		}
	}
	if p.tally != nil {
		if err := repo.InsertPollObservation(ctx, tx, *p.tally); err != nil {
			return Post{}, err
		}
		if err := tx.Emit(ctx, PollTallyObserved{Observation: *p.tally}); err != nil {
			return Post{}, err
		}
	}
	return Post{Status: p.st, Revision: rev, ParentStored: ins.ParentStored}, nil
}

// prepareCreate checks and builds the status of in by author, outside any transaction.
func (s *Service) prepareCreate(ctx context.Context, author appaccount.Actor, in CreateInput) (PreparedCreate, error) {
	restrictions, err := s.postingRestrictions(ctx, in.AuthorAccountID)
	if err != nil {
		return PreparedCreate{}, err
	}
	if in.Poll != nil && len(in.Media) > 0 {
		return PreparedCreate{}, ErrPollWithMedia
	}
	body, err := s.prepareText(ctx, author, in.TextInput)
	if err != nil {
		return PreparedCreate{}, err
	}

	id := s.d.IDs.NewID()
	now := s.d.Clock.Now()
	var poll *content.Poll
	if in.Poll != nil {
		p, err := content.NewPoll(s.d.IDs.NewID(), *in.Poll, now)
		if err != nil {
			return PreparedCreate{}, err
		}
		poll = &p
	}
	st := content.Status{
		ID:              id,
		URI:             content.StatusURI(s.d.Host, id),
		AuthorAccountID: in.AuthorAccountID,
		IsLocal:         true,
		Visibility:      content.EffectiveVisibility(in.Visibility, restrictions.Author),
		PublishedAt:     now,
	}
	if in.InReplyToID != "" {
		if !restrictions.CanInteract() {
			return PreparedCreate{}, fmt.Errorf("account %s: %w", in.AuthorAccountID, ErrInteractionRestricted)
		}
		parent, err := s.replyTarget(ctx, in.AuthorAccountID, in.InReplyToID)
		if err != nil {
			return PreparedCreate{}, err
		}
		st.InReplyToURI = parent.URI
	}
	interaction, err := s.newInteractionSettings(ctx, author, in.InteractionPolicy)
	if err != nil {
		return PreparedCreate{}, err
	}
	var quote *localQuote
	if in.QuotedStatusID != "" {
		if !restrictions.CanInteract() {
			return PreparedCreate{}, fmt.Errorf("account %s: %w", in.AuthorAccountID, ErrInteractionRestricted)
		}
		q, err := s.quoteTarget(ctx, in.AuthorAccountID, in.QuotedStatusID, st.Visibility)
		if err != nil {
			return PreparedCreate{}, err
		}
		quote = &q
	}
	local := content.LocalStatus{
		StatusID: id, AuthorAccountID: in.AuthorAccountID, ClientID: in.ClientID, IdempotencyKey: in.IdempotencyKey,
	}
	rev := body.revision(s.d.IDs.NewID(), id, 1, now)
	rev.Poll = poll
	tags := body.tags(s.d.IDs, now)
	var tally *content.PollTallyObservation
	if poll != nil {
		// A local poll starts with no votes; every vote records its new counts (pollTally).
		var none int64
		tally = &content.PollTallyObservation{
			ID: s.d.IDs.NewID(), PollID: poll.ID, VotersCount: &none, ObservedAt: now, Votes: make([]int64, len(poll.Options)),
		}
	}
	verdicts, err := s.attachmentVerdicts(ctx, in.AuthorAccountID, rev.Attachments, nil)
	if err != nil {
		return PreparedCreate{}, err
	}
	return PreparedCreate{st: st, local: local, rev: rev, tags: tags, interaction: interaction, quote: quote, tally: tally, media: verdicts, now: now}, nil
}

// Replay returns the post that an author posted with an Idempotency-Key, so that a retry is
// answered before the checks of a new post (such as its rate limit). It fails with ErrNotFound
// when the author posted nothing with key, with ErrAlreadyDeleted when that post is deleted, and
// with content.ErrInvalidIdempotencyKey when key is not valid.
func (s *Service) Replay(ctx context.Context, authorID, key string) (Post, error) {
	if err := content.ValidateIdempotencyKey(key); err != nil {
		return Post{}, err
	}
	if key == "" {
		return Post{}, ErrNotFound
	}
	if _, err := s.localActive(ctx, authorID); err != nil {
		return Post{}, err
	}
	return s.replay(ctx, authorID, key)
}

// replay returns the post that an author posted with an Idempotency-Key.
func (s *Service) replay(ctx context.Context, authorID, key string) (Post, error) {
	ls, err := s.d.Repository.FindLocalStatusByIdempotencyKey(ctx, authorID, key)
	if err != nil {
		return Post{}, err
	}
	post, err := s.Find(ctx, ls.StatusID)
	if errors.Is(err, ErrNotFound) {
		return Post{}, fmt.Errorf("status %s: %w", ls.StatusID, ErrAlreadyDeleted)
	}
	return post, err
}

// replyTarget returns the status that authorID replies to, provided that authorID may see it.
// A status that does not exist, is deleted, or is hidden from the author fails alike with
// ErrReplyTargetNotFound, so that a reply does not reveal it.
func (s *Service) replyTarget(ctx context.Context, authorID, statusID string) (content.Status, error) {
	st, _, err := s.visibleTo(ctx, authorID, statusID)
	return st, err
}

// EditInput describes an edit of a local status.
type EditInput struct {
	TextInput
	// AccountID is the account editing; it must be the author.
	AccountID string
	StatusID  string
	// DetachQuote takes the status's quote away for good (ADR-0069 §3).
	DetachQuote bool
}

// Edit records a new revision of a local status with new content (FR-109), checking the text
// and the attached media as Create does, and asking local.status about the edit as Create asks
// about a post: a reject fails with ErrRejectedByReview and leaves the post as it was, with no
// decision to appeal (ADR-0147 §2). The visibility,
// author, and reply target never change. The new revision becomes the head. A status that does
// not exist or is deleted fails with ErrNotFound; another account's status with ErrNotAuthor;
// a status with a poll with ErrPollNotEditable.
func (s *Service) Edit(ctx context.Context, in EditInput) (Post, error) {
	author, err := s.localActive(ctx, in.AccountID)
	if err != nil {
		return Post{}, err
	}
	st, err := s.ownStatus(ctx, in.AccountID, in.StatusID)
	if err != nil {
		return Post{}, err
	}
	current, err := s.d.Repository.FindCurrentRevision(ctx, st.ID)
	if err != nil {
		return Post{}, err
	}
	if current.Poll != nil {
		// The votes are of the poll of the current revision; a new revision would leave them.
		return Post{}, fmt.Errorf("status %s: %w", st.ID, ErrPollNotEditable)
	}
	if _, err := s.postingRestrictions(ctx, in.AccountID); err != nil {
		return Post{}, err
	}
	body, err := s.prepareText(ctx, author, in.TextInput)
	if err != nil {
		return Post{}, err
	}
	now := s.d.Clock.Now()
	rev := body.revision(s.d.IDs.NewID(), st.ID, 0, now)
	tags := body.tags(s.d.IDs, now)
	// The files the current revision attaches were acted on when it was stored.
	verdicts, err := s.attachmentVerdicts(ctx, in.AccountID, rev.Attachments, current.Attachments)
	if err != nil {
		return Post{}, err
	}
	review, err := s.reviewEdit(ctx, author, st, rev, tags, in.DetachQuote)
	if err != nil {
		return Post{}, err
	}
	if review.Effect == apphook.Reject {
		return Post{}, fmt.Errorf("status %s: %w", st.ID, ErrRejectedByReview)
	}
	verdict, err := s.verdict(ctx, review)
	if err != nil {
		return Post{}, err
	}

	repo := s.d.Repository
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		// The lock makes a deletion that another transaction is recording visible before the
		// check, so that the edit of a status being deleted fails.
		if err := repo.LockStatusKeys(ctx, tx, statusLockKeys(content.Status{ID: st.ID}, true)); err != nil {
			return err
		}
		deleted, err := repo.IsDeleted(ctx, tx, st.ID)
		if err != nil {
			return err
		}
		if deleted {
			return fmt.Errorf("status %s: %w", st.ID, ErrNotFound)
		}
		head, err := repo.Head(ctx, tx, st.ID)
		if err != nil {
			return err
		}
		rev.Seq = head.Seq + 1
		if rev.TagIDs, err = ensureTags(ctx, tx, repo, tags); err != nil {
			return err
		}
		if err := repo.InsertRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := tx.Emit(ctx, StatusRevised{Status: st, Revision: rev}); err != nil {
			return err
		}
		if err := s.applyAttachmentVerdicts(ctx, tx, st.ID, verdicts); err != nil {
			return err
		}
		if err := s.reviewed(ctx, tx, verdict, Post{Status: st, Revision: rev}); err != nil {
			return err
		}
		if in.DetachQuote {
			return s.detachQuote(ctx, tx, st)
		}
		return nil
	})
	if err != nil {
		return Post{}, err
	}
	return Post{Status: st, Revision: rev}, nil
}

// Delete deletes a local status by its author (FR-108). It records the deletion, which stays as
// the tombstone of the URI, and removes the status from every projection at once. A status that
// does not exist or is already deleted fails with ErrNotFound; another account's status with
// ErrNotAuthor.
func (s *Service) Delete(ctx context.Context, accountID, statusID string) (content.Deletion, error) {
	st, err := s.ownStatus(ctx, accountID, statusID)
	if err != nil {
		return content.Deletion{}, err
	}
	d := content.Deletion{
		StatusID: st.ID, URI: st.URI, AuthorAccountID: st.AuthorAccountID, Origin: content.DeletedByAuthor, DeletedAt: s.d.Clock.Now(),
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertDeletion(ctx, tx, d); err != nil {
			return err
		}
		return tx.Emit(ctx, StatusDeleted{Deletion: d, Status: st})
	})
	if errors.Is(err, ErrAlreadyDeleted) {
		return content.Deletion{}, fmt.Errorf("status %s: %w", st.ID, ErrNotFound)
	}
	if err != nil {
		return content.Deletion{}, err
	}
	return d, nil
}

// Find returns a status that is not deleted with its current revision. It does not decide who
// may see the status; callers check content.CanView.
func (s *Service) Find(ctx context.Context, statusID string) (Post, error) {
	st, err := s.d.Repository.FindStatus(ctx, statusID)
	if err != nil {
		return Post{}, err
	}
	rev, err := s.d.Repository.FindCurrentRevision(ctx, statusID)
	if err != nil {
		return Post{}, err
	}
	return Post{Status: st, Revision: rev}, nil
}

// FindMany is Find of each of ids, by ID, with one query for the statuses and one for their
// current revisions; the missing and deleted ones are left out.
func (s *Service) FindMany(ctx context.Context, ids []string) (map[string]Post, error) {
	out := make(map[string]Post, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	sts, err := s.d.Repository.FindStatuses(ctx, ids)
	if err != nil {
		return nil, err
	}
	revs, err := s.d.Repository.FindCurrentRevisions(ctx, ids)
	if err != nil {
		return nil, err
	}
	for id, st := range sts {
		if rev, ok := revs[id]; ok {
			out[id] = Post{Status: st, Revision: rev}
		}
	}
	return out, nil
}

// FindStatuses is FindStatus of each of ids that exists, by ID, with one query.
func (s *Service) FindStatuses(ctx context.Context, ids []string) (map[string]Status, error) {
	return s.d.Repository.FindStatuses(ctx, ids)
}

// Revisions returns the revisions of a status, oldest first, without their children: the edit
// history (FR-109). Like Find, it does not decide who may see them.
func (s *Service) Revisions(ctx context.Context, statusID string) ([]content.Revision, error) {
	return s.d.Repository.ListRevisions(ctx, statusID)
}

// localActive returns a local, active account with a handle.
func (s *Service) localActive(ctx context.Context, id string) (appaccount.Actor, error) {
	a, err := s.d.Accounts.FindByID(ctx, id)
	if errors.Is(err, appaccount.ErrNotFound) {
		return appaccount.Actor{}, fmt.Errorf("account %s: %w", id, ErrAccountUnavailable)
	}
	if err != nil {
		return appaccount.Actor{}, err
	}
	if !a.Account.IsLocal() {
		return appaccount.Actor{}, fmt.Errorf("account %s: %w", id, ErrRemoteAccount)
	}
	if !a.Lifecycle.Active() || a.Handle == nil || a.Local == nil {
		return appaccount.Actor{}, fmt.Errorf("account %s: %w", id, ErrAccountUnavailable)
	}
	return a, nil
}

// ownStatus returns a local status of accountID.
func (s *Service) ownStatus(ctx context.Context, accountID, statusID string) (content.Status, error) {
	st, err := s.d.Repository.FindStatus(ctx, statusID)
	if err != nil {
		return content.Status{}, err
	}
	if st.AuthorAccountID != accountID || !st.IsLocal {
		return content.Status{}, fmt.Errorf("status %s: %w", statusID, ErrNotAuthor)
	}
	return st, nil
}

// PostingLimits returns what the account's current restrictions let it post: the visibility
// ceiling that Create applies and the visibilities that a new status keeps as asked for
// (data-model §5.4), so that a client can say so before posting. A one-person server leaves local
// out of those visibilities, since Create refuses it (ADR-0152 §5).
func (s *Service) PostingLimits(ctx context.Context, accountID string) (PostingLimits, error) {
	r, err := s.d.Restrictions.Restrictions(ctx, accountID)
	if err != nil {
		return PostingLimits{}, err
	}
	visibilities := content.PostingVisibilities(r.Author)
	// A one-person server posts nothing new as local (ADR-0152 §5).
	on, err := s.singleUser(ctx)
	if err != nil {
		return PostingLimits{}, err
	}
	if on {
		visibilities = slices.DeleteFunc(visibilities, func(v content.Visibility) bool { return v == content.VisibilityLocal })
	}
	return PostingLimits{VisibilityCeiling: r.VisibilityCeiling(), Visibilities: visibilities}, nil
}

func (s *Service) postingRestrictions(ctx context.Context, accountID string) (Restrictions, error) {
	r, err := s.d.Restrictions.Restrictions(ctx, accountID)
	if err != nil {
		return Restrictions{}, err
	}
	if !r.CanPost() {
		return Restrictions{}, fmt.Errorf("account %s: %w", accountID, ErrPostingRestricted)
	}
	return r, nil
}

// preparedText is validated, rendered content, ready to be stored as a revision.
type preparedText struct {
	in          TextInput
	language    string
	attachments []content.Attachment
	html        string
	mentions    []string // account IDs, sorted
	hashtags    []Hashtag
	emojis      []string // custom emoji IDs, sorted
}

// CountCharacters returns the length of a post as max_status_length counts it: the characters
// (Unicode code points) of the text and the content warning.
func CountCharacters(text, spoilerText string) int {
	return utf8.RuneCountInString(text) + utf8.RuneCountInString(spoilerText)
}

// prepareText validates the content against the instance settings, resolves its mentions to
// active accounts, and renders it. It runs before the transaction, since resolving a remote
// handle may fetch it.
func (s *Service) prepareText(ctx context.Context, author appaccount.Actor, in TextInput) (preparedText, error) {
	for _, v := range []string{in.Text, in.SpoilerText} {
		if !content.ValidText(v) {
			return preparedText{}, ErrInvalidText
		}
	}
	if strings.TrimSpace(in.Text) == "" {
		return preparedText{}, ErrEmptyText
	}
	limit, err := s.d.MaxStatusLength(ctx)
	if err != nil {
		return preparedText{}, err
	}
	if n := CountCharacters(in.Text, in.SpoilerText); n > limit {
		return preparedText{}, &TextTooLongError{Length: n, Max: limit}
	}
	language, err := s.language(ctx, author, in.Language)
	if err != nil {
		return preparedText{}, err
	}
	attachments, err := s.attachments(ctx, author.Account.ID, in.Media)
	if err != nil {
		return preparedText{}, err
	}

	parsed := ParseText(in.Text)
	html, mentions, err := s.render(ctx, parsed)
	if err != nil {
		return preparedText{}, err
	}
	var emojis []string
	if s.d.LocalEmojis != nil {
		if emojis, err = s.d.LocalEmojis(ctx, in.Text, in.SpoilerText); err != nil {
			return preparedText{}, err
		}
	}
	return preparedText{
		in:          in,
		language:    language,
		attachments: attachments,
		html:        html,
		mentions:    mentions,
		hashtags:    parsed.Hashtags(),
		emojis:      emojis,
	}, nil
}

// render resolves the mentions of parsed text to active accounts and renders it. It returns
// the HTML and the IDs of the mentioned accounts, sorted. It may do network I/O through
// RemoteMentions, so it never runs inside a transaction.
func (s *Service) render(ctx context.Context, parsed ParsedText) (string, []string, error) {
	targets := map[MentionHandle]MentionTarget{}
	var mentions []string
	r := &mentionResolver{s: s}
	defer r.close()
	if err := r.preload(ctx, parsed.Mentions()); err != nil {
		return "", nil, err
	}
	for _, h := range parsed.Mentions() {
		target, id, ok, err := r.resolve(ctx, h)
		if err != nil {
			return "", nil, err
		}
		if !ok {
			continue
		}
		targets[h] = target
		if !slices.Contains(mentions, id) {
			mentions = append(mentions, id)
		}
	}
	slices.Sort(mentions)
	return parsed.HTML(RenderOptions{Host: s.d.Host, Mentions: targets}), mentions, nil
}

// RenderProfileText renders the typed note or field value of a local profile to HTML through
// the same rules as the text of a post (FR-006, ADR-0019 §2): mentions of active accounts,
// hashtags, and URLs become links and everything else is escaped. It implements the
// account module's RenderProfileText.
func (s *Service) RenderProfileText(ctx context.Context, text string) (string, error) {
	html, _, err := s.render(ctx, ParseText(text))
	return html, err
}

// attachments checks the media files that authorID attaches (FR-105): at most MaxAttachments
// distinct files, each an upload of authorID whose newest processing result is ready, with a
// valid alt text, which must not be blank while the instance requires alt text.
func (s *Service) attachments(ctx context.Context, authorID string, in []MediaInput) ([]content.Attachment, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > MaxAttachments {
		return nil, fmt.Errorf("%w: %d, at most %d", ErrTooManyMedia, len(in), MaxAttachments)
	}
	requireDescription := false
	if s.d.RequireMediaDescription != nil {
		var err error
		if requireDescription, err = s.d.RequireMediaDescription(ctx); err != nil {
			return nil, err
		}
	}
	out := make([]content.Attachment, 0, len(in))
	for i, m := range in {
		if slices.ContainsFunc(in[:i], func(o MediaInput) bool { return o.MediaID == m.MediaID }) {
			return nil, fmt.Errorf("%w: %s is attached twice", ErrTooManyMedia, m.MediaID)
		}
		d := m.Description
		if !content.ValidText(d) || utf8.RuneCountInString(d) > MaxMediaDescription {
			return nil, fmt.Errorf("media %s: %w", m.MediaID, ErrInvalidMediaDescription)
		}
		if requireDescription && strings.TrimSpace(d) == "" {
			return nil, fmt.Errorf("media %s: %w", m.MediaID, ErrMissingMediaDescription)
		}
		if err := s.checkMedia(ctx, authorID, m.MediaID); err != nil {
			return nil, err
		}
		out = append(out, content.Attachment{Position: i, MediaID: m.MediaID, Description: d})
	}
	return out, nil
}

// checkMedia fails unless mediaID is an upload of authorID that was processed successfully.
func (s *Service) checkMedia(ctx context.Context, authorID, mediaID string) error {
	notFound := fmt.Errorf("media %s: %w", mediaID, ErrMediaNotFound)
	if s.d.Media == nil {
		return notFound
	}
	file, err := s.d.Media.FindMediaFile(ctx, mediaID)
	if errors.Is(err, ErrNotFound) {
		return notFound
	}
	if err != nil {
		return err
	}
	if file.OwnerAccountID != authorID || file.Origin != content.MediaOriginUpload {
		return notFound
	}
	result, err := s.d.Media.FindLatestProcessingResult(ctx, mediaID)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("media %s: %w", mediaID, ErrMediaNotReady)
	}
	if err != nil {
		return err
	}
	if result.Outcome != content.MediaReady {
		return fmt.Errorf("media %s: %w", mediaID, ErrMediaNotReady)
	}
	return nil
}

// language canonicalizes the requested language, falling back to the author's default.
func (s *Service) language(ctx context.Context, author appaccount.Actor, requested string) (string, error) {
	if requested == "" && s.d.DefaultLanguage != nil {
		def, err := s.d.DefaultLanguage(ctx, author.Local.UserID)
		if err != nil {
			return "", err
		}
		requested = def
	}
	if requested == "" {
		return UndeterminedLanguage, nil
	}
	tag := i18n.Canonicalize(requested)
	if tag == "" {
		return "", fmt.Errorf("%w: %q", ErrInvalidLanguage, requested)
	}
	return tag, nil
}

// revision returns the revision holding the prepared content; its TagIDs are set in the
// transaction.
func (p preparedText) revision(id, statusID string, seq int, at time.Time) content.Revision {
	return content.Revision{
		ID:                  id,
		StatusID:            statusID,
		Seq:                 seq,
		SpoilerText:         p.in.SpoilerText,
		Sensitive:           p.in.Sensitive,
		Language:            p.language,
		RevisedAt:           at,
		Bodies:              []content.Body{{Language: p.language, ContentHTML: p.html}},
		Source:              &content.Source{Text: p.in.Text, Format: content.SourceTextPlain},
		Attachments:         p.attachments,
		MentionedAccountIDs: p.mentions,
		EmojiIDs:            p.emojis,
	}
}

// tags returns the tags to ensure, with IDs generated outside the transaction.
func (p preparedText) tags(ids kernel.IDGenerator, at time.Time) []content.Tag {
	out := make([]content.Tag, 0, len(p.hashtags))
	for _, h := range p.hashtags {
		out = append(out, content.Tag{ID: ids.NewID(), Name: h.Name, NameNormalized: h.NameNormalized, CreatedAt: at})
	}
	return out
}

// ensureTags stores the tags that do not exist yet and returns the IDs of all of them, sorted.
func ensureTags(ctx context.Context, tx uow.Tx, repo Repository, tags []content.Tag) ([]string, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	stored, err := repo.EnsureTags(ctx, tx, tags)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(tags))
	for _, t := range tags {
		st, ok := stored[t.NameNormalized]
		if !ok {
			return nil, fmt.Errorf("tag %q after ensuring it: %w", t.NameNormalized, ErrNotFound)
		}
		ids = append(ids, st.ID)
	}
	slices.Sort(ids)
	return ids, nil
}

// profileURL returns the HTML profile page of a local account on host (ADR-0018).
func profileURL(host, username string) string { return "https://" + host + "/@" + username }
