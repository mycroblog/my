// SPDX-License-Identifier: AGPL-3.0-only

package translationtest

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/translation"
)

// TranslatorHarness is one translator under test, on a stand-in of its provider that the test
// controls, never the provider itself.
type TranslatorHarness struct {
	Translator translation.Translator
	// Source and Target are a pair of languages that it translates between.
	Source string
	Target string
	// UnsupportedLanguage is a language that it refuses, as a source and as a target.
	UnsupportedLanguage string
	// FailNext makes the provider fail the next call in the way that the adapter must report
	// as err: translation.ErrTemporary or translation.ErrProviderLimit.
	FailNext func(err error)
}

// translatorLanguageTag is the shape of a BCP 47 tag, enough to tell one from an empty value or
// a provider's own.
var translatorLanguageTag = regexp.MustCompile(`^[a-zA-Z]{2,8}(-[a-zA-Z0-9]{1,8})*$`)

// TranslatorContract verifies a translation.Translator adapter (ADR-0148 §2, §8): it answers
// one translation per fragment in the order of the fragments; every marker of a fragment comes
// back exactly once in the form of translation.Marker and translation.ClosingMarker, also from
// a fragment of nothing but markers; it reports the source language as a language tag, for a
// source of "und" too unless it refuses that as unsupported; it tells a temporary failure, a
// limit reported by the provider, and an unsupported language apart, and a failure stays with
// its one call; an ended context is a temporary failure; and no error contains the text of a
// fragment.
func TranslatorContract(t *testing.T, newHarness func(t *testing.T) TranslatorHarness) {
	t.Helper()
	const secret = "the words of a private post"
	t.Run("KeepsTheFragmentsInOrder", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		// Each fragment has a marker of its own, so the markers tell whose translation is where.
		fragments := make([]string, 8)
		for i := range fragments {
			fragments[i] = fmt.Sprintf("Sentence number %d by %s.", i+1, translation.Marker(i+1))
		}
		got := translatorTranslate(t, h, fragments)
		for i, tr := range got.Translations {
			if !translation.MarkersIntact(fragments[i], tr) {
				t.Errorf("translation %d = %q, not the one of %q", i, tr, fragments[i])
			}
		}
	})
	t.Run("ReturnsEveryMarkerOnce", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		first, second := translation.Marker(1), translation.Marker(2)
		opening, closing := translation.Marker(3), translation.ClosingMarker(3)
		fragments := []string{
			"Hello " + first + ", this is " + opening + "important" + closing + " for " + second + ".",
			first + second + " are next to each other.",
			"A list:\n" + first + "\n" + second,
			first,
			opening + closing,
		}
		got := translatorTranslate(t, h, fragments)
		for i, tr := range got.Translations {
			if !translation.MarkersIntact(fragments[i], tr) {
				t.Errorf("translation of %q = %q, its markers are not intact", fragments[i], tr)
			}
		}
	})
	t.Run("ReportsTheSourceLanguage", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		got := translatorTranslate(t, h, []string{"Hello."})
		if !translatorLanguageTag.MatchString(got.DetectedLanguage) {
			t.Errorf("DetectedLanguage = %q, want a language tag", got.DetectedLanguage)
		}
	})
	t.Run("DetectsAnUndeterminedLanguage", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		got, err := h.Translator.Translate(t.Context(), translation.Request{
			Fragments:      []string{"Hello."},
			SourceLanguage: translation.UndeterminedLanguage,
			TargetLanguage: h.Target,
		})
		if errors.Is(err, translation.ErrUnsupportedLanguage) {
			return // A provider that cannot detect a language says so.
		}
		if err != nil {
			t.Fatalf("Translate(und) = %v", err)
		}
		if len(got.Translations) != 1 {
			t.Errorf("got %d translations, want 1", len(got.Translations))
		}
		if !translatorLanguageTag.MatchString(got.DetectedLanguage) {
			t.Errorf("DetectedLanguage = %q, want a language tag or und", got.DetectedLanguage)
		}
	})
	t.Run("RefusesAnUnsupportedLanguage", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		for _, req := range []translation.Request{
			{Fragments: []string{secret}, SourceLanguage: h.UnsupportedLanguage, TargetLanguage: h.Target},
			{Fragments: []string{secret}, SourceLanguage: h.Source, TargetLanguage: h.UnsupportedLanguage},
		} {
			_, err := h.Translator.Translate(t.Context(), req)
			translatorWantKind(t, err, translation.ErrUnsupportedLanguage, secret)
		}
	})
	t.Run("TellsTheFailuresApart", func(t *testing.T) {
		t.Parallel()
		for _, kind := range []error{translation.ErrTemporary, translation.ErrProviderLimit} {
			h := newHarness(t)
			req := translation.Request{Fragments: []string{secret}, SourceLanguage: h.Source, TargetLanguage: h.Target}
			h.FailNext(kind)
			_, err := h.Translator.Translate(t.Context(), req)
			translatorWantKind(t, err, kind, secret)
			if _, err := h.Translator.Translate(t.Context(), req); err != nil {
				t.Errorf("Translate() after the failure = %v, want success", err)
			}
		}
	})
	t.Run("ReportsAnEndedContextAsTemporary", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := h.Translator.Translate(ctx, translation.Request{Fragments: []string{secret}, SourceLanguage: h.Source, TargetLanguage: h.Target})
		translatorWantKind(t, err, translation.ErrTemporary, secret)
	})
}

// translatorTranslate translates fragments from the harness's source to its target and checks
// that the answer has one translation per fragment.
func translatorTranslate(t *testing.T, h TranslatorHarness, fragments []string) translation.Result {
	t.Helper()
	got, err := h.Translator.Translate(t.Context(), translation.Request{Fragments: fragments, SourceLanguage: h.Source, TargetLanguage: h.Target})
	if err != nil {
		t.Fatalf("Translate() = %v", err)
	}
	if len(got.Translations) != len(fragments) {
		t.Fatalf("got %d translations of %d fragments", len(got.Translations), len(fragments))
	}
	return got
}

// translatorWantKind checks that err is of kind alone among the port's errors and holds no text
// of a fragment.
func translatorWantKind(t *testing.T, err, kind error, text string) {
	t.Helper()
	for _, k := range []error{translation.ErrTemporary, translation.ErrProviderLimit, translation.ErrUnsupportedLanguage} {
		if got, want := errors.Is(err, k), errors.Is(kind, k); got != want {
			t.Errorf("errors.Is(%v, %v) = %t, want %t", err, k, got, want)
		}
	}
	if err != nil && strings.Contains(err.Error(), text) {
		t.Errorf("the error %q holds the text of a fragment", err)
	}
}
