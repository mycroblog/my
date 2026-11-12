// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// TextKey names an instance text (instance_text_revisions.key). A text holds
// one value, written by an administrator in any language and shown as it is in every locale.
type TextKey string

// Text keys. Adding one means adding its definition below and its value to the CHECK
// constraint of instance_text_revisions.key in schema/<dialect>/09-instance.sql.
const (
	TextName             TextKey = "name"
	TextShortDescription TextKey = "short_description"
	TextDescription      TextKey = "description"
	TextTerms            TextKey = "terms"
	TextPrivacyPolicy    TextKey = "privacy_policy"
	TextPreviewImageAlt  TextKey = "preview_image_alt"
	TextThumbnailAlt     TextKey = "thumbnail_alt"
	// TextCommerceDisclosure is the page of the disclosures that the law of the operator's
	// country asks of a seller (特定商取引法に基づく表記, ADR-0072 §13), at /about/commerce. The
	// server never vouches for what it says.
	TextCommerceDisclosure TextKey = "commerce_disclosure"
)

// TextDefinition describes a text key.
type TextDefinition struct {
	Key TextKey
	// MaxLength bounds the body, in characters.
	MaxLength int
	// SingleLine keys reject line breaks.
	SingleLine bool
	// Document keys hold a page of Markdown that the instance publishes (the terms of service,
	// the privacy policy, and the commerce disclosures); the other keys are plain text.
	Document bool
	Doc      string
}

var textDefinitions = []TextDefinition{
	{Key: TextName, MaxLength: 100, SingleLine: true, Doc: "Name of the instance."},
	{Key: TextShortDescription, MaxLength: 500, Doc: "One-paragraph summary of the instance."},
	{Key: TextDescription, MaxLength: 20000, Doc: "Full description of the instance."},
	{Key: TextTerms, MaxLength: 100000, Document: true, Doc: "Terms of service, in Markdown."},
	{Key: TextPrivacyPolicy, MaxLength: 100000, Document: true, Doc: "Privacy policy, in Markdown."},
	{Key: TextPreviewImageAlt, MaxLength: 1500, Doc: "Alt text of the preview_image setting's image (og:image:alt)."},
	{Key: TextThumbnailAlt, MaxLength: 1500, Doc: "Alt text of the thumbnail setting's image."},
	{Key: TextCommerceDisclosure, MaxLength: 100000, Document: true, Doc: "Disclosures of the seller of the supporter plans, in Markdown."},
}

// TextDefinitions returns the definitions of every text key, in a fixed order.
func TextDefinitions() []TextDefinition { return slices.Clone(textDefinitions) }

// LookupText returns the definition of key, or an error wrapping ErrUnknownKey.
func LookupText(key TextKey) (TextDefinition, error) {
	for _, d := range textDefinitions {
		if d.Key == key {
			return d, nil
		}
	}
	return TextDefinition{}, fmt.Errorf("text %q: %w", key, ErrUnknownKey)
}

// Validate checks body against d. An empty body is valid: it clears the text. A SingleLine
// text of only spaces is not: it would name the instance with nothing visible (TextName).
func (d TextDefinition) Validate(body string) error {
	invalid := func(why string) error { return fmt.Errorf("text %s: %s: %w", d.Key, why, ErrInvalidValue) }
	switch {
	case !utf8.ValidString(body):
		return invalid("not valid UTF-8")
	case strings.ContainsRune(body, 0):
		return invalid("contains NUL") // which PostgreSQL's text cannot hold
	case utf8.RuneCountInString(body) > d.MaxLength:
		return invalid(fmt.Sprintf("longer than %d characters", d.MaxLength))
	case d.SingleLine && strings.ContainsAny(body, "\r\n"):
		return invalid("contains a line break")
	case d.SingleLine && body != "" && strings.TrimSpace(body) == "":
		return invalid("is blank")
	}
	return nil
}

// TextRevision is one version of a text (instance_text_revisions). The revision with the
// greatest ID is the current text of its key; an empty Body means the text is cleared.
type TextRevision struct {
	ID   string
	Key  TextKey
	Body string
	// RevisedByUserID is the user who made the change, or "" for a change from the CLI.
	RevisedByUserID string
	RevisedAt       time.Time
}

// CurrentText points at the current revision of a text (the instance_texts projection).
type CurrentText struct {
	Key        TextKey
	RevisionID string
}

// Supersedes reports whether r should replace the current text c, which is the zero value
// when the key has no text yet.
func (r TextRevision) Supersedes(c CurrentText) bool {
	return c.RevisionID == "" || r.ID > c.RevisionID
}

// Text is the current text of a key.
type Text struct {
	Key        TextKey
	Body       string
	RevisionID string
	RevisedAt  time.Time
}
