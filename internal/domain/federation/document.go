// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"encoding/json"
	"html"
	"strconv"
	"time"
)

// This file holds the shape of the documents this server publishes and delivers: the JSON-LD
// context and the activity envelope (federation.md §7.4). The parts of an object document that
// depend on its kind are built by the caller and passed as the activity's object.

// LDContext returns the JSON-LD context of the documents this server publishes: ActivityStreams,
// the security vocabulary of publicKey, the Mastodon extensions in use (ADR-0006), and the
// vocabulary of quotes (FEP-044f, GoToSocial's interaction policies, and the quote properties of
// Misskey and Fedibird; ADR-0069 §3). Actor
// documents and outbound activities share it. Each call returns a new value, so callers may not
// change what others send.
func LDContext() []any {
	return []any{
		"https://www.w3.org/ns/activitystreams",
		"https://w3id.org/security/v1",
		map[string]any{
			"manuallyApprovesFollowers": "as:manuallyApprovesFollowers",
			"toot":                      "http://joinmastodon.org/ns#",
			"featured":                  map[string]string{"@id": "toot:featured", "@type": "@id"},
			"featuredTags":              map[string]string{"@id": "toot:featuredTags", "@type": "@id"},
			"discoverable":              "toot:discoverable",
			"indexable":                 "toot:indexable",
			"suspended":                 "toot:suspended",
			"movedTo":                   map[string]string{"@id": "as:movedTo", "@type": "@id"},
			"schema":                    "http://schema.org#",
			"PropertyValue":             "schema:PropertyValue",
			"value":                     "schema:value",
			"sensitive":                 "as:sensitive",
			"Hashtag":                   "as:Hashtag",
			"Emoji":                     "toot:Emoji",
			"blurhash":                  "toot:blurhash",
			"focalPoint":                map[string]string{"@container": "@list", "@id": "toot:focalPoint"},
			"votersCount":               "toot:votersCount",
			"gts":                       "https://gotosocial.org/ns#",
			"interactionPolicy":         map[string]string{"@id": "gts:interactionPolicy", "@type": "@id"},
			"canQuote":                  map[string]string{"@id": "gts:canQuote", "@type": "@id"},
			"automaticApproval":         map[string]string{"@id": "gts:automaticApproval", "@type": "@id"},
			"interactingObject":         map[string]string{"@id": "gts:interactingObject", "@type": "@id"},
			"interactionTarget":         map[string]string{"@id": "gts:interactionTarget", "@type": "@id"},
			"quote":                     map[string]string{"@id": "https://w3id.org/fep/044f#quote", "@type": "@id"},
			"quoteAuthorization":        map[string]string{"@id": "https://w3id.org/fep/044f#quoteAuthorization", "@type": "@id"},
			"QuoteRequest":              "https://w3id.org/fep/044f#QuoteRequest",
			"QuoteAuthorization":        "https://w3id.org/fep/044f#QuoteAuthorization",
			"_misskey_quote":            "https://misskey-hub.net/ns#_misskey_quote",
			"quoteUrl":                  "as:quoteUrl",
			"quoteUri":                  "http://fedibird.com/ns#quoteUri",
		},
	}
}

// ActorLDContext returns the JSON-LD context of a local actor's document and of the Update that
// carries it: LDContext with the Controlled Identifiers context, which defines the
// assertionMethod and Multikey of FEP-521a, after the security vocabulary.
func ActorLDContext() []any {
	c := LDContext()
	return append(c[:2:2], append([]any{ControlledIdentifierContext}, c[2:]...)...)
}

// FormatTime formats t as the documents carry times: RFC 3339 in UTC, to the second, as
// Mastodon publishes them.
func FormatTime(t time.Time) string { return t.UTC().Truncate(time.Second).Format(time.RFC3339) }

// ActivityDocument is an outbound activity as it is delivered. Context is set on the top-level
// document only; an activity embedded in another (the Announce of an Undo) has none.
type ActivityDocument struct {
	Context   []any        `json:"@context,omitempty"`
	ID        IRI          `json:"id"`
	Type      ActivityType `json:"type"`
	Actor     IRI          `json:"actor"`
	Published string       `json:"published,omitempty"`
	To        []IRI        `json:"to"`
	Cc        []IRI        `json:"cc"`
	// Object is an IRI or an embedded document.
	Object any `json:"object"`
	// Instrument is the quoting post of a QuoteRequest; Result the QuoteAuthorization of the
	// Accept of one (FEP-044f). Other activities have neither.
	Instrument IRI `json:"instrument,omitempty"`
	Result     IRI `json:"result,omitempty"`
	// Target is the actor that a Move moves its object to (ADR-0089); other activities have
	// none.
	Target IRI `json:"target,omitempty"`
}

// NewActivityDocument returns the top-level document of an activity with its context. A zero
// published leaves it out.
func NewActivityDocument(id IRI, t ActivityType, actor IRI, published time.Time, aud Audience, object any) ActivityDocument {
	d := ActivityDocument{Context: LDContext(), ID: id, Type: t, Actor: actor, Object: object}
	d.To, d.Cc = nonNil(aud.To), nonNil(aud.Cc)
	if !published.IsZero() {
		d.Published = FormatTime(published)
	}
	return d
}

// Embedded returns d without its context, to embed it in another activity.
func (d ActivityDocument) Embedded() ActivityDocument {
	d.Context = nil
	return d
}

// Marshal returns the JSON of d, the bytes every delivery of it sends.
func (d ActivityDocument) Marshal() ([]byte, error) { return json.Marshal(d) }

// ReactionLDContext returns the JSON-LD context of the Like of an emoji reaction: LDContext with
// Misskey's _misskey_reaction (ADR-0052 §3).
func ReactionLDContext() []any {
	c := LDContext()
	terms := c[len(c)-1].(map[string]any)
	terms["misskey"] = "https://misskey-hub.net/ns#"
	terms["_misskey_reaction"] = "misskey:_misskey_reaction"
	return c
}

// LikeDocument is the Like of an emoji reaction (ADR-0052 §3): the Like of a favourite with the
// emoji as its content and its _misskey_reaction, the same string (the Unicode emoji, or
// ":shortcode:" of a custom emoji), and the Emoji tag of a custom emoji. Misskey and Akkoma read
// it as a reaction, Mastodon and GoToSocial as a favourite.
type LikeDocument struct {
	ActivityDocument
	Content         string        `json:"content,omitempty"`
	MisskeyReaction string        `json:"_misskey_reaction,omitempty"`
	Tag             []TagDocument `json:"tag,omitempty"`
}

// NewReactionLike returns like, a Like built by NewActivityDocument, as the Like of the emoji
// reaction emoji, with the Emoji tag of a custom emoji (nil for a Unicode emoji) and the context
// of ReactionLDContext.
func NewReactionLike(like ActivityDocument, emoji string, tag *TagDocument) LikeDocument {
	like.Context = ReactionLDContext()
	d := LikeDocument{ActivityDocument: like, Content: emoji, MisskeyReaction: emoji}
	if tag != nil {
		d.Tag = []TagDocument{*tag}
	}
	return d
}

// Embedded returns d without its context, to embed it in an Undo.
func (d LikeDocument) Embedded() LikeDocument {
	d.Context = nil
	return d
}

// Marshal returns the JSON of d.
func (d LikeDocument) Marshal() ([]byte, error) { return json.Marshal(d) }

// FlagDocument is the Flag that forwards a report to the server of the reported account
// (federation.md §11), in the shape that Mastodon sends and reads: the instance actor as its
// actor, the reported actor and statuses as its object, and the comment as plain-text content.
// It has no audience and names nothing of the reporter.
type FlagDocument struct {
	Context []any        `json:"@context"`
	ID      IRI          `json:"id"`
	Type    ActivityType `json:"type"`
	Actor   IRI          `json:"actor"`
	Content string       `json:"content"`
	Object  []IRI        `json:"object"`
}

// NewFlagDocument returns the Flag id of actor about the reported actor target and its
// statuses, with the comment as its content.
func NewFlagDocument(id, actor, target IRI, statuses []IRI, comment string) FlagDocument {
	return FlagDocument{
		Context: LDContext(), ID: id, Type: ActivityFlag, Actor: actor, Content: comment,
		Object: append([]IRI{target}, statuses...),
	}
}

// Marshal returns the JSON of d, the bytes every delivery of it sends.
func (d FlagDocument) Marshal() ([]byte, error) { return json.Marshal(d) }

// ObjectDocument is the part of an object document that every kind shares: its identity, author,
// and addressing. A Note adds its body in NoteDocument.
type ObjectDocument struct {
	ID           IRI        `json:"id"`
	Type         ObjectType `json:"type"`
	InReplyTo    *IRI       `json:"inReplyTo,omitempty"`
	Published    string     `json:"published,omitempty"`
	Updated      string     `json:"updated,omitempty"`
	URL          string     `json:"url,omitempty"`
	AttributedTo IRI        `json:"attributedTo,omitempty"`
	To           []IRI      `json:"to,omitempty"`
	Cc           []IRI      `json:"cc,omitempty"`
}

// VoteNoteDocument is the Note that carries a local account's choice in a remote poll (FR-112)
// as Mastodon sends it: the title of the option as its name, in reply to the Question, and
// addressed to the poll's author alone. Mastodon, Misskey, Pleroma, and GoToSocial count a Note
// with a name in reply to their poll as a vote for the option of that title.
type VoteNoteDocument struct {
	ObjectDocument
	Name string `json:"name"`
}

// NewPollVote returns the Create of the Note id by actor that votes for the option titled name
// in the poll question of author. The Create is at id + "/activity" (ADR-0018).
func NewPollVote(id, actor, author, question IRI, name string, published time.Time) ActivityDocument {
	to := []IRI{author}
	note := VoteNoteDocument{
		ObjectDocument: ObjectDocument{ID: id, Type: ObjectNote, InReplyTo: &question, AttributedTo: actor, To: to},
		Name:           name,
	}
	return NewActivityDocument(id+"/activity", ActivityCreate, actor, published, Audience{To: to}, note)
}

// TopLevelObject is an object document served on its own, with its context: the answer to a GET
// of an object IRI (federation.md §10).
type TopLevelObject struct {
	Context []any `json:"@context"`
	ObjectDocument
}

// TopLevel returns o as a top-level document.
func (o ObjectDocument) TopLevel() TopLevelObject {
	return TopLevelObject{Context: LDContext(), ObjectDocument: o}
}

// NoteDocument is the Note of a status (federation.md §7.4): the shared part, then the body of
// one revision. Sensitive, Content, Tag, and Attachment are always present, as Mastodon
// publishes them; Summary and ContentMap are left out when empty.
type NoteDocument struct {
	ObjectDocument
	// Summary is the content warning.
	Summary   string `json:"summary,omitempty"`
	Sensitive bool   `json:"sensitive"`
	// Content is the HTML of the revision's language; ContentMap has the HTML per language
	// (NFR-I18N-5), without the undetermined language "und".
	Content    string               `json:"content"`
	ContentMap map[string]string    `json:"contentMap,omitempty"`
	Attachment []AttachmentDocument `json:"attachment"`
	Tag        []TagDocument        `json:"tag"`
	// The poll of a Question (FR-112), left out of a Note: the choices of a single-choice poll
	// in OneOf or of a multiple-choice one in AnyOf, when it ends, when it ended (Closed, once
	// it has), and how many voted, as Mastodon publishes them.
	OneOf       []QuestionOption `json:"oneOf,omitempty"`
	AnyOf       []QuestionOption `json:"anyOf,omitempty"`
	EndTime     string           `json:"endTime,omitempty"`
	Closed      string           `json:"closed,omitempty"`
	VotersCount *int64           `json:"votersCount,omitempty"`
	// The quote of the status (ADR-0069 §3): FEP-044f's quote with the same IRI in the spellings
	// of Misskey, Akkoma, and Fedibird, and the QuoteAuthorization that approves it.
	Quote              IRI `json:"quote,omitempty"`
	MisskeyQuote       IRI `json:"_misskey_quote,omitempty"`
	QuoteURL           IRI `json:"quoteUrl,omitempty"`
	QuoteURI           IRI `json:"quoteUri,omitempty"`
	QuoteAuthorization IRI `json:"quoteAuthorization,omitempty"`
	// InteractionPolicy says who may quote the status.
	InteractionPolicy *InteractionPolicy `json:"interactionPolicy,omitempty"`
}

// InteractionPolicy is the interactionPolicy of a status (GoToSocial's interaction policies,
// FEP-044f): only canQuote.
type InteractionPolicy struct {
	CanQuote QuoteApprovalPolicy `json:"canQuote"`
}

// QuoteApprovalPolicy is the canQuote of an interactionPolicy: whose quotes are approved
// automatically. This server approves no quote by hand.
type QuoteApprovalPolicy struct {
	AutomaticApproval []IRI `json:"automaticApproval"`
}

// WithQuote returns n quoting target: with quote and its aliases, the QuoteAuthorization
// authorization ("" for none), and the fallback of FEP-044f before its content, a paragraph of
// the class quote-inline that links page, the quoted post's HTML page (Mastodon's form).
func (n NoteDocument) WithQuote(target, authorization IRI, page string) NoteDocument {
	n.Quote, n.MisskeyQuote, n.QuoteURL, n.QuoteURI, n.QuoteAuthorization = target, target, target, target, authorization
	fallback := `<p class="quote-inline">RE: <a href="` + html.EscapeString(page) + `">` + html.EscapeString(page) + `</a></p>`
	n.Content = fallback + n.Content
	if len(n.ContentMap) > 0 {
		m := make(map[string]string, len(n.ContentMap))
		for lang, c := range n.ContentMap {
			m[lang] = fallback + c
		}
		n.ContentMap = m
	}
	return n
}

// QuoteAuthorizationDocument is this server's approval of a quote (FEP-044f), served at its IRI.
type QuoteAuthorizationDocument struct {
	Context           []any      `json:"@context"`
	ID                IRI        `json:"id"`
	Type              ObjectType `json:"type"`
	AttributedTo      IRI        `json:"attributedTo"`
	InteractingObject IRI        `json:"interactingObject"`
	InteractionTarget IRI        `json:"interactionTarget"`
}

// NewQuoteAuthorizationDocument returns the QuoteAuthorization id by which author approves the
// quote quoting of quoted.
func NewQuoteAuthorizationDocument(id, author, quoting, quoted IRI) QuoteAuthorizationDocument {
	return QuoteAuthorizationDocument{
		Context: LDContext(), ID: id, Type: ObjectQuoteAuthorization, AttributedTo: author, InteractingObject: quoting, InteractionTarget: quoted,
	}
}

// QuestionOption is one choice of a Question: a Note named after it, whose replies count its
// votes.
type QuestionOption struct {
	Type    ObjectType      `json:"type"`
	Name    string          `json:"name"`
	Replies QuestionReplies `json:"replies"`
}

// QuestionReplies is the replies collection of a QuestionOption, with only its size.
type QuestionReplies struct {
	Type       string `json:"type"`
	TotalItems int64  `json:"totalItems"`
}

// QuestionPoll is the poll of a status as its Question carries it.
type QuestionPoll struct {
	Multiple bool
	// Options are the titles of the choices, in order, and Votes their counts.
	Options []string
	Votes   []int64
	// EndTime is when the poll ends, zero for no end; Closed is set once it has ended.
	EndTime     time.Time
	Closed      bool
	VotersCount *int64
}

// AsQuestion returns n as the Question of a status with the poll p.
func (n NoteDocument) AsQuestion(p QuestionPoll) NoteDocument {
	n.Type = ObjectQuestion
	options := make([]QuestionOption, len(p.Options))
	for i, title := range p.Options {
		var votes int64
		if i < len(p.Votes) {
			votes = p.Votes[i]
		}
		options[i] = QuestionOption{Type: ObjectNote, Name: title, Replies: QuestionReplies{Type: "Collection", TotalItems: votes}}
	}
	if p.Multiple {
		n.AnyOf = options
	} else {
		n.OneOf = options
	}
	if !p.EndTime.IsZero() {
		n.EndTime = FormatTime(p.EndTime)
		if p.Closed {
			n.Closed = n.EndTime
		}
	}
	n.VotersCount = p.VotersCount
	return n
}

// TopLevelNote is a Note served on its own, with its context.
type TopLevelNote struct {
	Context []any `json:"@context"`
	NoteDocument
}

// TopLevel returns n as a top-level document.
func (n NoteDocument) TopLevel() TopLevelNote {
	return TopLevelNote{Context: LDContext(), NoteDocument: n}
}

// AttachmentType is the type of every attachment of a Note: Mastodon publishes images, videos,
// and audio alike as Document, and the other implementations read the kind from MediaType.
const AttachmentType ObjectType = "Document"

// ImageDocument is an image of an actor: its icon (avatar) or image (header).
type ImageDocument struct {
	Type      ObjectType `json:"type"`
	MediaType string     `json:"mediaType,omitempty"`
	URL       string     `json:"url"`
}

// ImageType is the type of an ImageDocument.
const ImageType ObjectType = "Image"

// AttachmentDocument is a media attachment of a Note. Name is the alternative text. FocalPoint is x, y from -1.0 to 1.0, left out at
// the center; Width, Height, and Blurhash are left out when unknown. A video or audio file has
// its poster as Icon, left out when it has none, and its length as Duration, an xsd:duration
// such as "PT12.5S", as Mastodon sends them (ADR-0054 §5).
type AttachmentDocument struct {
	Type       ObjectType     `json:"type"`
	MediaType  string         `json:"mediaType"`
	URL        string         `json:"url"`
	Name       string         `json:"name,omitempty"`
	Blurhash   string         `json:"blurhash,omitempty"`
	FocalPoint []float64      `json:"focalPoint,omitempty"`
	Width      int            `json:"width,omitempty"`
	Height     int            `json:"height,omitempty"`
	Icon       *ImageDocument `json:"icon,omitempty"`
	Duration   string         `json:"duration,omitempty"`
}

// FormatDuration returns d as an xsd:duration in seconds with up to three decimals ("PT12.5S").
func FormatDuration(d time.Duration) string {
	s := strconv.FormatFloat(d.Round(time.Millisecond).Seconds(), 'f', -1, 64)
	return "PT" + s + "S"
}

// TagType is the type of an entry of a Note's tag.
type TagType string

// Tag types.
const (
	TagMention TagType = "Mention"
	TagHashtag TagType = "Hashtag"
	TagEmoji   TagType = "Emoji"
)

// TagDocument is an entry of the tag of a Note or an actor: a Mention, whose Href is the actor
// and Name the handle "@user@domain"; a Hashtag, whose Href is the tag's page and Name "#name";
// or an Emoji (EmojiTag), a custom emoji with its id, its Name ":shortcode:", when its image
// was Updated, and the image as its Icon.
type TagDocument struct {
	ID      IRI            `json:"id,omitempty"`
	Type    TagType        `json:"type"`
	Href    string         `json:"href,omitempty"`
	Name    string         `json:"name"`
	Updated string         `json:"updated,omitempty"`
	Icon    *ImageDocument `json:"icon,omitempty"`
}

// EmojiTag returns the Emoji tag of a custom emoji in the shape that Mastodon publishes and
// Misskey, GoToSocial, and Akkoma read (ADR-0051 §3): its id ("" leaves it out, for a remote
// emoji without one), its name :shortcode:, when its current image was set, and the image at
// url, of mediaType ("" leaves it out). Notes, actors, and reactions share it.
func EmojiTag(id IRI, shortcode string, updated time.Time, mediaType, url string) TagDocument {
	t := TagDocument{
		ID: id, Type: TagEmoji, Name: ":" + shortcode + ":",
		Icon: &ImageDocument{Type: ImageType, MediaType: mediaType, URL: url},
	}
	if !updated.IsZero() {
		t.Updated = FormatTime(updated)
	}
	return t
}

// TopLevelEmoji is the Emoji of a local custom emoji served on its own, with its context: the
// answer to a GET of its id (ADR-0051 §3).
type TopLevelEmoji struct {
	Context []any `json:"@context"`
	TagDocument
}

// TopLevelEmojiOf returns t, an Emoji tag, as a top-level document.
func TopLevelEmojiOf(t TagDocument) TopLevelEmoji {
	return TopLevelEmoji{Context: LDContext(), TagDocument: t}
}

// TombstoneOf returns the object of the Delete of id.
func TombstoneOf(id IRI) ObjectDocument { return ObjectDocument{ID: id, Type: ObjectTombstone} }

// ActorDocument is a local actor as it is published: the answer to a GET of its id and the
// object of its Update (federation.md §7.1). It has the shape Mastodon publishes, which Misskey,
// GoToSocial, and Akkoma also read, with the fields in Mastodon's order. Context is set on the
// top-level document only.
type ActorDocument struct {
	Context                   []any              `json:"@context,omitempty"`
	ID                        IRI                `json:"id"`
	Type                      ActorType          `json:"type"`
	Following                 IRI                `json:"following"`
	Followers                 IRI                `json:"followers"`
	Inbox                     IRI                `json:"inbox"`
	Outbox                    IRI                `json:"outbox"`
	Featured                  IRI                `json:"featured,omitempty"`
	FeaturedTags              IRI                `json:"featuredTags,omitempty"`
	PreferredUsername         string             `json:"preferredUsername"`
	Name                      *string            `json:"name,omitempty"`
	Summary                   *string            `json:"summary,omitempty"`
	URL                       string             `json:"url,omitempty"`
	ManuallyApprovesFollowers bool               `json:"manuallyApprovesFollowers"`
	Discoverable              *bool              `json:"discoverable,omitempty"`
	Indexable                 *bool              `json:"indexable,omitempty"`
	Published                 string             `json:"published"`
	PublicKey                 *PublicKeyDocument `json:"publicKey,omitempty"`
	// AssertionMethod holds the Ed25519 key that signs the actor's integrity proofs (FEP-521a,
	// FEP-8b32); it is left out when the actor has none.
	AssertionMethod []MultikeyDocument `json:"assertionMethod,omitempty"`
	Tag             []any              `json:"tag"`
	Attachment      []PropertyValue    `json:"attachment"`
	Endpoints       map[string]IRI     `json:"endpoints"`
	// Icon and Image are the avatar and the header, left out when the actor has none.
	Icon  *ImageDocument `json:"icon,omitempty"`
	Image *ImageDocument `json:"image,omitempty"`
	// AlsoKnownAs are the other actors that the actor declares as its own (as:alsoKnownAs, the
	// accounts it moves from, ADR-0089), left out when it has none.
	AlsoKnownAs []IRI `json:"alsoKnownAs,omitempty"`
	// Suspended marks an actor that moderation suspended (Mastodon's toot:suspended); it is
	// left out otherwise.
	Suspended bool `json:"suspended,omitempty"`
	// MovedTo is the actor that the actor moved to (as:movedTo, ADR-0089); it is left out when
	// the actor has not moved.
	MovedTo IRI `json:"movedTo,omitempty"`
}

// Embedded returns d without its context, to embed it in an activity.
func (d ActorDocument) Embedded() ActorDocument {
	d.Context = nil
	return d
}

// Suspend returns d as a suspended actor is published (federation.md §11.1, ADR-0036): marked
// suspended, with the profile emptied (name, summary, fields, tags, aliases) and nothing that
// invites following or discovery, while the id, the collections, the inboxes, and the key stay,
// so that servers can still address the actor and verify its signatures.
func (d ActorDocument) Suspend() ActorDocument {
	empty, no := "", false
	d.Name, d.Summary = &empty, &empty
	d.Tag, d.Attachment = []any{}, []PropertyValue{}
	d.Icon, d.Image = nil, nil
	d.AlsoKnownAs = nil
	d.ManuallyApprovesFollowers = false
	if d.Discoverable != nil {
		d.Discoverable = &no
	}
	if d.Indexable != nil {
		d.Indexable = &no
	}
	d.Suspended = true
	return d
}

// PublicKeyDocument is the publicKey of an actor document (ADR-0032).
type PublicKeyDocument struct {
	ID           string `json:"id"`
	Owner        IRI    `json:"owner"`
	PublicKeyPEM string `json:"publicKeyPem"`
}

// PropertyValue is a profile field (FR-006) as Mastodon's schema:PropertyValue.
type PropertyValue struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

func nonNil(s []IRI) []IRI {
	if s == nil {
		return []IRI{}
	}
	return s
}
