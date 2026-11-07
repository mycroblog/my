// SPDX-License-Identifier: AGPL-3.0-only

package aws

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go/encoding/cbor"
	nethtml "golang.org/x/net/html"

	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/pkg/testkit/translationtest"
)

// standIn is a stand-in of the TranslateText operation of Amazon Translate in the protocol of the
// SDK (Smithy RPC v2 CBOR). It "translates" by upper-casing the text outside the translate="no"
// elements, keeps the markup, detects every source as English, and refuses the language xx.
type standIn struct {
	mu       sync.Mutex
	failNext string // the error code of the next answer
	calls    []cbor.Map
	answer   func(text string) string // replaces the translation when set
}

func (s *standIn) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/service/AWSShineFrontendService_20170701/operation/TranslateText" {
		http.NotFound(w, r)
		return
	}
	body, _ := io.ReadAll(r.Body)
	v, err := cbor.Decode(body)
	in, ok := v.(cbor.Map)
	if err != nil || !ok {
		s.fail(w, "InvalidRequestException")
		return
	}
	text, _ := in["Text"].(cbor.String)
	source, _ := in["SourceLanguageCode"].(cbor.String)
	target, _ := in["TargetLanguageCode"].(cbor.String)
	s.mu.Lock()
	s.calls = append(s.calls, in)
	code := s.failNext
	s.failNext = ""
	answer := s.answer
	s.mu.Unlock()
	switch {
	case code != "":
		s.fail(w, code)
		return
	case len(text) > MaxRequestBytes:
		s.fail(w, "TextSizeLimitExceededException")
		return
	case source == "xx" || target == "xx":
		s.fail(w, "UnsupportedLanguagePairException")
		return
	}
	if source == autoDetect {
		source = "en"
	}
	translated := upperOutsideKept(string(text))
	if answer != nil {
		translated = answer(string(text))
	}
	w.Header().Set("Smithy-Protocol", "rpc-v2-cbor")
	w.Header().Set("Content-Type", "application/cbor")
	_, _ = w.Write(cbor.Encode(cbor.Map{
		"TranslatedText":     cbor.String(translated),
		"SourceLanguageCode": source,
		"TargetLanguageCode": target,
	}))
}

func (s *standIn) fail(w http.ResponseWriter, code string) {
	w.Header().Set("Smithy-Protocol", "rpc-v2-cbor")
	w.Header().Set("Content-Type", "application/cbor")
	status := http.StatusBadRequest
	if code == "ServiceUnavailableException" || code == "InternalServerException" {
		status = http.StatusInternalServerError
	}
	w.WriteHeader(status)
	// Amazon's message may repeat the input, which no error of the adapter may.
	_, _ = w.Write(cbor.Encode(cbor.Map{
		"__type":  cbor.String("com.amazonaws.translate#" + code),
		"message": cbor.String("refused: the words of a private post"),
	}))
}

func (s *standIn) FailNext(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case errors.Is(err, translation.ErrProviderLimit):
		s.failNext = "TooManyRequestsException"
	default:
		s.failNext = "ServiceUnavailableException"
	}
}

func (s *standIn) texts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.calls))
	for i, c := range s.calls {
		t, _ := c["Text"].(cbor.String)
		out[i] = string(t)
	}
	return out
}

// upperOutsideKept upper-cases the text of the HTML outside the translate="no" elements and
// writes the markup back as it came, the way Amazon Translate keeps it.
func upperOutsideKept(in string) string {
	z := nethtml.NewTokenizer(strings.NewReader(in))
	var b strings.Builder
	keep := 0
	for {
		switch z.Next() {
		case nethtml.ErrorToken:
			return b.String()
		case nethtml.TextToken:
			raw := string(z.Raw())
			if keep == 0 {
				raw = strings.ToUpper(raw)
			}
			b.WriteString(raw)
		case nethtml.StartTagToken:
			raw := string(z.Raw())
			if strings.Contains(raw, `translate="no"`) {
				keep++
			}
			b.WriteString(raw)
		case nethtml.EndTagToken:
			if name, _ := z.TagName(); string(name) == "span" && keep > 0 {
				keep--
			}
			b.WriteString(string(z.Raw()))
		default:
			b.WriteString(string(z.Raw()))
		}
	}
}

// newStandIn returns a Translator on a new stand-in, through the SDK with static credentials.
func newStandIn(t *testing.T) (*Translator, *standIn) {
	t.Helper()
	s := &standIn{}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	tr, err := NewFromConfig(awssdk.Config{
		Region: "us-east-1", BaseEndpoint: awssdk.String(srv.URL), HTTPClient: srv.Client(), RetryMaxAttempts: 1,
		Credentials: awssdk.CredentialsProviderFunc(func(context.Context) (awssdk.Credentials, error) {
			return awssdk.Credentials{AccessKeyID: "test", SecretAccessKey: "test", Source: "test"}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return tr, s
}

func TestContract(t *testing.T) {
	t.Parallel()
	translationtest.TranslatorContract(t, func(t *testing.T) translationtest.TranslatorHarness {
		tr, s := newStandIn(t)
		return translationtest.TranslatorHarness{
			Translator: tr, Source: "en", Target: "ja", UnsupportedLanguage: "xx", FailNext: s.FailNext,
		}
	})
}

func TestSendsTheMarkersAsKeptElements(t *testing.T) {
	t.Parallel()
	tr, s := newStandIn(t)
	got, err := tr.Translate(t.Context(), translation.Request{
		Fragments:      []string{"a < b & ⟦1⟧ \"q\"\nnext ⟦2⟧bold⟦/2⟧", "⟦1⟧"},
		SourceLanguage: "en-US", TargetLanguage: "ja",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `<div>a &lt; b &amp; <span translate="no">⟦1⟧</span> &#34;q&#34;<br>next <span translate="no">⟦2⟧</span>bold<span translate="no">⟦/2⟧</span></div>`
	if texts := s.texts(); len(texts) != 1 || texts[0] != want {
		t.Errorf("sent %q, want one call of %q", texts, want)
	}
	if want := []string{"A < B & ⟦1⟧ \"Q\"\nNEXT ⟦2⟧BOLD⟦/2⟧", "⟦1⟧"}; got.Translations[0] != want[0] || got.Translations[1] != want[1] {
		t.Errorf("Translations = %q, want %q", got.Translations, want)
	}
	if got.DetectedLanguage != "en-US" {
		t.Errorf("DetectedLanguage = %q, want the request's en-US", got.DetectedLanguage)
	}
}

func TestDetectsOnceAndSendsTheLanguageAfter(t *testing.T) {
	t.Parallel()
	tr, s := newStandIn(t)
	tr.maxBytes = 200
	fragments := []string{strings.Repeat("word ", 30), strings.Repeat("more ", 30)}
	got, err := tr.Translate(t.Context(), translation.Request{Fragments: fragments, SourceLanguage: translation.UndeterminedLanguage, TargetLanguage: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	if got.DetectedLanguage != "en" {
		t.Errorf("DetectedLanguage = %q, want en", got.DetectedLanguage)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.calls) < 2 {
		t.Fatalf("%d calls, want several", len(s.calls))
	}
	for i, c := range s.calls {
		want := cbor.String("en")
		if i == 0 {
			want = autoDetect
		}
		if c["SourceLanguageCode"] != want {
			t.Errorf("call %d has the source %v, want %v", i, c["SourceLanguageCode"], want)
		}
	}
}

func TestSplitsByTheSizeLimit(t *testing.T) {
	t.Parallel()
	tr, s := newStandIn(t)
	// Three fragments that fit together two at a time, and one larger than a call, with markers.
	long := strings.Repeat("Ünïcödé text in a sentence. ", 600) + "⟦1⟧ the end\nlast ⟦2⟧line⟦/2⟧"
	fragments := []string{strings.Repeat("a", 4000), strings.Repeat("b", 4000), strings.Repeat("c", 4000), long}
	got, err := tr.Translate(t.Context(), translation.Request{Fragments: fragments, SourceLanguage: "en", TargetLanguage: "de"})
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range fragments {
		if want := strings.ToUpper(f); got.Translations[i] != want {
			t.Errorf("translation %d differs from the upper-cased fragment (%d and %d bytes)", i, len(got.Translations[i]), len(want))
		}
	}
	texts := s.texts()
	if len(texts) < 4 {
		t.Errorf("%d calls, want the long fragment in several", len(texts))
	}
	for i, text := range texts {
		if len(text) > MaxRequestBytes {
			t.Errorf("call %d has %d bytes", i, len(text))
		}
	}
}

func TestSplitCutsAtBreaksAndNeverInAMarker(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		fragment string
		max      int
	}{
		{"sentences", strings.Repeat("One sentence here. ", 40), 120},
		{"lines", strings.Repeat("a line of text\n", 40), 120},
		{"no break", strings.Repeat("あ", 200), 120},
		{"markers", strings.Repeat("⟦12⟧⟦/12⟧x", 60), 120},
	} {
		pieces := split(tc.fragment, tc.max)
		if strings.Join(pieces, "") != tc.fragment {
			t.Errorf("%s: the pieces do not make the fragment", tc.name)
		}
		for _, p := range pieces {
			if unitSize(p) > tc.max {
				t.Errorf("%s: a piece of %d bytes", tc.name, unitSize(p))
			}
			if strings.Count(p, "⟦") != strings.Count(p, "⟧") {
				t.Errorf("%s: a piece %q cuts a marker", tc.name, p)
			}
		}
		if tc.name == "sentences" && !strings.HasSuffix(pieces[0], ". ") {
			t.Errorf("sentences: the first piece %q does not end a sentence", pieces[0])
		}
		if tc.name == "lines" && !strings.HasSuffix(pieces[0], "\n") {
			t.Errorf("lines: the first piece %q does not end a line", pieces[0])
		}
	}
}

func TestMapsTheErrors(t *testing.T) {
	t.Parallel()
	for code, want := range map[string]error{
		"TooManyRequestsException":               translation.ErrProviderLimit,
		"ThrottlingException":                    translation.ErrProviderLimit,
		"LimitExceededException":                 translation.ErrProviderLimit,
		"ServiceUnavailableException":            translation.ErrTemporary,
		"InternalServerException":                translation.ErrTemporary,
		"TextSizeLimitExceededException":         translation.ErrTemporary,
		"InvalidRequestException":                translation.ErrTemporary,
		"UnsupportedLanguagePairException":       translation.ErrUnsupportedLanguage,
		"DetectedLanguageLowConfidenceException": translation.ErrUnsupportedLanguage,
	} {
		tr, s := newStandIn(t)
		s.mu.Lock()
		s.failNext = code
		s.mu.Unlock()
		_, err := tr.Translate(t.Context(), translation.Request{Fragments: []string{"the words of a private post"}, SourceLanguage: translation.UndeterminedLanguage, TargetLanguage: "ja"})
		if !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", code, err, want)
		}
		if err != nil && (strings.Contains(err.Error(), "private post") || !strings.Contains(err.Error(), code)) {
			t.Errorf("%s: err = %q, want the code and no text", code, err)
		}
	}
}

func TestAnAnswerWithoutTheFragmentsFails(t *testing.T) {
	t.Parallel()
	for name, answer := range map[string]func(string) string{
		"no divisions":    func(string) string { return "Just text" },
		"lost marker":     func(string) string { return "<div>HELLO</div>" },
		"one div too few": func(string) string { return "<div>HELLO ⟦1⟧</div>" },
		"empty":           func(string) string { return "<div> </div><div>x</div>" },
	} {
		tr, s := newStandIn(t)
		s.answer = answer
		_, err := tr.Translate(t.Context(), translation.Request{Fragments: []string{"Hello ⟦1⟧", "x"}, SourceLanguage: "en", TargetLanguage: "ja"})
		if !errors.Is(err, translation.ErrTemporary) {
			t.Errorf("%s: err = %v, want ErrTemporary", name, err)
		}
	}
}

func TestDecodeDropsTheProvidersMarkup(t *testing.T) {
	t.Parallel()
	got, err := decode(`<div>Hallo <span translate=no> ⟦1⟧ </span>, <b>fett</b> &amp; &lt;i&gt;<br/>neu</div> <div>⟦1⟧</div>`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Hallo ⟦1⟧, fett & <i>\nneu", "⟦1⟧"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("decode = %q, want %q", got, want)
	}
}

func TestLanguageCode(t *testing.T) {
	t.Parallel()
	for tag, want := range map[string]string{
		"en": "en", "en-US": "en", "ja": "ja", "zh": "zh", "zh-Hans": "zh", "zh-CN": "zh", "zh-Hant": "zh-TW",
		"zh-TW": "zh-TW", "zh-HK": "zh-TW", "zh-Hans-HK": "zh", "fr-CA": "fr-CA", "fr-FR": "fr", "pt-PT": "pt-PT",
		"pt-BR": "pt", "es-MX": "es-MX", "es-419": "es", "fa-AF": "fa-AF", "fil": "tl", "iw": "he", "sr-Latn-RS": "sr",
	} {
		if got, ok := languageCode(tag); !ok || got != want {
			t.Errorf("languageCode(%q) = %q, %t, want %q", tag, got, ok, want)
		}
	}
	for _, tag := range []string{"", "und", "x-private", "toolongtag", "1a"} {
		if got, ok := languageCode(tag); ok {
			t.Errorf("languageCode(%q) = %q, want none", tag, got)
		}
	}
	if got := detectedLanguage("zh-TW", "und"); got != "zh-TW" {
		t.Errorf("detectedLanguage(zh-TW) = %q", got)
	}
	if got := detectedLanguage("", "und"); got != "und" {
		t.Errorf("detectedLanguage() = %q, want und", got)
	}
}

func TestNewFromConfigNeedsARegion(t *testing.T) {
	t.Parallel()
	if _, err := NewFromConfig(awssdk.Config{}); err == nil {
		t.Error("NewFromConfig without a region succeeds")
	}
}
