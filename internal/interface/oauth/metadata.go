// SPDX-License-Identifier: AGPL-3.0-only

package oauth

import (
	"encoding/json"
	"net/http"
	"slices"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
)

// metadataCacheControl lets caches keep the metadata for a few minutes: it is anonymous and the
// same for everyone, and changes only when the administrator turns a registration path on or
// off (ADR-0016).
const metadataCacheControl = "public, max-age=300"

// authorizationServerMetadata is the document of RFC 8414 §2, with the Client ID Metadata
// Document extension (draft-ietf-oauth-client-id-metadata-document §6). OpenID Connect
// discovery is not served (ADR-0090 §6).
type authorizationServerMetadata struct {
	Issuer                                 string   `json:"issuer"`
	AuthorizationEndpoint                  string   `json:"authorization_endpoint"`
	TokenEndpoint                          string   `json:"token_endpoint"`
	RegistrationEndpoint                   string   `json:"registration_endpoint,omitempty"`
	RevocationEndpoint                     string   `json:"revocation_endpoint"`
	ScopesSupported                        []string `json:"scopes_supported"`
	ResponseTypesSupported                 []string `json:"response_types_supported"`
	GrantTypesSupported                    []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported          []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported      []string `json:"token_endpoint_auth_methods_supported"`
	RevocationEndpointAuthMethodsSupported []string `json:"revocation_endpoint_auth_methods_supported"`
	ClientIDMetadataDocumentSupported      bool     `json:"client_id_metadata_document_supported,omitempty"`
}

// protectedResourceMetadata is the document of RFC 9728 §2 for the client API.
type protectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
}

// thirdPartyScopes returns the scopes that a third-party client may hold: every known scope
// except the ones that only first-party clients may hold (ADR-0090 §4).
func thirdPartyScopes() []string {
	return slices.DeleteFunc(identity.KnownScopes(), identity.FirstPartyOnlyScope)
}

func writeMetadata(w http.ResponseWriter, doc any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", metadataCacheControl)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(doc)
}

// authorizationServerMetadata serves RFC 8414 metadata that follows the instance settings.
func (h *Handler) authorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	dynamic, err := h.o.Identity.DynamicClientRegistrationEnabled(r.Context())
	if err != nil {
		h.o.Logger.ErrorContext(r.Context(), "oauth: read the dynamic registration setting", "error", err)
		writeError(w, http.StatusInternalServerError, &appidentity.OAuthError{Code: "server_error"})
		return
	}
	cimd, err := h.o.Identity.MetadataDocumentClientsEnabled(r.Context())
	if err != nil {
		h.o.Logger.ErrorContext(r.Context(), "oauth: read the metadata document setting", "error", err)
		writeError(w, http.StatusInternalServerError, &appidentity.OAuthError{Code: "server_error"})
		return
	}
	authMethods := []string{"none", "client_secret_basic", "client_secret_post"}
	doc := authorizationServerMetadata{
		Issuer:                                 h.o.Origin,
		AuthorizationEndpoint:                  h.o.Origin + PathAuthorize,
		TokenEndpoint:                          h.o.Origin + PathToken,
		RevocationEndpoint:                     h.o.Origin + PathRevoke,
		ScopesSupported:                        thirdPartyScopes(),
		ResponseTypesSupported:                 []string{"code"},
		GrantTypesSupported:                    []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:          []string{"S256"},
		TokenEndpointAuthMethodsSupported:      authMethods,
		RevocationEndpointAuthMethodsSupported: authMethods,
		ClientIDMetadataDocumentSupported:      cimd,
	}
	if dynamic {
		doc.RegistrationEndpoint = h.o.Origin + PathRegister
	}
	writeMetadata(w, doc)
}

// protectedResourceMetadata serves RFC 9728 metadata: the client API is protected by the
// bearer tokens of this server's own authorization server.
func (h *Handler) protectedResourceMetadata(w http.ResponseWriter, _ *http.Request) {
	writeMetadata(w, protectedResourceMetadata{
		Resource:               h.o.Origin + "/api/v1",
		AuthorizationServers:   []string{h.o.Origin},
		ScopesSupported:        thirdPartyScopes(),
		BearerMethodsSupported: []string{"header"},
	})
}
