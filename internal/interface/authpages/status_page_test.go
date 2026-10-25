// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
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
// when status_page_url is set (ADR-0085 §6), in each locale, and that no page has the link otherwise.
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
		for lang, text := range map[string]string{"en": "Service status", "ja": "稼働状況"} {
			w := serve(tt.id, url).do(http.MethodGet, authpages.PathSignIn, nil, map[string]string{"Accept-Language": lang})
			if w.Code != tt.code || !strings.Contains(w.Body.String(), `<a href="`+url+`" rel="noopener">`+text+`</a>`) {
				t.Errorf("%s in %s = %d, lacks the status page link:\n%s", tt.name, lang, w.Code, w.Body)
			}
		}
		w := serve(tt.id, "").do(http.MethodGet, authpages.PathSignIn, nil, nil)
		if body := w.Body.String(); strings.Contains(body, "<footer") || strings.Contains(body, "Service status") {
			t.Errorf("%s without status_page_url has a footer:\n%s", tt.name, body)
		}
	}
}
