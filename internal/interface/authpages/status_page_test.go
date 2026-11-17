// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/authpages"
)

// unreachableIdentity fails every session lookup, as when the database is out of reach.
type unreachableIdentity struct{ *fakeIdentity }

func (unreachableIdentity) Authenticate(context.Context, string) (identity.Session, error) {
	return identity.Session{}, errors.New("database unreachable")
}

// TestStatusPageLink checks that the sign-in page and the error page of a database outage link to the status page
// when status_page_url is set (ADR-0088 §20), and that no page has the link otherwise.
func TestStatusPageLink(t *testing.T) {
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	const url = "https://status.example.org/"
	serve := func(id authpages.Identity, statusPageURL string) *browser {
		m := mux{http.NewServeMux()}
		authpages.New(authpages.Options{Identity: id, Locales: &i18n.Resolver{Bundle: b}, StatusPageURL: statusPageURL}).Register(m)
		return newBrowser(t, m)
	}
	for _, tt := range []struct {
		name string
		id   authpages.Identity
		code int
	}{
		{"sign-in", newFakeIdentity(), http.StatusOK},
		{"database outage", unreachableIdentity{newFakeIdentity()}, http.StatusInternalServerError},
	} {
		w := serve(tt.id, url).do(http.MethodGet, authpages.PathSignIn, nil, nil)
		if w.Code != tt.code || text(screenData(t, w), "statusPageUrl") != url {
			t.Errorf("%s = %d, lacks the status page link:\n%s", tt.name, w.Code, w.Body)
		}
		w = serve(tt.id, "").do(http.MethodGet, authpages.PathSignIn, nil, nil)
		if data := screenData(t, w); at(data, "statusPageUrl") != nil {
			t.Errorf("%s without status_page_url links to a status page: %v", tt.name, data)
		}
	}
}
