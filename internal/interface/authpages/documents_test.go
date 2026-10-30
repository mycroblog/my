// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/authpages"
)

// fakeDocuments holds documents by key. err fails every read.
type fakeDocuments struct {
	docs map[instance.TextKey]appinstance.Document
	err  error
}

func (f *fakeDocuments) HasDocument(_ context.Context, key instance.TextKey) (bool, error) {
	_, ok := f.docs[key]
	return ok, f.err
}

func newDocumentServer(t *testing.T, id authpages.Identity, docs authpages.Documents) http.Handler {
	t.Helper()
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := mux{http.NewServeMux()}
	authpages.New(authpages.Options{Identity: id, Locales: &i18n.Resolver{Bundle: b}, Documents: docs}).Register(m)
	return m
}

var revised = time.Date(2026, 10, 1, 23, 30, 0, 0, time.FixedZone("JST", 9*60*60))

func TestMissingDocumentsAreNotLinked(t *testing.T) {
	docs := &fakeDocuments{docs: map[instance.TextKey]appinstance.Document{
		instance.TextPrivacyPolicy: {Key: instance.TextPrivacyPolicy, HTML: "<p>p</p>", RevisedAt: revised},
	}}
	b := newBrowser(t, newDocumentServer(t, newFakeIdentity(), docs))
	body := b.do(http.MethodGet, authpages.PathSignIn, nil, nil).Body.String()
	wantContains(t, body, `<a href="/about/privacy">Privacy policy</a>`)
	wantLacks(t, body, `href="/about/terms"`)

	// Without any document, no page has the footer or the agreement.
	for _, server := range []http.Handler{
		newDocumentServer(t, newFakeIdentity(), &fakeDocuments{}),
		newServer(t, newFakeIdentity()), // no Options.Documents
	} {
		b := newBrowser(t, server)
		for _, path := range []string{authpages.PathSignIn, authpages.PathSignUp} {
			wantLacks(t, b.do(http.MethodGet, path, nil, nil).Body.String(), "<footer", "sign-up-agreement", "/about/terms", "/about/privacy")
		}
	}
}

func TestSignUpAsksToAgreeToTheDocuments(t *testing.T) {
	docs := &fakeDocuments{docs: map[instance.TextKey]appinstance.Document{
		instance.TextTerms: {Key: instance.TextTerms, HTML: "<p>t</p>", RevisedAt: revised},
	}}
	b := newBrowser(t, newDocumentServer(t, newFakeIdentity(), docs))
	body := b.do(http.MethodGet, authpages.PathSignUp, nil, map[string]string{"Accept-Language": "ja"}).Body.String()
	wantContains(t, body,
		`<p id="sign-up-agreement">アカウントを作成する前に、このサーバーの次の文書を読んでください。</p>`,
		// The form keeps what was typed: the agreement's links open in a new tab, which assistive technology reads.
		`<li><a href="/about/terms" target="_blank" rel="noopener noreferrer">利用規約<span class="visually-hidden"> （新しいタブで開きます）</span></a></li>`,
		// A required checkbox agrees to them, described by the notice above the links.
		`<label class="check"><input id="sign-up-agreement-check" name="agreement" type="checkbox" value="yes" required aria-describedby="sign-up-agreement"><span>上の文書を読み、同意します</span></label>`,
		// The footer's links lead from page to page of the server in the same tab.
		`<li><a href="/about/terms">利用規約</a></li>`,
	)
	wantLacks(t, body, "/about/privacy")
	// The sign-in page links to them in the footer only.
	body = b.do(http.MethodGet, authpages.PathSignIn, nil, nil).Body.String()
	wantContains(t, body, `<footer class="footer">`, `<a href="/about/terms">Terms of service</a>`)
	wantLacks(t, body, "sign-up-agreement")
}

// TestSignUpNeedsTheAgreement checks that the server, not only the browser's required checkbox,
// refuses a registration without the agreement while the instance has published a document, and
// keeps the box checked when the form comes back with other errors.
func TestSignUpNeedsTheAgreement(t *testing.T) {
	docs := &fakeDocuments{docs: map[instance.TextKey]appinstance.Document{
		instance.TextTerms: {Key: instance.TextTerms, HTML: "<p>t</p>", RevisedAt: revised},
	}}
	id := newFakeIdentity()
	b := newBrowser(t, newDocumentServer(t, id, docs))
	fields := []string{"username", "alice", "email", "alice@example.com", "password", "correct horse"}
	w := b.do(http.MethodPost, authpages.PathSignUp, signUpForm(b, fields...), nil)
	if w.Code != http.StatusUnprocessableEntity || len(id.signUps) != 0 {
		t.Fatalf("POST sign_up without the agreement = %d with %d sign-ups, want 422 and none", w.Code, len(id.signUps))
	}
	wantContains(t, w.Body.String(),
		`<p id="sign-up-agreement-error" class="field-error">Agree to the documents above to create an account.</p>`,
		`aria-describedby="sign-up-agreement sign-up-agreement-error" aria-invalid="true"`,
	)

	w = b.do(http.MethodPost, authpages.PathSignUp, signUpForm(b, "username", "", "agreement", "yes"), nil)
	wantContains(t, w.Body.String(), `value="yes" required checked`)

	w = b.do(http.MethodPost, authpages.PathSignUp, signUpForm(b, append(fields, "agreement", "yes")...), nil)
	if w.Code != http.StatusSeeOther || len(id.signUps) != 1 {
		t.Fatalf("POST sign_up with the agreement = %d with %d sign-ups, want 303 and one\n%s", w.Code, len(id.signUps), w.Body.String())
	}
}

func TestDocumentFailures(t *testing.T) {
	docs := &fakeDocuments{err: errors.New("db down")}
	b := newBrowser(t, newDocumentServer(t, newFakeIdentity(), docs))
	// The pages still work, without the links.
	w := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	if w.Code != http.StatusOK {
		t.Errorf("GET sign_in with a failing store = %d, want 200", w.Code)
	}
	wantLacks(t, w.Body.String(), "<footer")
}
