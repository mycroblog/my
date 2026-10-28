// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/authpages"
)

// fakeUnsubscribe knows the unsubscribe token "my_eut_good" of the admin_report notification
// emails and counts the unsubscribes.
type fakeUnsubscribe struct{ unsubscribes int }

func (f *fakeUnsubscribe) CheckUnsubscribeToken(_ context.Context, token string) (identity.UnsubscribeCategory, error) {
	if token != "my_eut_good" {
		return "", appidentity.ErrInvalidToken
	}
	return "admin_report", nil
}

func (f *fakeUnsubscribe) Unsubscribe(ctx context.Context, token string) (identity.UnsubscribeCategory, error) {
	c, err := f.CheckUnsubscribeToken(ctx, token)
	if err == nil {
		f.unsubscribes++
	}
	return c, err
}

func newUnsubscribeServer(t *testing.T, u authpages.MailUnsubscribe) http.Handler {
	t.Helper()
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := mux{http.NewServeMux()}
	authpages.New(authpages.Options{Identity: newFakeIdentity(), Locales: &i18n.Resolver{Bundle: b}, MailUnsubscribe: u}).Register(m)
	return m
}

// oneClick sends the POST of RFC 8058 as a mail provider does: from its server, with neither a
// cookie nor Origin nor Sec-Fetch-Site.
func oneClick(h http.Handler, target string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "https://social.example"+target, strings.NewReader("List-Unsubscribe=One-Click"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestMailUnsubscribe(t *testing.T) {
	f := &fakeUnsubscribe{}
	h := newUnsubscribeServer(t, f)
	b := newBrowser(t, h)
	link := authpages.PathMailUnsubscribe + "?token=my_eut_good"

	// The GET asks and changes nothing.
	w := b.do(http.MethodGet, link, nil, nil)
	if w.Code != http.StatusOK || f.unsubscribes != 0 {
		t.Fatalf("GET = %d with %d unsubscribes, want 200 and none", w.Code, f.unsubscribes)
	}
	wantContains(t, w.Body.String(), `action="/mail/unsubscribe?token=my_eut_good"`, `name="List-Unsubscribe" value="One-Click"`,
		"emails of “New reports” notifications")

	// The one-click POST stops the emails, and again with the same token answers the same.
	for i := 1; i <= 2; i++ {
		w = oneClick(h, link)
		if w.Code != http.StatusOK || f.unsubscribes != i {
			t.Fatalf("POST %d = %d with %d unsubscribes, want 200", i, w.Code, f.unsubscribes)
		}
		wantContains(t, w.Body.String(), "You no longer get emails of “New reports” notifications.")
	}

	// The button of the page sends the same POST from the browser.
	if w := b.do(http.MethodPost, link, map[string][]string{"List-Unsubscribe": {"One-Click"}}, nil); w.Code != http.StatusOK || f.unsubscribes != 3 {
		t.Errorf("POST from the page = %d with %d unsubscribes, want 200", w.Code, f.unsubscribes)
	}

	// An unknown or expired token, or none, changes nothing and shows the error page.
	for _, target := range []string{
		authpages.PathMailUnsubscribe + "?token=my_eut_bad", authpages.PathMailUnsubscribe,
		authpages.PathMailUnsubscribe + "?token=" + strings.Repeat("x", 300),
	} {
		if w := b.do(http.MethodGet, target, nil, nil); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "This link is not valid.") {
			t.Errorf("GET %s = %d, want 400 with the error", target, w.Code)
		}
		if w := oneClick(h, target); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "This link is not valid.") {
			t.Errorf("POST %s = %d, want 400 with the error", target, w.Code)
		}
	}
	if f.unsubscribes != 3 {
		t.Errorf("unsubscribes = %d, want still 3", f.unsubscribes)
	}
}

func TestMailUnsubscribeOff(t *testing.T) {
	b := newBrowser(t, newServer(t, newFakeIdentity()))
	if w := b.do(http.MethodGet, authpages.PathMailUnsubscribe+"?token=my_eut_good", nil, nil); w.Code != http.StatusNotFound {
		t.Errorf("GET without MailUnsubscribe = %d, want 404", w.Code)
	}
}
