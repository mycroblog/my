// SPDX-License-Identifier: AGPL-3.0-only

package oauth

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
)

// registerCORS lets any origin call the registration endpoint, which takes no credentials, so
// that browser-based clients can register themselves. next nil answers a preflight.
func registerCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Access-Control-Allow-Origin", "*")
		if next == nil {
			hd.Set("Access-Control-Allow-Methods", "POST")
			hd.Set("Access-Control-Allow-Headers", "Content-Type")
			hd.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// registrationRequest is the JSON body of a registration request (RFC 7591 §3.1). Metadata the
// server does not know are ignored (§2).
type registrationRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri"`
	LogoURI                 string   `json:"logo_uri"`
	Scope                   string   `json:"scope"`
	SoftwareStatement       string   `json:"software_statement"`
}

// registrationResponse is the client information response (RFC 7591 §3.2.1). It carries no
// client_secret and no registration access token: the client is public and RFC 7592 is not
// supported.
type registrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	Scope                   string   `json:"scope"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// register serves dynamic client registration (RFC 7591): no authentication, public clients
// only.
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	if h.limited(w, r, limitKey{RegisterIPLimit, kernel.ClientKey(kernel.RequestClientIP(r))}, limitKey{RegisterLimit, "instance"}) {
		return
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, http.StatusBadRequest, &appidentity.OAuthError{Code: appidentity.OAuthInvalidClientMetadata, Description: "the body must be application/json"})
		return
	}
	var req registrationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxFormBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, &appidentity.OAuthError{Code: appidentity.OAuthInvalidClientMetadata, Description: "the body is not a JSON object of client metadata"})
		return
	}
	reg, err := h.o.Identity.RegisterDynamicClient(r.Context(), appidentity.DynamicRegistrationRequest{
		RedirectURIs: req.RedirectURIs, TokenEndpointAuthMethod: req.TokenEndpointAuthMethod, GrantTypes: req.GrantTypes,
		ResponseTypes: req.ResponseTypes, ClientName: req.ClientName, ClientURI: req.ClientURI, LogoURI: req.LogoURI,
		Scope: req.Scope, SoftwareStatement: req.SoftwareStatement,
	})
	if errors.Is(err, appidentity.ErrDynamicRegistrationDisabled) {
		writeError(w, http.StatusForbidden, &appidentity.OAuthError{Code: appidentity.OAuthAccessDenied, Description: "dynamic client registration is disabled"})
		return
	}
	if err != nil {
		h.fail(w, r, err, false)
		return
	}
	c := reg.Client
	writeJSON(w, http.StatusCreated, registrationResponse{
		ClientID: c.ClientID, ClientIDIssuedAt: c.CreatedAt.Unix(), ClientName: c.Name, ClientURI: c.Website,
		RedirectURIs: reg.RedirectURIs, Scope: string(c.Scope), GrantTypes: reg.GrantTypes, ResponseTypes: reg.ResponseTypes,
		TokenEndpointAuthMethod: reg.TokenEndpointAuthMethod,
	})
}
