// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Limits on a received document (federation.md §4). A document over any of them cannot be
// normalized.
const (
	MaxDepth         = 16        // nesting depth of JSON objects and arrays
	MaxTags          = 100       // entries of one tag property
	MaxRecipients    = 1000      // to and cc together, per object or activity
	MaxBodyBytes     = 100 << 10 // one HTML value: content, a contentMap entry, a summary
	MaxAttachments   = 4         // attachments kept per object; the rest are dropped
	MaxItems         = 100       // items kept per collection or collection page; the rest are dropped
	MaxShortcode     = 128       // characters of a custom emoji's shortcode; a longer emoji is dropped (ADR-0051)
	UndeterminedLang = "und"     // BCP 47 "undetermined"
)

var (
	// ErrMalformed reports a document that is not a JSON object or has no usable type.
	ErrMalformed = errors.New("federation: malformed AS2 document")
	// ErrLimitExceeded reports a document over one of the limits.
	ErrLimitExceeded = errors.New("federation: AS2 document exceeds a limit")
	// ErrInvalidIRI reports a value that is not an absolute IRI with a valid host.
	ErrInvalidIRI = errors.New("federation: invalid IRI")
)

// IRI is an absolute IRI with a lower-case scheme and a canonical host (IDNA A-labels, see
// NormalizeIRI), so that equal hosts compare equal. The empty IRI means "absent".
type IRI string

// PublicIRI is the special collection that addresses everyone (ActivityPub §5.6). Every
// spelling a document uses for it is normalized to this value.
const PublicIRI IRI = "https://www.w3.org/ns/activitystreams#Public"

// Origin returns scheme://host[:port] of i, or "" when i is not an absolute IRI.
func (i IRI) Origin() string {
	u, err := url.Parse(string(i))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// Host returns the host (without port) of i, or "".
func (i IRI) Host() string {
	u, err := url.Parse(string(i))
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// SameOrigin reports whether a and b are absolute IRIs with the same origin.
func SameOrigin(a, b IRI) bool {
	o := a.Origin()
	return o != "" && o == b.Origin()
}

// ActivityType is a known activity type (federation.md §3.4).
type ActivityType string

// Known activity types.
const (
	ActivityCreate     ActivityType = "Create"
	ActivityUpdate     ActivityType = "Update"
	ActivityDelete     ActivityType = "Delete"
	ActivityFollow     ActivityType = "Follow"
	ActivityAccept     ActivityType = "Accept"
	ActivityReject     ActivityType = "Reject"
	ActivityUndo       ActivityType = "Undo"
	ActivityAnnounce   ActivityType = "Announce"
	ActivityLike       ActivityType = "Like"
	ActivityEmojiReact ActivityType = "EmojiReact"
	ActivityBlock      ActivityType = "Block"
	ActivityFlag       ActivityType = "Flag"
	ActivityAdd        ActivityType = "Add"
	ActivityRemove     ActivityType = "Remove"
	ActivityMove       ActivityType = "Move"
	// ActivityQuoteRequest asks the author of a post to approve a quote of it (FEP-044f,
	// ADR-0069 §3): object is the quoted post, instrument the quoting one.
	ActivityQuoteRequest ActivityType = "QuoteRequest"
	// ActivityUnknown is any other type of a document with an actor; RawType keeps the name.
	ActivityUnknown ActivityType = ""
)

// ObjectType is a known object type.
type ObjectType string

// Known object types. Article, Page, Event, Image, Video, and Audio are converted to a Note-like
// body (Object.Converted).
const (
	ObjectNote      ObjectType = "Note"
	ObjectQuestion  ObjectType = "Question"
	ObjectArticle   ObjectType = "Article"
	ObjectPage      ObjectType = "Page"
	ObjectEvent     ObjectType = "Event"
	ObjectImage     ObjectType = "Image"
	ObjectVideo     ObjectType = "Video"
	ObjectAudio     ObjectType = "Audio"
	ObjectTombstone ObjectType = "Tombstone"
	// ObjectQuoteAuthorization is the stamp by which the author of a post approves a quote of it
	// (FEP-044f, ADR-0069 §2).
	ObjectQuoteAuthorization ObjectType = "QuoteAuthorization"
	// ObjectUnknown is any other type of a document without an actor; RawType keeps the name.
	ObjectUnknown ObjectType = ""
)

// ActorType is a known actor type.
type ActorType string

// Known actor types.
const (
	ActorPerson       ActorType = "Person"
	ActorService      ActorType = "Service"
	ActorApplication  ActorType = "Application"
	ActorGroup        ActorType = "Group"
	ActorOrganization ActorType = "Organization"
)

// Document is a normalized AS2 document: exactly one of the fields is set.
type Document struct {
	Activity   *Activity
	Object     *Object
	Actor      *Actor
	Collection *Collection
}

// ID returns the id of whichever document is set.
func (d *Document) ID() IRI {
	switch {
	case d == nil:
		return ""
	case d.Activity != nil:
		return d.Activity.ID
	case d.Object != nil:
		return d.Object.ID
	case d.Actor != nil:
		return d.Actor.ID
	case d.Collection != nil:
		return d.Collection.ID
	}
	return ""
}

// Ref is a property that refers to another document: an IRI, a Link, or an embedded document.
// ID is the referred IRI in every case. The embedded document is kept with a flag saying whether
// it has the same origin as the document that embeds it (or no id of its own); only then may its
// contents be trusted (federation.md §4, ActivityPub appendix B.1). Otherwise it must be fetched
// from ID.
type Ref struct {
	ID         IRI
	embedded   *Document
	sameOrigin bool
}

// Trusted returns the embedded document when its contents may be used, or nil.
func (r Ref) Trusted() *Document {
	if r.sameOrigin {
		return r.embedded
	}
	return nil
}

// Untrusted returns the embedded document regardless of its origin, or nil. Use it only for
// hints that are verified elsewhere, such as matching an Accept to a pending Follow.
func (r Ref) Untrusted() *Document { return r.embedded }

// Distrusted returns r with its embedded document no longer trusted: the Ref of an activity whose
// signer is not its actor (Trust TrustRefetch), whose embedded documents nobody vouches for.
func (r Ref) Distrusted() Ref {
	r.sameOrigin = false
	return r
}

// FetchedRef returns the Ref to a document fetched from its origin, whose contents are trusted.
func FetchedRef(doc *Document) Ref {
	return Ref{ID: doc.ID(), embedded: doc, sameOrigin: true}
}

// Audience is the addressing of an object or activity. bto and bcc are ignored: a sender must
// strip them before delivery.
type Audience struct {
	To []IRI
	Cc []IRI
}

// Activity is a normalized activity.
type Activity struct {
	ID        IRI
	Type      ActivityType
	RawType   string // the first type name of the document, for logs
	Actor     IRI
	Objects   []Ref // object; Flag carries several
	Target    IRI
	Audience  Audience
	Published time.Time // zero when absent
	// Content is the plain text of content: the comment of a Flag, the emoji of an EmojiReact.
	Content string
	// Reaction is the emoji of a Misskey reaction (_misskey_reaction on a Like) or of an
	// EmojiReact; empty for a plain Like.
	Reaction string
	Emojis   []Emoji
	// Instrument is the instrument of a QuoteRequest: the quoting post.
	Instrument Ref
	// Result is the result of an Accept: the QuoteAuthorization of an accepted QuoteRequest.
	Result IRI
}

// Object returns the first object of a, or the zero Ref.
func (a *Activity) Object() Ref {
	if len(a.Objects) == 0 {
		return Ref{}
	}
	return a.Objects[0]
}

// Body is the HTML of a post in one language. HTML is as received; it must go through the
// server-side sanitizer before it is stored (federation.md §5).
type Body struct {
	Language string // canonical BCP 47 tag, or UndeterminedLang
	HTML     string
}

// Mention is a Mention tag.
type Mention struct {
	Href IRI
	Name string
}

// Emoji is a custom emoji tag.
type Emoji struct {
	ID        IRI
	Shortcode string // without colons
	IconURL   IRI
}

// Link is a normalized AS2 Link (or a bare IRI used where a Link is allowed).
type Link struct {
	Href      IRI
	MediaType string
	Name      string
}

// Attachment is a media attachment.
type Attachment struct {
	Type       string // Document, Image, Video, or Audio
	URL        IRI
	MediaType  string
	Name       string // alt text
	Blurhash   string
	FocalPoint *[2]float64
	Width      int
	Height     int
}

// PollOption is one choice of a Question.
type PollOption struct {
	Name  string
	Votes int
}

// Poll is the poll of a Question.
type Poll struct {
	Multiple    bool // anyOf instead of oneOf
	Options     []PollOption
	EndTime     time.Time
	Closed      time.Time
	VotersCount int
}

// Object is a normalized object: a post, a converted post, or a Tombstone.
type Object struct {
	ID           IRI
	Type         ObjectType
	RawType      string
	Converted    bool // Type is one of the converted types; Bodies holds the converted body
	AttributedTo IRI
	InReplyTo    IRI
	Audience     Audience
	Published    time.Time
	Updated      time.Time
	URL          IRI    // display URL (the first text/html url)
	Bodies       []Body // one per language, in document order
	SpoilerText  string // plain text
	Sensitive    bool
	Mentions     []Mention
	Hashtags     []string // without the leading '#'
	Emojis       []Emoji
	Attachments  []Attachment
	// OmittedAttachments is the number of media attachments after the first MaxAttachments,
	// which are left out.
	OmittedAttachments int
	Poll               *Poll
	// Name is the name of a post that is not converted, as received, "" for none: the title of
	// the choice that a Note in reply to a Question votes for (federation.md §3.4).
	Name string
	// Replies is the collection of the replies to the object, the zero Ref when it declares none.
	Replies Ref
	// Quote is the post that the object quotes, nil for none (FR-114, ADR-0069 §2).
	Quote *Quote
	// CanQuote is the interactionPolicy.canQuote of a post, nil when it declares none: the
	// actors and collections whose quotes are approved automatically or by hand.
	CanQuote *CanQuote
	// InteractingObject and InteractionTarget are the quote and the quoted post of a
	// QuoteAuthorization (FEP-044f).
	InteractingObject IRI
	InteractionTarget IRI
}

// CanQuote is who may quote a post (FEP-044f, GoToSocial's interaction policies).
type CanQuote struct {
	AutomaticApproval []IRI
	ManualApproval    []IRI
}

// Quote is the quoted post of a post (ADR-0069 §2).
type Quote struct {
	// Target is the IRI of the quoted post.
	Target IRI
	// Consent is whether the post names Target with FEP-044f's quote, which its author must
	// approve; a quote in another spelling (Misskey's _misskey_quote, quoteUrl, Fedibird's
	// quoteUri, or an FEP-e232 Link) is a legacy quote, which carries no approval.
	Consent bool
	// Authorization is the quoteAuthorization of a Consent quote: the IRI of the
	// QuoteAuthorization that approves it, "" for none.
	Authorization IRI
}

// Collection is a normalized Collection, OrderedCollection, CollectionPage, or
// OrderedCollectionPage (AS2 Core §5.5): the items of a collection or of one of its pages, and
// the links to its pages.
type Collection struct {
	ID IRI
	// Page is set for a CollectionPage or OrderedCollectionPage.
	Page bool
	// Items are the items (or orderedItems) in document order, at most MaxItems of them.
	Items []Ref
	// First is the first page of a collection; Next the page after a page.
	First Ref
	Next  Ref
}

// PublicKey is a publicKey entry of an actor.
type PublicKey struct {
	ID    IRI
	Owner IRI
	PEM   string
}

// Field is a PropertyValue of an actor's profile. Value is HTML as received.
type Field struct {
	Name  string
	Value string
}

// Actor is a normalized actor.
type Actor struct {
	ID                        IRI
	Type                      ActorType
	PreferredUsername         string
	Name                      string // plain text (AS2 name carries no markup)
	Summary                   string // HTML as received
	URL                       IRI
	Inbox                     IRI
	Outbox                    IRI
	SharedInbox               IRI
	Followers                 IRI
	Following                 IRI
	Featured                  IRI
	Icon                      IRI
	Image                     IRI
	ManuallyApprovesFollowers bool
	Discoverable              bool
	// Indexable is Mastodon's toot:indexable: whether the actor's public posts may be indexed
	// for search (ADR-0057).
	Indexable bool
	// AttributionDomains is Mastodon's toot:attributionDomains: the domains whose pages may
	// name the actor as their fediverse:creator (ADR-0062 §5), lowercase.
	AttributionDomains []string
	// AlsoKnownAs is as:alsoKnownAs: the other actors that the actor declares as its own, at
	// most MaxItems. A Move to the actor is accepted only from one of them (ADR-0089).
	AlsoKnownAs []IRI
	// MovedTo is as:movedTo: the actor that the actor moved to, "" when it has not moved.
	MovedTo    IRI
	PublicKeys []PublicKey
	Fields     []Field
	Emojis     []Emoji
	Published  time.Time
}

// Bot reports whether the actor is an automated account.
func (a *Actor) Bot() bool { return a.Type == ActorService || a.Type == ActorApplication }

// HostFunc canonicalizes the host of a remote IRI or the domain of a handle, without a port:
// the application layer maps it to IDNA A-labels (federation.md §1.2), which the domain cannot
// do with the standard library alone. It returns an error for a host that is not a valid name.
type HostFunc func(host string) (string, error)

// NormalizeIRI parses s as an absolute IRI with a host and no userinfo, lower-cases its scheme,
// and replaces its host with what host returns. An IP literal keeps its spelling, lower-cased.
// It returns ErrInvalidIRI when s is not such an IRI or host rejects its host.
func NormalizeIRI(s string, host HostFunc) (IRI, error) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidIRI, err)
	}
	if u.Scheme == "" || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("%w: %q is not an absolute IRI with a host", ErrInvalidIRI, s)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	name, port := u.Hostname(), u.Port()
	if strings.HasPrefix(u.Host, "[") {
		name = "[" + strings.ToLower(name) + "]"
	} else if name, err = host(name); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidIRI, err)
	}
	u.Host = name
	if port != "" {
		u.Host += ":" + port
	}
	return IRI(u.String()), nil
}

// DomainAndParents returns a canonical host and its parent domains, from the host itself to the
// last label, so that a domain-level moderation action on a domain also covers its subdomains
// (data-model §4.3): "a.b.example" gives "a.b.example", "b.example", "example".
func DomainAndParents(host string) []string {
	if host == "" {
		return nil
	}
	out := []string{host}
	for i := strings.IndexByte(host, '.'); i >= 0; i = strings.IndexByte(host, '.') {
		host = host[i+1:]
		if host == "" {
			break
		}
		out = append(out, host)
	}
	return out
}
