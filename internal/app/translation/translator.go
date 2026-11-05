// SPDX-License-Identifier: AGPL-3.0-only

// Package translation translates posts on a reader's request (ADR-0148). It defines the
// Translator port, which a provider implements: the adapters live in
// internal/adapter/translator/<provider> and pass testkit.TranslatorContract.
package translation

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// UndeterminedLanguage is the source language of a post whose language is unknown (BCP 47 "und").
const UndeterminedLanguage = "und"

// Translator is the port of a translation provider. The provider's name, link, and logo are
// not part of it (ADR-0148 §2). Translate calls the provider, so it runs outside transactions
// (ADR-0003).
type Translator interface {
	// Translate returns the translations of the request's fragments. A failure wraps
	// ErrTemporary, ErrProviderLimit, or ErrUnsupportedLanguage, and never contains the text of
	// a fragment.
	Translate(ctx context.Context, req Request) (Result, error)
}

// Request is what a provider receives: the text of a post and no IRI, author, viewer, handle,
// or URL (ADR-0148 §5).
type Request struct {
	// Fragments holds one or more non-empty fragments of text. The markers of Marker and
	// ClosingMarker stand in for what must not be translated or sent; a fragment may be
	// nothing but markers.
	Fragments []string
	// SourceLanguage is a BCP 47 tag, or UndeterminedLanguage to have the provider detect it.
	SourceLanguage string
	// TargetLanguage is a BCP 47 tag.
	TargetLanguage string
}

// Result is the provider's answer to a Request.
type Result struct {
	// Translations holds one translation per fragment, in the order of the fragments, each
	// with the markers of its fragment in the form that Marker and ClosingMarker write.
	Translations []string
	// DetectedLanguage is the BCP 47 tag of the source: what the provider detected, or the
	// request's SourceLanguage when the provider does not say. It is UndeterminedLanguage only
	// for a source of UndeterminedLanguage that the provider could not determine.
	DetectedLanguage string
}

var (
	// ErrTemporary reports a failure that may pass: the provider is unreachable, times out,
	// or answers with an error of its own, or the context ended.
	ErrTemporary = errors.New("translator: temporary failure")
	// ErrProviderLimit reports a limit that the provider stated: its rate limit or its quota.
	// The monthly limits of ADR-0148 §11 are the server's own and never come from a Translator.
	ErrProviderLimit = errors.New("translator: provider limit reached")
	// ErrUnsupportedLanguage reports a source or target language that the provider does not
	// translate. A provider that cannot detect a language answers it to UndeterminedLanguage.
	ErrUnsupportedLanguage = errors.New("translator: unsupported language")
)

// The markers replace mentions, hashtags, links, :shortcode:, pre, and code, so that their
// content is neither translated nor sent, and a pair keeps decoration such as b and em (ADR-0148
// §8). This is the only form on the port: an adapter whose provider needs another form rewrites
// the fragments on the way out and the markers on the way back.
//
// A marker holds no HTML metacharacter, so it means the same in the plain text of a content
// warning and in a body, and one left behind in an output is never taken for a tag. U+27E6 and
// U+27E7 have no compatibility decomposition, so Unicode normalization keeps them.
var markerPattern = regexp.MustCompile(`⟦/?[1-9][0-9]*⟧`)

// Marker returns marker number n, from 1, which stands for one replaced item or opens a pair:
// ⟦1⟧. A number appears once in a fragment, or twice for a pair. Text of a post that already
// has the form of a marker is replaced by a marker too, so that it cannot be mistaken for one.
func Marker(n int) string {
	return "⟦" + strconv.Itoa(n) + "⟧"
}

// ClosingMarker returns the marker that closes the pair opened by Marker(n): ⟦/1⟧.
func ClosingMarker(n int) string {
	return "⟦/" + strconv.Itoa(n) + "⟧"
}

// MarkersIntact reports whether every marker of the fragment comes back in the translation
// exactly once, and no other marker appears. A translation that fails it is a failure and is not
// stored (ADR-0148 §8).
//
// The items may move, as word order differs between languages, but a pair stays a pair: it opens
// before it closes, and pairs nest. A closing marker ahead of its opening one, or pairs that
// cross, would put markup around the words that the post never had there.
func MarkersIntact(fragment, translation string) bool {
	unseen := make(map[string]bool)
	for _, m := range markerPattern.FindAllString(fragment, -1) {
		if _, dup := unseen[m]; dup {
			return false
		}
		unseen[m] = true
	}
	var open []string // the closing markers of the pairs that are open, innermost last
	for _, m := range markerPattern.FindAllString(translation, -1) {
		if !unseen[m] {
			return false
		}
		unseen[m] = false
		if strings.HasPrefix(m, "⟦/") {
			if len(open) == 0 || open[len(open)-1] != m {
				return false
			}
			open = open[:len(open)-1]
			continue
		}
		closing := "⟦/" + strings.TrimPrefix(m, "⟦")
		if _, pair := unseen[closing]; pair {
			open = append(open, closing)
		}
	}
	if len(open) > 0 {
		return false
	}
	for _, left := range unseen {
		if left {
			return false
		}
	}
	return true
}

// CountCharacters returns the characters (code points) that the fragments send to a provider,
// the unit of the monthly limits (ADR-0148 §11). Markers are not counted.
func CountCharacters(fragments []string) int {
	n := 0
	for _, f := range fragments {
		n += utf8.RuneCountInString(markerPattern.ReplaceAllString(f, ""))
	}
	return n
}
