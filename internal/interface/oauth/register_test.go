// SPDX-License-Identifier: AGPL-3.0-only

package oauth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
)

func (f *fakeIdentity) RegisterDynamicClient(_ context.Context, req appidentity.DynamicRegistrationRequest) (appidentity.DynamicRegistration, error) {
	f.registrations = append(f.registrations, req)
	if f.registerErr != nil {
		return appidentity.DynamicRegistration{}, f.registerErr
	}
	return appidentity.DynamicRegistration{
		Client: identity.OAuthClient{
			ClientID: "new-client", Name: "App", Scope: "read", CreatedAt: time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC),
		},
		RedirectURIs: req.RedirectURIs, GrantTypes: []string{"authorization_code"}, ResponseTypes: []string{"code"},
		TokenEndpointAuthMethod: "none",
	}, nil
}

func postJSON(h http.Handler, body, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "https://social.example/oauth/register", strings.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("Origin", "https://app.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestRegisterReturnsTheClientInformationResponse(t *testing.T) {
	id := &fakeIdentity{}
	w := postJSON(newServer(t, id), `{"redirect_uris":["https://app.example/cb"],"client_name":"App","unknown":1}`, "application/json; charset=utf-8")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d %s, want 201", w.Code, w.Body.String())
	}
	got := decode(t, w)
	if got["client_id"] != "new-client" || got["token_endpoint_auth_method"] != "none" {
		t.Errorf("response = %v", got)
	}
	if _, ok := got["client_secret"]; ok {
		t.Errorf("a public client got a client_secret: %v", got)
	}
	if got["client_id_issued_at"] != float64(time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC).Unix()) {
		t.Errorf("client_id_issued_at = %v", got["client_id_issued_at"])
	}
	if len(id.registrations) != 1 || id.registrations[0].ClientName != "App" {
		t.Errorf("registrations = %+v", id.registrations)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Errorf("CORS headers = %v", w.Header())
	}
}

func TestRegisterRefusals(t *testing.T) {
	for _, c := range []struct {
		name, body, contentType string
		err                     error
		status                  int
		code                    string
	}{
		{"not json content type", `redirect_uris=x`, "application/x-www-form-urlencoded", nil, http.StatusBadRequest, "invalid_client_metadata"},
		{"malformed json", `{`, "application/json", nil, http.StatusBadRequest, "invalid_client_metadata"},
		{"trailing value", `{} {}`, "application/json", nil, http.StatusBadRequest, "invalid_client_metadata"},
		{"trailing garbage", `{}x`, "application/json", nil, http.StatusBadRequest, "invalid_client_metadata"},
		{"oversized body", `{}` + strings.Repeat(" ", 16<<10), "application/json", nil, http.StatusBadRequest, "invalid_client_metadata"},
		{"wrong type", `{"redirect_uris":"https://app.example/cb"}`, "application/json", nil, http.StatusBadRequest, "invalid_client_metadata"},
		{"invalid metadata", `{}`, "application/json", &appidentity.OAuthError{Code: appidentity.OAuthInvalidClientMetadata}, http.StatusBadRequest, "invalid_client_metadata"},
		{"invalid redirect uri", `{}`, "application/json", &appidentity.OAuthError{Code: appidentity.OAuthInvalidRedirectURI}, http.StatusBadRequest, "invalid_redirect_uri"},
		{"disabled", `{}`, "application/json", appidentity.ErrDynamicRegistrationDisabled, http.StatusForbidden, "access_denied"},
	} {
		t.Run(c.name, func(t *testing.T) {
			wantError(t, postJSON(newServer(t, &fakeIdentity{registerErr: c.err}), c.body, c.contentType), c.status, c.code, c.name)
		})
	}
}

func TestRegisterPreflightAllowsAnyOriginWithoutCredentials(t *testing.T) {
	r := httptest.NewRequest(http.MethodOptions, "https://social.example/oauth/register", nil)
	r.Header.Set("Origin", "https://app.example")
	w := httptest.NewRecorder()
	newServer(t, &fakeIdentity{}).ServeHTTP(w, r)
	if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Origin") != "*" ||
		w.Header().Get("Access-Control-Allow-Credentials") != "" || w.Header().Get("Access-Control-Allow-Headers") != "Content-Type" {
		t.Errorf("preflight = %d %v", w.Code, w.Header())
	}
}
