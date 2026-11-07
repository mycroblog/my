// SPDX-License-Identifier: AGPL-3.0-only

package google_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"

	"github.com/mycroblog/my/internal/adapter/translator/google"
	"github.com/mycroblog/my/internal/adapter/translator/google/googletest"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/pkg/testkit"
)

const secret = "the words of a private post"

func newTranslator(t *testing.T) (*google.Translator, *googletest.Server) {
	t.Helper()
	s, opts := googletest.Start(t)
	tr, err := google.New(t.Context(), google.Options{Project: "my-project", ClientOptions: opts})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	return tr, s
}

// The adapter passes the contract of the port against the stand-in of Cloud Translation.
func TestContract(t *testing.T) {
	t.Parallel()
	testkit.TranslatorContract(t, func(t *testing.T) testkit.TranslatorHarness {
		t.Helper()
		tr, s := newTranslator(t)
		return testkit.TranslatorHarness{
			Translator: tr, Source: "en", Target: "ja", UnsupportedLanguage: "qaa",
			FailNext: func(err error) {
				if errors.Is(err, translation.ErrProviderLimit) {
					s.FailNext(codes.ResourceExhausted)
					return
				}
				s.FailNext(codes.Unavailable)
			},
		}
	})
}

// A request goes to the configured project and location as HTML, with no source for und, and the text comes back
// translated with its markers.
func TestTranslateSendsHTMLToTheParent(t *testing.T) {
	t.Parallel()
	s, opts := googletest.Start(t)
	tr, err := google.New(t.Context(), google.Options{Project: "my-project", Location: "us-central1", ClientOptions: opts})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	res, err := tr.Translate(t.Context(), translation.Request{
		Fragments:      []string{"a < b & ⟦1⟧ is ⟦2⟧nice⟦/2⟧\nbye"},
		SourceLanguage: translation.UndeterminedLanguage, TargetLanguage: "zh-Hant",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "A < B & ⟦1⟧ IS ⟦2⟧NICE⟦/2⟧\nBYE"; res.Translations[0] != want {
		t.Errorf("translation = %q, want %q", res.Translations[0], want)
	}
	if res.DetectedLanguage != "en" {
		t.Errorf("DetectedLanguage = %q, want en", res.DetectedLanguage)
	}
	req := s.Requests()[0]
	if req.GetParent() != "projects/my-project/locations/us-central1" || req.GetMimeType() != "text/html" ||
		req.GetSourceLanguageCode() != "" || req.GetTargetLanguageCode() != "zh-TW" {
		t.Errorf("request = %v", req)
	}
	if got := req.GetContents()[0]; !strings.HasPrefix(got, "a &lt; b &amp; <span") || !strings.Contains(got, "<br>bye") {
		t.Errorf("content = %q, want escaped HTML", got)
	}
}

// More fragments than one request holds go out in several, and the translations keep their order.
func TestTranslateBatchesManyFragments(t *testing.T) {
	t.Parallel()
	tr, s := newTranslator(t)
	fragments := make([]string, 2500)
	for i := range fragments {
		fragments[i] = "w"
	}
	fragments[2499] = "last"
	res, err := tr.Translate(t.Context(), translation.Request{Fragments: fragments, SourceLanguage: "en", TargetLanguage: "ja"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Translations) != len(fragments) || res.Translations[2499] != "LAST" {
		t.Errorf("got %d translations, the last %q", len(res.Translations), res.Translations[len(res.Translations)-1])
	}
	var sizes []int
	for _, r := range s.Requests() {
		sizes = append(sizes, len(r.GetContents()))
	}
	if !slices.Equal(sizes, []int{1024, 1024, 452}) {
		t.Errorf("requests of %v contents", sizes)
	}
}

// An answer whose markers do not come back is a temporary failure and returns nothing of itself.
func TestAnAnswerWithoutTheMarkersFails(t *testing.T) {
	t.Parallel()
	tr, s := newTranslator(t)
	s.Rewrite(func(string) string { return secret })
	_, err := tr.Translate(t.Context(), translation.Request{Fragments: []string{"a ⟦1⟧"}, SourceLanguage: "en", TargetLanguage: "ja"})
	if !errors.Is(err, translation.ErrTemporary) || strings.Contains(err.Error(), secret) {
		t.Errorf("Translate() = %v, want a temporary failure without the answer", err)
	}
}

// Every failure that is not a limit or a language is temporary.
func TestFailuresAreTemporary(t *testing.T) {
	t.Parallel()
	for _, code := range []codes.Code{codes.Unavailable, codes.DeadlineExceeded, codes.Internal, codes.PermissionDenied, codes.Unauthenticated} {
		tr, s := newTranslator(t)
		s.FailNext(code)
		_, err := tr.Translate(t.Context(), translation.Request{Fragments: []string{secret}, SourceLanguage: "en", TargetLanguage: "ja"})
		if !errors.Is(err, translation.ErrTemporary) || errors.Is(err, translation.ErrProviderLimit) {
			t.Errorf("%v: Translate() = %v, want a temporary failure", code, err)
		}
	}
}

// The configuration is checked before any client is made.
func TestNewChecksTheOptions(t *testing.T) {
	t.Parallel()
	for _, opts := range []google.Options{{}, {Project: "Bad Project"}, {Project: "my-project", Location: "us central"}} {
		if _, err := google.New(t.Context(), opts); err == nil {
			t.Errorf("New(%+v) = nil error", opts)
		}
	}
	for _, v := range []string{"my-project", "example.com:my-project"} {
		if err := google.CheckProject(v); err != nil {
			t.Errorf("CheckProject(%q) = %v", v, err)
		}
	}
	for _, v := range []string{"global", "us-central1", "europe-west4"} {
		if err := google.CheckLocation(v); err != nil {
			t.Errorf("CheckLocation(%q) = %v", v, err)
		}
	}
}
