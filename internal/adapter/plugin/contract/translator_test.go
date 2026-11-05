// SPDX-License-Identifier: AGPL-3.0-only

package contract_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/contract"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/adapter/translator/fake"
	"github.com/mycroblog/my/internal/app/translation"
)

func runTranslator(t *testing.T, o fakeplugin.Options, from, to string) map[string]contract.Result {
	t.Helper()
	k, _ := kind.ByName(kind.Translator)
	h, _ := fakeplugin.JSON(o)
	conn, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, KeepDetail: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	out := map[string]contract.Result{}
	contract.Run(t.Context(), k, contract.Env{Conn: conn, Protocol: k.Protocol, TranslateFrom: from, TranslateTo: to}, func(r contract.Result) { out[r.Case] = r })
	return out
}

var translatorCaseNames = []string{"translate", "translate_markers_only", "translate_undetermined", "translate_unsupported_language"}

func TestTranslatorCases(t *testing.T) {
	results := runTranslator(t, fakeplugin.Options{Translator: fake.New(fake.Options{Detected: "en", Unsupported: []string{"qaa"}})}, "", "")
	for _, name := range translatorCaseNames {
		if r := results[name]; r.Outcome != contract.Pass {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

// translatorFunc is a plugin's service as a function.
type translatorFunc func(translation.Request) (translation.Result, error)

func (f translatorFunc) Translate(_ context.Context, req translation.Request) (translation.Result, error) {
	return f(req)
}

// The flags choose the languages, for a plugin that has only some.
func TestTranslatorCasesUseTheLanguagesOfTheFlags(t *testing.T) {
	var sources, targets []string
	service := translatorFunc(func(req translation.Request) (translation.Result, error) {
		sources, targets = append(sources, req.SourceLanguage), append(targets, req.TargetLanguage)
		if req.SourceLanguage == "qaa" || req.TargetLanguage == "qaa" {
			return translation.Result{}, translation.ErrUnsupportedLanguage
		}
		return translation.Result{Translations: req.Fragments}, nil
	})
	results := runTranslator(t, fakeplugin.Options{Translator: service}, "de", "fr")
	for _, name := range translatorCaseNames {
		if r := results[name]; r.Outcome != contract.Pass {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if len(sources) == 0 || sources[0] != "de" || targets[0] != "fr" {
		t.Errorf("sources = %v, targets = %v", sources, targets)
	}
}

// A plugin that cannot detect a language may say so.
func TestTranslatorMayRefuseToDetect(t *testing.T) {
	service := translatorFunc(func(req translation.Request) (translation.Result, error) {
		if req.SourceLanguage == translation.UndeterminedLanguage {
			return translation.Result{}, translation.ErrUnsupportedLanguage
		}
		return translation.Result{}, translation.ErrTemporary
	})
	results := runTranslator(t, fakeplugin.Options{Translator: service}, "", "")
	if r := results["translate_undetermined"]; r.Outcome != contract.Pass {
		t.Errorf("%+v", r)
	}
}

func TestTranslatorCasesCatchABrokenPlugin(t *testing.T) {
	for name, tc := range map[string]struct {
		options fakeplugin.Options
		fails   []string
		detail  string
	}{
		"no_rpc": {options: fakeplugin.Options{}, fails: translatorCaseNames, detail: "does the plugin serve this RPC"},
		"empty":  {options: fakeplugin.Options{Translator: fake.New(fake.Options{}), Broken: true}, fails: translatorCaseNames, detail: "not a result"},
		"drops_the_markers": {
			options: fakeplugin.Options{Translator: translatorFunc(func(req translation.Request) (translation.Result, error) {
				return translation.Result{Translations: slicesOf(len(req.Fragments), "訳")}, nil
			})},
			fails:  []string{"translate", "translate_markers_only"},
			detail: "does not hold the markers",
		},
		"loses_a_fragment": {
			options: fakeplugin.Options{Translator: translatorFunc(func(req translation.Request) (translation.Result, error) {
				return translation.Result{Translations: req.Fragments[1:]}, nil
			})},
			fails:  []string{"translate", "translate_markers_only", "translate_undetermined"},
			detail: "translations of",
		},
		"translates_anything": {
			options: fakeplugin.Options{Translator: translatorFunc(func(req translation.Request) (translation.Result, error) {
				return translation.Result{Translations: req.Fragments}, nil
			})},
			fails:  []string{"translate_unsupported_language"},
			detail: "OUTCOME_UNSUPPORTED_LANGUAGE",
		},
	} {
		t.Run(name, func(t *testing.T) {
			results := runTranslator(t, tc.options, "", "")
			for _, c := range tc.fails {
				r := results[c]
				if r.Outcome != contract.Fail {
					t.Errorf("%s: %+v", c, r)
				}
			}
			if detail := results[tc.fails[0]].Detail; !strings.Contains(detail, tc.detail) {
				t.Errorf("detail = %q, want %q", detail, tc.detail)
			}
		})
	}
}

func slicesOf(n int, s string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}
