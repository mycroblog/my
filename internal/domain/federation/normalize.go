// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Normalize reads a received AS2 document (federation.md §4). The document is read as JSON
// compacted with the normative ActivityStreams context, never processed as JSON-LD: @context is
// not interpreted (a missing one means the normative context, AS2 Core §2.1), and a Linked Data
// signature is ignored. Unknown properties, including dialect extensions such as Misskey's
// _misskey_content, are ignored; every spelling of a quote is read (ADR-0069 §2). now bounds future timestamps. host
// canonicalizes the host of every IRI in the document (see NormalizeIRI); an IRI whose host it
// rejects is read as absent.
//
// It returns ErrMalformed for a document that is not a JSON object with a type, and
// ErrLimitExceeded for one over a limit (MaxDepth, MaxTags, MaxRecipients, MaxBodyBytes).
func Normalize(raw []byte, now time.Time, host HostFunc) (*Document, error) {
	v, err := decode(raw, MaxDepth)
	if err != nil {
		return nil, err
	}
	o, ok := v.(object)
	if !ok {
		return nil, fmt.Errorf("%w: not a JSON object", ErrMalformed)
	}
	n := normalizer{now: now.UTC(), host: host}
	return n.document(o)
}

type normalizer struct {
	now  time.Time
	host HostFunc
}

func (n normalizer) document(o object) (*Document, error) {
	names := strs(o.get("type"))
	if len(names) == 0 {
		return nil, fmt.Errorf("%w: no type", ErrMalformed)
	}
	for _, t := range names {
		switch {
		case slices.Contains(activityTypes, ActivityType(t)):
			a, err := n.activity(o, ActivityType(t), names[0])
			return &Document{Activity: a}, err
		case slices.Contains(objectTypes, ObjectType(t)):
			obj, err := n.object(o, ObjectType(t), names[0])
			return &Document{Object: obj}, err
		case slices.Contains(actorTypes, ActorType(t)):
			a, err := n.actor(o, ActorType(t))
			return &Document{Actor: a}, err
		case slices.Contains(collectionTypes, t):
			c, err := n.collection(o, strings.HasSuffix(t, "Page"))
			return &Document{Collection: c}, err
		}
	}
	if o.has("actor") {
		a, err := n.activity(o, ActivityUnknown, names[0])
		return &Document{Activity: a}, err
	}
	obj, err := n.object(o, ObjectUnknown, names[0])
	return &Document{Object: obj}, err
}

// KnownActivityType reports whether t names one of the known activity types.
func KnownActivityType(t string) bool {
	return t != "" && slices.Contains(activityTypes, ActivityType(t))
}

var (
	activityTypes = []ActivityType{
		ActivityCreate, ActivityUpdate, ActivityDelete, ActivityFollow, ActivityAccept, ActivityReject,
		ActivityUndo, ActivityAnnounce, ActivityLike, ActivityEmojiReact, ActivityBlock, ActivityFlag,
		ActivityAdd, ActivityRemove, ActivityMove, ActivityQuoteRequest,
	}
	objectTypes = []ObjectType{
		ObjectNote, ObjectQuestion, ObjectArticle, ObjectPage, ObjectEvent, ObjectImage, ObjectVideo,
		ObjectAudio, ObjectTombstone, ObjectQuoteAuthorization,
	}
	convertedTypes  = []ObjectType{ObjectArticle, ObjectPage, ObjectEvent, ObjectImage, ObjectVideo, ObjectAudio}
	actorTypes      = []ActorType{ActorPerson, ActorService, ActorApplication, ActorGroup, ActorOrganization}
	collectionTypes = []string{"Collection", "OrderedCollection", "CollectionPage", "OrderedCollectionPage"}
)

func (n normalizer) activity(o object, t ActivityType, raw string) (*Activity, error) {
	a := &Activity{
		ID:        n.iri(o.get("id")),
		Type:      t,
		RawType:   raw,
		Actor:     n.firstRef(o.get("actor")),
		Target:    n.firstRef(o.get("target")),
		Published: n.time(o.get("published")),
	}
	var err error
	if a.Audience, err = n.audience(o); err != nil {
		return nil, err
	}
	container := a.ID
	if container == "" {
		container = a.Actor
	}
	for _, v := range list(o.get("object")) {
		r, err := n.ref(v, container)
		if err != nil {
			return nil, err
		}
		if r.ID != "" || r.embedded != nil {
			a.Objects = append(a.Objects, r)
		}
	}
	content, err := body(o.get("content"))
	if err != nil {
		return nil, err
	}
	a.Content = plainText(content)
	switch t {
	case ActivityEmojiReact:
		a.Reaction = a.Content
	case ActivityLike:
		a.Reaction = strings.TrimSpace(str(o.get("_misskey_reaction")))
	}
	if _, _, a.Emojis, err = n.tags(o.get("tag")); err != nil {
		return nil, err
	}
	switch t {
	case ActivityQuoteRequest:
		if a.Instrument, err = n.ref(o.get("instrument"), container); err != nil {
			return nil, err
		}
	case ActivityAccept:
		a.Result = n.firstRef(o.get("result"))
	}
	return a, nil
}

func (n normalizer) object(o object, t ObjectType, raw string) (*Object, error) {
	obj := &Object{
		ID:           n.iri(o.get("id")),
		Type:         t,
		RawType:      raw,
		Converted:    slices.Contains(convertedTypes, t),
		AttributedTo: n.firstRef(o.get("attributedTo")),
		InReplyTo:    n.firstRef(o.get("inReplyTo")),
		Published:    n.time(o.get("published")),
		Updated:      n.time(o.get("updated")),
		URL:          n.displayURL(o.get("url")),
		Sensitive:    o.get("sensitive") == true,
	}
	var err error
	if obj.Audience, err = n.audience(o); err != nil {
		return nil, err
	}
	if obj.Bodies, err = bodies(o); err != nil {
		return nil, err
	}
	summary, err := body(o.get("summary"))
	if err != nil {
		return nil, err
	}
	if summary == "" {
		if summary, err = firstMapValue(o.get("summaryMap")); err != nil {
			return nil, err
		}
	}
	summary = plainText(summary)
	if obj.Converted {
		name := strings.TrimSpace(str(o.get("name")))
		if name == "" {
			name, _ = firstMapValue(o.get("nameMap"))
			name = strings.TrimSpace(name)
		}
		link := obj.URL
		if !webLink(link) {
			link = obj.ID
		}
		lang := UndeterminedLang
		if len(obj.Bodies) > 0 {
			lang = obj.Bodies[0].Language
		}
		obj.Bodies = []Body{{Language: lang, HTML: convertedBody(name, summary, link)}}
	} else {
		obj.SpoilerText = summary
		obj.Name = strings.TrimSpace(str(o.get("name")))
	}
	if obj.Mentions, obj.Hashtags, obj.Emojis, err = n.tags(o.get("tag")); err != nil {
		return nil, err
	}
	obj.Attachments = n.attachments(o.get("attachment"))
	if t == ObjectQuestion {
		obj.Poll = n.poll(o)
	}
	if obj.Replies, err = n.ref(o.get("replies"), obj.ID); err != nil {
		return nil, err
	}
	obj.Quote = n.quote(o)
	obj.CanQuote = n.canQuote(o)
	if t == ObjectQuoteAuthorization {
		obj.InteractingObject = n.firstRef(o.get("interactingObject"))
		obj.InteractionTarget = n.firstRef(o.get("interactionTarget"))
	}
	return obj, nil
}

// quoteProperties are the properties that name the quoted post, in the order they are read
// (ADR-0069 §2): FEP-044f's quote, then the spellings of Misskey, Akkoma, and Fedibird.
var quoteProperties = []string{"quote", "_misskey_quote", "quoteUrl", "quoteURL", "quoteUri"}

// misskeyQuoteRels are the rel values of an FEP-e232 Link that names the quoted post: the IRI
// of _misskey_quote, also in the spelling of FEP-044f's context.
var misskeyQuoteRels = []string{"https://misskey-hub.net/ns#_misskey_quote", "https://misskey-hub.net/ns/#_misskey_quote"}

// quote reads the quoted post: the first of quoteProperties that has an IRI, else the first
// FEP-e232 Link to an ActivityStreams document whose rel is _misskey_quote. The tag property is
// bounded by MaxTags before.
func (n normalizer) quote(o object) *Quote {
	for _, p := range quoteProperties {
		target := n.firstRef(o.get(p))
		if target == "" || target == PublicIRI {
			continue
		}
		q := &Quote{Target: target, Consent: p == "quote"}
		if q.Consent {
			q.Authorization = n.firstRef(o.get("quoteAuthorization"))
		}
		return q
	}
	for _, v := range list(o.get("tag")) {
		t, ok := v.(object)
		if !ok || !slices.Contains(strs(t.get("type")), "Link") || !activityStreamsMediaType(str(t.get("mediaType"))) ||
			!slices.ContainsFunc(strs(t.get("rel")), func(rel string) bool { return slices.Contains(misskeyQuoteRels, rel) }) {
			continue
		}
		if target := n.normalizeIRI(str(t.get("href"))); target != "" && target != PublicIRI {
			return &Quote{Target: target}
		}
	}
	return nil
}

// canQuote reads interactionPolicy.canQuote, nil when the object has none.
func (n normalizer) canQuote(o object) *CanQuote {
	policy, ok := o.get("interactionPolicy").(object)
	if !ok {
		return nil
	}
	c, ok := policy.get("canQuote").(object)
	if !ok {
		return nil
	}
	return &CanQuote{AutomaticApproval: n.recipients(list(c.get("automaticApproval"))), ManualApproval: n.recipients(list(c.get("manualApproval")))}
}

// activityStreamsMediaType reports whether the mediaType of a Link names an ActivityStreams
// document (FEP-e232).
func activityStreamsMediaType(mt string) bool {
	mt = strings.ToLower(strings.TrimSpace(mt))
	return mt == "application/activity+json" ||
		strings.HasPrefix(mt, "application/ld+json") && strings.Contains(mt, "https://www.w3.org/ns/activitystreams")
}

// collection reads a collection or a page of one: its items (orderedItems in an ordered one) up
// to MaxItems, and the links to its first and next pages. Embedded pages and items are trusted
// only when they have the collection's origin.
func (n normalizer) collection(o object, page bool) (*Collection, error) {
	c := &Collection{ID: n.iri(o.get("id")), Page: page}
	items := o.get("orderedItems")
	if items == nil {
		items = o.get("items")
	}
	for _, v := range list(items) {
		if len(c.Items) == MaxItems {
			break
		}
		r, err := n.ref(v, c.ID)
		if err != nil {
			return nil, err
		}
		if r.ID != "" {
			c.Items = append(c.Items, r)
		}
	}
	var err error
	if c.First, err = n.ref(o.get("first"), c.ID); err != nil {
		return nil, err
	}
	if c.Next, err = n.ref(o.get("next"), c.ID); err != nil {
		return nil, err
	}
	return c, nil
}

func (n normalizer) actor(o object, t ActorType) (*Actor, error) {
	a := &Actor{
		ID:                        n.iri(o.get("id")),
		Type:                      t,
		PreferredUsername:         strings.TrimSpace(str(o.get("preferredUsername"))),
		Name:                      strings.TrimSpace(str(o.get("name"))),
		URL:                       n.displayURL(o.get("url")),
		Inbox:                     n.firstRef(o.get("inbox")),
		Outbox:                    n.firstRef(o.get("outbox")),
		Followers:                 n.firstRef(o.get("followers")),
		Following:                 n.firstRef(o.get("following")),
		Featured:                  n.firstRef(o.get("featured")),
		Icon:                      n.imageURL(o.get("icon")),
		Image:                     n.imageURL(o.get("image")),
		ManuallyApprovesFollowers: o.get("manuallyApprovesFollowers") == true,
		Discoverable:              o.get("discoverable") == true,
		Indexable:                 o.get("indexable") == true,
		Published:                 n.time(o.get("published")),
	}
	var err error
	if a.Summary, err = body(o.get("summary")); err != nil {
		return nil, err
	}
	if ep, ok := o.get("endpoints").(object); ok {
		a.SharedInbox = n.firstRef(ep.get("sharedInbox"))
	}
	for _, d := range strs(o.get("attributionDomains")) {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" && len(a.AttributionDomains) < MaxItems {
			a.AttributionDomains = append(a.AttributionDomains, d)
		}
	}
	if aka := n.recipients(list(o.get("alsoKnownAs"))); len(aka) > 0 {
		a.AlsoKnownAs = aka[:min(len(aka), MaxItems)]
	}
	a.MovedTo = n.firstRef(o.get("movedTo"))
	for _, v := range list(o.get("publicKey")) {
		k, ok := v.(object)
		if !ok {
			continue
		}
		if pem := str(k.get("publicKeyPem")); pem != "" {
			a.PublicKeys = append(a.PublicKeys, PublicKey{ID: n.iri(k.get("id")), Owner: n.firstRef(k.get("owner")), PEM: pem})
		}
	}
	for _, v := range list(o.get("attachment")) {
		f, ok := v.(object)
		if !ok || !slices.Contains(strs(f.get("type")), "PropertyValue") {
			continue
		}
		value, err := body(f.get("value"))
		if err != nil {
			return nil, err
		}
		a.Fields = append(a.Fields, Field{Name: strings.TrimSpace(str(f.get("name"))), Value: value})
	}
	if _, _, a.Emojis, err = n.tags(o.get("tag")); err != nil {
		return nil, err
	}
	return a, nil
}

// ref reads a property that refers to a document: an IRI, a Link, or an embedded document whose
// contents are trusted only when it has no id or the same origin as container.
func (n normalizer) ref(v any, container IRI) (Ref, error) {
	switch v := v.(type) {
	case string:
		return Ref{ID: n.normalizeIRI(v)}, nil
	case object:
		names := strs(v.get("type"))
		if len(names) == 0 || isLink(v) {
			return Ref{ID: n.linkIRI(v)}, nil
		}
		doc, err := n.document(v)
		if err != nil {
			return Ref{}, err
		}
		id := doc.ID()
		return Ref{ID: id, embedded: doc, sameOrigin: id == "" || SameOrigin(id, container)}, nil
	}
	return Ref{}, nil
}

func (n normalizer) poll(o object) *Poll {
	// The end of a poll is in the future until it closes, so it is not clamped to now.
	p := &Poll{EndTime: parseTime(o.get("endTime")), Closed: n.time(o.get("closed")), VotersCount: integer(o.get("votersCount"))}
	choices := o.get("oneOf")
	if choices == nil {
		choices = o.get("anyOf")
		p.Multiple = choices != nil
	}
	for _, v := range list(choices) {
		c, ok := v.(object)
		if !ok {
			continue
		}
		opt := PollOption{Name: strings.TrimSpace(str(c.get("name")))}
		if r, ok := c.get("replies").(object); ok {
			opt.Votes = integer(r.get("totalItems"))
		}
		p.Options = append(p.Options, opt)
	}
	return p
}

// time reads an RFC 3339 timestamp; a future one is clamped to now, a malformed one is zero.
func (n normalizer) time(v any) time.Time {
	t := parseTime(v)
	if t.After(n.now) {
		return n.now
	}
	return t
}

// parseTime reads an RFC 3339 timestamp in UTC; a malformed one is zero.
func parseTime(v any) time.Time {
	s, ok := v.(string)
	if !ok {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// audience reads to and cc, normalizing the spellings of Public and dropping duplicates.
func (n normalizer) audience(o object) (Audience, error) {
	to, cc := list(o.get("to")), list(o.get("cc"))
	if len(to)+len(cc) > MaxRecipients {
		return Audience{}, fmt.Errorf("%w: %d recipients (max %d)", ErrLimitExceeded, len(to)+len(cc), MaxRecipients)
	}
	return Audience{To: n.recipients(to), Cc: n.recipients(cc)}, nil
}

func (n normalizer) recipients(vs []any) []IRI {
	var out []IRI
	for _, v := range vs {
		var r IRI
		switch v := v.(type) {
		case string:
			r = n.normalizeIRI(v)
		case object:
			r = n.linkIRI(v)
		}
		if r != "" && !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

// bodies reads contentMap (one Body per language, in document order) or, without it, content as
// one Body in UndeterminedLang.
func bodies(o object) ([]Body, error) {
	content, err := body(o.get("content"))
	if err != nil {
		return nil, err
	}
	var out []Body
	if m, ok := o.get("contentMap").(object); ok {
		for _, e := range m {
			h, err := body(e.value)
			if err != nil {
				return nil, err
			}
			lang := canonicalLanguage(e.key)
			if h == "" || slices.ContainsFunc(out, func(b Body) bool { return b.Language == lang }) {
				continue
			}
			out = append(out, Body{Language: lang, HTML: h})
		}
	}
	if len(out) == 0 && content != "" {
		out = []Body{{Language: UndeterminedLang, HTML: content}}
	}
	return out, nil
}

// body reads one HTML value and enforces MaxBodyBytes.
func body(v any) (string, error) {
	s := str(v)
	if len(s) > MaxBodyBytes {
		return "", fmt.Errorf("%w: a %d-byte HTML value (max %d)", ErrLimitExceeded, len(s), MaxBodyBytes)
	}
	return s, nil
}

func firstMapValue(v any) (string, error) {
	m, ok := v.(object)
	if !ok {
		return "", nil
	}
	for _, e := range m {
		if s, err := body(e.value); err != nil || s != "" {
			return s, err
		}
	}
	return "", nil
}

// tags reads the tag property: Mention, Hashtag, and Emoji entries.
func (n normalizer) tags(v any) (mentions []Mention, hashtags []string, emojis []Emoji, err error) {
	vs := list(v)
	if len(vs) > MaxTags {
		return nil, nil, nil, fmt.Errorf("%w: %d tags (max %d)", ErrLimitExceeded, len(vs), MaxTags)
	}
	for _, v := range vs {
		t, ok := v.(object)
		if !ok {
			continue
		}
		types := strs(t.get("type"))
		name := strings.TrimSpace(str(t.get("name")))
		switch {
		case slices.Contains(types, "Mention"):
			if href := n.firstRef(t.get("href")); href != "" {
				mentions = append(mentions, Mention{Href: href, Name: name})
			}
		case slices.Contains(types, "Hashtag"):
			if h := strings.TrimPrefix(name, "#"); h != "" && !slices.Contains(hashtags, h) {
				hashtags = append(hashtags, h)
			}
		case slices.Contains(types, "Emoji"):
			code := strings.Trim(name, ":")
			if utf8.RuneCountInString(code) > MaxShortcode || slices.ContainsFunc(emojis, func(e Emoji) bool { return e.Shortcode == code }) {
				continue
			}
			if icon := n.imageURL(t.get("icon")); code != "" && icon != "" {
				emojis = append(emojis, Emoji{ID: n.iri(t.get("id")), Shortcode: code, IconURL: icon})
			}
		}
	}
	return mentions, hashtags, emojis, nil
}

var attachmentTypes = []string{"Document", "Image", "Video", "Audio"}

// attachments reads up to MaxAttachments media attachments. A bare Link is not an attachment.
func (n normalizer) attachments(v any) []Attachment {
	var out []Attachment
	for _, v := range list(v) {
		if len(out) == MaxAttachments {
			break
		}
		o, ok := v.(object)
		if !ok {
			continue
		}
		i := slices.IndexFunc(strs(o.get("type")), func(t string) bool { return slices.Contains(attachmentTypes, t) })
		if i < 0 {
			continue
		}
		l := n.firstLink(o.get("url"))
		if l.Href == "" {
			continue
		}
		a := Attachment{
			Type:      strs(o.get("type"))[i],
			URL:       l.Href,
			MediaType: strings.TrimSpace(str(o.get("mediaType"))),
			Name:      strings.TrimSpace(str(o.get("name"))),
			Blurhash:  str(o.get("blurhash")),
			Width:     integer(o.get("width")),
			Height:    integer(o.get("height")),
		}
		if a.MediaType == "" {
			a.MediaType = l.MediaType
		}
		if fp := list(o.get("focalPoint")); len(fp) == 2 {
			x, okX := number(fp[0])
			y, okY := number(fp[1])
			if okX && okY {
				a.FocalPoint = &[2]float64{x, y}
			}
		}
		out = append(out, a)
	}
	return out
}

// links reads a property whose values are IRIs or Links.
func (n normalizer) links(v any) []Link {
	var out []Link
	for _, v := range list(v) {
		switch v := v.(type) {
		case string:
			if h := n.normalizeIRI(v); h != "" {
				out = append(out, Link{Href: h})
			}
		case object:
			if h := n.linkIRI(v); h != "" {
				out = append(out, Link{Href: h, MediaType: strings.TrimSpace(str(v.get("mediaType"))), Name: str(v.get("name"))})
			}
		}
	}
	return out
}

func (n normalizer) firstLink(v any) Link {
	if ls := n.links(v); len(ls) > 0 {
		return ls[0]
	}
	return Link{}
}

// displayURL returns the first text/html (or untyped) url, else the first url.
func (n normalizer) displayURL(v any) IRI {
	ls := n.links(v)
	for _, l := range ls {
		if l.MediaType == "" || strings.HasPrefix(l.MediaType, "text/html") {
			return l.Href
		}
	}
	if len(ls) > 0 {
		return ls[0].Href
	}
	return ""
}

// imageURL reads an icon or image: an IRI, a Link, or an Image object (or a list of them).
func (n normalizer) imageURL(v any) IRI {
	for _, v := range list(v) {
		switch v := v.(type) {
		case string:
			if h := n.normalizeIRI(v); h != "" {
				return h
			}
		case object:
			if h := n.firstLink(v.get("url")).Href; h != "" {
				return h
			}
			if h := n.normalizeIRI(str(v.get("href"))); h != "" {
				return h
			}
		}
	}
	return ""
}

func isLink(o object) bool {
	types := strs(o.get("type"))
	return slices.Contains(types, "Link") || slices.Contains(types, "Mention") || (o.has("href") && !o.has("id"))
}

// linkIRI returns the href of a Link or the id of an object.
func (n normalizer) linkIRI(o object) IRI {
	if isLink(o) {
		return n.normalizeIRI(str(o.get("href")))
	}
	return n.iri(o.get("id"))
}

// firstRef returns the IRI of the first value of a property that refers to a document.
func (n normalizer) firstRef(v any) IRI {
	for _, v := range list(v) {
		var r IRI
		switch v := v.(type) {
		case string:
			r = n.normalizeIRI(v)
		case object:
			r = n.linkIRI(v)
		}
		if r != "" {
			return r
		}
	}
	return ""
}

func (n normalizer) iri(v any) IRI { return n.normalizeIRI(str(v)) }

// normalizeIRI reads one IRI value: every spelling of Public becomes PublicIRI, and any other
// value goes through NormalizeIRI. A value NormalizeIRI rejects is "".
func (n normalizer) normalizeIRI(s string) IRI {
	switch strings.TrimSpace(s) {
	case "as:Public", "Public", string(PublicIRI):
		return PublicIRI
	}
	i, err := NormalizeIRI(s, n.host)
	if err != nil {
		return ""
	}
	return i
}

// list reads a property that may be single, an array, or null as a slice.
func list(v any) []any {
	switch v := v.(type) {
	case nil:
		return nil
	case []any:
		return v
	}
	return []any{v}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func strs(v any) []string {
	var out []string
	for _, v := range list(v) {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func number(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil
}

func integer(v any) int {
	f, ok := number(v)
	if !ok || f < 0 || f > 1<<31 {
		return 0
	}
	return int(f)
}
