// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/interface/oauth"
	"github.com/mycroblog/my/internal/platform"
)

// badTexts are the values that no text column of PostgreSQL can hold (data-model §1.2): a NUL
// and a byte that is not UTF-8.
var badTexts = []string{"\x00", "\xff"}

// placeholderID stands for the path parameters that a request does not test, in the canonical
// form that the client API accepts for an ID (ADR-0078 §5).
const placeholderID = "01890a5d-ac96-774b-bcce-b302099a8057"

// sweepRequest is one request of the sweep: an input that a handler may look up holds a bad
// text.
type sweepRequest struct {
	name   string
	method string
	target string // path and query, already escaped
	header http.Header
	body   string
	// api marks client API requests, whose responses must conform to the spec.
	api bool
	// want is the status that the request must get, or 0 for any status but a server error.
	want int
}

// TestLookupsOfInvalidText sends NUL and invalid UTF-8 in every input that a lookup may read,
// path parameters, query parameters, headers, cookies, and form and JSON fields, to every
// operation of the client API and to the other HTTP endpoints, on both dialects: no request is
// a 500, and both dialects answer each request with the same status (#323).
func TestLookupsOfInvalidText(t *testing.T) {
	t.Parallel()
	spec := apitest.Load(t)
	reqs := sweepRequests(spec.Doc())
	statuses := map[string]map[string]int{}
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dbURL := newDatabase(t, true)
			if dialect == "postgres" {
				dbURL = newPostgresDatabase(t)
			}
			statuses[dialect] = runSweep(t, spec, dialect, dbURL, reqs)
		})
	}
	if len(statuses) < 2 {
		return
	}
	for _, r := range reqs {
		if s, p := statuses["sqlite"][r.name], statuses["postgres"][r.name]; s != p {
			t.Errorf("%s: sqlite = %d, postgres = %d", r.name, s, p)
		}
	}
}

func runSweep(t *testing.T, spec *apitest.Spec, dialect, dbURL string, reqs []sweepRequest) map[string]int {
	cfg := loadConfig(t, map[string]string{"MY_DB": dialect, "MY_DATABASE_URL": dbURL, "MY_WEB_ORIGIN": webOrigin, "MY_VOICE": "p2p"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	admin, err := app.Identity.CreateAdmin(t.Context(), identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatal(err)
	}
	token := issueTokens(t, app, admin.User.ID, "").AccessToken
	h := app.Handler()
	got := map[string]int{}
	for i, r := range reqs {
		// The streaming endpoint answers a valid request with an endless stream.
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		req := httptest.NewRequestWithContext(ctx, r.method, r.target, strings.NewReader(r.body))
		req.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", i%250+1)
		for k, v := range r.header {
			req.Header[k] = v
		}
		if r.api && req.Header.Get("Authorization") == "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		var status int
		if r.api {
			status = spec.Serve(t, h, req).StatusCode
		} else {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			status = rec.Code
		}
		cancel()
		if status >= 500 || (r.want != 0 && status != r.want) {
			t.Errorf("%s = %d, want %d", r.name, status, r.want)
		}
		got[r.name] = status
	}
	return got
}

// sweepRequests returns the requests of the sweep, in a stable order.
func sweepRequests(doc *openapi3.T) []sweepRequest {
	var reqs []sweepRequest
	add := func(r sweepRequest) {
		if r.header == nil {
			r.header = http.Header{}
		}
		reqs = append(reqs, r)
	}
	paths := doc.Paths.InMatchingOrder()
	slices.Sort(paths)
	for _, path := range paths {
		item := doc.Paths.Value(path)
		methods := make([]string, 0, len(item.Operations()))
		for m := range item.Operations() {
			methods = append(methods, m)
		}
		slices.Sort(methods)
		for _, method := range methods {
			op := item.GetOperation(method)
			params := append(slices.Clone(item.Parameters), op.Parameters...)
			pathWith := func(bad string) string {
				p := path
				for _, ref := range params {
					if ref.Value.In == openapi3.ParameterInPath {
						p = strings.ReplaceAll(p, "{"+ref.Value.Name+"}", bad)
					}
				}
				return clientapi.BasePath + p
			}
			body, contentType := "", ""
			if op.RequestBody != nil && op.RequestBody.Value.Content.Get("application/json") != nil {
				body, contentType = "{}", "application/json"
			}
			jsonHeader := http.Header{}
			if contentType != "" {
				jsonHeader.Set("Content-Type", contentType)
			}
			for _, bad := range badTexts {
				esc := url.PathEscape(bad)
				label := fmt.Sprintf("%s %s %q", method, path, bad)
				if strings.Contains(path, "{") {
					add(sweepRequest{name: label + " in the path", method: method, target: pathWith(esc), header: jsonHeader.Clone(), body: body, api: true, want: http.StatusBadRequest})
				}
				for _, ref := range params {
					p := ref.Value
					switch p.In {
					case openapi3.ParameterInQuery:
						add(sweepRequest{
							name: label + " in query " + p.Name, method: method, header: jsonHeader.Clone(), body: body, api: true,
							target: pathWith(placeholderID) + "?" + url.QueryEscape(p.Name) + "=" + url.QueryEscape(bad),
							want:   http.StatusBadRequest,
						})
					case openapi3.ParameterInHeader:
						hdr := jsonHeader.Clone()
						hdr.Set(p.Name, bad)
						add(sweepRequest{name: label + " in header " + p.Name, method: method, target: pathWith(placeholderID), header: hdr, body: body, api: true})
					}
				}
			}
			if contentType == "" {
				continue
			}
			schema := op.RequestBody.Value.Content.Get(contentType).Schema.Value
			for _, field := range stringFields(schema, "") {
				b, err := json.Marshal(bodyWith(schema, field, "\x00"))
				if err != nil {
					panic(err)
				}
				// A NUL is no date-time, so the body does not decode: the request is malformed. In any
				// other string, it is a value that the operation rejects.
				want := http.StatusUnprocessableEntity
				if fieldFormat(schema, field) == "date-time" {
					want = http.StatusBadRequest
				}
				// The error reports of the web client are always a 204: the endpoint drops the NUL
				// and looks nothing up.
				if op.OperationID == "reportClientError" {
					want = http.StatusNoContent
				}
				add(sweepRequest{
					name:   fmt.Sprintf("%s %s NUL in body %s", method, path, field),
					method: method, target: pathWith(placeholderID), header: jsonHeader.Clone(), body: string(b), api: true,
					want: want,
				})
			}
		}
	}
	for _, bad := range badTexts {
		esc, q := url.PathEscape(bad), url.QueryEscape(bad)
		label := fmt.Sprintf("%q", bad)
		for _, target := range []string{
			"/.well-known/webfinger?resource=acct:" + q + "@example.test",
			"/.well-known/webfinger?resource=" + q,
			"/ap/actors/" + esc,
			"/ap/actors/" + esc + "/outbox",
			"/ap/actors/x/" + esc,
			"/ap/actors/x/outbox?page=" + q,
			"/ap/objects/" + esc,
			"/ap/objects/" + esc + "/activity",
			"/@" + esc,
			"/@" + esc + "@example.test",
			"/@alice@" + esc,
			"/@alice/" + esc,
			"/@alice/status/" + esc,
			"/@" + esc + "/x",
			"/media/" + esc,
			"/media/proxy/" + esc + "/original",
			"/media/proxy/x/" + esc,
			"/media/proxy/x/original?sig=" + q,
			"/assets/" + esc,
			authpages.PathSignUp + "?invite=" + q,
			authpages.PathVerifyEmail + "?token=" + q,
			authpages.PathSignIn + "?return_to=" + q,
			"/oauth/authorize?response_type=code&client_id=" + q,
			"/oauth/authorize?response_type=code&client_id=" + identity.DefaultWebClientID + "&redirect_uri=" + q,
			"/" + esc,
		} {
			add(sweepRequest{name: label + " GET " + target, method: http.MethodGet, target: target, want: http.StatusBadRequest})
		}
		add(sweepRequest{
			name: label + " POST actor inbox", method: http.MethodPost, target: "/ap/actors/" + esc + "/inbox", body: "{}",
			header: http.Header{"Content-Type": {"application/activity+json"}}, want: http.StatusBadRequest,
		})
		// Cookies that the server looks up.
		add(sweepRequest{
			name: label + " session cookie", method: http.MethodGet, target: authpages.PathSignIn,
			header: http.Header{"Cookie": {authpages.CookieSession + "=" + bad}},
		})
		add(sweepRequest{
			name: label + " refresh cookie", method: http.MethodPost, target: oauth.PathRefresh,
			header: http.Header{
				"Cookie": {oauth.CookieRefreshToken + "=" + bad}, "X-My-Token-Refresh": {"1"},
				"Origin": {webOrigin}, "Sec-Fetch-Site": {"same-site"},
			},
		})
		add(sweepRequest{
			name: label + " bearer token", method: http.MethodGet, target: clientapi.BasePath + "/accounts/verify_credentials",
			header: http.Header{"Authorization": {"Bearer my_at_" + bad}}, api: true,
		})
		// Form fields of the sign-in screens and the token endpoint.
		csrf := strings.Repeat("A", 43)
		formHeader := func() http.Header {
			return http.Header{
				"Content-Type":   {"application/x-www-form-urlencoded"},
				"Sec-Fetch-Site": {"same-origin"},
				"Cookie":         {authpages.CookieCSRF + "=" + csrf},
			}
		}
		forms := []struct {
			path   string
			fields []string
		}{
			{authpages.PathSignIn, []string{"email", "password", "return_to"}},
			{authpages.PathSignUp, []string{"username", "email", "password", "invite_code", "reason", "locale", "time_zone"}},
			{authpages.PathVerifyEmail, []string{"token"}},
			{authpages.PathResendVerification, []string{"email"}},
			{authpages.PathAuthorize, []string{"client_id", "redirect_uri", "scope", "state", "code_challenge", "decision"}},
			{oauth.PathToken, []string{"client_id", "code", "redirect_uri", "code_verifier", "refresh_token"}},
			{oauth.PathRevoke, []string{"client_id", "token"}},
		}
		for _, f := range forms {
			for _, field := range f.fields {
				v := url.Values{
					"csrf_token": {csrf}, "email": {"alice@example.com"}, "password": {"correct horse battery staple"},
					"username": {"bob"}, "client_id": {identity.DefaultWebClientID}, "response_type": {"code"},
					"redirect_uri": {webOrigin + platform.WebClientCallbackPath}, "code_challenge_method": {"S256"},
					"code_challenge": {strings.Repeat("c", 43)}, "code_verifier": {strings.Repeat("v", 43)},
					"grant_type": {"authorization_code"}, "code": {"my_ac_x"}, "token": {"x"}, "decision": {"approve"},
				}
				if field == "refresh_token" {
					v.Set("grant_type", "refresh_token")
				}
				v.Set(field, bad)
				add(sweepRequest{
					name:   fmt.Sprintf("%s POST %s form %s", label, f.path, field),
					method: http.MethodPost, target: f.path, header: formHeader(), body: v.Encode(), want: http.StatusBadRequest,
				})
			}
		}
	}
	return reqs
}

// stringFields returns the paths of the string properties of an object schema, and of the
// strings in its arrays (path[]), sorted.
func stringFields(s *openapi3.Schema, prefix string) []string {
	var out []string
	for _, name := range sortedKeys(s.Properties) {
		p := s.Properties[name].Value
		path := prefix + name
		switch {
		case p.Type.Is(openapi3.TypeString):
			out = append(out, path)
		case p.Type.Is(openapi3.TypeArray) && p.Items != nil && p.Items.Value.Type.Is(openapi3.TypeString):
			out = append(out, path+"[]")
		case p.Type.Is(openapi3.TypeObject):
			out = append(out, stringFields(p, path+".")...)
		}
	}
	return out
}

// bodyWith returns a JSON value of s whose string at field (a path from stringFields) is bad
// and whose other properties hold valid-looking values.
func bodyWith(s *openapi3.Schema, field, bad string) map[string]any {
	return objectWith(s, "", field, bad)
}

func objectWith(s *openapi3.Schema, prefix, field, bad string) map[string]any {
	m := map[string]any{}
	for _, name := range sortedKeys(s.Properties) {
		p := s.Properties[name].Value
		path := prefix + name
		switch {
		case p.Type.Is(openapi3.TypeString):
			m[name] = goodString(p)
			if path == field {
				m[name] = bad
			}
		case p.Type.Is(openapi3.TypeArray):
			if p.Items != nil && p.Items.Value.Type.Is(openapi3.TypeString) {
				if path+"[]" == field {
					m[name] = []string{bad}
				} else {
					m[name] = []string{}
				}
			} else {
				m[name] = []any{}
			}
		case p.Type.Is(openapi3.TypeObject):
			m[name] = objectWith(p, path+".", field, bad)
		case p.Type.Is(openapi3.TypeBoolean):
			m[name] = false
		case p.Type.Is(openapi3.TypeInteger), p.Type.Is(openapi3.TypeNumber):
			m[name] = 1
		}
	}
	return m
}

func goodString(p *openapi3.Schema) any {
	switch {
	case len(p.Enum) > 0:
		return p.Enum[0]
	case p.Format == "date-time":
		return "2026-10-21T00:00:00Z"
	}
	return "x"
}

// fieldFormat returns the format of the string at field (a path from stringFields) of s.
func fieldFormat(s *openapi3.Schema, field string) string {
	name, rest, nested := strings.Cut(strings.TrimSuffix(field, "[]"), ".")
	p, ok := s.Properties[name]
	if !ok || p.Value == nil {
		return ""
	}
	if nested {
		return fieldFormat(p.Value, rest)
	}
	if p.Value.Type.Is(openapi3.TypeArray) && p.Value.Items != nil {
		return p.Value.Items.Value.Format
	}
	return p.Value.Format
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
