// SPDX-License-Identifier: AGPL-3.0-only

package aws

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/translate"
	"github.com/aws/smithy-go"

	"github.com/mycroblog/my/internal/app/translation"
)

// The registration of the provider (ADR-0148 §15): its ID, which translation_providers and
// translation_provider_visibilities name, and what readers see on its translations.
const (
	ProviderID   = "aws"
	ProviderName = "Amazon Translate"
	ProviderURL  = "https://aws.amazon.com/translate/"
)

// MaxRequestBytes is the most text, in bytes of UTF-8, that one TranslateText call takes.
const MaxRequestBytes = 10000

// Client is the part of *translate.Client that the Translator uses.
type Client interface {
	TranslateText(ctx context.Context, in *translate.TranslateTextInput, optFns ...func(*translate.Options)) (*translate.TranslateTextOutput, error)
}

// Translator is the translation.Translator of Amazon Translate. It is safe for concurrent use.
type Translator struct {
	client   Client
	maxBytes int
}

var _ translation.Translator = (*Translator)(nil)

// New returns the Translator that calls client.
func New(client Client) *Translator {
	return &Translator{client: client, maxBytes: MaxRequestBytes}
}

// NewFromConfig returns a Translator on a *translate.Client of cfg (its region, credentials, HTTP
// client, and endpoint). It fails when cfg has no region.
func NewFromConfig(cfg awssdk.Config) (*Translator, error) {
	if cfg.Region == "" {
		return nil, errors.New("translator aws: no AWS region: set translator_aws_region")
	}
	return New(translate.NewFromConfig(cfg)), nil
}

// unit is one piece of a fragment in a call.
type unit struct {
	fragment int
	text     string
}

// Translate implements translation.Translator.
func (t *Translator) Translate(ctx context.Context, req translation.Request) (translation.Result, error) {
	if err := ctx.Err(); err != nil {
		return translation.Result{}, fmt.Errorf("%w: translator aws: %w", translation.ErrTemporary, err)
	}
	target, ok := languageCode(req.TargetLanguage)
	if !ok {
		return translation.Result{}, fmt.Errorf("%w: translator aws: the target is not a language", translation.ErrUnsupportedLanguage)
	}
	source := autoDetect
	if req.SourceLanguage != translation.UndeterminedLanguage {
		if source, ok = languageCode(req.SourceLanguage); !ok {
			return translation.Result{}, fmt.Errorf("%w: translator aws: the source is not a language", translation.ErrUnsupportedLanguage)
		}
	}

	// A fragment without words, such as markers alone, comes back as it is without being sent.
	translations := make([]string, len(req.Fragments))
	var units []unit
	for i, f := range req.Fragments {
		translations[i] = f
		if !strings.ContainsFunc(markerPattern.ReplaceAllString(f, ""), unicode.IsLetter) {
			continue
		}
		translations[i] = ""
		for _, p := range split(f, t.maxBytes) {
			units = append(units, unit{fragment: i, text: p})
		}
	}

	detected := req.SourceLanguage
	for _, batch := range batches(units, t.maxBytes) {
		var text strings.Builder
		for _, u := range batch {
			text.WriteString(unitOpen + encode(u.text) + unitClose)
		}
		out, err := t.client.TranslateText(ctx, &translate.TranslateTextInput{
			Text:               awssdk.String(text.String()),
			SourceLanguageCode: awssdk.String(source),
			TargetLanguageCode: awssdk.String(target),
		})
		if err != nil {
			return translation.Result{}, classify(ctx, err)
		}
		pieces, err := decode(awssdk.ToString(out.TranslatedText))
		if err == nil && len(pieces) != len(batch) {
			err = errUnits
		}
		if err != nil {
			return translation.Result{}, fmt.Errorf("%w: translator aws: %w", translation.ErrTemporary, err)
		}
		for j, u := range batch {
			translations[u.fragment] += joinPiece(u.text, pieces[j])
		}
		if source == autoDetect {
			detected = detectedLanguage(awssdk.ToString(out.SourceLanguageCode), req.SourceLanguage)
			// The other calls of the request translate from the language of the first.
			if code, ok := languageCode(detected); ok && detected != translation.UndeterminedLanguage {
				source = code
			}
		}
	}

	for i, f := range req.Fragments {
		if !translation.MarkersIntact(f, translations[i]) {
			return translation.Result{}, fmt.Errorf("%w: translator aws: translation %d does not hold the markers of its fragment", translation.ErrTemporary, i+1)
		}
		if strings.TrimSpace(translations[i]) == "" {
			return translation.Result{}, fmt.Errorf("%w: translator aws: translation %d is empty", translation.ErrTemporary, i+1)
		}
	}
	return translation.Result{Translations: translations, DetectedLanguage: detected}, nil
}

// joinPiece returns the translation of a piece, with the white space that ended the piece, which
// a translation drops, so that the pieces of a fragment do not run together.
func joinPiece(piece, translated string) string {
	trail := piece[len(strings.TrimRightFunc(piece, unicode.IsSpace)):]
	if trail != "" && !strings.HasSuffix(translated, trail) {
		return strings.TrimRightFunc(translated, unicode.IsSpace) + trail
	}
	return translated
}

// batches packs the units, in order, into calls whose text is at most limit bytes.
func batches(units []unit, limit int) [][]unit {
	var out [][]unit
	var cur []unit
	size := 0
	for _, u := range units {
		n := unitSize(u.text)
		if len(cur) > 0 && size+n > limit {
			out = append(out, cur)
			cur, size = nil, 0
		}
		cur = append(cur, u)
		size += n
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// errorCodePattern is the shape of an error code of Amazon Translate, the only text of an error
// that goes into ours: its message may repeat the input.
var errorCodePattern = regexp.MustCompile(`^[A-Za-z]{1,64}$`)

// classify maps a failed call to the errors of the port.
func classify(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: translator aws: the call ended", translation.ErrTemporary)
	}
	var api smithy.APIError
	if !errors.As(err, &api) {
		return fmt.Errorf("%w: translator aws: the call failed", translation.ErrTemporary)
	}
	code := api.ErrorCode()
	if !errorCodePattern.MatchString(code) {
		code = "an unknown error"
	}
	switch code {
	case "TooManyRequestsException", "ThrottlingException", "LimitExceededException", "ServiceQuotaExceededException":
		return fmt.Errorf("%w: translator aws: %s", translation.ErrProviderLimit, code)
	case "UnsupportedLanguagePairException", "DetectedLanguageLowConfidenceException":
		return fmt.Errorf("%w: translator aws: %s", translation.ErrUnsupportedLanguage, code)
	}
	return fmt.Errorf("%w: translator aws: Amazon Translate answers %s", translation.ErrTemporary, code)
}
