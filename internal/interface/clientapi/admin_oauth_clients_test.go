// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const (
	oauthClientWeb   = "01890a5d-ac96-774b-bcce-b302099a8101"
	oauthClientCIMD  = "01890a5d-ac96-774b-bcce-b302099a8102"
	oauthClientUser  = "01890a5d-ac96-774b-bcce-b302099a8103"
	oauthClientGone  = "01890a5d-ac96-774b-bcce-b302099a8999"
	oauthDenialFirst = "01890a5d-ac96-774b-bcce-b302099a8201"
)

// fakeOAuthClients keeps clients newest first with the semantics of appidentity.Service.
type fakeOAuthClients struct {
	clients []identity.OAuthClientListing
	denials []identity.OAuthClientHostDenial
	calls   []string
}

func (f *fakeOAuthClients) ListOAuthClients(_ context.Context, registration identity.OAuthClientRegistration, before string, limit int) ([]identity.OAuthClientListing, error) {
	var out []identity.OAuthClientListing
	for _, c := range f.clients {
		if (registration == "" || c.Registration == registration) && (before == "" || c.ID < before) && len(out) < limit {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeOAuthClients) find(id string) (int, error) {
	i := slices.IndexFunc(f.clients, func(c identity.OAuthClientListing) bool { return c.ID == id })
	switch {
	case i < 0:
		return 0, appidentity.ErrNotFound
	case f.clients[i].FirstParty():
		return 0, appidentity.ErrFirstPartyClient
	}
	return i, nil
}

func (f *fakeOAuthClients) FindOAuthClientListing(_ context.Context, id string) (identity.OAuthClientListing, error) {
	i := slices.IndexFunc(f.clients, func(c identity.OAuthClientListing) bool { return c.ID == id })
	if i < 0 {
		return identity.OAuthClientListing{}, appidentity.ErrNotFound
	}
	return f.clients[i], nil
}

func (f *fakeOAuthClients) DisableOAuthClient(_ context.Context, id, by string) error {
	i, err := f.find(id)
	if err == nil {
		f.clients[i].Disabled = true
		f.calls = append(f.calls, "disable "+id+" by "+by)
	}
	return err
}

func (f *fakeOAuthClients) EnableOAuthClient(_ context.Context, id, by string) error {
	i, err := f.find(id)
	if err == nil {
		f.clients[i].Disabled = false
		f.calls = append(f.calls, "enable "+id+" by "+by)
	}
	return err
}

func (f *fakeOAuthClients) ListOAuthClientHostDenials(_ context.Context, before string, limit int) ([]identity.OAuthClientHostDenial, error) {
	var out []identity.OAuthClientHostDenial
	for _, d := range f.denials {
		if (before == "" || d.ID < before) && len(out) < limit {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeOAuthClients) DenyOAuthClientHost(_ context.Context, host, by string) error {
	h, err := identity.ClientHost(host)
	if err == nil {
		f.calls = append(f.calls, "deny "+h+" by "+by)
	}
	return err
}

func (f *fakeOAuthClients) AllowOAuthClientHost(_ context.Context, host, by string) error {
	h, err := identity.ClientHost(host)
	if err == nil {
		f.calls = append(f.calls, "allow "+h+" by "+by)
	}
	return err
}

// newOAuthClientsEnv is newAuthEnv with the tokens of newAdminEnv and the OAuth clients: alice
// holds every permission and bob none.
func newOAuthClientsEnv(t *testing.T) (*authEnv, *fakeOAuthClients) {
	t.Helper()
	f := &fakeOAuthClients{
		clients: []identity.OAuthClientListing{
			{OAuthClient: identity.OAuthClient{
				ID: oauthClientUser, ClientID: "bot", Name: "Bot", Registration: identity.ClientRegisteredByUser,
				OwnerUserID: userOf(aliceAccountID), SecretHash: []byte("x"), Scope: "read", CreatedAt: adminNow,
			}},
			{OAuthClient: identity.OAuthClient{
				ID: oauthClientCIMD, ClientID: "https://app.example/client.json", Name: "App", Website: "https://app.example/",
				Registration: identity.ClientRegisteredByMetadataDocument, Scope: "read write", CreatedAt: adminNow,
			}},
			{OAuthClient: identity.OAuthClient{
				ID: oauthClientWeb, ClientID: "web", Name: "Web", Registration: identity.ClientRegisteredFirstParty, Scope: "read", CreatedAt: adminNow,
			}},
		},
		denials: []identity.OAuthClientHostDenial{
			{ID: oauthDenialFirst, Host: "bad.example", DeniedByUserID: userOf(aliceAccountID), DeniedAt: adminNow},
		},
	}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.OAuthClients = f
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, f
}

func TestOAuthClientOperationsNeedAPermission(t *testing.T) {
	e, f := newOAuthClientsEnv(t)
	for _, op := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/oauth_clients", ""},
		{http.MethodPost, "/admin/oauth_clients/" + oauthClientCIMD + "/disable", ""},
		{http.MethodPost, "/admin/oauth_clients/" + oauthClientCIMD + "/enable", ""},
		{http.MethodGet, "/admin/oauth_client_host_denials", ""},
		{http.MethodPost, "/admin/oauth_client_host_denials", `{"host":"app.example"}`},
		{http.MethodDelete, "/admin/oauth_client_host_denials/bad.example", ""},
	} {
		res := e.send(t, op.method, op.path, "bob-admin", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_oauth_apps") {
			t.Errorf("%s %s by bob = %d %+v, want 403 for manage_oauth_apps", op.method, op.path, res.StatusCode, p)
		}
	}
	if len(f.calls) != 0 || slices.ContainsFunc(f.clients, func(c identity.OAuthClientListing) bool { return c.Disabled }) {
		t.Errorf("a refused operation changed something: %v", f.calls)
	}
}

func TestListOAuthClients(t *testing.T) {
	e, _ := newOAuthClientsEnv(t)
	res := e.admin(t, http.MethodGet, "/admin/oauth_clients?limit=2", "admin-read", nil)
	page := decodeJSON[clientapi.AdminOAuthClientPage](t, res)
	if res.StatusCode != http.StatusOK || len(page.Items) != 2 || page.Items[0].Id != oauthClientUser || page.NextCursor == nil {
		t.Fatalf("first page = %d %+v", res.StatusCode, page)
	}
	if c := page.Items[0]; !c.Confidential || c.OwnerId == nil || *c.OwnerId != userOf(aliceAccountID) || c.Registration != clientapi.OAuthClientRegistrationUser {
		t.Errorf("user client = %+v", c)
	}
	if c := page.Items[1]; c.Confidential || c.Website == nil || c.OwnerId != nil || c.Registration != clientapi.OAuthClientRegistrationMetadataDocument {
		t.Errorf("metadata document client = %+v", c)
	}
	page = decodeJSON[clientapi.AdminOAuthClientPage](t, e.admin(t, http.MethodGet, "/admin/oauth_clients?limit=2&cursor="+*page.NextCursor, "admin-read", nil))
	if len(page.Items) != 1 || page.Items[0].Id != oauthClientWeb || page.NextCursor != nil {
		t.Errorf("second page = %+v, want the first-party client and no cursor", page)
	}
	page = decodeJSON[clientapi.AdminOAuthClientPage](t, e.admin(t, http.MethodGet, "/admin/oauth_clients?registration=metadata_document", "admin-read", nil))
	if len(page.Items) != 1 || page.Items[0].Id != oauthClientCIMD {
		t.Errorf("filtered page = %+v, want the metadata document client", page)
	}
	if res := e.admin(t, http.MethodGet, "/admin/oauth_clients?registration=other", "admin-read", nil); res.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown registration = %d, want 400", res.StatusCode)
	}
}

func TestDisableAndEnableOAuthClients(t *testing.T) {
	e, f := newOAuthClientsEnv(t)
	res := e.send(t, http.MethodPost, "/admin/oauth_clients/"+oauthClientCIMD+"/disable", "admin", "")
	if c := decodeJSON[clientapi.AdminOAuthClient](t, res); res.StatusCode != http.StatusOK || !c.Disabled || c.Id != oauthClientCIMD {
		t.Fatalf("disable = %d %+v, want the disabled client", res.StatusCode, c)
	}
	res = e.send(t, http.MethodPost, "/admin/oauth_clients/"+oauthClientCIMD+"/enable", "admin", "")
	if c := decodeJSON[clientapi.AdminOAuthClient](t, res); res.StatusCode != http.StatusOK || c.Disabled {
		t.Fatalf("enable = %d %+v, want the client enabled", res.StatusCode, c)
	}
	want := []string{"disable " + oauthClientCIMD + " by " + userOf(aliceAccountID), "enable " + oauthClientCIMD + " by " + userOf(aliceAccountID)}
	if !slices.Equal(f.calls, want) {
		t.Errorf("calls = %v, want %v", f.calls, want)
	}
	// A first-party client cannot be disabled; an unknown client is not found.
	if res := e.send(t, http.MethodPost, "/admin/oauth_clients/"+oauthClientWeb+"/disable", "admin", ""); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("disable first-party = %d, want 422", res.StatusCode)
	}
	if res := e.send(t, http.MethodPost, "/admin/oauth_clients/"+oauthClientGone+"/disable", "admin", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("disable unknown = %d, want 404", res.StatusCode)
	}
	if res := e.send(t, http.MethodPost, "/admin/oauth_clients/"+oauthClientGone+"/enable", "admin", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("enable unknown = %d, want 404", res.StatusCode)
	}
}

func TestDenyAndAllowOAuthClientHosts(t *testing.T) {
	e, f := newOAuthClientsEnv(t)
	res := e.admin(t, http.MethodGet, "/admin/oauth_client_host_denials", "admin-read", nil)
	page := decodeJSON[clientapi.AdminOAuthClientHostDenialPage](t, res)
	if res.StatusCode != http.StatusOK || len(page.Items) != 1 || page.Items[0].Host != "bad.example" || page.Items[0].DeniedBy == nil || page.NextCursor != nil {
		t.Fatalf("list = %d %+v", res.StatusCode, page)
	}
	if res := e.sendJSON(t, http.MethodPost, "/admin/oauth_client_host_denials", "admin", `{"host":"App.Example"}`); res.StatusCode != http.StatusNoContent {
		t.Errorf("deny = %d, want 204", res.StatusCode)
	}
	if res := e.send(t, http.MethodDelete, "/admin/oauth_client_host_denials/bad.example", "admin", ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("allow = %d, want 204", res.StatusCode)
	}
	want := []string{"deny app.example by " + userOf(aliceAccountID), "allow bad.example by " + userOf(aliceAccountID)}
	if !slices.Equal(f.calls, want) {
		t.Errorf("calls = %v, want %v", f.calls, want)
	}
	// A host with a port or a path is refused.
	for _, host := range []string{"app.example:8443", "app.example/x"} {
		body := `{"host":"` + host + `"}`
		if res := e.sendJSON(t, http.MethodPost, "/admin/oauth_client_host_denials", "admin", body); res.StatusCode != http.StatusUnprocessableEntity && res.StatusCode != http.StatusBadRequest {
			t.Errorf("deny %q = %d, want 422 or 400", host, res.StatusCode)
		}
	}
}
