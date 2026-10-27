// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sync"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/i18n"
)

// ContentTypeProblem is the media type of RFC 9457 problem documents.
const ContentTypeProblem = "application/problem+json"

// ProblemTypeBlank is the problem type of problems that mean no more than their HTTP status.
const ProblemTypeBlank = "about:blank"

type localeKey struct{}

// requestLocale is the locale of one request. It is resolved on first use, so that a
// request whose response is not localized never reads the instance default, and so that a
// request authenticated before its first localized message gets its user's locale.
type requestLocale struct {
	acceptLanguage string
	once           sync.Once
	l              *i18n.Localizer
}

// localize stores the request's Accept-Language in its context, where the strict handlers,
// which see only the context, find the request's localizer with Localizer. Every response may
// be localized (problems, instance texts), so every response varies with Accept-Language.
func (s *Server) localize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		varyAcceptLanguage(w.Header())
		next.ServeHTTP(w, r.WithContext(withLocale(r)))
	})
}

func varyAcceptLanguage(h http.Header) {
	if !slices.Contains(h.Values("Vary"), "Accept-Language") {
		h.Add("Vary", "Accept-Language")
	}
}

// withLocale returns the context of r with its requestLocale.
func withLocale(r *http.Request) context.Context {
	if _, ok := r.Context().Value(localeKey{}).(*requestLocale); ok {
		return r.Context()
	}
	return context.WithValue(r.Context(), localeKey{}, &requestLocale{acceptLanguage: r.Header.Get("Accept-Language")})
}

func (s *Server) localizer(r *http.Request) *i18n.Localizer { return s.Localizer(withLocale(r)) }

// Localizer returns the localizer of the request that ctx belongs to (ADR-0009 §2), for the
// messages of problem documents that a handler returns. Outside a request it resolves
// without Accept-Language.
func (s *Server) Localizer(ctx context.Context) *i18n.Localizer {
	rl, ok := ctx.Value(localeKey{}).(*requestLocale)
	if !ok {
		return s.o.Locales.Localizer(ctx, "")
	}
	rl.once.Do(func() { rl.l = s.o.Locales.Localizer(ctx, rl.acceptLanguage) })
	return rl.l
}

// NewProblem returns a problem document of status with the localized title and optional
// detail, and the response headers that go with it (Content-Language, and Cache-Control:
// no-store since problems are never cached). Handlers return it as the body of their
// operation's default response, for example
//
//	body, headers := s.NewProblem(ctx, http.StatusNotFound, ProblemTypeBlank, l.ServerErrorsNotFound(), "")
//	return GetStatusdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
func (s *Server) NewProblem(ctx context.Context, status int, typ, title, detail string) (Problem, ProblemResponseHeaders) {
	p := Problem{Type: typ, Title: title, Status: status}
	if detail != "" {
		p.Detail = &detail
	}
	if id := s.o.RequestID(ctx); id != "" {
		p.RequestId = &id
	}
	locale, cache := s.Localizer(ctx).Locale(), cacheNoStore
	return p, ProblemResponseHeaders{ContentLanguage: &locale, CacheControl: &cache}
}

// invalidParameterResponse is invalidParameter as a default response (problem, statuses.go).
func (s *Server) invalidParameterResponse(ctx context.Context, name string) problem {
	body, headers := s.invalidParameter(ctx, name)
	return problem{Body: body, Headers: headers, StatusCode: http.StatusBadRequest}
}

// writeProblem writes a problem document outside the strict handlers: for requests that the
// generated code rejects, that authentication, authorization, or rate limiting refuse, and for
// errors that handlers return. Headers already set on w (WWW-Authenticate, Retry-After) are
// kept. Problems are never cached.
func (s *Server) writeProblem(w http.ResponseWriter, r *http.Request, status int, typ, title, detail string) {
	p, headers := s.NewProblem(withLocale(r), status, typ, title, detail)
	body, err := json.Marshal(p)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", ContentTypeProblem)
	h.Set("Content-Language", *headers.ContentLanguage)
	h.Set("Cache-Control", "no-store")
	varyAcceptLanguage(h)
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

// parameterName returns the name of the parameter that a generated request error is about,
// or "".
func parameterName(err error) string {
	var (
		invalid     *InvalidParamFormatError
		required    *RequiredParamError
		header      *RequiredHeaderError
		unmarshal   *UnmarshalingParamError
		tooMany     *TooManyValuesForParamError
		cookieParam *UnescapedCookieParamError
	)
	switch {
	case errors.As(err, &invalid):
		return invalid.ParamName
	case errors.As(err, &required):
		return required.ParamName
	case errors.As(err, &header):
		return header.ParamName
	case errors.As(err, &unmarshal):
		return unmarshal.ParamName
	case errors.As(err, &tooMany):
		return tooMany.ParamName
	case errors.As(err, &cookieParam):
		return cookieParam.ParamName
	}
	return ""
}

// requestError answers a request that the generated code cannot parse with a 400 problem;
// the detail names the parameter when the error is about one.
func (s *Server) requestError(w http.ResponseWriter, r *http.Request, err error) {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		s.writeTooLarge(w, r, r.Pattern)
		return
	}
	l := s.localizer(r)
	var detail string
	if name := parameterName(err); name != "" {
		detail = l.ServerErrorsInvalidParameter(name)
	}
	s.writeProblem(w, r, http.StatusBadRequest, ProblemTypeBlank, l.ServerErrorsBadRequest(), detail)
}

// responseError answers an error that a handler returned, or a response that cannot be
// written, with a 500 problem that reveals nothing about the error.
func (s *Server) responseError(w http.ResponseWriter, r *http.Request, err error) {
	s.o.Logger.ErrorContext(r.Context(), "client api handler failed", slog.Any("error", err))
	if s.o.ErrorReporter != nil {
		s.o.ErrorReporter.Report(r.Context(), kernel.NewErrorEvent(err, "")) // the reporter names the route
	}
	l := s.localizer(r)
	s.writeProblem(w, r, http.StatusInternalServerError, ProblemTypeBlank, l.ServerErrorsInternal(), "")
}
