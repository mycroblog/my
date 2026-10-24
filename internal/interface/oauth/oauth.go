// SPDX-License-Identifier: AGPL-3.0-only

// Package oauth serves the token endpoints of the OAuth 2.0 authorization server
// (ADR-0016, ADR-0019 §4, ADR-0030): the token endpoint (RFC 6749 §3.2) for the
// authorization_code and refresh_token grants, revocation (RFC 7009), and the web client's
// refresh endpoint. The authorization endpoint is a server-rendered screen in authpages.
//
// The web client never sees its refresh token: the token endpoint hands it over in the
// __Host-my_rt cookie (HttpOnly, Secure, SameSite=Strict, Path=/) instead of the response body,
// and POST /oauth/token/refresh is the only endpoint that reads the cookie. That endpoint
// returns only a new access token, rotates the cookie, and passes
// net/http.CrossOriginProtection and the required X-My-Token-Refresh header, which a
// cross-site form cannot send. Every other endpoint ignores cookies.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
)

// Paths of the endpoints.
const (
	PathToken   = "/oauth/token" //nolint:gosec // G101: a path, not a credential
	PathRefresh = "/oauth/token/refresh"
	PathRevoke  = "/oauth/revoke"
	// PathRegister is the dynamic client registration endpoint (RFC 7591).
	PathRegister = "/oauth/register"
	// PathAuthorize is the authorization endpoint, served by authpages.
	PathAuthorize = "/oauth/authorize"
	// PathAuthorizationServerMetadata is the authorization server metadata (RFC 8414).
	PathAuthorizationServerMetadata = "/.well-known/oauth-authorization-server"
	// PathProtectedResourceMetadata is the protected resource metadata (RFC 9728).
	PathProtectedResourceMetadata = "/.well-known/oauth-protected-resource"
)

// CookieRefreshToken carries the web client's refresh token. The __Host- prefix makes
// browsers accept it only when Secure, without a Domain, and with Path=/.
const CookieRefreshToken = "__Host-my_rt" //nolint:gosec // G101: a cookie name, not a credential

// HeaderRefresh must be "1" on POST /oauth/token/refresh. It is not a CORS-safelisted header,
// so no other site can send it without a preflight that the server does not allow.
const HeaderRefresh = "X-My-Token-Refresh"

// maxFormBytes bounds a request body; the fields are a few tokens and URIs.
const maxFormBytes = 16 << 10

// Rate limits of the endpoints that are open to third-party apps (ADR-0090 §7, ADR-0019 §5).
var (
	// TokenLimit bounds the requests to /oauth/token of one client from one IP address.
	TokenLimit = kernel.Limit{Name: "oauth.token", Max: 60, Window: time.Minute}
	// RegisterIPLimit bounds the dynamic registrations from one IP address (IPv6 by /64),
	// RegisterLimit those of the whole instance.
	RegisterIPLimit = kernel.Limit{Name: "oauth.register.ip", Max: 5, Window: time.Hour}
	RegisterLimit   = kernel.Limit{Name: "oauth.register", Max: 100, Window: time.Hour}
)

// limited takes one request of key from each limit in order and, over one of them, answers
// 429 with Retry-After and returns true. Without a RateLimiter, or when the store fails, it
// lets the request through: the limit protects the server and must not take it down.
func (h *Handler) limited(w http.ResponseWriter, r *http.Request, limits ...limitKey) bool {
	if h.o.RateLimiter == nil {
		return false
	}
	for _, l := range limits {
		d, err := h.o.RateLimiter.Take(r.Context(), l.limit, l.key)
		if err != nil {
			h.o.Logger.WarnContext(r.Context(), "oauth rate limit unavailable", slog.String("limit", l.limit.Name), slog.Any("error", err))
			continue
		}
		if !d.Allowed {
			w.Header().Set("Retry-After", strconv.Itoa(max(int(math.Ceil(d.RetryAfter.Seconds())), 1)))
			writeError(w, http.StatusTooManyRequests, &appidentity.OAuthError{Code: "temporarily_unavailable", Description: "too many requests; retry after the time in Retry-After"})
			return true
		}
	}
	return false
}

// limitKey is a limit with the key to take from it.
type limitKey struct {
	limit kernel.Limit
	key   string
}

// Identity is what the endpoints need from the identity module; *identity.Service implements it.
type Identity interface {
	WebClientID() string
	ExchangeAuthorizationCode(ctx context.Context, in appidentity.CodeExchange) (appidentity.IssuedTokens, error)
	RefreshAccessToken(ctx context.Context, in appidentity.RefreshRequest) (appidentity.IssuedTokens, error)
	RefreshWebClient(ctx context.Context, refreshToken string) (appidentity.IssuedTokens, error)
	RevokeToken(ctx context.Context, in appidentity.RevokeRequest) error
	DynamicClientRegistrationEnabled(ctx context.Context) (bool, error)
	MetadataDocumentClientsEnabled(ctx context.Context) (bool, error)
	RegisterDynamicClient(ctx context.Context, req appidentity.DynamicRegistrationRequest) (appidentity.DynamicRegistration, error)
}

// Options configure a Handler.
type Options struct {
	Identity Identity
	// WebOrigin is the web client's origin. When it differs from the server's, it is trusted by
	// the cross-origin protection and allowed by CORS with credentials, so that the web client
	// can receive and send its cookie.
	WebOrigin string
	// RateLimiter applies TokenLimit, RegisterIPLimit, and RegisterLimit; nil applies none.
	RateLimiter *kernel.RateLimiter
	// Origin is the origin of the server, the issuer of the tokens and the base of the
	// endpoint URLs in the metadata (RFC 8414 §2).
	Origin string
	// Logger logs unexpected errors; nil means slog.Default().
	Logger *slog.Logger
}

// Handler serves the endpoints.
type Handler struct {
	o   Options
	cop *http.CrossOriginProtection
}

// New returns a Handler on o. It fails when WebOrigin is not an origin.
func New(o Options) (*Handler, error) {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	h := &Handler{o: o, cop: http.NewCrossOriginProtection()}
	if o.WebOrigin != "" {
		if err := h.cop.AddTrustedOrigin(o.WebOrigin); err != nil {
			return nil, err
		}
	}
	h.cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusForbidden, &appidentity.OAuthError{Code: appidentity.OAuthInvalidRequest, Description: "cross-origin request"})
	}))
	return h, nil
}

// Router registers handlers for http.ServeMux patterns.
type Router interface {
	Handle(pattern string, h http.Handler)
}

// Register registers the endpoints on r.
func (h *Handler) Register(r Router) {
	for path, fn := range map[string]http.HandlerFunc{PathToken: h.token, PathRefresh: h.refresh, PathRevoke: h.revoke} {
		r.Handle("POST "+path, h.cors(fn))
		r.Handle("OPTIONS "+path, h.cors(nil))
	}
	r.Handle("GET "+PathAuthorizationServerMetadata, http.HandlerFunc(h.authorizationServerMetadata))
	r.Handle("GET "+PathProtectedResourceMetadata, http.HandlerFunc(h.protectedResourceMetadata))
	r.Handle("POST "+PathRegister, registerCORS(http.HandlerFunc(h.register)))
	r.Handle("OPTIONS "+PathRegister, registerCORS(nil))
}

// cors lets the web client's origin call the endpoints with its cookie. next nil answers a
// preflight.
func (h *Handler) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Add("Vary", "Origin")
		allowed := h.o.WebOrigin != "" && r.Header.Get("Origin") == h.o.WebOrigin
		if allowed {
			hd.Set("Access-Control-Allow-Origin", h.o.WebOrigin)
			hd.Set("Access-Control-Allow-Credentials", "true")
		}
		if next == nil {
			if allowed {
				hd.Set("Access-Control-Allow-Methods", "POST")
				hd.Set("Access-Control-Allow-Headers", "Content-Type, "+HeaderRefresh)
				hd.Set("Access-Control-Max-Age", "600")
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tokenResponse is the body of a successful token response (RFC 6749 §5.1).
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

// errorResponse is the body of an error response (RFC 6749 §5.2).
type errorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// token serves the token endpoint. It reads only the form, never the refresh token cookie.
func (h *Handler) token(w http.ResponseWriter, r *http.Request) {
	form, creds, basic, ok := h.parse(w, r)
	if !ok {
		return
	}
	if h.limited(w, r, limitKey{TokenLimit, creds.ClientID + " " + kernel.ClientKey(kernel.RequestClientIP(r))}) {
		return
	}
	ctx := r.Context()
	var (
		issued appidentity.IssuedTokens
		err    error
	)
	switch grant := form.Get("grant_type"); grant {
	case "authorization_code":
		web := creds.ClientID != "" && creds.ClientID == h.o.Identity.WebClientID()
		if web {
			// The response sets the web client's cookie, so a cross-site form must not reach it
			// (it could plant another account's refresh token).
			if err := h.cop.Check(r); err != nil {
				writeError(w, http.StatusForbidden, &appidentity.OAuthError{Code: appidentity.OAuthInvalidRequest, Description: "cross-origin request"})
				return
			}
		}
		issued, err = h.o.Identity.ExchangeAuthorizationCode(ctx, appidentity.CodeExchange{
			ClientCredentials: creds, Code: form.Get("code"), RedirectURI: form.Get("redirect_uri"),
			CodeVerifier: form.Get("code_verifier"),
		})
		if err == nil && web {
			setRefreshCookie(w, issued)
			issued.RefreshToken = ""
		}
	case "refresh_token":
		issued, err = h.o.Identity.RefreshAccessToken(ctx, appidentity.RefreshRequest{
			ClientCredentials: creds, RefreshToken: form.Get("refresh_token"), Scope: form.Get("scope"),
		})
	case "":
		err = &appidentity.OAuthError{Code: appidentity.OAuthInvalidRequest, Description: "grant_type is required"}
	default:
		err = &appidentity.OAuthError{Code: appidentity.OAuthUnsupportedGrantType}
	}
	if err != nil {
		h.fail(w, r, err, basic)
		return
	}
	writeTokens(w, issued, issued.RefreshToken)
}

// refresh serves the web client's refresh endpoint: it reads the refresh token from the cookie
// only, rotates it, and returns a new access token.
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	if err := h.cop.Check(r); err != nil {
		writeError(w, http.StatusForbidden, &appidentity.OAuthError{Code: appidentity.OAuthInvalidRequest, Description: "cross-origin request"})
		return
	}
	if r.Header.Get(HeaderRefresh) != "1" {
		writeError(w, http.StatusForbidden, &appidentity.OAuthError{Code: appidentity.OAuthInvalidRequest, Description: HeaderRefresh + ": 1 is required"})
		return
	}
	var token string
	if c, err := r.Cookie(CookieRefreshToken); err == nil {
		token = c.Value
	}
	issued, err := h.o.Identity.RefreshWebClient(r.Context(), token)
	if err != nil {
		var oe *appidentity.OAuthError
		if errors.As(err, &oe) && oe.Code == appidentity.OAuthInvalidGrant {
			clearRefreshCookie(w)
		}
		h.fail(w, r, err, false)
		return
	}
	setRefreshCookie(w, issued)
	writeTokens(w, issued, "")
}

// revoke serves the revocation endpoint (RFC 7009). It answers 200 for unknown tokens too.
// A revocation by the web client also deletes its refresh token cookie; the identity module
// ends the session of the authorization screens that the token family came from.
func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	form, creds, basic, ok := h.parse(w, r)
	if !ok {
		return
	}
	err := h.o.Identity.RevokeToken(r.Context(), appidentity.RevokeRequest{
		ClientCredentials: creds, Token: form.Get("token"), TokenTypeHint: form.Get("token_type_hint"),
	})
	if err != nil {
		h.fail(w, r, err, basic)
		return
	}
	if creds.ClientID == h.o.Identity.WebClientID() {
		clearRefreshCookie(w)
	}
	noStore(w)
	w.WriteHeader(http.StatusOK)
}

// parse reads a form-encoded request body and the client's credentials, from HTTP Basic
// authentication (RFC 6749 §2.3.1) or the form, but not both. It writes the error response
// and returns ok false when the request is malformed.
func (h *Handler) parse(w http.ResponseWriter, r *http.Request) (form url.Values, creds appidentity.ClientCredentials, basic, ok bool) {
	invalid := func(desc string) {
		writeError(w, http.StatusBadRequest, &appidentity.OAuthError{Code: appidentity.OAuthInvalidRequest, Description: desc})
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/x-www-form-urlencoded" {
		invalid("the body must be application/x-www-form-urlencoded")
		return nil, creds, false, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		invalid("malformed body")
		return nil, creds, false, false
	}
	form = r.PostForm
	for k, v := range form {
		if len(v) > 1 { // RFC 6749 §3.2: parameters must not be repeated
			invalid(k + " is repeated")
			return nil, creds, false, false
		}
	}
	creds = appidentity.ClientCredentials{ClientID: form.Get("client_id"), ClientSecret: form.Get("client_secret")}
	if user, pass, has := r.BasicAuth(); has {
		id, err1 := url.QueryUnescape(user)
		secret, err2 := url.QueryUnescape(pass)
		if err1 != nil || err2 != nil {
			invalid("malformed client credentials")
			return nil, creds, false, false
		}
		if creds.ClientSecret != "" || (creds.ClientID != "" && creds.ClientID != id) {
			invalid("use one client authentication method")
			return nil, creds, false, false
		}
		creds, basic = appidentity.ClientCredentials{ClientID: id, ClientSecret: secret}, true
	} else if r.Header.Get("Authorization") != "" {
		invalid("unsupported client authentication")
		return nil, creds, false, false
	}
	return form, creds, basic, true
}

// fail writes the error response of err: an OAuth error with its status, or a server error.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error, basic bool) {
	var oe *appidentity.OAuthError
	if !errors.As(err, &oe) {
		h.o.Logger.ErrorContext(r.Context(), "oauth endpoint failed", slog.String("path", r.URL.Path), slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, &appidentity.OAuthError{Code: "server_error"})
		return
	}
	status := http.StatusBadRequest
	if oe.Code == appidentity.OAuthInvalidClient {
		status = http.StatusUnauthorized
		if basic {
			w.Header().Set("WWW-Authenticate", `Basic realm="oauth"`)
		}
	}
	writeError(w, status, oe)
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	noStore(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, oe *appidentity.OAuthError) {
	writeJSON(w, status, errorResponse{Error: oe.Code, ErrorDescription: oe.Description})
}

func writeTokens(w http.ResponseWriter, issued appidentity.IssuedTokens, refreshToken string) {
	t := issued.Token
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken: issued.AccessToken, TokenType: "Bearer", ExpiresIn: int64(t.ExpiresAt.Sub(t.IssuedAt).Seconds()),
		RefreshToken: refreshToken, Scope: string(t.Scope),
	})
}

func setRefreshCookie(w http.ResponseWriter, issued appidentity.IssuedTokens) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieRefreshToken, Value: issued.RefreshToken, Path: "/", Expires: issued.Token.RefreshExpiresAt,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}

func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieRefreshToken, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}
