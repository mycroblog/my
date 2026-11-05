// SPDX-License-Identifier: AGPL-3.0-only

package contract

import (
	"context"
	"errors"
	"fmt"

	translatorplugin "github.com/mycroblog/my/internal/adapter/translator/plugin"
	"github.com/mycroblog/my/internal/app/translation"
)

// The languages of the cases, unless --translate-from and --translate-to name others. unsupportedLanguage is a code that
// ISO 639 leaves for private use, so no service has it.
const (
	defaultTranslateFrom = "en"
	defaultTranslateTo   = "ja"
	unsupportedLanguage  = "qaa"
)

// translatorCases check a translator plugin through the server's adapter, so that they hold it to what a post needs. A real
// plugin may stand before a service that bills per character, so they send a few fixed sentences (ADR-0146 §16).
var translatorCases = []Case{
	{
		Name: "translate",
		Run:  translatorTranslate,
		// Nothing here can judge the words, which a reader takes for what the post says (ADR-0148 §1).
		Note: "the plugin kept the form; read a translation yourself to check its words",
	},
	{Name: "translate_markers_only", Run: translatorMarkersOnly},
	{Name: "translate_undetermined", Run: translatorUndetermined},
	{Name: "translate_unsupported_language", Run: translatorUnsupported},
}

// translateFrom and translateTo are the pair of languages of the cases.
func translateFrom(env Env) string {
	if env.TranslateFrom != "" {
		return env.TranslateFrom
	}
	return defaultTranslateFrom
}

func translateTo(env Env) string {
	if env.TranslateTo != "" {
		return env.TranslateTo
	}
	return defaultTranslateTo
}

// translatorError returns what the plugin did wrong in a call, or nil.
func translatorError(env Env, err error) error {
	if err == nil {
		return nil
	}
	return describeCallError(env, err)
}

// translatorTranslate sends what a post makes: fragments with items that move and a pair that keeps its decoration.
func translatorTranslate(ctx context.Context, env Env) error {
	first, second := translation.Marker(1), translation.Marker(2)
	opening, closing := translation.Marker(3), translation.ClosingMarker(3)
	_, err := translatorplugin.New(env.Conn).Translate(ctx, translation.Request{
		Fragments: []string{
			"Good morning " + first + ", this is " + opening + "important" + closing + " for " + second + ".",
			"This is a test of a translator plugin.",
			first,
		},
		SourceLanguage: translateFrom(env),
		TargetLanguage: translateTo(env),
	})
	return translatorError(env, err)
}

// translatorMarkersOnly sends fragments with no words, which only the markers make up: a post of nothing but a link.
func translatorMarkersOnly(ctx context.Context, env Env) error {
	opening, closing := translation.Marker(1), translation.ClosingMarker(1)
	_, err := translatorplugin.New(env.Conn).Translate(ctx, translation.Request{
		Fragments:      []string{translation.Marker(1) + translation.Marker(2), opening + closing},
		SourceLanguage: translateFrom(env),
		TargetLanguage: translateTo(env),
	})
	return translatorError(env, err)
}

// translatorUndetermined asks the plugin to detect the source. It may answer that it cannot.
func translatorUndetermined(ctx context.Context, env Env) error {
	_, err := translatorplugin.New(env.Conn).Translate(ctx, translation.Request{
		Fragments:      []string{"This is a test of a translator plugin."},
		SourceLanguage: translation.UndeterminedLanguage,
		TargetLanguage: translateTo(env),
	})
	if errors.Is(err, translation.ErrUnsupportedLanguage) {
		return nil
	}
	return translatorError(env, err)
}

// translatorUnsupported checks that a language no service has is refused as such, in either direction. The server would
// otherwise store whatever comes back as a translation into it.
func translatorUnsupported(ctx context.Context, env Env) error {
	for _, req := range []translation.Request{
		{Fragments: []string{"This is a test."}, SourceLanguage: unsupportedLanguage, TargetLanguage: translateTo(env)},
		{Fragments: []string{"This is a test."}, SourceLanguage: translateFrom(env), TargetLanguage: unsupportedLanguage},
	} {
		_, err := translatorplugin.New(env.Conn).Translate(ctx, req)
		if errors.Is(err, translation.ErrUnsupportedLanguage) {
			continue
		}
		if err != nil {
			return describeCallError(env, err)
		}
		return fmt.Errorf("the language %s of no service (from %s to %s): the outcome is OUTCOME_OK, not OUTCOME_UNSUPPORTED_LANGUAGE", unsupportedLanguage, req.SourceLanguage, req.TargetLanguage)
	}
	return nil
}
