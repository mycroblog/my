// SPDX-License-Identifier: AGPL-3.0-only

package deepl

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/pkg/testkit/translationtest"
)

const testKey = "0123abcd-4567-89ef-0123-456789abcdef:fx"

// standIn is a stand-in of DeepL's API: it lists a few languages and "translates" by putting the target code before each
// text, keeping its elements as DeepL does with tag_handling = xml.
type standIn struct {
	t *testing.T

	mu        sync.Mutex
	failNext  int // the status of the next translation, 0 for none
	requests  []translateRequest
	langCalls int
	// rewrite changes each text that the stand-in answers, after the translation.
	rewrite func(string) string
}

var standInLanguages = []languageEntry{
	{Lang: "de", UsableAsSource: true, UsableAsTarget: true},
	{Lang: "en", UsableAsSource: true},
	{Lang: "en-GB", UsableAsTarget: true},
	{Lang: "en-US", UsableAsTarget: true},
	{Lang: "ja", UsableAsSource: true, UsableAsTarget: true},
	{Lang: "pt", UsableAsSource: true},
	{Lang: "pt-BR", UsableAsTarget: true},
	{Lang: "pt-PT", UsableAsTarget: true},
	{Lang: "zh", UsableAsSource: true},
	{Lang: "zh-Hans", UsableAsTarget: true},
	{Lang: "zh-Hant", UsableAsTarget: true},
}

func (s *standIn) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "DeepL-Auth-Key "+testKey {
		http.Error(w, `{"message":"Authorization failed"}`, http.StatusForbidden)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v3/languages" && r.URL.Query().Get("resource") == "translate_text":
		s.mu.Lock()
		s.langCalls++
		s.mu.Unlock()
		_ = json.NewEncoder(w).Encode(standInLanguages)
	case r.Method == http.MethodPost && r.URL.Path == "/v2/translate":
		s.translate(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *standIn) translate(w http.ResponseWriter, r *http.Request) {
	var req translateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, `{"message":"Bad request"}`, http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.requests = append(s.requests, req)
	fail := s.failNext
	s.failNext = 0
	rewrite := s.rewrite
	s.mu.Unlock()
	if fail != 0 {
		// DeepL's answers carry no text of the request, but the stand-in's do, to prove that none reaches an error.
		w.WriteHeader(fail)
		_, _ = fmt.Fprintf(w, `{"message":"failed %s %s"}`, strings.Join(req.Text, " "), testKey)
		return
	}
	detected := strings.ToUpper(req.SourceLang)
	if detected == "" {
		detected = "EN"
	}
	type tr struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	}
	out := struct {
		Translations []tr `json:"translations"`
	}{}
	for _, text := range req.Text {
		text = "[" + req.TargetLang + "] " + text
		if rewrite != nil {
			text = rewrite(text)
		}
		out.Translations = append(out.Translations, tr{DetectedSourceLanguage: detected, Text: text})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func newStandIn(t *testing.T) (*standIn, *Translator) {
	t.Helper()
	s := &standIn{t: t}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return s, New(Options{APIKey: testKey, URL: srv.URL, HTTPClient: srv.Client()})
}

func TestContract(t *testing.T) {
	t.Parallel()
	translationtest.TranslatorContract(t, func(t *testing.T) translationtest.TranslatorHarness {
		s, tr := newStandIn(t)
		return translationtest.TranslatorHarness{
			Translator:          tr,
			Source:              "en",
			Target:              "ja",
			UnsupportedLanguage: "tlh",
			FailNext: func(err error) {
				s.mu.Lock()
				defer s.mu.Unlock()
				switch {
				case errors.Is(err, translation.ErrProviderLimit):
					s.failNext = 456
				default:
					s.failNext = http.StatusServiceUnavailable
				}
			},
		}
	})
}

func TestTranslateRequest(t *testing.T) {
	t.Parallel()
	s, tr := newStandIn(t)
	got, err := tr.Translate(t.Context(), translation.Request{
		Fragments:      []string{"Hi ⟦1⟧, <b>&</b> ⟦2⟧very⟦/2⟧ ⟦3⟧", "⟦1⟧"},
		SourceLanguage: translation.UndeterminedLanguage,
		TargetLanguage: "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"[en-US] Hi ⟦1⟧, <b>&</b> ⟦2⟧very⟦/2⟧ ⟦3⟧", "[en-US] ⟦1⟧"}; !slices.Equal(got.Translations, want) {
		t.Errorf("Translations = %q, want %q", got.Translations, want)
	}
	if got.DetectedLanguage != "en" {
		t.Errorf("DetectedLanguage = %q, want en", got.DetectedLanguage)
	}
	if len(s.requests) != 1 {
		t.Fatalf("%d requests, want the fragments in one", len(s.requests))
	}
	req := s.requests[0]
	if want := []string{"Hi <m1/>, &lt;b&gt;&amp;&lt;/b&gt; <p2>very</p2> <m3/>", "<m1/>"}; !slices.Equal(req.Text, want) {
		t.Errorf("text = %q, want %q", req.Text, want)
	}
	if req.SourceLang != "" || req.TargetLang != "en-US" || req.TagHandling != "xml" {
		t.Errorf("request = %+v, want no source_lang, target_lang en-US, and tag_handling xml", req)
	}
	if want := []string{"m1", "p2", "m3"}; !slices.Equal(req.NonSplittingTags, want) {
		t.Errorf("non_splitting_tags = %q, want %q", req.NonSplittingTags, want)
	}
}

func TestTranslateSourceLanguage(t *testing.T) {
	t.Parallel()
	s, tr := newStandIn(t)
	got, err := tr.Translate(t.Context(), translation.Request{Fragments: []string{"Olá"}, SourceLanguage: "pt-BR", TargetLanguage: "de"})
	if err != nil {
		t.Fatal(err)
	}
	if s.requests[0].SourceLang != "pt" || got.DetectedLanguage != "pt" {
		t.Errorf("source_lang = %q, DetectedLanguage = %q, want pt", s.requests[0].SourceLang, got.DetectedLanguage)
	}
}

func TestTranslateBrokenMarkers(t *testing.T) {
	t.Parallel()
	s, tr := newStandIn(t)
	s.rewrite = func(text string) string { return strings.ReplaceAll(text, "<m1/>", "") }
	_, err := tr.Translate(t.Context(), translation.Request{Fragments: []string{"Hi ⟦1⟧"}, SourceLanguage: "en", TargetLanguage: "ja"})
	if !errors.Is(err, translation.ErrTemporary) {
		t.Errorf("Translate() = %v, want ErrTemporary", err)
	}
}

func TestTranslateBatches(t *testing.T) {
	t.Parallel()
	s, tr := newStandIn(t)
	fragments := make([]string, 6)
	for i := range fragments {
		fragments[i] = fmt.Sprintf("%d %s", i, strings.Repeat("word ", 10000))
	}
	got, err := tr.Translate(t.Context(), translation.Request{Fragments: fragments, SourceLanguage: "en", TargetLanguage: "ja"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.requests) < 2 {
		t.Errorf("%d requests, want the fragments split under the size limit", len(s.requests))
	}
	for _, req := range s.requests {
		if n := encodedLen(req); n > maxRequestBytes {
			t.Errorf("a request of %d bytes, over %d", n, maxRequestBytes)
		}
	}
	for i, f := range fragments {
		if got.Translations[i] != "[ja] "+f {
			t.Errorf("translation %d is not the one of its fragment", i)
		}
	}
}

func TestStatusErrors(t *testing.T) {
	t.Parallel()
	for status, want := range map[int]error{
		456:                              translation.ErrProviderLimit,
		http.StatusTooManyRequests:       translation.ErrProviderLimit,
		529:                              translation.ErrProviderLimit,
		http.StatusBadRequest:            translation.ErrTemporary,
		http.StatusForbidden:             translation.ErrTemporary,
		http.StatusRequestEntityTooLarge: translation.ErrTemporary,
		http.StatusInternalServerError:   translation.ErrTemporary,
		http.StatusServiceUnavailable:    translation.ErrTemporary,
	} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			t.Parallel()
			s, tr := newStandIn(t)
			s.failNext = status
			_, err := tr.Translate(t.Context(), translation.Request{Fragments: []string{"secret words"}, SourceLanguage: "en", TargetLanguage: "ja"})
			if !errors.Is(err, want) {
				t.Fatalf("Translate() = %v, want %v", err, want)
			}
			if msg := err.Error(); strings.Contains(msg, "secret words") || strings.Contains(msg, testKey) || strings.Contains(msg, "failed") {
				t.Errorf("the error %q holds what the answer said", msg)
			}
		})
	}
}

func TestRefusedKey(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(&standIn{t: t})
	t.Cleanup(srv.Close)
	tr := New(Options{APIKey: "wrong-key", URL: srv.URL, HTTPClient: srv.Client()})
	_, err := tr.Translate(t.Context(), translation.Request{Fragments: []string{"Hi"}, SourceLanguage: "en", TargetLanguage: "ja"})
	if !errors.Is(err, translation.ErrTemporary) || strings.Contains(err.Error(), "wrong-key") {
		t.Errorf("Translate() = %v, want ErrTemporary without the key", err)
	}
}

// keyEchoingTransport fails every request with an error that holds its Authorization header, as a broken proxy or
// transport might.
type keyEchoingTransport struct{}

func (keyEchoingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, errors.New("dial failed for " + r.Header.Get("Authorization") + " at " + r.URL.String())
}

func TestErrorsHideTheKey(t *testing.T) {
	t.Parallel()
	t.Run("TransportError", func(t *testing.T) {
		t.Parallel()
		tr := New(Options{APIKey: testKey, URL: "https://deepl.invalid", HTTPClient: &http.Client{Transport: keyEchoingTransport{}}})
		_, err := tr.Translate(t.Context(), translation.Request{Fragments: []string{"Hi"}, SourceLanguage: "en", TargetLanguage: "ja"})
		if !errors.Is(err, translation.ErrTemporary) {
			t.Fatalf("Translate() = %v, want ErrTemporary", err)
		}
		if strings.Contains(err.Error(), testKey) || !strings.Contains(err.Error(), "[redacted]") {
			t.Errorf("the error %q does not hide the key", err)
		}
	})
	t.Run("Unreachable", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		tr := New(Options{APIKey: testKey, URL: url})
		_, err := tr.Translate(t.Context(), translation.Request{Fragments: []string{"Hi"}, SourceLanguage: "en", TargetLanguage: "ja"})
		if !errors.Is(err, translation.ErrTemporary) || strings.Contains(err.Error(), testKey) {
			t.Errorf("Translate() = %v, want ErrTemporary without the key", err)
		}
	})
	t.Run("Redirect", func(t *testing.T) {
		t.Parallel()
		other := httptest.NewServer(&standIn{t: t})
		t.Cleanup(other.Close)
		srv := httptest.NewServer(http.RedirectHandler(other.URL+"/v3/languages?resource=translate_text", http.StatusFound))
		t.Cleanup(srv.Close)
		tr := New(Options{APIKey: testKey, URL: srv.URL})
		if _, err := tr.Translate(t.Context(), translation.Request{Fragments: []string{"Hi"}, SourceLanguage: "en", TargetLanguage: "ja"}); !errors.Is(err, translation.ErrTemporary) {
			t.Errorf("Translate() = %v, want ErrTemporary: the key must not follow a redirect", err)
		}
	})
}

func TestBaseURL(t *testing.T) {
	t.Parallel()
	for key, want := range map[string]string{
		"0123abcd-4567-89ef-0123-456789abcdef:fx": FreeURL,
		"0123abcd-4567-89ef-0123-456789abcdef":    ProURL,
		"":                                        ProURL,
	} {
		if got := BaseURL(key); got != want {
			t.Errorf("BaseURL(%q) = %q, want %q", key, got, want)
		}
		if got := New(Options{APIKey: key}).url; got != want {
			t.Errorf("New(%q).url = %q, want %q", key, got, want)
		}
	}
	if got := New(Options{APIKey: testKey, URL: "https://proxy.example/deepl/"}).url; got != "https://proxy.example/deepl" {
		t.Errorf("New with a URL = %q", got)
	}
}

func TestLanguages(t *testing.T) {
	t.Parallel()
	l := newLanguages(standInLanguages, time.Time{})
	for tag, want := range map[string]string{
		"en": "en-US", "en-US": "en-US", "en-GB": "en-GB", "en-AU": "en-GB",
		"pt": "pt-BR", "pt-PT": "pt-PT", "zh": "zh-Hans", "zh-TW": "zh-Hant", "zh-Hant": "zh-Hant",
		"ja": "ja", "ja-JP": "ja", "de-CH": "de",
		"tlh": "", "fr": "", "": "", "und": "",
	} {
		got, ok := l.target(tag)
		if got != want || ok != (want != "") {
			t.Errorf("target(%q) = %q, %t, want %q", tag, got, ok, want)
		}
	}
	for tag, want := range map[string]string{
		"en": "en", "en-US": "en", "pt-BR": "pt", "zh-Hant": "zh", "zh-TW": "zh", "ja": "ja", "tlh": "", "fr": "",
	} {
		got, ok := l.source(tag)
		if got != want || ok != (want != "") {
			t.Errorf("source(%q) = %q, %t, want %q", tag, got, ok, want)
		}
	}
}

func TestLanguagesCache(t *testing.T) {
	t.Parallel()
	s := &standIn{t: t}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	tr := New(Options{APIKey: testKey, URL: srv.URL, HTTPClient: srv.Client(), Now: func() time.Time { return now }})
	req := translation.Request{Fragments: []string{"Hi"}, SourceLanguage: "en", TargetLanguage: "ja"}
	for range 2 {
		if _, err := tr.Translate(t.Context(), req); err != nil {
			t.Fatal(err)
		}
	}
	if s.langCalls != 1 {
		t.Errorf("%d calls of the languages, want 1", s.langCalls)
	}
	now = now.Add(languagesTTL)
	if _, err := tr.Translate(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if s.langCalls != 2 {
		t.Errorf("%d calls of the languages after a day, want 2", s.langCalls)
	}
}

func TestDetectedLanguage(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ code, source, want string }{
		{"EN", "und", "en"},
		{"ZH", "und", "zh"},
		{"", "und", "und"},
		{"", "ja", "ja"},
		{"not a tag!", "und", "und"},
		{"DE", "en", "de"},
	} {
		if got := detectedLanguage(c.code, c.source); got != c.want {
			t.Errorf("detectedLanguage(%q, %q) = %q, want %q", c.code, c.source, got, c.want)
		}
	}
}

func TestFromTags(t *testing.T) {
	t.Parallel()
	for answer, want := range map[string]string{
		"Hi <m1/>, <p2>very</p2>":    "Hi ⟦1⟧, ⟦2⟧very⟦/2⟧",
		"Hi <m1></m1> and <m2 />":    "Hi ⟦1⟧ and ⟦2⟧",
		"<p3/>":                      "⟦3⟧⟦/3⟧",
		"&lt;m1/&gt; &amp; &#39;":    "<m1/> & '",
		"<b>not ours</b>":            "<b>not ours</b>",
		"<p1>outer <p2>in</p2></p1>": "⟦1⟧outer ⟦2⟧in⟦/2⟧⟦/1⟧",
	} {
		if got := fromTags(answer); got != want {
			t.Errorf("fromTags(%q) = %q, want %q", answer, got, want)
		}
	}
}
