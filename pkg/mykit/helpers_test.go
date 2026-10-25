// SPDX-License-Identifier: MIT

package mykit

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestAllFollowsNextCursor(t *testing.T) {
	pages := map[string]struct {
		items []int
		next  string
	}{
		"":   {[]int{1, 2}, "c1"},
		"c1": {[]int{3}, "c2"},
		"c2": {[]int{4, 5}, ""},
	}
	var cursors []string
	fetch := func(_ context.Context, c *Cursor) ([]int, *Cursor, error) {
		key := ""
		if c != nil {
			key = *c
		}
		cursors = append(cursors, key)
		p := pages[key]
		if p.next == "" {
			return p.items, nil, nil
		}
		next := p.next
		return p.items, &next, nil
	}

	var got []int
	for v, err := range All(context.Background(), fetch) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, v)
	}
	if !slices.Equal(got, []int{1, 2, 3, 4, 5}) || !slices.Equal(cursors, []string{"", "c1", "c2"}) {
		t.Errorf("items = %v, cursors = %v", got, cursors)
	}

	// Breaking out of the loop fetches no more pages.
	cursors = nil
	for v := range All(context.Background(), fetch) {
		if v == 2 {
			break
		}
	}
	if !slices.Equal(cursors, []string{""}) {
		t.Errorf("cursors after a break = %v, want only the first page", cursors)
	}
}

func TestAllStopsAtAnError(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	fetch := func(_ context.Context, c *Cursor) ([]string, *Cursor, error) {
		calls++
		if c != nil {
			return nil, nil, boom
		}
		next := "c1"
		return []string{"a"}, &next, nil
	}
	var got []string
	var gotErr error
	for v, err := range All(context.Background(), fetch) {
		if err != nil {
			gotErr = err
			continue
		}
		got = append(got, v)
	}
	if !errors.Is(gotErr, boom) || !slices.Equal(got, []string{"a"}) || calls != 2 {
		t.Errorf("items = %v, err = %v, calls = %d", got, gotErr, calls)
	}
}

func TestUploadMedia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/media" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := r.ParseMultipartForm(1 << 20); err != nil { //nolint:gosec // G120: the body is bounded above
			t.Fatal(err)
		}
		f, h, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(f)
		if string(body) != "PNGDATA" || h.Filename != `a"b.png` || h.Header.Get("Content-Type") != "image/png" {
			t.Errorf("file = %q %q %q", body, h.Filename, h.Header.Get("Content-Type"))
		}
		if r.FormValue("description") != "a cat" || r.FormValue("purpose") != "avatar" {
			t.Errorf("fields = %v", r.MultipartForm.Value)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"id":"01890a5d-ac96-774b-bcce-b302099a8057","state":"processing"}`)
	}))
	defer srv.Close()
	api, _ := newTestAPI(t, srv, Config{})
	m, err := api.UploadMedia(context.Background(), MediaUploadInput{
		File:        MediaFile{Name: `a"b.png`, ContentType: "image/png", Body: strings.NewReader("PNGDATA")},
		Description: "a cat",
		Purpose:     MediaUploadPurposeAvatar,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Id != "01890a5d-ac96-774b-bcce-b302099a8057" {
		t.Errorf("media = %+v", m)
	}
}

func TestUploadMediaProblem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeProblem(w, http.StatusRequestEntityTooLarge, "/api/v1/problems/media-too-large", nil)
	}))
	defer srv.Close()
	api, _ := newTestAPI(t, srv, Config{})
	_, err := api.UploadMedia(context.Background(), MediaUploadInput{File: MediaFile{Body: strings.NewReader(strings.Repeat("x", 1<<20))}})
	var p *Problem
	if !errors.As(err, &p) || p.Type != "/api/v1/problems/media-too-large" {
		t.Fatalf("err = %v, want the media-too-large problem", err)
	}
}

func TestPKCE(t *testing.T) {
	// RFC 7636 Appendix B.
	if got := CodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Errorf("CodeChallenge = %q", got)
	}
	v1, v2 := NewCodeVerifier(), NewCodeVerifier()
	if len(v1) != 43 || v1 == v2 || strings.ContainsAny(v1, "+/=") {
		t.Errorf("verifiers %q %q", v1, v2)
	}
}

func TestExchange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != TokenPath || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("%s %s", r.URL.Path, r.Header.Get("Content-Type"))
		}
		_ = r.ParseForm()
		user, pass, basic := r.BasicAuth()
		f := r.PostForm
		if f.Get("grant_type") != "authorization_code" || f.Get("code") != "my_ac_1" || f.Get("code_verifier") != "v" ||
			f.Get("redirect_uri") != "http://127.0.0.1:9/cb" {
			t.Errorf("form = %v", f)
		}
		switch {
		case basic && user == "app" && pass == "s%2Fecret" && f.Get("client_id") == "": //nolint:gosec // G101: a test secret
		case !basic && f.Get("client_id") == "app":
		default:
			t.Errorf("client authentication: basic %v %q %q, form %v", basic, user, pass, f)
		}
		if f.Get("code") == "my_ac_1" {
			writeJSON(w, map[string]any{"access_token": "my_at_1", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "my_rt_1", "scope": "read write"})
		}
	}))
	defer srv.Close()
	for _, secret := range []string{"", "s/ecret"} {
		cfg := OAuthConfig{Server: srv.URL, ClientID: "app", ClientSecret: secret, RedirectURI: "http://127.0.0.1:9/cb", HTTPClient: srv.Client()}
		tok, err := cfg.Exchange(context.Background(), "my_ac_1", "v")
		if err != nil {
			t.Fatal(err)
		}
		if tok.AccessToken != "my_at_1" || tok.RefreshToken != "my_rt_1" || tok.Scope != "read write" || tok.Expiry.IsZero() {
			t.Errorf("token = %+v", tok)
		}
	}
}

func TestExchangeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"the code has expired"}`)
	}))
	defer srv.Close()
	_, err := OAuthConfig{Server: srv.URL, ClientID: "app", HTTPClient: srv.Client()}.Exchange(context.Background(), "c", "v")
	var oe *OAuthError
	if !errors.As(err, &oe) || oe.Code != "invalid_grant" || oe.Status != http.StatusBadRequest || oe.Description != "the code has expired" {
		t.Fatalf("err = %v", err)
	}
}

func TestAcceptsIdempotencyKey(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{http.MethodPost, "/api/v1/statuses", true},
		{http.MethodGet, "/api/v1/statuses", false},
		{http.MethodPost, "/api/v1/statuses/x/favourite", false},
		{http.MethodPost, "/statuses", false},
	} {
		if got := acceptsIdempotencyKey(tc.method, tc.path, APIBasePath); got != tc.want {
			t.Errorf("acceptsIdempotencyKey(%s %s) = %v", tc.method, tc.path, got)
		}
	}
	if !pathMatches("/a/*/b", "/a/1/b") || pathMatches("/a/*/b", "/a//b") || pathMatches("/a/*", "/a/1/b") {
		t.Error("pathMatches")
	}
}

// TestIdempotencyKeyOperationsMatchGenerated keeps idempotencyKeyOperations equal to the operations whose
// generated request builder sets the Idempotency-Key header, that is, those whose spec takes it.
func TestIdempotencyKeyOperationsMatchGenerated(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "api.gen.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var generated []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "New") || !strings.HasSuffix(fn.Name.Name, "RequestWithBody") && !strings.HasSuffix(fn.Name.Name, "Request") {
			continue
		}
		var method, path string
		setsKey := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch callName(call.Fun) {
			case "fmt.Sprintf":
				if lit, ok := call.Args[0].(*ast.BasicLit); ok && path == "" {
					path, _ = strconv.Unquote(lit.Value)
				}
			case "http.NewRequest", "http.NewRequestWithContext":
				for _, a := range call.Args {
					if s, ok := a.(*ast.SelectorExpr); ok && strings.HasPrefix(s.Sel.Name, "Method") {
						method = strings.ToUpper(strings.TrimPrefix(s.Sel.Name, "Method"))
					}
				}
			case "req.Header.Set":
				if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Value == strconv.Quote(idempotencyKeyHeader) {
					setsKey = true
				}
			}
			return true
		})
		if setsKey {
			generated = append(generated, method+" "+strings.ReplaceAll(path, "%s", "*"))
		}
	}
	var listed []string
	for _, op := range idempotencyKeyOperations {
		listed = append(listed, op.method+" "+op.path)
	}
	slices.Sort(generated)
	slices.Sort(listed)
	if len(generated) == 0 || !slices.Equal(generated, listed) {
		t.Errorf("operations that take Idempotency-Key: api.gen.go has %v, idempotencyKeyOperations %v", generated, listed)
	}
}

// callName is the dotted name of a call's function, such as "req.Header.Set".
func callName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return callName(e.X) + "." + e.Sel.Name
	}
	return ""
}
