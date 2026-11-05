// SPDX-License-Identifier: AGPL-3.0-only

package fake

import (
	"errors"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestContract(t *testing.T) {
	t.Parallel()
	testkit.TranslatorContract(t, func(*testing.T) testkit.TranslatorHarness {
		f := New(Options{Detected: "en", Unsupported: []string{"tlh"}})
		return testkit.TranslatorHarness{
			Translator:          f,
			Source:              "en",
			Target:              "ja",
			UnsupportedLanguage: "tlh",
			FailNext:            f.FailNext,
		}
	})
}

func TestTranslate(t *testing.T) {
	t.Parallel()
	got, err := New(Options{Detected: "fr"}).Translate(t.Context(), translation.Request{
		Fragments:      []string{"Bonjour ⟦1⟧", "⟦2⟧"},
		SourceLanguage: translation.UndeterminedLanguage,
		TargetLanguage: "ja",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"[ja] Bonjour ⟦1⟧", "[ja] ⟦2⟧"}; !slices.Equal(got.Translations, want) {
		t.Errorf("Translations = %q, want %q", got.Translations, want)
	}
	if got.DetectedLanguage != "fr" {
		t.Errorf("DetectedLanguage = %q, want fr", got.DetectedLanguage)
	}
}

func TestTranslateWithoutDetection(t *testing.T) {
	t.Parallel()
	got, err := New(Options{}).Translate(t.Context(), translation.Request{
		Fragments:      []string{"Bonjour"},
		SourceLanguage: translation.UndeterminedLanguage,
		TargetLanguage: "ja",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.DetectedLanguage != translation.UndeterminedLanguage {
		t.Errorf("DetectedLanguage = %q, want und", got.DetectedLanguage)
	}
}

func TestFailNextQueuesInOrder(t *testing.T) {
	t.Parallel()
	f := New(Options{})
	f.FailNext(translation.ErrProviderLimit)
	f.FailNext(translation.ErrTemporary)
	req := translation.Request{Fragments: []string{"Hello"}, SourceLanguage: "en", TargetLanguage: "ja"}
	for _, want := range []error{translation.ErrProviderLimit, translation.ErrTemporary, nil} {
		if _, err := f.Translate(t.Context(), req); !errors.Is(err, want) {
			t.Errorf("Translate() = %v, want %v", err, want)
		}
	}
}
