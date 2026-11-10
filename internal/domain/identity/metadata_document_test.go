// SPDX-License-Identifier: AGPL-3.0-only

package identity_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/identity"
)

func TestParseMetadataDocumentClientID(t *testing.T) {
	for id, want := range map[string]string{
		"https://app.example/client.json":                  "app.example",
		"https://App.Example:8443/oauth/client":            "app.example",
		"https://[2001:db8::1]/client.json":                "2001:db8::1",
		"https://app.example/client.json?v=1":              "app.example",
		"http://app.example/client.json":                   "",
		"https://app.example":                              "",
		"https://app.example/":                             "",
		"https://app.example/a/../client.json":             "",
		"https://app.example/a/./client.json":              "",
		"https://app.example/a/%2E%2E/client":              "",
		"https://user@app.example/client.json":             "",
		"https://app.example/client.json#x":                "",
		"https:///client.json":                             "",
		"https://app.example/" + strings.Repeat("a", 2048): "",
	} {
		host, err := identity.ParseMetadataDocumentClientID(id)
		if want == "" {
			if !errors.Is(err, identity.ErrInvalidClientIDURL) {
				t.Errorf("ParseMetadataDocumentClientID(%q) = %q, %v; want ErrInvalidClientIDURL", id, host, err)
			}
			continue
		}
		if err != nil || host != want {
			t.Errorf("ParseMetadataDocumentClientID(%q) = %q, %v; want %q", id, host, err, want)
		}
	}
}

func TestClientHost(t *testing.T) {
	for in, want := range map[string]string{
		"App.Example": "app.example", "[::1]": "::1", "127.0.0.1": "127.0.0.1",
		"": "", "app.example:443": "", "a/b": "", "a@b": "",
	} {
		got, err := identity.ClientHost(in)
		if (want == "") != (err != nil) || got != want {
			t.Errorf("ClientHost(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestParseClientMetadataDocument(t *testing.T) {
	const id = "https://app.example/client.json"
	m, err := identity.ParseClientMetadataDocument(id, []byte(`{
		"client_id": "https://app.example/client.json", "client_name": " Pocket ", "client_uri": "https://app.example/",
		"logo_uri": "javascript:alert(1)", "scope": "read write:statuses admin write:supporters admin:manage_invites unknown",
		"redirect_uris": ["com.example.app:/cb", "http://127.0.0.1/cb", "com.example.app:/cb"],
		"token_endpoint_auth_method": "none", "grant_types": ["authorization_code", "refresh_token"], "response_types": ["code"]
	}`))
	if err != nil {
		t.Fatalf("ParseClientMetadataDocument = %v", err)
	}
	if m.Name != "Pocket" || m.Website != "https://app.example/" || m.LogoURI != "" || m.Scope != "read write:statuses" ||
		strings.Join(m.RedirectURIs, " ") != "com.example.app:/cb http://127.0.0.1/cb" {
		t.Errorf("metadata = %+v", m)
	}
	m, err = identity.ParseClientMetadataDocument(id, []byte(`{"client_id":"`+id+`","redirect_uris":["https://app.example/cb"]}`))
	if err != nil || m.Name != "app.example" || m.Scope != identity.MetadataDocumentDefaultScope() || m.Scope.HasFirstPartyOnly() {
		t.Errorf("minimal document = %+v, %v; want the host as the name and the default scope", m, err)
	}

	redirect := `"redirect_uris":["https://app.example/cb"]`
	for what, doc := range map[string]string{
		"not an object":         `[]`,
		"null":                  `null`,
		"another client_id":     `{"client_id":"https://app.example/other.json",` + redirect + `}`,
		"a trailing slash":      `{"client_id":"https://app.example/client.json/",` + redirect + `}`,
		"no client_id":          `{` + redirect + `}`,
		"a client secret":       `{"client_id":"` + id + `","client_secret":"x",` + redirect + `}`,
		"a secret expiry":       `{"client_id":"` + id + `","client_secret_expires_at":0,` + redirect + `}`,
		"client_secret_post":    `{"client_id":"` + id + `","token_endpoint_auth_method":"client_secret_post",` + redirect + `}`,
		"private_key_jwt":       `{"client_id":"` + id + `","token_endpoint_auth_method":"private_key_jwt",` + redirect + `}`,
		"no redirect URIs":      `{"client_id":"` + id + `"}`,
		"a bad redirect URI":    `{"client_id":"` + id + `","redirect_uris":["http://localhost/cb"]}`,
		"no code grant":         `{"client_id":"` + id + `","grant_types":["client_credentials"],` + redirect + `}`,
		"no code response type": `{"client_id":"` + id + `","response_types":["token"],` + redirect + `}`,
		"a long name":           `{"client_id":"` + id + `","client_name":"` + strings.Repeat("x", 201) + `",` + redirect + `}`,
		"a wrongly typed name":  `{"client_id":"` + id + `","client_name":1,` + redirect + `}`,
	} {
		if _, err := identity.ParseClientMetadataDocument(id, []byte(doc)); !errors.Is(err, identity.ErrInvalidClientMetadata) {
			t.Errorf("ParseClientMetadataDocument(%s) = %v, want ErrInvalidClientMetadata", what, err)
		}
	}
}
