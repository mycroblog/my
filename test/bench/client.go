// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// person is one account of the seed, signed in.
type person struct {
	Username, Email, Password string
	// From is the client IP that the person's requests come from (X-Forwarded-For).
	From  string
	ID    string
	Token string
}

// api sends the requests of the API (/api/v1) to one server.
type api struct {
	origin string
	http   *http.Client
}

func newAPI(origin string, conns int) *api {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns, t.MaxIdleConnsPerHost, t.MaxConnsPerHost = 0, conns, 0
	return &api{origin: origin, http: &http.Client{Transport: t, Timeout: time.Minute}}
}

// do sends method path as p with body as JSON (none when nil), reads the whole answer, and
// returns its status and body. A status that is not 2xx is no error here: the load counts it.
func (a *api) do(ctx context.Context, p *person, method, path string, body any, header map[string]string) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.origin+"/api/v1"+path, rd)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("X-Forwarded-For", p.From)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

// must sends as do does and fails unless the answer is 2xx, decoding it into out when not nil.
func (a *api) must(ctx context.Context, p *person, method, path string, body, out any, header map[string]string) error {
	status, b, err := a.do(ctx, p, method, path, body, header)
	if err != nil {
		return fmt.Errorf("%s %s as %s: %w", method, path, p.Username, err)
	}
	if status/100 != 2 {
		return fmt.Errorf("%s %s as %s: %d %s", method, path, p.Username, status, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

// The authorization code flow of the web client (client my-web), with PKCE, through the sign-in
// screen: what web/e2e/src/seed/sign-in.ts does without a browser.
const webClientID = "my-web"

var authScreenData = regexp.MustCompile(`(?s)<script type="application/json" id="auth">(.*?)</script>`)

// signIn signs p in and sets p.Token.
func signIn(ctx context.Context, origin string, p *person) error {
	// The cookies are kept by hand, as a browser keeps them on localhost: a cookie jar sends no
	// Secure cookie over plain HTTP.
	cookies := map[string]string{}
	c := &http.Client{Timeout: time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	send := func(method, target string, form url.Values) (*http.Response, error) {
		var rd io.Reader
		if form != nil {
			rd = strings.NewReader(form.Encode())
		}
		u, err := url.Parse(origin)
		if err != nil {
			return nil, err
		}
		ref, err := url.Parse(target)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, method, u.ResolveReference(ref).String(), rd)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Forwarded-For", p.From)
		for k, v := range cookies {
			req.AddCookie(&http.Cookie{Name: k, Value: v}) //nolint:gosec // G124: a cookie sent back, as a browser does, not set
		}
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", origin)
			req.Header.Set("Sec-Fetch-Site", "same-origin")
		}
		resp, err := c.Do(req)
		if err == nil {
			for _, ck := range resp.Cookies() {
				cookies[ck.Name] = ck.Value
			}
		}
		return resp, err
	}
	location := func(resp *http.Response) (string, error) {
		_ = resp.Body.Close()
		to := resp.Header.Get("Location")
		if resp.StatusCode/100 != 3 || to == "" {
			return "", fmt.Errorf("sign in as %s: expected a redirect from %s, got %d", p.Username, resp.Request.URL.Path, resp.StatusCode)
		}
		return to, nil
	}
	verifier := randomHex(32)
	sum := sha256.Sum256([]byte(verifier))
	redirect := origin + "/login/callback"
	authorize := "/oauth/authorize?" + url.Values{
		"response_type": {"code"}, "client_id": {webClientID}, "redirect_uri": {redirect}, "state": {"bench"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
	}.Encode()
	resp, err := send(http.MethodGet, authorize, nil)
	if err != nil {
		return err
	}
	screen, err := location(resp)
	if err != nil {
		return err
	}
	if resp, err = send(http.MethodGet, screen, nil); err != nil {
		return err
	}
	page, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	m := authScreenData.FindSubmatch(page)
	if m == nil {
		return fmt.Errorf("sign in as %s: %s is not a sign-in screen (%d)", p.Username, screen, resp.StatusCode)
	}
	var data struct {
		CSRFToken string `json:"csrfToken"`
		ReturnTo  string `json:"returnTo"`
	}
	if err := json.Unmarshal([]byte(html.UnescapeString(string(m[1]))), &data); err != nil {
		return fmt.Errorf("sign in as %s: the screen's data: %w", p.Username, err)
	}
	resp, err = send(http.MethodPost, "/auth/sign_in", url.Values{
		"csrf_token": {data.CSRFToken}, "return_to": {data.ReturnTo}, "email": {p.Email}, "password": {p.Password},
	})
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusSeeOther {
		_ = resp.Body.Close()
		return fmt.Errorf("sign in as %s: the sign-in answered %d", p.Username, resp.StatusCode)
	}
	_ = resp.Body.Close()
	if resp, err = send(http.MethodGet, authorize, nil); err != nil {
		return err
	}
	back, err := location(resp)
	if err != nil {
		return err
	}
	bu, err := url.Parse(back)
	if err != nil {
		return err
	}
	code := bu.Query().Get("code")
	if code == "" {
		return fmt.Errorf("sign in as %s: the authorization returned no code (%s)", p.Username, back)
	}
	resp, err = send(http.MethodPost, "/oauth/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect},
		"client_id": {webClientID}, "code_verifier": {verifier},
	})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sign in as %s: the token endpoint answered %d %s", p.Username, resp.StatusCode, b)
	}
	if err := json.Unmarshal(b, &tok); err != nil || tok.AccessToken == "" {
		return errors.Join(fmt.Errorf("sign in as %s: no access token", p.Username), err)
	}
	p.Token = tok.AccessToken
	return nil
}
