// SPDX-License-Identifier: AGPL-3.0-only

package libretranslate

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	nethtml "golang.org/x/net/html"

	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/pkg/testkit"
)

const testKey = "lt-secret-key-0123456789"

// standIn is a stand-in of a LibreTranslate server: it "translates" HTML by upper-casing its text
// nodes and keeping its elements, as LibreTranslate keeps them with format=html.
type standIn struct {
	*httptest.Server
	mu       sync.Mutex
	failNext int // the status of the next answer of /translate, 0 for none
	requests []translateRequest
}

func newStandIn(t *testing.T) *standIn {
	t.Helper()
	s := &standIn{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /languages", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, []languageEntry{
			{Code: "en", Targets: []string{"en", "ja", "zh-Hans", "zh-Hant"}},
			{Code: "ja", Targets: []string{"en", "ja"}},
			{Code: "zh-Hans", Targets: []string{"en", "zh-Hant"}},
			{Code: "zh-Hant", Targets: []string{"en", "zh-Hans"}},
		})
	})
	mux.HandleFunc("POST /translate", func(w http.ResponseWriter, r *http.Request) {
		var req translateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
			return
		}
		s.mu.Lock()
		s.requests = append(s.requests, req)
		fail := s.failNext
		s.failNext = 0
		s.mu.Unlock()
		switch {
		case fail != 0:
			writeJSON(w, fail, map[string]string{"error": "Slowdown: 10 per 1 minute"})
			return
		case req.APIKey != testKey:
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Invalid API key"})
			return
		case req.Format != "html":
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "format must be html"})
			return
		}
		out := make([]string, len(req.Q))
		for i, q := range req.Q {
			out[i] = upperText(q)
		}
		res := map[string]any{"translatedText": out}
		if req.Source == "auto" {
			ds := make([]detection, len(req.Q))
			for i := range ds {
				ds[i] = detection{Language: "en", Confidence: 90}
			}
			res["detectedLanguage"] = ds
		}
		writeJSON(w, http.StatusOK, res)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *standIn) translator(t *testing.T, key string) *Translator {
	t.Helper()
	tr, err := New(Options{URL: s.URL + "/", APIKey: key, HTTPClient: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// upperText upper-cases the text of HTML and keeps its elements as they are.
func upperText(in string) string {
	var b strings.Builder
	z := nethtml.NewTokenizer(strings.NewReader(in))
	for {
		switch z.Next() {
		case nethtml.ErrorToken:
			return b.String()
		case nethtml.TextToken:
			b.WriteString(strings.ToUpper(string(z.Raw())))
		default:
			b.Write(z.Raw())
		}
	}
}

func TestContract(t *testing.T) {
	t.Parallel()
	testkit.TranslatorContract(t, func(t *testing.T) testkit.TranslatorHarness {
		s := newStandIn(t)
		return testkit.TranslatorHarness{
			Translator:          s.translator(t, testKey),
			Source:              "en",
			Target:              "ja",
			UnsupportedLanguage: "tlh",
			FailNext: func(err error) {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.failNext = http.StatusInternalServerError
				if errors.Is(err, translation.ErrProviderLimit) {
					s.failNext = http.StatusTooManyRequests
				}
			},
		}
	})
}

func TestTranslate(t *testing.T) {
	t.Parallel()
	s := newStandIn(t)
	got, err := s.translator(t, testKey).Translate(t.Context(), translation.Request{
		Fragments:      []string{"Hi ⟦1⟧, this is ⟦2⟧very⟦/2⟧ <b>raw</b> & fine.", "⟦1⟧ ⟦2⟧"},
		SourceLanguage: translation.UndeterminedLanguage,
		TargetLanguage: "zh-TW",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"HI ⟦1⟧, THIS IS ⟦2⟧VERY⟦/2⟧ <B>RAW</B> & FINE.", "⟦1⟧ ⟦2⟧"}
	if !slices.Equal(got.Translations, want) {
		t.Errorf("Translations = %q, want %q", got.Translations, want)
	}
	if got.DetectedLanguage != "en" {
		t.Errorf("DetectedLanguage = %q, want en", got.DetectedLanguage)
	}
	if len(s.requests) != 1 {
		t.Fatalf("%d calls of /translate, want 1", len(s.requests))
	}
	req := s.requests[0]
	if req.Source != "auto" || req.Target != "zh-Hant" {
		t.Errorf("source, target = %q, %q, want auto, zh-Hant", req.Source, req.Target)
	}
	// The fragment of nothing but markers is not sent, and the text is escaped.
	wantQ := []string{`Hi <code id="m1" translate="no"></code>, this is <span id="m2">very</span> &lt;b&gt;raw&lt;/b&gt; &amp; fine.`}
	if !slices.Equal(req.Q, wantQ) {
		t.Errorf("q = %q, want %q", req.Q, wantQ)
	}
}

func TestTranslateNamesTheSourceLanguage(t *testing.T) {
	t.Parallel()
	s := newStandIn(t)
	got, err := s.translator(t, testKey).Translate(t.Context(), translation.Request{
		Fragments: []string{"Hello."}, SourceLanguage: "en-US", TargetLanguage: "ja",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.DetectedLanguage != "en-US" {
		t.Errorf("DetectedLanguage = %q, want en-US", got.DetectedLanguage)
	}
	if req := s.requests[0]; req.Source != "en" || req.Target != "ja" {
		t.Errorf("source, target = %q, %q, want en, ja", req.Source, req.Target)
	}
}

func TestTranslateRefusesAPairWithoutACall(t *testing.T) {
	t.Parallel()
	s := newStandIn(t)
	_, err := s.translator(t, testKey).Translate(t.Context(), translation.Request{
		Fragments: []string{"こんにちは"}, SourceLanguage: "ja", TargetLanguage: "zh-Hans",
	})
	if !errors.Is(err, translation.ErrUnsupportedLanguage) {
		t.Errorf("Translate(ja → zh-Hans) = %v, want ErrUnsupportedLanguage", err)
	}
	if len(s.requests) != 0 {
		t.Errorf("%d calls of /translate, want none", len(s.requests))
	}
}

func TestStatusError(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		code int
		msg  string
		want error
	}{
		{http.StatusTooManyRequests, "Slowdown: 10 per 1 minute", translation.ErrProviderLimit},
		{http.StatusBadRequest, "tlh is not supported", translation.ErrUnsupportedLanguage},
		{http.StatusBadRequest, "Japanese (ja) is not available as a target language from Klingon (tlh)", translation.ErrUnsupportedLanguage},
		{http.StatusBadRequest, "Invalid request: request (6000) exceeds text limit (5000)", translation.ErrProviderLimit},
		{http.StatusBadRequest, "Please contact the server operator to get an API key", translation.ErrTemporary},
		{http.StatusBadRequest, "Invalid request: missing q parameter", translation.ErrTemporary},
		{http.StatusForbidden, "Too many request limits violations", translation.ErrProviderLimit},
		{http.StatusForbidden, "Invalid API key", translation.ErrTemporary},
		{http.StatusInternalServerError, "Cannot translate text: boom", translation.ErrTemporary},
		{http.StatusBadGateway, "", translation.ErrTemporary},
	} {
		body, _ := json.Marshal(map[string]string{"error": c.msg})
		err := statusError(c.code, body)
		for _, k := range []error{translation.ErrTemporary, translation.ErrProviderLimit, translation.ErrUnsupportedLanguage} {
			if errors.Is(err, k) != errors.Is(c.want, k) {
				t.Errorf("statusError(%d, %q) = %v, want %v", c.code, c.msg, err, c.want)
			}
		}
		if c.msg != "" && strings.Contains(err.Error(), c.msg) {
			t.Errorf("statusError(%d, %q) = %q holds the server's message", c.code, c.msg, err)
		}
	}
}

func TestErrorsHoldNoKey(t *testing.T) {
	t.Parallel()
	// A server that echoes what it received in its error.
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, []languageEntry{{Code: "en"}, {Code: "ja"}})
			return
		}
		var req translateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Invalid API key " + req.APIKey})
	}))
	t.Cleanup(echo.Close)
	tr, err := New(Options{URL: echo.URL, APIKey: testKey, HTTPClient: echo.Client()})
	if err != nil {
		t.Fatal(err)
	}
	req := translation.Request{Fragments: []string{"Hello."}, SourceLanguage: "en", TargetLanguage: "ja"}
	_, err = tr.Translate(t.Context(), req)
	if err == nil || strings.Contains(err.Error(), testKey) {
		t.Errorf("Translate() = %v, want an error without the key", err)
	}
	echo.Close() // now unreachable: the error of the client names the URL only
	tr.langs = nil
	_, err = tr.Translate(t.Context(), req)
	if !errors.Is(err, translation.ErrTemporary) || strings.Contains(err.Error(), testKey) {
		t.Errorf("Translate() on a closed server = %v, want a temporary error without the key", err)
	}
}

func TestFromHTML(t *testing.T) {
	t.Parallel()
	ms := markers{1: false, 2: true}
	for _, c := range []struct{ in, want string }{
		// What the server leaves in an item's element, and elements it adds, are dropped.
		{`<p>A <code id="m1" translate="no">x</code> <span id="m2">b <i>c</i></span></p>`, "A ⟦1⟧ ⟦2⟧b c⟦/2⟧"},
		// The items may move; text is unescaped.
		{`<span id="m2">&lt;b&gt;</span> &amp; <code id="m1"/>`, "⟦2⟧<b>⟦/2⟧ & ⟦1⟧"},
		// An id that no marker of the fragment has stays out.
		{`x <span id="m9">y</span><br>z`, "x yz"},
	} {
		if got := fromHTML(c.in, ms); got != c.want {
			t.Errorf("fromHTML(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLanguageCodes(t *testing.T) {
	t.Parallel()
	l, err := parseLanguages([]byte(`[{"code":"en"},{"code":"ja"},{"code":"pt-BR"},{"code":"zh-Hans"},{"code":"zh-Hant"},{"code":"not a tag!"}]`))
	if err != nil {
		t.Fatal(err)
	}
	for tag, want := range map[string]string{
		"en": "en", "en-GB": "en", "ja-JP": "ja", "pt-BR": "pt-BR", "zh": "zh-Hans", "zh-TW": "zh-Hant", "zh-Hant-HK": "zh-Hant", "fr": "", "tlh": "",
	} {
		got, ok := l.code(tag)
		if (want == "") == ok || got != want {
			t.Errorf("code(%q) = %q, %t, want %q", tag, got, ok, want)
		}
	}
}

func TestCheckURL(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"https://libretranslate.com", "http://10.0.0.5:5000", "http://libretranslate:5000/", "https://example.org/translate-api"} {
		if err := CheckURL(v); err != nil {
			t.Errorf("CheckURL(%q) = %v, want nil", v, err)
		}
	}
	for _, v := range []string{"", "libretranslate:5000", "ftp://example.org", "https://user:pass@example.org", "https://example.org/?a=1", "https://example.org/#x"} {
		if err := CheckURL(v); err == nil {
			t.Errorf("CheckURL(%q) = nil, want an error", v)
		}
	}
}
