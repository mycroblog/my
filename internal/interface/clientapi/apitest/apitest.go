// SPDX-License-Identifier: AGPL-3.0-only

// Package apitest checks client API handlers against api/openapi.yaml in tests (ADR-0016).
// Every handler test of the client API serves its requests through Spec.Serve, so that a
// response that drifts from the spec fails the test:
//
//	spec := apitest.Load(t)
//	res := spec.Serve(t, handler, httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil))
//
// It validates with kin-openapi's openapi3filter. Only tests import it.
package apitest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/mycroblog/my/api"
)

// Spec is the loaded and validated client API document.
type Spec struct {
	doc    *openapi3.T
	router routers.Router
}

var load = sync.OnceValues(func() (*Spec, error) {
	// The streaming endpoint's body is a text/event-stream, a string to the spec; tests check
	// its events with ValidateSchema.
	openapi3filter.RegisterBodyDecoder("text/event-stream", openapi3filter.PlainBodyDecoder)
	doc, err := openapi3.NewLoader().LoadFromData(api.Spec)
	if err != nil {
		return nil, err
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, err
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		return nil, err
	}
	return &Spec{doc: doc, router: router}, nil
})

// Load returns the client API document, failing t when it does not load or is not a valid
// OpenAPI document.
func Load(t testing.TB) *Spec {
	t.Helper()
	s, err := load()
	if err != nil {
		t.Fatalf("apitest: load api/openapi.yaml: %v", err)
	}
	return s
}

// Doc returns the parsed document, for tests that inspect the spec itself.
func (s *Spec) Doc() *openapi3.T { return s.doc }

// Serve sends req to h and returns the recorded response with its body readable. It fails t
// when req matches no operation of the spec or the response does not conform to that
// operation: an undocumented status, content type, or header, or a body that the schema
// rejects. The request itself is not validated, so tests may send invalid requests on
// purpose; use ValidateRequest for that.
func (s *Spec) Serve(t testing.TB, h http.Handler, req *http.Request) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	body := rec.Body.Bytes()
	s.ValidateResponse(t, req, res.StatusCode, res.Header, body)
	res.Body = io.NopCloser(bytes.NewReader(body))
	return res
}

// ValidateSchema fails t when the JSON document data does not conform to the component schema
// name (#/components/schemas/<name>), for payloads that no response body describes: the data
// of the streaming endpoint's events.
func (s *Spec) ValidateSchema(t testing.TB, name string, data []byte) {
	t.Helper()
	ref := s.doc.Components.Schemas[name]
	if ref == nil || ref.Value == nil {
		t.Fatalf("apitest: no schema %q in the spec", name)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Errorf("apitest: %s: not JSON: %v\ndata: %s", name, err, data)
		return
	}
	if err := ref.Value.VisitJSON(v, openapi3.MultiErrors()); err != nil {
		t.Errorf("apitest: data does not conform to %s: %v\ndata: %s", name, err, data)
	}
}

// ValidateRequest fails t when req does not conform to its operation in the spec.
// Authentication is not checked.
func (s *Spec) ValidateRequest(t testing.TB, req *http.Request) {
	t.Helper()
	in := s.requestInput(t, req)
	if err := openapi3filter.ValidateRequest(req.Context(), in); err != nil {
		t.Errorf("apitest: %s %s: request does not conform to the spec: %v", req.Method, req.URL.Path, err)
	}
}

// ValidateResponse fails t when a response to req with status, header, and body does not
// conform to the spec.
func (s *Spec) ValidateResponse(t testing.TB, req *http.Request, status int, header http.Header, body []byte) {
	t.Helper()
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: s.requestInput(t, req),
		Status:                 status,
		Header:                 header,
		Options:                &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true},
	}
	in.SetBodyBytes(body)
	if err := openapi3filter.ValidateResponse(req.Context(), in); err != nil {
		t.Errorf("apitest: %s %s: %d response does not conform to the spec: %v\nbody: %s", req.Method, req.URL.Path, status, err, body)
	}
}

func (s *Spec) requestInput(t testing.TB, req *http.Request) *openapi3filter.RequestValidationInput {
	t.Helper()
	// The spec's server URL is the relative /api/v1, which matches only a URL without
	// scheme and host.
	r := req.Clone(req.Context())
	r.URL = &url.URL{Path: req.URL.Path, RawPath: req.URL.RawPath, RawQuery: req.URL.RawQuery}
	route, params, err := s.router.FindRoute(r)
	if err != nil {
		t.Fatalf("apitest: %s %s: no operation in the spec: %v", req.Method, req.URL.Path, err)
	}
	return &openapi3filter.RequestValidationInput{
		Request: r, PathParams: params, Route: route,
		Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
	}
}
