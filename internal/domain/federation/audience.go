// SPDX-License-Identifier: AGPL-3.0-only

package federation

import "slices"

// Visibility is the visibility of a post as federation sees it. The values equal the content
// module's visibilities. A received post maps to every value but local: a remote post is never
// local (ADR-0017); only a local post is, and Address keeps it to the author's
// followers.
type Visibility string

// Visibilities.
const (
	VisibilityPublic   Visibility = "public"
	VisibilityLocal    Visibility = "local"
	VisibilityUnlisted Visibility = "unlisted"
	VisibilityPrivate  Visibility = "private"
	VisibilityDirect   Visibility = "direct"
)

// Visibility maps the addressing to a visibility: Public in to → public, Public in cc →
// unlisted, the author's followers collection in to or cc → private, otherwise direct.
// followers is the author's followers collection IRI; "" never matches.
func (a Audience) Visibility(followers IRI) Visibility {
	switch {
	case slices.Contains(a.To, PublicIRI):
		return VisibilityPublic
	case slices.Contains(a.Cc, PublicIRI):
		return VisibilityUnlisted
	case followers != "" && (slices.Contains(a.To, followers) || slices.Contains(a.Cc, followers)):
		return VisibilityPrivate
	}
	return VisibilityDirect
}

// Addressed returns the recipients in to and cc other than Public and the followers
// collection, without duplicates, in order. The caller keeps the local actors among them as
// mentions.
func (a Audience) Addressed(followers IRI) []IRI {
	var out []IRI
	for _, r := range slices.Concat(a.To, a.Cc) {
		if r == PublicIRI || (followers != "" && r == followers) || slices.Contains(out, r) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// MentionedIRIs returns the actors a post mentions: the Mention tags, then the recipients for
// which isLocal is true even without a tag, without duplicates.
func (o *Object) MentionedIRIs(followers IRI, isLocal func(IRI) bool) []IRI {
	var out []IRI
	for _, m := range o.Mentions {
		if m.Href != "" && !slices.Contains(out, m.Href) {
			out = append(out, m.Href)
		}
	}
	for _, r := range o.Audience.Addressed(followers) {
		if isLocal(r) && !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}
