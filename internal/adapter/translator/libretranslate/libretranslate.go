// SPDX-License-Identifier: AGPL-3.0-only

// Package libretranslate is the translation.Translator of a LibreTranslate server (ADR-0148 §15):
// libretranslate.com or an instance that the operator runs, which may be on a private address.
// The operator configured the URL, so the adapter calls it with its own HTTP client rather than
// the SSRF-guarded kernel.HTTPClient.
//
// The fragments are the text of a post and the API key is a secret, so an error holds neither,
// nor any text of LibreTranslate's answers: the adapter reads the message of an error only to
// tell the kinds of failures apart.
package libretranslate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/language"

	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/i18n"
)

// ProviderID is the provider's ID, which translation_providers and
// translation_provider_visibilities name.
const ProviderID = "libretranslate"

// languagesTTL is how long the list of the server's languages is used before it is read again,
// so that a language that the operator installs is soon available.
const languagesTTL = time.Hour

// maxResponseBytes bounds an answer that the adapter reads.
const maxResponseBytes = 4 << 20

// Options configure the Translator.
type Options struct {
	// URL is the base URL of the LibreTranslate server (translator_libretranslate_url), with
	// http: or https: and an optional path under which the API is served.
	URL string
	// APIKey is sent with each translation when it is set (translator_libretranslate_api_key).
	APIKey string
	// HTTPClient calls the server: the operator configured it, so it is the adapter's own client
	// that follows no redirects, never the SSRF-guarded kernel.HTTPClient.
	HTTPClient *http.Client
	// Now returns the time, for the expiry of the list of languages; nil is time.Now.
	Now func() time.Time
}

// Translator is the translation.Translator of a LibreTranslate server. It is safe for concurrent
// use.
type Translator struct {
	base   string
	apiKey string
	client *http.Client
	now    func() time.Time

	mu        sync.Mutex
	langs     *languages
	langsRead time.Time
}

var _ translation.Translator = (*Translator)(nil)

// New returns the Translator of o. It makes no call.
func New(o Options) (*Translator, error) {
	if err := CheckURL(o.URL); err != nil {
		return nil, fmt.Errorf("libretranslate: the URL %w", err)
	}
	if o.HTTPClient == nil {
		return nil, errors.New("libretranslate: an HTTP client is required")
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Translator{base: strings.TrimSuffix(o.URL, "/"), apiKey: o.APIKey, client: o.HTTPClient, now: now}, nil
}

// CheckURL accepts the base URL of a LibreTranslate server: http: or https:, a host, and no user
// info, query, or fragment. http: is accepted on any host, as a server that the operator runs
// is often on a private network.
func CheckURL(v string) error {
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("must be an absolute http or https URL without user info, query, or fragment, such as https://libretranslate.com or http://libretranslate:5000")
	}
	return nil
}

// Translate implements translation.Translator with one call of POST /translate for every
// fragment that has words: q is the list of their HTML (markup.go), and the answer has the
// translations in the same order.
func (t *Translator) Translate(ctx context.Context, req translation.Request) (translation.Result, error) {
	if err := ctx.Err(); err != nil {
		return translation.Result{}, fmt.Errorf("%w: libretranslate: %w", translation.ErrTemporary, err)
	}
	langs, err := t.languages(ctx)
	if err != nil {
		return translation.Result{}, err
	}
	source, target, err := langs.pair(req.SourceLanguage, req.TargetLanguage)
	if err != nil {
		return translation.Result{}, err
	}
	out := make([]string, len(req.Fragments))
	var q []string
	var sent []int // the index of the fragment of each q
	var sentMarkers []markers
	for i, f := range req.Fragments {
		if !hasWords(f) {
			out[i] = f // nothing but markers and white space comes back as it is
			continue
		}
		h, ms := toHTML(f)
		q = append(q, h)
		sent = append(sent, i)
		sentMarkers = append(sentMarkers, ms)
	}
	detected := req.SourceLanguage
	if len(q) > 0 {
		res, err := t.translate(ctx, translateRequest{Q: q, Source: source, Target: target, Format: "html", APIKey: t.apiKey})
		if err != nil {
			return translation.Result{}, err
		}
		if len(res.texts) != len(q) {
			return translation.Result{}, fmt.Errorf("%w: libretranslate: %d translations of %d fragments", translation.ErrTemporary, len(res.texts), len(q))
		}
		for j, i := range sent {
			tr := fromHTML(res.texts[j], sentMarkers[j])
			if strings.TrimSpace(tr) == "" {
				return translation.Result{}, fmt.Errorf("%w: libretranslate: translation %d is empty", translation.ErrTemporary, i+1)
			}
			if !translation.MarkersIntact(req.Fragments[i], tr) {
				return translation.Result{}, fmt.Errorf("%w: libretranslate: translation %d does not hold the markers of its fragment", translation.ErrTemporary, i+1)
			}
			out[i] = tr
		}
		if source == "auto" {
			detected = detectedLanguage(res.detected)
		}
	}
	return translation.Result{Translations: out, DetectedLanguage: detected}, nil
}

// translateRequest is the JSON body of POST /translate. q is always a list, which LibreTranslate
// answers with a list.
type translateRequest struct {
	Q      []string `json:"q"`
	Source string   `json:"source"`
	Target string   `json:"target"`
	Format string   `json:"format"`
	APIKey string   `json:"api_key,omitempty"`
}

// translateResponse is the answer of POST /translate to a list. detectedLanguage is there only
// for source = auto: a list of one entry per text, or one object from a server that answers so.
type translateResponse struct {
	TranslatedText   json.RawMessage `json:"translatedText"`
	DetectedLanguage json.RawMessage `json:"detectedLanguage"`
}

type detection struct {
	Language   string  `json:"language"`
	Confidence float64 `json:"confidence"`
}

type translated struct {
	texts    []string
	detected []detection
}

func (t *Translator) translate(ctx context.Context, body translateRequest) (translated, error) {
	raw, err := json.Marshal(body) //nolint:gosec // G117: LibreTranslate takes the key in the body, which goes only to the configured server
	if err != nil {
		return translated{}, fmt.Errorf("%w: libretranslate: encode the request", translation.ErrTemporary)
	}
	data, err := t.do(ctx, http.MethodPost, "/translate", raw)
	if err != nil {
		return translated{}, err
	}
	var res translateResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return translated{}, fmt.Errorf("%w: libretranslate: the answer is not the JSON of a translation", translation.ErrTemporary)
	}
	var out translated
	if err := json.Unmarshal(res.TranslatedText, &out.texts); err != nil {
		var one string
		if json.Unmarshal(res.TranslatedText, &one) != nil {
			return translated{}, fmt.Errorf("%w: libretranslate: the answer has no translatedText", translation.ErrTemporary)
		}
		out.texts = []string{one}
	}
	if len(res.DetectedLanguage) > 0 {
		if json.Unmarshal(res.DetectedLanguage, &out.detected) != nil {
			var one detection
			if json.Unmarshal(res.DetectedLanguage, &one) == nil {
				out.detected = []detection{one}
			}
		}
	}
	return out, nil
}

// detectedLanguage returns the language that LibreTranslate detected, which it detects once for
// all the texts of a call, as a BCP 47 tag, or UndeterminedLanguage when it says none. Only a
// well-formed tag passes, as the value goes into a stored row and the API.
func detectedLanguage(ds []detection) string {
	for _, d := range ds {
		if tag := i18n.Canonicalize(d.Language); tag != "" {
			return tag
		}
	}
	return translation.UndeterminedLanguage
}

// do sends a request to the server and returns the body of a 2xx answer. Any other answer, and a
// failure to reach the server, is one of the port's errors.
func (t *Translator) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.base+path, rd)
	if err != nil {
		return nil, fmt.Errorf("%w: libretranslate: build the request", translation.ErrTemporary)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := t.client.Do(req)
	if err != nil {
		// The error of the client names the URL, which the operator configured and which holds no
		// key: the key is only ever in the body.
		return nil, fmt.Errorf("%w: libretranslate: %w", translation.ErrTemporary, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: libretranslate: read the answer: %w", translation.ErrTemporary, err)
	}
	if len(data) > maxResponseBytes {
		return nil, fmt.Errorf("%w: libretranslate: the answer is over %d bytes", translation.ErrTemporary, maxResponseBytes)
	}
	if resp.StatusCode/100 != 2 {
		return nil, statusError(resp.StatusCode, data)
	}
	return data, nil
}

// statusError returns the port's error of an answer with the status code. LibreTranslate answers
// 400 for an unsupported language and for a text over its limit as for any other bad request,
// 403 for a key that it refuses and for an address that it banned for its rate limits, and 429
// for its rate limit, each with {"error": message}; the message tells those apart, and goes no
// further than this function.
func statusError(code int, body []byte) error {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &e)
	msg := strings.ToLower(e.Error)
	switch {
	case code == http.StatusTooManyRequests:
		return fmt.Errorf("%w: libretranslate: the server's rate limit (status %d)", translation.ErrProviderLimit, code)
	case code == http.StatusBadRequest && (strings.Contains(msg, "is not supported") || strings.Contains(msg, "is not available as a target language")):
		return fmt.Errorf("%w: libretranslate: the server does not translate the language (status %d)", translation.ErrUnsupportedLanguage, code)
	case code == http.StatusBadRequest && strings.Contains(msg, "exceeds"):
		return fmt.Errorf("%w: libretranslate: the text is over the server's limit (status %d)", translation.ErrProviderLimit, code)
	case code == http.StatusForbidden && strings.Contains(msg, "too many request limits violations"):
		return fmt.Errorf("%w: libretranslate: the server blocked this server for its rate limits (status %d)", translation.ErrProviderLimit, code)
	case code == http.StatusForbidden:
		return fmt.Errorf("%w: libretranslate: the server refused the request, check translator_libretranslate_api_key (status %d)", translation.ErrTemporary, code)
	case code == http.StatusBadRequest && strings.Contains(msg, "api key"):
		return fmt.Errorf("%w: libretranslate: the server requires an API key, set translator_libretranslate_api_key (status %d)", translation.ErrTemporary, code)
	}
	return fmt.Errorf("%w: libretranslate: the server answered with status %d", translation.ErrTemporary, code)
}

// languages are the languages of the server, from GET /languages, and the targets of each.
type languages struct {
	codes   []string
	tags    []language.Tag
	targets map[string]map[string]bool
	matcher language.Matcher
}

type languageEntry struct {
	Code    string   `json:"code"`
	Targets []string `json:"targets"`
}

// languages returns the server's languages, read again after languagesTTL. A failure to read
// them is temporary, and the list that was read last stays in use until a read succeeds.
func (t *Translator) languages(ctx context.Context) (*languages, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.langs != nil && t.now().Sub(t.langsRead) < languagesTTL {
		return t.langs, nil
	}
	data, err := t.do(ctx, http.MethodGet, "/languages", nil)
	if err == nil {
		var l *languages
		if l, err = parseLanguages(data); err == nil {
			t.langs, t.langsRead = l, t.now()
			return l, nil
		}
	}
	if t.langs != nil && ctx.Err() == nil {
		return t.langs, nil
	}
	return nil, err // do and parseLanguages return the port's errors
}

func parseLanguages(data []byte) (*languages, error) {
	var entries []languageEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%w: libretranslate: the answer of /languages is not a list of languages", translation.ErrTemporary)
	}
	l := &languages{targets: make(map[string]map[string]bool)}
	var tags []language.Tag
	for _, e := range entries {
		tag, err := language.Parse(e.Code)
		if err != nil {
			continue // a code that is no language tag cannot be asked for
		}
		targets := make(map[string]bool, len(e.Targets))
		for _, c := range e.Targets {
			targets[c] = true
		}
		l.codes = append(l.codes, e.Code)
		l.targets[e.Code] = targets
		tags = append(tags, tag)
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("%w: libretranslate: the server lists no language", translation.ErrTemporary)
	}
	l.tags = tags
	l.matcher = language.NewMatcher(tags)
	return l, nil
}

// code returns the server's code of a BCP 47 tag: the one that matches it with high confidence,
// so that en-US asks for en and zh-TW for zh-Hant, and never another language: the matcher falls
// back to its first language for a tag that it does not know, so the languages must be the same.
func (l *languages) code(tag string) (string, bool) {
	t, err := language.Parse(tag)
	if err != nil {
		return "", false
	}
	_, i, conf := l.matcher.Match(t)
	want, _ := t.Base()
	got, _ := l.tags[i].Base()
	if conf < language.High || want != got {
		return "", false
	}
	return l.codes[i], true
}

// pair returns the codes of the source, "auto" for UndeterminedLanguage, and of the target, or
// ErrUnsupportedLanguage without a call when the server does not translate between them.
func (l *languages) pair(source, target string) (string, string, error) {
	unsupported := fmt.Errorf("%w: libretranslate: the server does not translate the language pair", translation.ErrUnsupportedLanguage)
	tgt, ok := l.code(target)
	if !ok {
		return "", "", unsupported
	}
	if source == translation.UndeterminedLanguage {
		return "auto", tgt, nil
	}
	src, ok := l.code(source)
	if !ok {
		return "", "", unsupported
	}
	// A server lists the targets of each source; one that lists none is taken to translate to
	// every language that it has.
	if ts := l.targets[src]; len(ts) > 0 && !ts[tgt] {
		return "", "", unsupported
	}
	return src, tgt, nil
}
