// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The Client ID Metadata Document cases of AccountRepositoryContract (ADR-0090 §1): the cache of
// a fetched document, its refresh, and the denial of a host.

// metadataWeb serves one metadata document per URL as a kernel.HTTPClient and counts the
// fetches.
type metadataWeb struct {
	mu     sync.Mutex
	docs   map[string]string
	maxAge string
	hits   int
}

func (w *metadataWeb) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if req.Purpose != kernel.HTTPPurposeClientMetadata {
		return nil, kernel.ErrInvalidHTTPRequest
	}
	w.hits++
	doc, ok := w.docs[req.URL]
	status := http.StatusOK
	if !ok {
		status = http.StatusNotFound
	}
	return &kernel.HTTPResponse{
		StatusCode: status, URL: req.URL, Body: io.NopCloser(bytes.NewReader([]byte(doc))),
		Header: http.Header{"Content-Type": {"application/json"}, "Cache-Control": {"max-age=" + w.maxAge}},
	}, nil
}

func (w *metadataWeb) set(url, doc string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.docs[url] = doc
}

func (w *metadataWeb) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hits
}

func oauthUsesClientMetadataDocuments(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	const (
		id        = "https://app.example/client.json"
		redirect  = "https://app.example/cb"
		redirect2 = "https://app.example/cb2"
	)
	web := &metadataWeb{docs: map[string]string{}, maxAge: "600"}
	web.set(id, `{"client_id":"`+id+`","client_name":"Pocket","redirect_uris":["`+redirect+`"],"scope":"read write"}`)
	s := e.newIdentityWith(func(d *appidentity.Deps) { d.HTTP = web })
	userID := e.register(t, "alice").User.ID
	request := func(uri string) appidentity.AuthorizationRequest {
		return appidentity.AuthorizationRequest{
			ResponseType: "code", ClientID: id, RedirectURI: uri,
			CodeChallenge: identity.S256Challenge(contractCodeVerifier), CodeChallengeMethod: "S256",
		}
	}
	authorize := func(uri string) (appidentity.Authorization, error) { return s.ValidateAuthorization(ctx, request(uri)) }

	a, err := authorize(redirect)
	if err != nil {
		t.Fatalf("ValidateAuthorization = %v", err)
	}
	if a.Client.Name != "Pocket" || a.Scope != "read write" || a.Client.Registration != identity.ClientRegisteredByMetadataDocument || !a.Client.Public() {
		t.Errorf("authorization = %+v, want the public client Pocket with read write", a)
	}
	code, err := s.IssueAuthorizationCode(ctx, a, userID)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := s.ExchangeAuthorizationCode(ctx, appidentity.CodeExchange{
		ClientCredentials: appidentity.ClientCredentials{ClientID: id}, Code: code, RedirectURI: redirect, CodeVerifier: contractCodeVerifier,
	})
	if err != nil || tokens.Client.Name != "Pocket" {
		t.Fatalf("ExchangeAuthorizationCode = %+v, %v", tokens.Client, err)
	}
	stored, uris, err := e.h.Identity.FindOAuthClient(ctx, id)
	if err != nil || stored.Name != "app.example" || stored.Registration != identity.ClientRegisteredByMetadataDocument || len(uris) != 1 || uris[0] != redirect {
		t.Errorf("stored client = %+v %v, %v; want the host as its fixed name and the document's redirect URI", stored, uris, err)
	}
	doc, err := e.h.Identity.FindOAuthClientMetadataDocument(ctx, stored.ID)
	if err != nil || doc.Name != "Pocket" || !doc.FetchedAt.Equal(e.clock.Now()) || !doc.ExpiresAt.Equal(e.clock.Now().Add(10*time.Minute)) {
		t.Errorf("stored document = %+v, %v; want Pocket fetched now for ten minutes", doc, err)
	}

	// Within max-age the stored document answers; after it the document is fetched again and
	// replaces the name, the scope, and the redirect URIs. max-age is above the 5 minutes that
	// the URL limit leaves between two fetches (ADR-0090 §7).
	e.clock.Advance(599 * time.Second)
	if _, err := authorize(redirect); err != nil || web.count() != 1 {
		t.Errorf("ValidateAuthorization(cached) = %v after %d fetches, want 1", err, web.count())
	}
	web.set(id, `{"client_id":"`+id+`","client_name":"Pocket 2","redirect_uris":["`+redirect2+`"],"scope":"read"}`)
	e.clock.Advance(time.Second)
	a, err = authorize(redirect2)
	if err != nil || a.Client.Name != "Pocket 2" || a.Scope != "read" || web.count() != 2 {
		t.Errorf("ValidateAuthorization(refetched) = %+v, %v after %d fetches; want Pocket 2 with read after 2", a.Client, err, web.count())
	}
	if _, err := authorize(redirect); !errors.Is(err, appidentity.ErrInvalidRedirectURI) {
		t.Errorf("ValidateAuthorization(removed redirect URI) = %v, want ErrInvalidRedirectURI", err)
	}

	// A denied host is refused before any fetch, for authorizations and token requests alike;
	// lifting the denial lets the client back in.
	for range 2 { // safe to repeat
		if err := s.DenyOAuthClientHost(ctx, "App.Example", userID); err != nil {
			t.Fatalf("DenyOAuthClientHost = %v", err)
		}
	}
	e.clock.Advance(time.Hour)
	if _, err := authorize(redirect2); !errors.Is(err, appidentity.ErrUnknownClient) || web.count() != 2 {
		t.Errorf("ValidateAuthorization(denied host) = %v after %d fetches, want ErrUnknownClient after 2", err, web.count())
	}
	_, err = s.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: appidentity.ClientCredentials{ClientID: id}, RefreshToken: tokens.RefreshToken})
	wantOAuthError(t, err, appidentity.OAuthInvalidClient, "refreshing a token of a denied host")
	if err := s.AllowOAuthClientHost(ctx, "app.example", userID); err != nil {
		t.Fatalf("AllowOAuthClientHost = %v", err)
	}
	if denied, err := e.h.Identity.OAuthClientHostDenied(ctx, "app.example"); err != nil || denied {
		t.Errorf("OAuthClientHostDenied(allowed) = %v, %v", denied, err)
	}
	if _, err := authorize(redirect2); err != nil || web.count() != 3 {
		t.Errorf("ValidateAuthorization(allowed host) = %v after %d fetches, want 3", err, web.count())
	}

	// A disabled client is refused without a fetch.
	if err := s.DisableOAuthClient(ctx, stored.ID, userID); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Hour)
	if _, err := authorize(redirect2); !errors.Is(err, appidentity.ErrUnknownClient) || web.count() != 3 {
		t.Errorf("ValidateAuthorization(disabled) = %v after %d fetches, want ErrUnknownClient after 3", err, web.count())
	}
}
