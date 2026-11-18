// SPDX-License-Identifier: AGPL-3.0-only

// Package fake is a fake OpenID Connect provider for the end-to-end tests and the local
// demonstration: it stands in for an identity provider of an organization, which a test cannot
// sign in to. Only the test builds (build tag e2e_fakes) link it, as the command my dev
// oidc-provider.
//
// It answers the Authorization Code flow of OpenID Connect Core 1.0 for one client, as a provider
// that the server's login provider of kind oidc talks to: Discovery, an authorization page, the
// token endpoint, and the JWKS of the key it signs ID tokens with (ES256). The authorization page
// asks for no password: its form names who signs in (the subject, the email address and whether
// the provider verified it, the name, the username), so that one provider serves every person of
// a test. Each submission of the form is a new authentication, whose time is the auth_time of the
// ID token, so a request with max_age=0 and prompt=login is answered like any other. It checks
// what a provider checks of its client: the client's ID and secret, the redirect URI of the code,
// that a code is redeemed once, and the PKCE verifier (S256, required). Its state lives in memory
// only, and its signing key is made anew on every start.
//
// HTTP API:
//
//	GET  /.well-known/openid-configuration  the Discovery document
//	GET  /jwks                              the JSON Web Key Set
//	GET  /authorize?...                     the authorization page of an authentication request
//	POST /authorize?...                     the form: sub, email, email_verified, name,
//	                                        preferred_username, and action (allow or deny) -> 303
//	                                        to the redirect URI with code and state, or with
//	                                        error=access_denied
//	POST /token                             the code for an ID token (client_secret_basic)
package fake

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	// CodeTTL is how long an authorization code can be redeemed.
	CodeTTL = time.Minute
	// TokenTTL is the lifetime of an ID token.
	TokenTTL = 5 * time.Minute
	// maxClaimBytes bounds each value of the form.
	maxClaimBytes = 255
)

// Options configure a Provider.
type Options struct {
	// Issuer is the provider's issuer, the origin it is served at over HTTPS, such as
	// https://localhost:8931 (required, without a trailing slash).
	Issuer string
	// ClientID and ClientSecret are the credentials of the one client (required).
	ClientID     string
	ClientSecret string
	// Now returns the current time; nil means time.Now.
	Now func() time.Time
	// Random is the source of the signing key and the codes; nil means crypto/rand.
	Random io.Reader
}

// Provider is the fake OpenID Connect provider. It is safe for concurrent use.
type Provider struct {
	o     Options
	key   *ecdsa.PrivateKey
	kid   string
	mu    sync.Mutex
	codes map[string]grant
}

// grant is what an authorization code stands for.
type grant struct {
	redirectURI string
	challenge   string
	nonce       string
	person      person
	authTime    time.Time
	expires     time.Time
}

// person is who the form of the authorization page named.
type person struct {
	subject       string
	email         string
	emailVerified bool
	name          string
	username      string
}

// New returns a Provider with o and a new signing key.
func New(o Options) (*Provider, error) {
	if u, err := url.Parse(o.Issuer); err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("fake OpenID Connect provider: the issuer must be an https origin, without a trailing slash")
	}
	if o.ClientID == "" || o.ClientSecret == "" {
		return nil, errors.New("fake OpenID Connect provider: the client ID and the client secret are required")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Random == nil {
		o.Random = rand.Reader
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), o.Random)
	if err != nil {
		return nil, err
	}
	p := &Provider{o: o, key: key, codes: map[string]grant{}}
	if p.kid, err = p.token(8); err != nil {
		return nil, err
	}
	return p, nil
}

// token returns n random bytes in unpadded base64url.
func (p *Provider) token(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(p.o.Random, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Handler returns the HTTP API of the fake.
func (p *Provider) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", p.serveDiscovery)
	mux.HandleFunc("GET /jwks", p.serveJWKS)
	mux.HandleFunc("GET /authorize", p.serveAuthorize)
	mux.HandleFunc("POST /authorize", p.serveAuthorize)
	mux.HandleFunc("POST /token", p.serveToken)
	return mux
}

// serveDiscovery answers the provider's configuration (OpenID Connect Discovery 1.0 §3).
func (p *Provider) serveDiscovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                p.o.Issuer,
		"authorization_endpoint":                p.o.Issuer + "/authorize",
		"token_endpoint":                        p.o.Issuer + "/token",
		"jwks_uri":                              p.o.Issuer + "/jwks",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"ES256"},
		"scopes_supported":                      []string{"openid", "email", "profile"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
		"code_challenge_methods_supported":      []string{"S256"},
		"claims_supported": []string{
			"iss", "sub", "aud", "exp", "iat", "auth_time", "nonce", "email", "email_verified", "name", "preferred_username",
		},
	})
}

// serveJWKS answers the public half of the signing key (RFC 7517 §5, RFC 7518 §6.2.1).
func (p *Provider) serveJWKS(w http.ResponseWriter, _ *http.Request) {
	// An uncompressed point: 0x04, then X and Y of 32 bytes each.
	pub, err := p.key.PublicKey.Bytes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{{
		"kty": "EC", "crv": "P-256", "use": "sig", "alg": "ES256", "kid": p.kid,
		"x": base64.RawURLEncoding.EncodeToString(pub[1:33]),
		"y": base64.RawURLEncoding.EncodeToString(pub[33:]),
	}}})
}

// authorizePage is the authorization page. Its form posts to the page's own address, so the
// parameters of the authentication request stay in the query.
var authorizePage = template.Must(template.New("authorize").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sign in - fake OpenID Connect provider</title>
<style>
body { font-family: system-ui, sans-serif; max-width: 28rem; margin: 2rem auto; padding: 0 1rem; }
label { display: block; margin-top: 1rem; }
input[type="text"], input[type="email"] { display: block; width: 100%; box-sizing: border-box; padding: 0.5rem; font: inherit; }
button { margin-top: 1.5rem; margin-right: 0.5rem; padding: 0.5rem 1rem; font: inherit; }
</style>
</head>
<body>
<main>
<h1>Sign in to the fake provider</h1>
<p>This is the stand-in OpenID Connect provider of a test build. It asks for no password: it signs in the person you name here.</p>
{{with .Error}}<p role="alert">{{.}}</p>{{end}}
<form method="post">
<label>Subject <input type="text" name="sub" required autofocus maxlength="255"></label>
<label>Email address <input type="email" name="email" maxlength="255"></label>
<label><input type="checkbox" name="email_verified" value="true"> The provider has verified the email address</label>
<label>Name <input type="text" name="name" maxlength="255"></label>
<label>Username <input type="text" name="preferred_username" maxlength="255"></label>
<button type="submit" name="action" value="allow">Sign in</button>
<button type="submit" name="action" value="deny" formnovalidate>Deny</button>
</form>
</main>
</body>
</html>
`))

// serveAuthorize answers an authentication request (OpenID Connect Core 1.0 §3.1.2): the page
// on GET, and on POST the redirect back to the client with a code for the person of the form.
func (p *Provider) serveAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// Without a client and a redirect URI to trust, the error stays here (§3.1.2.6).
	redirect, err := url.Parse(q.Get("redirect_uri"))
	switch {
	case q.Get("client_id") != p.o.ClientID:
		http.Error(w, "client_id is not the client of this provider", http.StatusBadRequest)
		return
	case err != nil || !redirect.IsAbs() || redirect.Host == "" || redirect.Fragment != "":
		http.Error(w, "redirect_uri must be an absolute URL without a fragment", http.StatusBadRequest)
		return
	}
	back := func(params url.Values) {
		rq := redirect.Query()
		for k, v := range params {
			rq[k] = v
		}
		if state := q.Get("state"); state != "" {
			rq.Set("state", state)
		}
		to := *redirect
		to.RawQuery = rq.Encode()
		w.Header().Set("Cache-Control", "no-store")
		// The flow sends the browser back to the redirect URI of the request; this fake registers none to check it by.
		http.Redirect(w, r, to.String(), http.StatusSeeOther) //nolint:gosec // G710: a fake for tests, on this machine
	}
	refuse := func(code, description string) {
		back(url.Values{"error": {code}, "error_description": {description}})
	}
	switch {
	case q.Get("response_type") != "code":
		refuse("unsupported_response_type", "response_type must be code")
		return
	case !slices.Contains(strings.Fields(q.Get("scope")), "openid"):
		refuse("invalid_scope", "scope must have openid")
		return
	case q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256":
		refuse("invalid_request", "code_challenge with code_challenge_method=S256 is required")
		return
	}
	page := func(status int, message string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_ = authorizePage.Execute(w, map[string]string{"Error": message})
	}
	if r.Method != http.MethodPost {
		page(http.StatusOK, "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		page(http.StatusBadRequest, "The form could not be read.")
		return
	}
	if r.PostForm.Get("action") == "deny" {
		refuse("access_denied", "the person denied the request")
		return
	}
	who := person{
		subject: strings.TrimSpace(r.PostForm.Get("sub")), email: strings.TrimSpace(r.PostForm.Get("email")),
		emailVerified: r.PostForm.Get("email_verified") == "true",
		name:          strings.TrimSpace(r.PostForm.Get("name")), username: strings.TrimSpace(r.PostForm.Get("preferred_username")),
	}
	switch {
	case who.subject == "":
		page(http.StatusBadRequest, "Enter a subject.")
		return
	case max(len(who.subject), len(who.email), len(who.name), len(who.username)) > maxClaimBytes:
		page(http.StatusBadRequest, "Each value is at most 255 bytes.")
		return
	}
	code, err := p.token(24)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	now := p.o.Now()
	p.mu.Lock()
	for c, g := range p.codes {
		if !now.Before(g.expires) {
			delete(p.codes, c)
		}
	}
	p.codes[code] = grant{
		redirectURI: q.Get("redirect_uri"), challenge: q.Get("code_challenge"), nonce: q.Get("nonce"),
		person: who, authTime: now, expires: now.Add(CodeTTL),
	}
	p.mu.Unlock()
	back(url.Values{"code": {code}})
}

// serveToken redeems an authorization code for an ID token (OpenID Connect Core 1.0 §3.1.3,
// RFC 6749 §4.1.3, RFC 7636 §4.6). A code works once.
func (p *Provider) serveToken(w http.ResponseWriter, r *http.Request) {
	fail := func(status int, code, description string) {
		writeJSON(w, status, map[string]string{"error": code, "error_description": description})
	}
	if !p.authenticated(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="fake OpenID Connect provider"`)
		fail(http.StatusUnauthorized, "invalid_client", "the client must authenticate with client_secret_basic")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		fail(http.StatusBadRequest, "invalid_request", "the body must be a form")
		return
	}
	if r.PostForm.Get("grant_type") != "authorization_code" {
		fail(http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code")
		return
	}
	now := p.o.Now()
	code := r.PostForm.Get("code")
	p.mu.Lock()
	g, ok := p.codes[code]
	delete(p.codes, code)
	p.mu.Unlock()
	challenge := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	switch {
	case !ok || !now.Before(g.expires):
		fail(http.StatusBadRequest, "invalid_grant", "the code is unknown, used, or expired")
		return
	case r.PostForm.Get("redirect_uri") != g.redirectURI:
		fail(http.StatusBadRequest, "invalid_grant", "redirect_uri is not the one of the authentication request")
		return
	case base64.RawURLEncoding.EncodeToString(challenge[:]) != g.challenge:
		fail(http.StatusBadRequest, "invalid_grant", "code_verifier does not match the code_challenge")
		return
	}
	idToken, err := p.idToken(g, now)
	if err != nil {
		fail(http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	accessToken, err := p.token(24)
	if err != nil {
		fail(http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken, "token_type": "Bearer", "expires_in": int(TokenTTL.Seconds()), "id_token": idToken,
	})
}

// authenticated reports whether r carries the client's credentials in HTTP Basic authentication,
// each form-encoded before Basic encodes them (RFC 6749 §2.3.1).
func (p *Provider) authenticated(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	id, err := url.QueryUnescape(user)
	if err != nil {
		return false
	}
	secret, err := url.QueryUnescape(pass)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(id), []byte(p.o.ClientID))&subtle.ConstantTimeCompare([]byte(secret), []byte(p.o.ClientSecret)) == 1
}

// idToken returns the signed ID token of g (OpenID Connect Core 1.0 §2, RFC 7515 §7.1): a
// compact JWS with ES256, whose signature is R and S of 32 bytes each (RFC 7518 §3.4).
func (p *Provider) idToken(g grant, now time.Time) (string, error) {
	claims := map[string]any{
		"iss": p.o.Issuer, "sub": g.person.subject, "aud": p.o.ClientID,
		"iat": now.Unix(), "exp": now.Add(TokenTTL).Unix(), "auth_time": g.authTime.Unix(),
	}
	if g.nonce != "" {
		claims["nonce"] = g.nonce
	}
	if g.person.email != "" {
		claims["email"], claims["email_verified"] = g.person.email, g.person.emailVerified
	}
	if g.person.name != "" {
		claims["name"] = g.person.name
	}
	if g.person.username != "" {
		claims["preferred_username"] = g.person.username
	}
	header, err := json.Marshal(map[string]string{"alg": "ES256", "typ": "JWT", "kid": p.kid})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(p.o.Random, p.key, digest[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
