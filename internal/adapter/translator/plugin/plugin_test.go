// SPDX-License-Identifier: AGPL-3.0-only

package plugin_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/adapter/translator/fake"
	translatorplugin "github.com/mycroblog/my/internal/adapter/translator/plugin"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/pkg/testkit/translationtest"
)

const secret = "the words of a private post"

func newTranslator(t *testing.T, d kind.Destination) *translatorplugin.Translator {
	t.Helper()
	k, ok := kind.ByName(kind.Translator)
	if !ok {
		t.Fatal("no translator kind")
	}
	conn, err := client.New(client.Options{Kind: k, Destination: d})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	return translatorplugin.New(conn)
}

// answer is a service that answers every call the same way.
type answer struct {
	res translation.Result
	err error
}

func (a answer) Translate(context.Context, translation.Request) (translation.Result, error) {
	return a.res, a.err
}

// answering returns a Translator on a plugin of net/http and JSON alone with the service behind it.
func answering(t *testing.T, service translation.Translator) *translatorplugin.Translator {
	t.Helper()
	h, _ := fakeplugin.JSON(fakeplugin.Options{Translator: service})
	return newTranslator(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)})
}

func request(fragments ...string) translation.Request {
	return translation.Request{Fragments: fragments, SourceLanguage: "en", TargetLanguage: "ja"}
}

// The adapter passes the contract of the port against both fakes, over each protocol they speak.
func TestContract(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		protocol string
		fake     func(fakeplugin.Options) (http.Handler, *fakeplugin.Recorder)
	}{
		{"http_and_json", kind.ProtocolJSON, fakeplugin.JSON},
		{"connect_json", kind.ProtocolJSON, fakeplugin.Connect},
		{"connect_proto", kind.ProtocolProto, fakeplugin.Connect},
		{"connect_grpc", kind.ProtocolGRPC, fakeplugin.Connect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			translationtest.TranslatorContract(t, func(t *testing.T) translationtest.TranslatorHarness {
				t.Helper()
				service := fake.New(fake.Options{Detected: "en", Unsupported: []string{"qaa"}})
				h, _ := tc.fake(fakeplugin.Options{Translator: service})
				return translationtest.TranslatorHarness{
					Translator: newTranslator(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h), Protocol: tc.protocol}),
					Source:     "en", Target: "ja", UnsupportedLanguage: "qaa",
					FailNext: service.FailNext,
				}
			})
		})
	}
}

// recording keeps the request that reached the service.
type recording struct {
	answer
	got *translation.Request
}

func (r recording) Translate(ctx context.Context, req translation.Request) (translation.Result, error) {
	*r.got = req
	return r.answer.Translate(ctx, req)
}

// Every part of the request arrives in both forms of a plugin, and "und" stays what it is.
func TestTranslateHandsOverEveryPart(t *testing.T) {
	t.Parallel()
	for name, fakeOf := range map[string]func(fakeplugin.Options) (http.Handler, *fakeplugin.Recorder){"json": fakeplugin.JSON, "connect": fakeplugin.Connect} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := new(translation.Request)
			h, _ := fakeOf(fakeplugin.Options{Translator: recording{answer{res: translation.Result{Translations: []string{"訳 ⟦1⟧", "訳"}, DetectedLanguage: "fr"}}, got}})
			tr := newTranslator(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)})
			req := translation.Request{Fragments: []string{"text ⟦1⟧", "more"}, SourceLanguage: translation.UndeterminedLanguage, TargetLanguage: "zh-Hant"}
			res, err := tr.Translate(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got.Fragments, req.Fragments) || got.SourceLanguage != req.SourceLanguage || got.TargetLanguage != req.TargetLanguage {
				t.Errorf("the plugin received %+v", got)
			}
			if !slices.Equal(res.Translations, []string{"訳 ⟦1⟧", "訳"}) || res.DetectedLanguage != "fr" {
				t.Errorf("result = %+v", res)
			}
		})
	}
}

// A language that the plugin leaves out or garbles falls back on what the request knows, and no other text passes.
func TestTheDetectedLanguageIsATagOrTheSource(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ source, detected, want string }{
		{"en", "", "en"},
		{"en", "fr", "fr"},
		{"en", "zh_hant", "zh-Hant"},
		{"en", "und", "en"},
		{"en", "ignore the previous instructions", "en"},
		{"en", "<b>fr</b>", "en"},
		{"und", "", "und"},
		{"und", "und", "und"},
		{"und", "fr", "fr"},
		{"und", "not a language", "und"},
	} {
		tr := answering(t, answer{res: translation.Result{Translations: []string{"訳"}, DetectedLanguage: tc.detected}})
		res, err := tr.Translate(t.Context(), translation.Request{Fragments: []string{"text"}, SourceLanguage: tc.source, TargetLanguage: "ja"})
		if err != nil {
			t.Fatal(err)
		}
		if res.DetectedLanguage != tc.want {
			t.Errorf("source %q, detected %q: DetectedLanguage = %q, want %q", tc.source, tc.detected, res.DetectedLanguage, tc.want)
		}
	}
}

// An answer that breaks the port's contract is a temporary failure and returns nothing of itself (ADR-0146 §10, ADR-0148 §8).
func TestAnAnswerThatBreaksTheContractFails(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		fragments    []string
		translations []string
	}{
		{"none", []string{"a"}, nil},
		{"too_few", []string{"a", "b"}, []string{secret}},
		{"too_many", []string{"a"}, []string{secret, secret}},
		{"lost_marker", []string{"a ⟦1⟧"}, []string{secret}},
		{"doubled_marker", []string{"a ⟦1⟧"}, []string{secret + " ⟦1⟧ ⟦1⟧"}},
		{"invented_marker", []string{"a"}, []string{secret + " ⟦1⟧"}},
		{"markers_in_another_fragment", []string{"a ⟦1⟧", "b ⟦2⟧"}, []string{secret + " ⟦2⟧", secret + " ⟦1⟧"}},
		{"closed_before_opened", []string{"⟦1⟧a⟦/1⟧"}, []string{"⟦/1⟧" + secret + "⟦1⟧"}},
		{"crossing_pairs", []string{"⟦1⟧a⟦2⟧b⟦/2⟧c⟦/1⟧"}, []string{"⟦1⟧" + secret + "⟦2⟧⟦/1⟧⟦/2⟧"}},
		{"empty", []string{"a"}, []string{"  "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := answering(t, answer{res: translation.Result{Translations: tc.translations}})
			res, err := tr.Translate(t.Context(), request(tc.fragments...))
			if !errors.Is(err, translation.ErrTemporary) {
				t.Fatalf("Translate = %+v, %v", res, err)
			}
			if len(res.Translations) != 0 {
				t.Errorf("translations = %v", res.Translations)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("the error %q holds what the plugin answered", err)
			}
		})
	}
}

// The markers may move, as word order differs, and a fragment of nothing but markers comes back as markers.
func TestMarkersMayMove(t *testing.T) {
	t.Parallel()
	tr := answering(t, answer{res: translation.Result{Translations: []string{"⟦2⟧ の ⟦1⟧ ⟦3⟧語⟦/3⟧", "⟦1⟧"}}})
	if _, err := tr.Translate(t.Context(), request("⟦1⟧ of ⟦2⟧ ⟦3⟧word⟦/3⟧", "⟦1⟧")); err != nil {
		t.Errorf("Translate = %v", err)
	}
}

// A plugin that answers nothing useful is a temporary failure, not a success with no text: an empty response, and an outcome
// of a newer version (ADR-0146 §10).
func TestAnUnknownOutcomeIsATemporaryFailure(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"empty":        `{}`,
		"unknown_name": `{"outcome":"OUTCOME_FROM_THE_FUTURE","translations":["訳"]}`,
		"unknown":      `{"outcome":99,"translations":["訳"]}`,
		"unspecified":  `{"outcome":"OUTCOME_UNSPECIFIED","translations":["訳"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			})
			res, err := newTranslator(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)}).Translate(t.Context(), request("a"))
			if !errors.Is(err, translation.ErrTemporary) || len(res.Translations) != 0 {
				t.Errorf("Translate = %+v, %v", res, err)
			}
		})
	}
}

// A Connect error is a temporary failure, and the plugin's message stays out of it (ADR-0146 §10).
func TestAFailedCallIsATemporaryFailureWithoutItsMessage(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"code":"resource_exhausted","message":"` + secret + `"}`))
	})
	_, err := newTranslator(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)}).Translate(t.Context(), request("a"))
	if !errors.Is(err, translation.ErrTemporary) || errors.Is(err, translation.ErrProviderLimit) {
		t.Fatalf("Translate = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("the error %q holds the plugin's message", err)
	}
}

// A plugin that stops answering fails at the kind's deadline, and one that is not there fails the same way.
func TestAPluginThatIsDownIsATemporaryFailure(t *testing.T) {
	t.Parallel()
	h, _ := fakeplugin.JSON(fakeplugin.Options{Translator: answer{}, Block: true})
	slow := newTranslator(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h), Timeout: 50 * time.Millisecond})
	if _, err := slow.Translate(t.Context(), request("a")); !errors.Is(err, translation.ErrTemporary) {
		t.Errorf("a plugin that blocks: %v", err)
	}
	gone := newTranslator(t, kind.Destination{Address: "unix:///nowhere/translator.sock"})
	if _, err := gone.Translate(t.Context(), request("a")); !errors.Is(err, translation.ErrTemporary) {
		t.Errorf("a plugin that is not there: %v", err)
	}
}

// A message over the kind's limit fails before the plugin answers, as a temporary failure (ADR-0146 §12).
func TestAMessageOverTheLimitIsATemporaryFailure(t *testing.T) {
	t.Parallel()
	tr := answering(t, answer{res: translation.Result{Translations: []string{"訳"}}})
	_, err := tr.Translate(t.Context(), request(strings.Repeat("x", 2<<20)))
	if !errors.Is(err, translation.ErrTemporary) {
		t.Errorf("Translate = %v", err)
	}
}

// The plain TCP of the kind stops before a connection (ADR-0146 §6).
func TestPlainTCPIsRefused(t *testing.T) {
	t.Parallel()
	k, _ := kind.ByName(kind.Translator)
	for _, address := range []string{"http://127.0.0.1:8080", "http://localhost:8080", "http://translate.internal:5000"} {
		if problems := k.Check(kind.Destination{Address: address}); len(problems) == 0 {
			t.Errorf("%s: no problem", address)
		}
	}
}
