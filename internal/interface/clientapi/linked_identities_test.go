// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const (
	linkCorp = "019a0000-0000-7000-8000-0000000000c1"
	linkID   = "019a0000-0000-7000-8000-0000000000c2"
)

// fakeLinkedIdentities are the links of the viewer: one to "corp", which is the last way to
// sign in when last. Password "secret" or the external confirmation "ext-link" confirms a
// link.
type fakeLinkedIdentities struct {
	links    []appidentity.LinkedIdentity
	last     bool
	returnTo string
	op       appidentity.ConfirmedOperation
}

var linkedAt = time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)

func (f *fakeLinkedIdentities) LinkedIdentities(context.Context, string) (appidentity.LinkedIdentities, error) {
	return appidentity.LinkedIdentities{
		Links:     f.links,
		Providers: []appidentity.LinkableProvider{{ID: linkCorp, Kind: identity.LoginProviderOIDC, DisplayName: "Corp", AllowLink: true}},
	}, nil
}

func (f *fakeLinkedIdentities) UnlinkExternalIdentity(_ context.Context, _, id string) error {
	if len(f.links) == 0 || f.links[0].LinkID != id {
		return appidentity.ErrNotFound
	}
	if f.last {
		return appidentity.ErrLastSignInMethod
	}
	f.links = nil
	return nil
}

func (f *fakeLinkedIdentities) BeginExternalLink(_ context.Context, _, providerID string, c appidentity.Confirmation, returnTo string) (appidentity.ExternalIntent, error) {
	f.returnTo = returnTo
	switch {
	case providerID != linkCorp:
		return appidentity.ExternalIntent{}, appidentity.ErrLoginProviderUnavailable
	case len(f.links) > 0:
		return appidentity.ExternalIntent{}, appidentity.ErrLoginProviderLinked
	case c.External != "" && c.External != "ext-link":
		return appidentity.ExternalIntent{}, appidentity.ErrExternalConfirmationExpired
	case c.External == "" && c.Password != "secret":
		return appidentity.ExternalIntent{}, appidentity.ErrWrongPassword
	}
	return appidentity.ExternalIntent{StartURL: "https://social.example/auth/external/corp/start?intent=x", ExpiresAt: linkedAt}, nil
}

func (f *fakeLinkedIdentities) BeginExternalConfirmation(_ context.Context, _, providerID string, op appidentity.ConfirmedOperation, returnTo string) (appidentity.ExternalIntent, error) {
	f.returnTo, f.op = returnTo, op
	if providerID != linkCorp || len(f.links) == 0 {
		return appidentity.ExternalIntent{}, appidentity.ErrLoginProviderNotLinked
	}
	return appidentity.ExternalIntent{StartURL: "https://social.example/auth/external/corp/start?intent=y", ExpiresAt: linkedAt}, nil
}

func newLinkedIdentityEnv(t *testing.T) (*authEnv, *fakeLinkedIdentities) {
	t.Helper()
	f := &fakeLinkedIdentities{}
	return newAuthEnv(t, func(o *clientapi.Options) { o.LinkedIdentities = f }), f
}

func TestLinkedIdentities(t *testing.T) {
	e, f := newLinkedIdentityEnv(t)
	res := e.send(t, http.MethodGet, "/linked_identities", "read-accounts", "")
	if list := decodeJSON[clientapi.LinkedIdentityList](t, res); res.StatusCode != http.StatusOK || len(list.Items) != 0 ||
		len(list.Providers) != 1 || list.Providers[0].DisplayName != "Corp" || !list.Providers[0].AllowLink ||
		res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("list = %d %+v", res.StatusCode, list)
	}

	for _, tc := range []struct {
		body, typ string
		status    int
	}{
		{`{"provider_id":"` + linkCorp + `","password":"wrong"}`, clientapi.ProblemTypeWrongPassword, http.StatusUnprocessableEntity},
		{`{"provider_id":"` + linkCorp + `"}`, clientapi.ProblemTypeBlank, http.StatusBadRequest},
		{`{"provider_id":"` + linkCorp + `","external_confirmation":"old"}`, clientapi.ProblemTypeExternalConfirmationExpired, http.StatusUnprocessableEntity},
		{`{"provider_id":"019a0000-0000-7000-8000-0000000000ff","password":"secret"}`, clientapi.ProblemTypeLoginProviderUnavailable, http.StatusUnprocessableEntity},
	} {
		res := e.send(t, http.MethodPost, "/linked_identities", "write", tc.body)
		if p := decodeProblem(t, res); res.StatusCode != tc.status || p.Type != tc.typ {
			t.Errorf("link %s = %d %+v, want %d %s", tc.body, res.StatusCode, p, tc.status, tc.typ)
		}
	}
	res = e.send(t, http.MethodPost, "/linked_identities", "write", `{"provider_id":"`+linkCorp+`","password":"secret","return_to":"/settings/linked-accounts"}`)
	start := decodeJSON[clientapi.ExternalAuthorizationStart](t, res)
	if res.StatusCode != http.StatusOK || start.StartUrl != "https://social.example/auth/external/corp/start?intent=x" ||
		!start.ExpiresAt.Equal(linkedAt) || f.returnTo != "/settings/linked-accounts" {
		t.Errorf("link = %d %+v, return_to %q", res.StatusCode, start, f.returnTo)
	}
	// A return target off this site is dropped.
	res = e.send(t, http.MethodPost, "/linked_identities", "write", `{"provider_id":"`+linkCorp+`","external_confirmation":"ext-link","return_to":"//evil.example"}`)
	if res.StatusCode != http.StatusOK || f.returnTo != "" {
		t.Errorf("link with another site's return_to = %d, return_to %q", res.StatusCode, f.returnTo)
	}

	f.links = []appidentity.LinkedIdentity{{LinkID: linkID, ProviderID: linkCorp, ProviderKind: identity.LoginProviderOIDC, ProviderName: "Corp", LinkedAt: linkedAt, Available: true}}
	res = e.send(t, http.MethodGet, "/linked_identities", "read-accounts", "")
	if list := decodeJSON[clientapi.LinkedIdentityList](t, res); len(list.Items) != 1 || list.Items[0].Id != linkID ||
		list.Items[0].ProviderName != "Corp" || list.Items[0].ProviderKind != clientapi.LoginProviderKindOidc || !list.Items[0].LinkedAt.Equal(linkedAt) {
		t.Errorf("list = %+v", list)
	}
	res = e.send(t, http.MethodPost, "/linked_identities", "write", `{"provider_id":"`+linkCorp+`","password":"secret"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeLoginProviderLinked {
		t.Errorf("second link = %d %+v", res.StatusCode, p)
	}

	// Confirmations.
	res = e.send(t, http.MethodPost, "/linked_identities/confirmation", "write", `{"provider_id":"`+linkCorp+`","operation":"nothing"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("confirmation of an unknown operation = %d", res.StatusCode)
	}
	res = e.send(t, http.MethodPost, "/linked_identities/confirmation", "write", `{"provider_id":"`+linkCorp+`","operation":"account_deletion","return_to":"/settings/account"}`)
	if start := decodeJSON[clientapi.ExternalAuthorizationStart](t, res); res.StatusCode != http.StatusOK || start.StartUrl == "" ||
		f.op != appidentity.OperationAccountDeletion || f.returnTo != "/settings/account" {
		t.Errorf("confirmation = %d %+v, op %q", res.StatusCode, start, f.op)
	}

	// Unlinking.
	f.last = true
	res = e.send(t, http.MethodDelete, "/linked_identities/"+linkID, "write", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeLastSignInMethod {
		t.Errorf("unlink of the last way to sign in = %d %+v", res.StatusCode, p)
	}
	f.last = false
	if res := e.send(t, http.MethodDelete, "/linked_identities/"+linkID, "write", ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("unlink = %d", res.StatusCode)
	}
	if res := e.send(t, http.MethodDelete, "/linked_identities/"+linkID, "write", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("unlink of an unlinked account = %d", res.StatusCode)
	}
	res = e.send(t, http.MethodPost, "/linked_identities/confirmation", "write", `{"provider_id":"`+linkCorp+`","operation":"link"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeLoginProviderUnavailable {
		t.Errorf("confirmation without a link = %d %+v", res.StatusCode, p)
	}
}
