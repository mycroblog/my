// SPDX-License-Identifier: AGPL-3.0-only

// Package plugin is the translation.Translator that delegates to an operator's plugin (ADR-0146 §11, ADR-0148 §14): to the
// server it is one more provider, under the visibilities, the marker check, the sanitizing, and the limits of any other.
//
// The fragments are the text of a post, so nothing here logs, and an error holds neither a fragment nor anything the plugin
// said: the client drops the text of a failed call, and no string of a response reaches an error (ADR-0146 §10).
package plugin

import (
	"context"
	"fmt"
	"strings"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	translatorv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/translator/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/translator/v1/translatorv1connect"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/i18n"
)

// ProviderID is the plugin's ID among the providers of the server, which translation_providers and
// translation_provider_visibilities name.
const ProviderID = "plugin"

// Translator is the translation.Translator of a plugin. It is safe for concurrent use.
type Translator struct {
	client translatorv1connect.TranslatorServiceClient
}

var _ translation.Translator = (*Translator)(nil)

// New returns the Translator that reaches its plugin through conn. It makes no call.
func New(conn *client.Conn) *Translator {
	return &Translator{client: translatorv1connect.NewTranslatorServiceClient(conn.RPC)}
}

// Translate implements translation.Translator. A failed call, a message over the kind's limit, an outcome the server does
// not know, and an answer that breaks the port's contract are temporary failures, so that the service moves on to its next
// provider (ADR-0146 §10, §12, §15, ADR-0148 §8).
func (t *Translator) Translate(ctx context.Context, req translation.Request) (translation.Result, error) {
	res, err := t.client.Translate(ctx, &translatorv1.TranslateRequest{
		Fragments:      req.Fragments,
		SourceLanguage: req.SourceLanguage,
		TargetLanguage: req.TargetLanguage,
	})
	if err != nil {
		return translation.Result{}, fmt.Errorf("%w: translator plugin: %w", translation.ErrTemporary, err)
	}
	switch res.GetOutcome() {
	case translatorv1.Outcome_OUTCOME_OK:
	case translatorv1.Outcome_OUTCOME_TEMPORARY:
		return translation.Result{}, fmt.Errorf("%w: translator plugin: the plugin reports a failure", translation.ErrTemporary)
	case translatorv1.Outcome_OUTCOME_PROVIDER_LIMIT:
		return translation.Result{}, fmt.Errorf("%w: translator plugin", translation.ErrProviderLimit)
	case translatorv1.Outcome_OUTCOME_UNSUPPORTED_LANGUAGE:
		return translation.Result{}, fmt.Errorf("%w: translator plugin", translation.ErrUnsupportedLanguage)
	default:
		return translation.Result{}, fmt.Errorf("%w: translator plugin: the outcome is %d, which is not a result", translation.ErrTemporary, int32(res.GetOutcome()))
	}
	translations := res.GetTranslations()
	if len(translations) != len(req.Fragments) {
		return translation.Result{}, fmt.Errorf("%w: translator plugin: %d translations of %d fragments", translation.ErrTemporary, len(translations), len(req.Fragments))
	}
	for i, fragment := range req.Fragments {
		if !translation.MarkersIntact(fragment, translations[i]) {
			return translation.Result{}, fmt.Errorf("%w: translator plugin: translation %d does not hold the markers of its fragment", translation.ErrTemporary, i+1)
		}
		if strings.TrimSpace(translations[i]) == "" {
			return translation.Result{}, fmt.Errorf("%w: translator plugin: translation %d is empty", translation.ErrTemporary, i+1)
		}
	}
	return translation.Result{Translations: translations, DetectedLanguage: detected(res.GetDetectedLanguage(), req.SourceLanguage)}, nil
}

// detected returns the language of the source: the plugin's tag, or the request's when the plugin says nothing, says
// something that is no language tag, or says it does not know a language that the request knows. The value goes into a
// stored row and the API, so no other text of the plugin passes.
func detected(tag, source string) string {
	tag = i18n.Canonicalize(tag)
	if tag == "" || (tag == translation.UndeterminedLanguage && source != translation.UndeterminedLanguage) {
		return source
	}
	return tag
}
