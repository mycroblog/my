// SPDX-License-Identifier: AGPL-3.0-only

package oauth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func getMetadata(t *testing.T, id *fakeIdentity, path string) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	newServer(t, id).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://social.example"+path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s, want 200", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Errorf("Cache-Control = %q", got)
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return m
}

func stringList(v any) []string {
	list, _ := v.([]any)
	out := make([]string, len(list))
	for i, e := range list {
		out[i], _ = e.(string)
	}
	return out
}

func TestAuthorizationServerMetadata(t *testing.T) {
	got := getMetadata(t, &fakeIdentity{dynamic: true, cimd: true}, "/.well-known/oauth-authorization-server")
	for k, want := range map[string]string{ //nolint:gosec // G101: URLs, not credentials
		"issuer": "https://social.example", "authorization_endpoint": "https://social.example/oauth/authorize",
		"token_endpoint":        "https://social.example/oauth/token",
		"revocation_endpoint":   "https://social.example/oauth/revoke",
		"registration_endpoint": "https://social.example/oauth/register",
	} {
		if got[k] != want {
			t.Errorf("%s = %v, want %s", k, got[k], want)
		}
	}
	if got["client_id_metadata_document_supported"] != true {
		t.Errorf("client_id_metadata_document_supported = %v", got["client_id_metadata_document_supported"])
	}
	for k, want := range map[string][]string{
		"response_types_supported":              {"code"},
		"grant_types_supported":                 {"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      {"S256"},
		"token_endpoint_auth_methods_supported": {"none", "client_secret_basic", "client_secret_post"},
	} {
		if g := stringList(got[k]); !slices.Equal(g, want) {
			t.Errorf("%s = %v, want %v", k, g, want)
		}
	}
	scopes := stringList(got["scopes_supported"])
	if !slices.Contains(scopes, "read:statuses") || !slices.Contains(scopes, "read:direct") {
		t.Errorf("scopes_supported = %v", scopes)
	}
	for _, s := range []string{"admin", "admin:read", "admin:write", "write:supporters"} {
		if slices.Contains(scopes, s) {
			t.Errorf("scopes_supported has the first-party-only scope %s", s)
		}
	}
}

func TestAuthorizationServerMetadataFollowsTheSettings(t *testing.T) {
	got := getMetadata(t, &fakeIdentity{}, "/.well-known/oauth-authorization-server")
	if _, ok := got["registration_endpoint"]; ok {
		t.Errorf("registration_endpoint is set while dynamic registration is off: %v", got)
	}
	if _, ok := got["client_id_metadata_document_supported"]; ok {
		t.Errorf("client_id_metadata_document_supported is set while CIMD is off: %v", got)
	}
	got = getMetadata(t, &fakeIdentity{dynamic: true}, "/.well-known/oauth-authorization-server")
	if got["registration_endpoint"] == nil || got["client_id_metadata_document_supported"] != nil {
		t.Errorf("only dynamic registration is on: %v", got)
	}
	got = getMetadata(t, &fakeIdentity{cimd: true}, "/.well-known/oauth-authorization-server")
	if got["registration_endpoint"] != nil || got["client_id_metadata_document_supported"] != true {
		t.Errorf("only CIMD is on: %v", got)
	}
}

func TestProtectedResourceMetadata(t *testing.T) {
	got := getMetadata(t, &fakeIdentity{}, "/.well-known/oauth-protected-resource")
	if got["resource"] != "https://social.example/api/v1" {
		t.Errorf("resource = %v", got["resource"])
	}
	if s := stringList(got["authorization_servers"]); !slices.Equal(s, []string{"https://social.example"}) {
		t.Errorf("authorization_servers = %v", s)
	}
	if s := stringList(got["scopes_supported"]); slices.Contains(s, "admin") || slices.Contains(s, "write:supporters") || !slices.Contains(s, "read") {
		t.Errorf("scopes_supported = %v", s)
	}
}
