// SPDX-License-Identifier: AGPL-3.0-only

// Package deepl is the translation.Translator of DeepL's API (ADR-0148 §15): POST /v2/translate with the fragments of a
// post as the texts of one request, and GET /v3/languages for the languages that it translates.
//
// The fragments are the text of a post and the key is a secret, so nothing here logs, and an error holds neither a
// fragment, nor the key, nor anything that DeepL answered: only the status code of an answer and a description of a
// failed request with the key taken out.
package deepl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/language"

	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/i18n"
)

// ProviderID is DeepL's ID among the providers of the server, which translation_providers and
// translation_provider_visibilities name.
const ProviderID = "deepl"

const (
	// FreeURL and ProURL are the APIs of DeepL's free and paid plans. A key of the free plan ends in ":fx".
	FreeURL = "https://api-free.deepl.com"
	ProURL  = "https://api.deepl.com"
)

const (
	// maxRequestBytes bounds the body of one request: DeepL takes at most 128 KiB, and the fragments of a longer post go
	// in more requests.
	maxRequestBytes = 120 << 10
	// maxResponseBytes bounds what is read of an answer.
	maxResponseBytes = 4 << 20
	// languagesTTL is how long the languages of GET /v3/languages are kept: DeepL adds languages now and then.
	languagesTTL = 24 * time.Hour
)

// Options configure the Translator.
type Options struct {
	// APIKey is the authentication key (translator_deepl_api_key).
	APIKey string
	// URL is the base URL of the API (translator_deepl_url); empty chooses FreeURL or ProURL by the key.
	URL string
	// HTTPClient calls the API. DeepL is a destination that the operator configured, so it is the adapter's own client,
	// which follows no redirects, rather than the SSRF-guarded kernel.HTTPClient.
	HTTPClient *http.Client
	// Now is the clock of the cache of the languages; nil is time.Now.
	Now func() time.Time
}

// Translator is the translation.Translator of DeepL. It is safe for concurrent use.
type Translator struct {
	key    string
	url    string
	client *http.Client
	now    func() time.Time

	mu        sync.Mutex
	languages *languages
}

var _ translation.Translator = (*Translator)(nil)

// New returns the Translator. It makes no call.
func New(o Options) *Translator {
	t := &Translator{key: o.APIKey, url: strings.TrimRight(o.URL, "/"), client: o.HTTPClient, now: o.Now}
	if t.url == "" {
		t.url = BaseURL(o.APIKey)
	}
	if t.client == nil {
		t.client = &http.Client{
			Timeout:       15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	if t.now == nil {
		t.now = time.Now
	}
	return t
}

// BaseURL returns the API of the plan of the key: FreeURL for a key of the free plan, which ends in ":fx", and ProURL
// otherwise.
func BaseURL(key string) string {
	if strings.HasSuffix(strings.TrimSpace(key), ":fx") {
		return FreeURL
	}
	return ProURL
}

// translateRequest is the body of POST /v2/translate. The markers go as XML elements (tags.go), which DeepL keeps with
// tag_handling = xml; non_splitting_tags tells it that they are inline, so that a pair does not end a sentence.
type translateRequest struct {
	Text             []string `json:"text"`
	SourceLang       string   `json:"source_lang,omitempty"`
	TargetLang       string   `json:"target_lang"`
	TagHandling      string   `json:"tag_handling"`
	NonSplittingTags []string `json:"non_splitting_tags,omitempty"`
}

type translateResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
}

// Translate implements translation.Translator. An answer that breaks the port's contract is a temporary failure, so that
// the service moves on to its next provider (ADR-0148 §8).
func (t *Translator) Translate(ctx context.Context, req translation.Request) (translation.Result, error) {
	langs, err := t.supported(ctx)
	if err != nil {
		return translation.Result{}, err
	}
	target, ok := langs.target(req.TargetLanguage)
	if !ok {
		return translation.Result{}, fmt.Errorf("%w: deepl: the target language is not one that DeepL translates into", translation.ErrUnsupportedLanguage)
	}
	source := ""
	if req.SourceLanguage != translation.UndeterminedLanguage {
		if source, ok = langs.source(req.SourceLanguage); !ok {
			return translation.Result{}, fmt.Errorf("%w: deepl: the source language is not one that DeepL translates from", translation.ErrUnsupportedLanguage)
		}
	}
	texts := make([]string, len(req.Fragments))
	var tagNames []string
	for i, f := range req.Fragments {
		texts[i], tagNames = toTags(f, tagNames)
	}
	translations := make([]string, 0, len(texts))
	detected := ""
	for _, batch := range batches(texts, source, target, tagNames) {
		var res translateResponse
		if err := t.call(ctx, http.MethodPost, "/v2/translate", batch, &res); err != nil {
			return translation.Result{}, err
		}
		if len(res.Translations) != len(batch.Text) {
			return translation.Result{}, fmt.Errorf("%w: deepl: %d translations of %d texts", translation.ErrTemporary, len(res.Translations), len(batch.Text))
		}
		for _, tr := range res.Translations {
			translations = append(translations, tr.Text)
			if detected == "" {
				detected = tr.DetectedSourceLanguage
			}
		}
	}
	for i, fragment := range req.Fragments {
		translations[i] = fromTags(translations[i])
		if !translation.MarkersIntact(fragment, translations[i]) {
			return translation.Result{}, fmt.Errorf("%w: deepl: translation %d does not hold the markers of its fragment", translation.ErrTemporary, i+1)
		}
		if strings.TrimSpace(translations[i]) == "" {
			return translation.Result{}, fmt.Errorf("%w: deepl: translation %d is empty", translation.ErrTemporary, i+1)
		}
	}
	return translation.Result{Translations: translations, DetectedLanguage: detectedLanguage(detected, req.SourceLanguage)}, nil
}

// batches splits the texts into requests whose bodies stay under maxRequestBytes, in order. A text too long for a request
// of its own still goes alone, and DeepL refuses it.
func batches(texts []string, source, target string, tagNames []string) []translateRequest {
	base := translateRequest{SourceLang: source, TargetLang: target, TagHandling: "xml", NonSplittingTags: tagNames}
	overhead := encodedLen(base)
	var out []translateRequest
	cur, size := base, overhead
	for _, text := range texts {
		n := encodedLen(text) + 1
		if len(cur.Text) > 0 && size+n > maxRequestBytes {
			out = append(out, cur)
			cur, size = base, overhead
		}
		cur.Text = append(cur.Text, text)
		size += n
	}
	return append(out, cur)
}

func encodedLen(v any) int {
	b, _ := json.Marshal(v)
	return len(b)
}

// detectedLanguage returns the language of the source: DeepL's code as a BCP 47 tag, or the request's when DeepL says
// nothing that is a language tag. The value goes into a stored row and the API, so no other text of DeepL passes.
func detectedLanguage(code, source string) string {
	tag := i18n.Canonicalize(code)
	if tag == "" || (tag == translation.UndeterminedLanguage && source != translation.UndeterminedLanguage) {
		return source
	}
	return tag
}

// call sends a request of the API and decodes its answer into out. The body is JSON, or nothing for a nil body.
func (t *Translator) call(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%w: deepl: encode the request: %w", translation.ErrTemporary, err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.url+path, reader)
	if err != nil {
		return fmt.Errorf("%w: deepl: %s", translation.ErrTemporary, t.redact(err.Error()))
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+t.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := t.client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("%w: deepl: %w", translation.ErrTemporary, ctxErr)
		}
		return fmt.Errorf("%w: deepl: %s", translation.ErrTemporary, t.redact(err.Error()))
	}
	defer func() { _ = res.Body.Close() }()
	if err := statusError(res); err != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, maxResponseBytes))
		return err
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxResponseBytes)).Decode(out); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("%w: deepl: %w", translation.ErrTemporary, ctxErr)
		}
		return fmt.Errorf("%w: deepl: the answer is not the JSON of the API", translation.ErrTemporary)
	}
	return nil
}

// statusError maps the status of an answer to the port's errors: 456 is the quota of the plan and 429 and 529 the rate
// limit, which DeepL states and the port reports as its limit; every other failure, a refused key and a request that
// DeepL refuses among them, is temporary, so that the service moves on to its next provider. The languages are checked
// before a request, so a refused language does not come to this.
func statusError(res *http.Response) error {
	switch code := res.StatusCode; {
	case code >= 200 && code < 300:
		return nil
	case code == 456:
		return fmt.Errorf("%w: deepl: the quota of the plan is used up (status 456)", translation.ErrProviderLimit)
	case code == http.StatusTooManyRequests || code == 529:
		return fmt.Errorf("%w: deepl: too many requests (status %d)", translation.ErrProviderLimit, code)
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return fmt.Errorf("%w: deepl: the key is refused (status %d)", translation.ErrTemporary, code)
	default:
		return fmt.Errorf("%w: deepl: status %d", translation.ErrTemporary, code)
	}
}

// redact removes the key from s, which describes a failed request.
func (t *Translator) redact(s string) string {
	if t.key == "" {
		return s
	}
	return strings.ReplaceAll(s, t.key, "[redacted]")
}

// languages are the languages of GET /v3/languages, whose codes are BCP 47 tags.
type languages struct {
	fetched time.Time
	sources []language.Tag
	targets []language.Tag
	codes   map[language.Tag]string
}

type languageEntry struct {
	Lang           string `json:"lang"`
	UsableAsSource bool   `json:"usable_as_source"`
	UsableAsTarget bool   `json:"usable_as_target"`
}

// supported returns the languages, from the cache while it is fresh. A failure to read them is temporary, and is not
// kept.
func (t *Translator) supported(ctx context.Context) (*languages, error) {
	t.mu.Lock()
	cached := t.languages
	t.mu.Unlock()
	if cached != nil && t.now().Sub(cached.fetched) < languagesTTL {
		return cached, nil
	}
	var entries []languageEntry
	if err := t.call(ctx, http.MethodGet, "/v3/languages?resource=translate_text", nil, &entries); err != nil {
		return nil, err
	}
	langs := newLanguages(entries, t.now())
	if len(langs.targets) == 0 {
		return nil, fmt.Errorf("%w: deepl: the API lists no target language", translation.ErrTemporary)
	}
	t.mu.Lock()
	t.languages = langs
	t.mu.Unlock()
	return langs, nil
}

func newLanguages(entries []languageEntry, now time.Time) *languages {
	l := &languages{fetched: now, codes: make(map[language.Tag]string)}
	for _, e := range entries {
		tag, err := language.Parse(e.Lang)
		if err != nil {
			continue
		}
		l.codes[tag] = e.Lang
		if e.UsableAsSource {
			l.sources = append(l.sources, tag)
		}
		if e.UsableAsTarget {
			l.targets = append(l.targets, tag)
		}
	}
	return l
}

func (l *languages) source(tag string) (string, bool) { return l.match(l.sources, tag) }
func (l *languages) target(tag string) (string, bool) { return l.match(l.targets, tag) }

// match returns DeepL's code of the language of tag among tags: DeepL's code of the same language, and of its closest
// variant when DeepL has several (en to en-US, zh-TW to zh-Hant). Another language never stands in.
func (l *languages) match(tags []language.Tag, tag string) (string, bool) {
	t, err := language.Parse(tag)
	if err != nil {
		return "", false
	}
	base, _, _ := t.Raw()
	if base.String() == translation.UndeterminedLanguage {
		return "", false
	}
	var same []language.Tag
	for _, c := range tags {
		if b, _, _ := c.Raw(); b == base {
			same = append(same, c)
		}
	}
	switch len(same) {
	case 0:
		return "", false
	case 1:
		return l.codes[same[0]], true
	}
	_, i, _ := language.NewMatcher(same).Match(t)
	return l.codes[same[i]], true
}
