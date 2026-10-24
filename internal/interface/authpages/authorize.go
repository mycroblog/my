// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// consent is the data of the consent page: what the user is asked, and the authorization
// request that the form posts back.
type consent struct {
	ClientName   string
	RedirectHost string
	// Unverified is the notice that this server has not verified the application; every
	// third-party application gets it (ADR-0090 §4).
	Unverified string
	// Origin says where the application comes from, by the route it was registered by.
	Origin string
	// Website is the home page the application declared, shown as text; "" when none.
	Website string
	// Impersonation warns that the answer goes to an address that another app may claim; ""
	// when the redirect URI is a claimed https URL or the client was registered by a user.
	Impersonation string
	// Scopes describes each requested scope.
	Scopes []consentScope

	ResponseType        string
	ClientID            string
	RedirectURI         string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
}

// consentScope is a requested permission and the cautions that go with it.
type consentScope struct {
	Text     string
	Cautions []string
}

// The form's answers to the consent page.
const (
	decisionApprove = "approve"
	decisionDeny    = "deny"
)

func authorizationRequest(v url.Values) appidentity.AuthorizationRequest {
	return appidentity.AuthorizationRequest{
		ResponseType:        v.Get("response_type"),
		ClientID:            v.Get("client_id"),
		RedirectURI:         v.Get("redirect_uri"),
		Scope:               v.Get("scope"),
		State:               v.Get("state"),
		CodeChallenge:       v.Get("code_challenge"),
		CodeChallengeMethod: v.Get("code_challenge_method"),
	}
}

// authorizeURL is the path of the authorization endpoint with req as its query, where the
// sign-in form returns to.
func authorizeURL(req appidentity.AuthorizationRequest) string {
	v := url.Values{}
	for k, s := range map[string]string{
		"response_type": req.ResponseType, "client_id": req.ClientID, "redirect_uri": req.RedirectURI, "scope": req.Scope,
		"state": req.State, "code_challenge": req.CodeChallenge, "code_challenge_method": req.CodeChallengeMethod,
	} {
		if s != "" {
			v.Set(k, s)
		}
	}
	return PathAuthorize + "?" + v.Encode()
}

// authorizePage answers an authorization request (RFC 6749 §4.1.1).
func (h *Handler) authorizePage(w http.ResponseWriter, r *http.Request) {
	h.handleAuthorization(w, r, authorizationRequest(r.URL.Query()), "")
}

// authorize handles the consent form.
func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return
	}
	if !h.validCSRF(r) {
		h.renderError(w, r, http.StatusForbidden, (*i18n.Localizer).ServerAuthFormExpired)
		return
	}
	decision := r.PostForm.Get("decision")
	if decision != decisionApprove && decision != decisionDeny {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return
	}
	h.handleAuthorization(w, r, authorizationRequest(r.PostForm), decision)
}

// handleAuthorization validates req, makes sure the browser is signed in, and then issues a
// code for a first-party client or an approved request, answers access_denied for a denied
// one, or shows the consent page.
func (h *Handler) handleAuthorization(w http.ResponseWriter, r *http.Request, req appidentity.AuthorizationRequest, decision string) {
	ctx := r.Context()
	a, err := h.o.OAuth.ValidateAuthorization(ctx, req)
	var oe *appidentity.OAuthError
	var limited *kernel.RateLimitedError
	switch {
	case errors.As(err, &limited):
		// The fetch of the client's metadata document is rate limited (ADR-0090 §7).
		w.Header().Set("Retry-After", strconv.Itoa(max(int(math.Ceil(limited.RetryAfter.Seconds())), 1)))
		h.renderError(w, r, http.StatusTooManyRequests, (*i18n.Localizer).ServerAuthAuthorizeRateLimited)
		return
	case errors.Is(err, appidentity.ErrUnknownClient), errors.Is(err, appidentity.ErrInvalidRedirectURI):
		// Never redirect to an unverified URI (RFC 6749 §4.1.2.1).
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerAuthAuthorizeInvalid)
		return
	case errors.As(err, &oe):
		h.redirectToClient(w, r, a, url.Values{"error": {oe.Code}, "error_description": {oe.Description}})
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	session, err := h.o.Identity.Authenticate(ctx, cookieValue(r, CookieSession))
	if errors.Is(err, appidentity.ErrInvalidSession) {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, PathSignIn+"?"+url.Values{"return_to": {authorizeURL(req)}}.Encode(), http.StatusSeeOther)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	access, err := h.o.Identity.UserAccess(ctx, session.UserID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if !access.Allowed() { // no tokens for an account that cannot be used yet
		h.renderStatus(w, r, http.StatusForbidden, session.UserID, access, nil, nil)
		return
	}
	switch {
	case a.Client.FirstParty() || decision == decisionApprove:
		a.SessionID = session.ID
		code, err := h.o.OAuth.IssueAuthorizationCode(ctx, a, session.UserID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		h.redirectToClient(w, r, a, url.Values{"code": {code}})
	case decision == decisionDeny:
		h.redirectToClient(w, r, a, url.Values{"error": {appidentity.OAuthAccessDenied}})
	default:
		h.renderConsent(w, r, a, req)
	}
}

// redirectToClient answers the client at the validated redirect URI, keeping the URI's own
// query (RFC 6749 §3.1.2) and adding the state and the issuer (RFC 9207). 303 makes the
// browser follow it with GET, also after the consent form (RFC 9700 §4.12).
func (h *Handler) redirectToClient(w http.ResponseWriter, r *http.Request, a appidentity.Authorization, params url.Values) {
	u, err := url.Parse(a.RedirectURI)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	q := u.Query()
	for k, v := range params {
		if len(v) > 0 && v[0] != "" {
			q[k] = v
		}
	}
	if a.State != "" {
		q.Set("state", a.State)
	}
	if h.o.Issuer != "" {
		q.Set("iss", h.o.Issuer)
	}
	u.RawQuery = q.Encode()
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

func (h *Handler) renderConsent(w http.ResponseWriter, r *http.Request, a appidentity.Authorization, req appidentity.AuthorizationRequest) {
	name := a.Client.Name
	p := h.newPage(r, func(l *i18n.Localizer) string { return l.ServerAuthAuthorizeTitle(name) })
	p.CSRFToken = h.csrfToken(w, r)
	u, err := url.Parse(a.RedirectURI)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	c := &consent{
		ClientName: name, RedirectHost: u.Host,
		ResponseType: req.ResponseType, ClientID: req.ClientID, RedirectURI: a.RedirectURI, Scope: string(a.Scope),
		State: a.State, CodeChallenge: a.CodeChallenge, CodeChallengeMethod: req.CodeChallengeMethod,
	}
	if c.RedirectHost == "" { // a private-use URI scheme of a native app (RFC 8252 §7.1)
		c.RedirectHost = u.Scheme
	}
	c.Unverified = p.L.ServerAuthAuthorizeUnverified()
	c.Origin = clientOrigin(p.L, a)
	if w, err := url.Parse(a.Client.Website); err == nil && (w.Scheme == "https" || w.Scheme == "http") && w.Host != "" {
		c.Website = p.L.ServerAuthAuthorizeWebsite(a.Client.Website)
	}
	if impersonable(a.Client, u) {
		c.Impersonation = p.L.ServerAuthAuthorizeImpersonation(c.RedirectHost)
	}
	for _, s := range a.Scope.Tokens() {
		cs := consentScope{Text: scopeDescription(p.L, s)}
		for _, caution := range scopeCautions[s] {
			cs.Cautions = append(cs.Cautions, caution(p.L))
		}
		c.Scopes = append(c.Scopes, cs)
	}
	p.Consent = c
	p.formAction = []string{redirectSource(u)}
	h.render(w, r, http.StatusOK, pageAuthorize, p)
}

// clientOrigin words where the client comes from, by the route it was registered by
// (ADR-0090 §4): the host of the client ID URL is verified, the name behind a dynamic
// registration is the application's own claim.
func clientOrigin(l *i18n.Localizer, a appidentity.Authorization) string {
	switch a.Client.Registration {
	case identity.ClientRegisteredByMetadataDocument:
		host := a.Client.ClientID
		if u, err := url.Parse(a.Client.ClientID); err == nil && u.Host != "" {
			host = u.Host
		}
		return l.ServerAuthAuthorizeOriginMetadataDocument(host)
	case identity.ClientRegisteredByUser:
		if a.OwnerUsername == "" {
			return l.ServerAuthAuthorizeOriginUserUnknown()
		}
		return l.ServerAuthAuthorizeOriginUser("@" + a.OwnerUsername)
	default:
		return l.ServerAuthAuthorizeOriginDynamic()
	}
}

// impersonable reports whether another app could receive the code meant for the client: the
// client came from a metadata document or a dynamic registration, where anyone can claim any
// name, and its redirect URI is not an https URL, which the operating system ties to one app
// (ADR-0090 §3). A loopback URL and a private-use scheme are the cases.
func impersonable(c identity.OAuthClient, redirect *url.URL) bool {
	switch c.Registration {
	case identity.ClientRegisteredByMetadataDocument, identity.ClientRegisteredDynamically:
		return redirect.Scheme != "https"
	}
	return false
}

// scopeCautions are the cautions shown with a scope: reading direct messages, writing posts,
// and changing the profile (ADR-0090 §4).
var scopeCautions = map[string][]func(*i18n.Localizer) string{
	identity.ScopeRead:          {(*i18n.Localizer).ServerAuthAuthorizeCautionDm},
	identity.ScopeReadDirect:    {(*i18n.Localizer).ServerAuthAuthorizeCautionDm},
	identity.ScopeWrite:         {(*i18n.Localizer).ServerAuthAuthorizeCautionPost, (*i18n.Localizer).ServerAuthAuthorizeCautionProfile},
	identity.ScopeWriteStatuses: {(*i18n.Localizer).ServerAuthAuthorizeCautionPost},
	identity.ScopeWriteAccounts: {(*i18n.Localizer).ServerAuthAuthorizeCautionProfile},
}

// redirectSource is the CSP source expression that matches the redirect URI u.
func redirectSource(u *url.URL) string {
	if u.Host == "" {
		return u.Scheme + ":"
	}
	return u.Scheme + "://" + u.Host
}

// scopeDescriptions word each scope on the consent screen; TestScopeDescriptions checks that
// every scope the server knows has one.
var scopeDescriptions = map[string]func(*i18n.Localizer) string{
	identity.ScopeAdmin:              (*i18n.Localizer).ServerAuthAuthorizeScopeAdmin,
	identity.ScopeAdminRead:          (*i18n.Localizer).ServerAuthAuthorizeScopeAdminRead,
	identity.ScopeAdminWrite:         (*i18n.Localizer).ServerAuthAuthorizeScopeAdminWrite,
	identity.ScopeRead:               (*i18n.Localizer).ServerAuthAuthorizeScopeRead,
	identity.ScopeWrite:              (*i18n.Localizer).ServerAuthAuthorizeScopeWrite,
	identity.ScopeFollow:             (*i18n.Localizer).ServerAuthAuthorizeScopeFollow,
	identity.ScopePush:               (*i18n.Localizer).ServerAuthAuthorizeScopePush,
	identity.ScopeReadAccounts:       (*i18n.Localizer).ServerAuthAuthorizeScopeReadAccounts,
	identity.ScopeReadBlocks:         (*i18n.Localizer).ServerAuthAuthorizeScopeReadBlocks,
	identity.ScopeReadBookmarks:      (*i18n.Localizer).ServerAuthAuthorizeScopeReadBookmarks,
	identity.ScopeReadCustomEmojis:   (*i18n.Localizer).ServerAuthAuthorizeScopeReadCustomEmojis,
	identity.ScopeReadDirect:         (*i18n.Localizer).ServerAuthAuthorizeScopeReadDirect,
	identity.ScopeReadFavourites:     (*i18n.Localizer).ServerAuthAuthorizeScopeReadFavourites,
	identity.ScopeReadFinance:        (*i18n.Localizer).ServerAuthAuthorizeScopeReadFinance,
	identity.ScopeReadFollows:        (*i18n.Localizer).ServerAuthAuthorizeScopeReadFollows,
	identity.ScopeReadMutes:          (*i18n.Localizer).ServerAuthAuthorizeScopeReadMutes,
	identity.ScopeReadNotifications:  (*i18n.Localizer).ServerAuthAuthorizeScopeReadNotifications,
	identity.ScopeReadReports:        (*i18n.Localizer).ServerAuthAuthorizeScopeReadReports,
	identity.ScopeReadStatuses:       (*i18n.Localizer).ServerAuthAuthorizeScopeReadStatuses,
	identity.ScopeReadSupporters:     (*i18n.Localizer).ServerAuthAuthorizeScopeReadSupporters,
	identity.ScopeWriteAccounts:      (*i18n.Localizer).ServerAuthAuthorizeScopeWriteAccounts,
	identity.ScopeWriteBlocks:        (*i18n.Localizer).ServerAuthAuthorizeScopeWriteBlocks,
	identity.ScopeWriteBookmarks:     (*i18n.Localizer).ServerAuthAuthorizeScopeWriteBookmarks,
	identity.ScopeWriteCustomEmojis:  (*i18n.Localizer).ServerAuthAuthorizeScopeWriteCustomEmojis,
	identity.ScopeWriteFavourites:    (*i18n.Localizer).ServerAuthAuthorizeScopeWriteFavourites,
	identity.ScopeWriteFollows:       (*i18n.Localizer).ServerAuthAuthorizeScopeWriteFollows,
	identity.ScopeWriteMedia:         (*i18n.Localizer).ServerAuthAuthorizeScopeWriteMedia,
	identity.ScopeWriteMutes:         (*i18n.Localizer).ServerAuthAuthorizeScopeWriteMutes,
	identity.ScopeWriteNotifications: (*i18n.Localizer).ServerAuthAuthorizeScopeWriteNotifications,
	identity.ScopeWriteReports:       (*i18n.Localizer).ServerAuthAuthorizeScopeWriteReports,
	identity.ScopeWriteStatuses:      (*i18n.Localizer).ServerAuthAuthorizeScopeWriteStatuses,
	identity.ScopeWriteSupporters:    (*i18n.Localizer).ServerAuthAuthorizeScopeWriteSupporters,
}

func scopeDescription(l *i18n.Localizer, scope string) string {
	if describe, ok := scopeDescriptions[scope]; ok {
		return describe(l)
	}
	return scope
}
