// SPDX-License-Identifier: AGPL-3.0-only

// Package google is the translation.Translator of Google Cloud Translation, the TranslateText method of its v3 API
// (ADR-0148 §15). The client authenticates with Application Default Credentials, as the other Google Cloud adapters do.
//
// Cloud Translation keeps HTML tags when the MIME type is text/html, so the port's markers go out as spans and come back
// from the spans of the answer: a marker that stands alone becomes a span that Google leaves untranslated, and a pair
// becomes a span around its words. The text between them is escaped on the way out and unescaped on the way back.
//
// The fragments are the text of a post, so nothing here logs, and an error holds neither a fragment nor anything Google
// said: only the status code of a failure reaches an error.
package google

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	translate "cloud.google.com/go/translate/apiv3"
	"cloud.google.com/go/translate/apiv3/translatepb"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mycroblog/my/internal/app/translation"
)

const (
	// ProviderID is the provider's ID among the providers of the server, which translation_providers and
	// translation_provider_visibilities name.
	ProviderID = "google"
	// Name is what readers see next to a translation by Google.
	Name = "Google Translate"
	// AttributionURL is where the attribution next to a translation links, as Google's attribution requirements ask.
	AttributionURL = "https://translate.google.com"
	// DefaultLocation is the location of the requests when the configuration names none: the global endpoint.
	DefaultLocation = "global"
)

// The limits of one TranslateText request: at most 1024 contents, and Google recommends less than 30,000 code points in
// all. A request with more fragments goes out in several calls.
const (
	maxContents   = 1024
	maxCodePoints = 30000
)

var (
	// projectID matches a Google Cloud project ID, possibly domain-scoped ("example.com:project").
	projectID = regexp.MustCompile(`^([a-z][a-z0-9.-]*[a-z0-9]:)?[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	// locationID matches a location of Cloud Translation: "global" or a region such as "us-central1".
	locationID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
)

// CheckProject accepts a Google Cloud project ID, such as my-project.
func CheckProject(v string) error {
	if !projectID.MatchString(v) {
		return fmt.Errorf("%q is not a Google Cloud project ID, such as my-project", v)
	}
	return nil
}

// CheckLocation accepts a location of Cloud Translation, such as global or us-central1.
func CheckLocation(v string) error {
	if !locationID.MatchString(v) {
		return fmt.Errorf("%q is not a location of Cloud Translation, such as global or us-central1", v)
	}
	return nil
}

// Options configure a Translator.
type Options struct {
	// Project is the Google Cloud project that the requests are billed to (translator_google_project).
	Project string
	// Location is the location of the requests (translator_google_location); empty is DefaultLocation.
	Location string
	// ClientOptions are passed to the Cloud Translation client, after the defaults (tests only: a connection to a
	// stand-in without authentication).
	ClientOptions []option.ClientOption
}

// Translator is the translation.Translator of Cloud Translation. It is safe for concurrent use.
type Translator struct {
	client *translate.TranslationClient
	parent string
}

var _ translation.Translator = (*Translator)(nil)

// New returns a Translator on a Cloud Translation client with Application Default Credentials. It makes no call. Close
// releases the client.
func New(ctx context.Context, opts Options) (*Translator, error) {
	if err := CheckProject(opts.Project); err != nil {
		return nil, fmt.Errorf("google translator: %w", err)
	}
	location := opts.Location
	if location == "" {
		location = DefaultLocation
	}
	if err := CheckLocation(location); err != nil {
		return nil, fmt.Errorf("google translator: %w", err)
	}
	client, err := translate.NewTranslationClient(ctx, opts.ClientOptions...)
	if err != nil {
		return nil, fmt.Errorf("google translator: %w", err)
	}
	return &Translator{client: client, parent: "projects/" + opts.Project + "/locations/" + location}, nil
}

// Close releases the Cloud Translation client.
func (t *Translator) Close() error { return t.client.Close() }

// Translate implements translation.Translator. The fragments go out in as few TranslateText calls as the limits of a
// request allow, in order; the language that Google detects in the first answer is the source of all of them.
func (t *Translator) Translate(ctx context.Context, req translation.Request) (translation.Result, error) {
	target := toGoogle(req.TargetLanguage)
	if target == "" {
		return translation.Result{}, fmt.Errorf("%w: google translator: the target is no language tag", translation.ErrUnsupportedLanguage)
	}
	source := ""
	if req.SourceLanguage != translation.UndeterminedLanguage {
		if source = toGoogle(req.SourceLanguage); source == "" {
			return translation.Result{}, fmt.Errorf("%w: google translator: the source is no language tag", translation.ErrUnsupportedLanguage)
		}
	}
	contents := make([]string, len(req.Fragments))
	for i, f := range req.Fragments {
		contents[i] = toHTML(f)
	}
	translations := make([]string, 0, len(contents))
	detectedCode := ""
	for _, batch := range batches(contents) {
		res, err := t.client.TranslateText(ctx, &translatepb.TranslateTextRequest{
			Parent:             t.parent,
			Contents:           batch,
			MimeType:           "text/html",
			SourceLanguageCode: source,
			TargetLanguageCode: target,
		})
		if err != nil {
			return translation.Result{}, mapError(ctx, err)
		}
		if len(res.GetTranslations()) != len(batch) {
			return translation.Result{}, fmt.Errorf("%w: google translator: %d translations of %d fragments", translation.ErrTemporary, len(res.GetTranslations()), len(batch))
		}
		for _, tr := range res.GetTranslations() {
			if detectedCode == "" {
				detectedCode = tr.GetDetectedLanguageCode()
			}
			translations = append(translations, tr.GetTranslatedText())
		}
	}
	for i, fragment := range req.Fragments {
		translations[i] = fromHTML(fragment, translations[i])
		if !translation.MarkersIntact(fragment, translations[i]) {
			return translation.Result{}, fmt.Errorf("%w: google translator: translation %d does not hold the markers of its fragment", translation.ErrTemporary, i+1)
		}
		if strings.TrimSpace(translations[i]) == "" {
			return translation.Result{}, fmt.Errorf("%w: google translator: translation %d is empty", translation.ErrTemporary, i+1)
		}
	}
	return translation.Result{Translations: translations, DetectedLanguage: fromGoogle(detectedCode, req.SourceLanguage)}, nil
}

// batches splits the contents into the requests that the limits of TranslateText allow, in order. A content over the
// limit of code points goes alone, for Google to judge.
func batches(contents []string) [][]string {
	var out [][]string
	start, points := 0, 0
	for i, c := range contents {
		n := utf8.RuneCountInString(c)
		if i > start && (i-start == maxContents || points+n > maxCodePoints) {
			out = append(out, contents[start:i])
			start, points = i, 0
		}
		points += n
	}
	if start < len(contents) {
		out = append(out, contents[start:])
	}
	return out
}

// mapError returns the port's error of a failed call. Only the status code passes, never the message, which may quote
// the request.
func mapError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: google translator: %w", translation.ErrTemporary, ctx.Err())
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: google translator: the call timed out", translation.ErrTemporary)
	}
	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("%w: google translator: the call failed", translation.ErrTemporary)
	}
	switch st.Code() {
	case codes.ResourceExhausted:
		// A rate limit or a quota of the project.
		return fmt.Errorf("%w: google translator: %s", translation.ErrProviderLimit, st.Code())
	case codes.InvalidArgument:
		if aboutLanguage(st) {
			return fmt.Errorf("%w: google translator: %s", translation.ErrUnsupportedLanguage, st.Code())
		}
	}
	return fmt.Errorf("%w: google translator: %s", translation.ErrTemporary, st.Code())
}

// aboutLanguage reports whether an INVALID_ARGUMENT rejects a language: Google names the field of the language code in
// the details, or says so in the message ("Target language is invalid.", "Unsupported language pair.").
func aboutLanguage(st *status.Status) bool {
	for _, d := range st.Details() {
		if br, ok := d.(*errdetails.BadRequest); ok {
			for _, v := range br.GetFieldViolations() {
				if strings.Contains(v.GetField(), "language_code") {
					return true
				}
			}
		}
	}
	return strings.Contains(strings.ToLower(st.Message()), "language")
}
