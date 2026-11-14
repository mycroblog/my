// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
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
	data := screenData(t, b.do(http.MethodGet, authpages.PathSignIn, nil, nil))
	if got := documentLinks(data); !slices.Equal(got, []string{"/about/privacy Privacy policy"}) {
		t.Errorf("documents = %q, want the privacy policy only", got)
	}

	// Without any document, no screen links to one.
	for _, server := range []http.Handler{
		newDocumentServer(t, newFakeIdentity(), &fakeDocuments{}),
		newServer(t, newFakeIdentity()), // no Options.Documents
	} {
		b := newBrowser(t, server)
		for _, path := range []string{authpages.PathSignIn, authpages.PathSignUp} {
			if data := screenData(t, b.do(http.MethodGet, path, nil, nil)); at(data, "documents") != nil {
				t.Errorf("GET %s without documents: documents = %v", path, at(data, "documents"))
			}
		}
	}
}

// documentLinks returns the screen's document links as "path title".
func documentLinks(data map[string]any) []string {
	docs, _ := at(data, "documents").([]any)
	out := make([]string, 0, len(docs))
	for i := range docs {
		p := "documents." + strconv.Itoa(i) + "."
		out = append(out, text(data, p+"path")+" "+text(data, p+"title"))
	}
	return out
}

// TestSignUpAsksToAgreeToTheDocuments checks that the registration screen carries the published
// documents, titled in the screen's locale, which its agreement links to.
func TestSignUpAsksToAgreeToTheDocuments(t *testing.T) {
	docs := &fakeDocuments{docs: map[instance.TextKey]appinstance.Document{
		instance.TextTerms: {Key: instance.TextTerms, HTML: "<p>t</p>", RevisedAt: revised},
	}}
	b := newBrowser(t, newDocumentServer(t, newFakeIdentity(), docs))
	data := screenData(t, b.do(http.MethodGet, authpages.PathSignUp, nil, map[string]string{"Accept-Language": "ja"}))
	if got := documentLinks(data); text(data, "screen") != "sign_up" || !slices.Equal(got, []string{"/about/terms 利用規約"}) {
		t.Errorf("sign-up documents = %q, want the terms in Japanese", got)
	}
	data = screenData(t, b.do(http.MethodGet, authpages.PathSignIn, nil, nil))
	if got := documentLinks(data); !slices.Equal(got, []string{"/about/terms Terms of service"}) {
		t.Errorf("sign-in documents = %q, want the terms", got)
	}
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
	if data := screenData(t, w); text(data, "signUp.errors.agreement") != "Agree to the documents above to create an account." ||
		at(data, "signUp.needAgreement") != true || at(data, "signUp.agreed") != nil {
		t.Errorf("sign-up without the agreement: %v", at(data, "signUp"))
	}

	w = b.do(http.MethodPost, authpages.PathSignUp, signUpForm(b, "username", "", "agreement", "yes"), nil)
	if data := screenData(t, w); at(data, "signUp.agreed") != true || at(data, "signUp.errors.agreement") != nil {
		t.Errorf("the form with other errors lost the agreement: %v", at(data, "signUp"))
	}

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
	if data := screenData(t, w); at(data, "documents") != nil {
		t.Errorf("documents = %v with a failing store, want none", at(data, "documents"))
	}
}
