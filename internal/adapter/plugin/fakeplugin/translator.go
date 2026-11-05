// SPDX-License-Identifier: AGPL-3.0-only

package fakeplugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"connectrpc.com/connect/v2"

	translatorv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/translator/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/translator/v1/translatorv1connect"
	"github.com/mycroblog/my/internal/app/translation"
)

type translatorServer struct {
	translatorv1connect.UnimplementedTranslatorServiceHandler

	o Options
}

// Translate implements translatorv1connect.TranslatorServiceHandler.
func (s *translatorServer) Translate(ctx context.Context, req *translatorv1.TranslateRequest) (*translatorv1.TranslateResponse, error) {
	switch {
	case s.o.Translator == nil:
		return nil, connect.NewError(connect.CodeUnimplemented, "this fake has no translator")
	case s.o.Block:
		<-ctx.Done()
		return nil, ctx.Err()
	case s.o.Broken:
		return &translatorv1.TranslateResponse{}, nil
	}
	return answerTranslate(ctx, s.o.Translator, req), nil
}

// answerTranslate answers a call from a port's translator, as a plugin that holds its provider does: the answer goes out as
// it is, right or wrong, so that a test can make the fake break the contract.
func answerTranslate(ctx context.Context, tr translation.Translator, req *translatorv1.TranslateRequest) *translatorv1.TranslateResponse {
	res, err := tr.Translate(ctx, translation.Request{Fragments: req.GetFragments(), SourceLanguage: req.GetSourceLanguage(), TargetLanguage: req.GetTargetLanguage()})
	switch {
	case err == nil:
		return &translatorv1.TranslateResponse{Outcome: translatorv1.Outcome_OUTCOME_OK, Translations: res.Translations, DetectedLanguage: res.DetectedLanguage}
	case errors.Is(err, translation.ErrProviderLimit):
		return &translatorv1.TranslateResponse{Outcome: translatorv1.Outcome_OUTCOME_PROVIDER_LIMIT}
	case errors.Is(err, translation.ErrUnsupportedLanguage):
		return &translatorv1.TranslateResponse{Outcome: translatorv1.Outcome_OUTCOME_UNSUPPORTED_LANGUAGE}
	default:
		return &translatorv1.TranslateResponse{Outcome: translatorv1.Outcome_OUTCOME_TEMPORARY}
	}
}

// serveTranslatorJSON adds the translator kind of the fake of net/http and encoding/json alone.
func serveTranslatorJSON(mux *http.ServeMux, o Options) {
	if o.Translator == nil {
		return
	}
	mux.HandleFunc("POST "+translatorv1connect.TranslatorServiceTranslateProcedure, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "send JSON", http.StatusUnsupportedMediaType)
			return
		}
		// The shape of proto3's JSON mapping: camelCase names, a name for an enum, and fields at their zero value left out.
		var req struct {
			Fragments      []string `json:"fragments"`
			SourceLanguage string   `json:"sourceLanguage"`
			TargetLanguage string   `json:"targetLanguage"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad JSON", http.StatusBadRequest)
			return
		}
		if o.Block {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if o.Broken {
			_, _ = w.Write([]byte("{}"))
			return
		}
		res := answerTranslate(r.Context(), o.Translator, &translatorv1.TranslateRequest{
			Fragments: req.Fragments, SourceLanguage: req.SourceLanguage, TargetLanguage: req.TargetLanguage,
		})
		out := map[string]any{"outcome": res.GetOutcome().String(), "extra": "a field the server does not know, which it ignores"}
		if t := res.GetTranslations(); len(t) > 0 {
			out["translations"] = t
		}
		if l := res.GetDetectedLanguage(); l != "" {
			out["detectedLanguage"] = l
		}
		_ = json.NewEncoder(w).Encode(out)
	})
}
