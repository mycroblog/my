// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/authpages"
)

const externalTestPassword = "correct horse battery staple"

// signedIn returns a browser signed in to the sign-in pages as the user with email.
func (e *externalEnv) signedIn(email string) *authClient {
	e.t.Helper()
	return e.signedInWith(email, externalTestPassword)
}

func (e *externalEnv) signedInWith(email, password string) *authClient {
	e.t.Helper()
	s, err := e.s.app.Identity.SignIn(e.t.Context(), identity.SignInInput{Email: email, Password: password})
	if err != nil {
		e.t.Fatal(err)
	}
	c := e.client()
	c.cookies[authpages.CookieSession] = s.Token
	return c
}

// runIntent opens the start URL of a link or a confirmation in c and answers as the provider
// with an ID token that has claims. It returns the authorization URL and the outcome in the
// fragment of the callback's redirect.
func (e *externalEnv) runIntent(c *authClient, start identity.ExternalIntent, providerID string, claims map[string]any) (string, url.Values) {
	e.t.Helper()
	u, err := url.Parse(start.StartURL)
	if err != nil {
		e.t.Fatal(err)
	}
	resp, body := c.do(http.MethodGet, u.RequestURI(), nil, nil)
	if resp.StatusCode != http.StatusSeeOther {
		e.t.Fatalf("start = %d\n%s", resp.StatusCode, body)
	}
	authURL := resp.Header.Get("Location")
	au, _ := url.Parse(authURL)
	resp, body = callback(c, providerID, e.idp.issue(authURL, claims), au.Query().Get("state"))
	loc := resp.Header.Get("Location")
	i := strings.IndexByte(loc, '#')
	if resp.StatusCode != http.StatusSeeOther || i < 0 {
		e.t.Fatalf("callback = %d %q\n%s", resp.StatusCode, loc, body)
	}
	fragment, err := url.ParseQuery(loc[i+1:])
	if err != nil {
		e.t.Fatal(err)
	}
	return authURL, fragment
}

// TestExternalLink links accounts at a provider from the settings, after a confirmation, and
// refuses one that another user links (ADR-0106 §3.5).
func TestExternalLink(t *testing.T) {
	t.Parallel()
	e := newExternalEnv(t)
	ctx := t.Context()
	svc := e.s.app.Identity
	alice := e.user("alice", "alice@example.com")
	bob := e.user("bob", "bob@example.com")
	id := e.provider("IdP", false)
	noLink := domain.NewLoginProviderSettings(domain.LoginProviderOIDC)
	noLink.DisplayName, noLink.AllowLink = "No links", false
	closed := e.providerWith(noLink)

	if _, err := svc.BeginExternalLink(ctx, alice, id, identity.Confirmation{Password: "wrong"}, "/settings/linked-accounts"); !errors.Is(err, identity.ErrWrongPassword) {
		t.Errorf("BeginExternalLink with a wrong password = %v", err)
	}
	if _, err := svc.BeginExternalLink(ctx, alice, closed, identity.Confirmation{Password: externalTestPassword}, ""); !errors.Is(err, identity.ErrLoginProviderUnavailable) {
		t.Errorf("BeginExternalLink to a provider without links = %v", err)
	}
	start, err := svc.BeginExternalLink(ctx, alice, id, identity.Confirmation{Password: externalTestPassword}, "/settings/linked-accounts")
	if err != nil {
		t.Fatal(err)
	}

	// The intent works only in a browser signed in as alice.
	u, _ := url.Parse(start.StartURL)
	if resp, body := e.signedIn("bob@example.com").do(http.MethodGet, u.RequestURI(), nil, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("start in bob's browser = %d\n%s", resp.StatusCode, body)
	}

	c := e.signedIn("alice@example.com")
	authURL, out := e.runIntent(c, start, id, map[string]any{"sub": "subject-alice"})
	if q, _ := url.Parse(authURL); q.Query().Has("prompt") || q.Query().Has("max_age") {
		t.Errorf("authorization request of a link = %s, want no re-authentication", authURL)
	}
	if out.Get("linked") != id {
		t.Fatalf("link outcome = %v", out)
	}
	if l, err := e.db.IdentityRepository().FindLinkedExternalIdentity(ctx, id, "subject-alice"); err != nil || l.UserID != alice {
		t.Errorf("FindLinkedExternalIdentity = %+v, %v", l, err)
	}
	list, err := svc.LinkedIdentities(ctx, alice)
	if err != nil || len(list.Links) != 1 || list.Links[0].ProviderID != id || !list.Links[0].Available || len(list.Providers) != 2 {
		t.Errorf("LinkedIdentities = %+v, %v", list, err)
	}
	// The link signs alice in.
	c2 := e.client()
	if resp, _ := e.signInWith(c2, id, map[string]any{"sub": "subject-alice"}); resp.StatusCode != http.StatusSeeOther || e.sessionUser(c2) != alice {
		t.Errorf("sign-in with the link = %d", resp.StatusCode)
	}
	// A second link to the same provider is refused.
	if _, err := svc.BeginExternalLink(ctx, alice, id, identity.Confirmation{Password: externalTestPassword}, ""); !errors.Is(err, identity.ErrLoginProviderLinked) {
		t.Errorf("second BeginExternalLink = %v", err)
	}

	// Bob cannot link the account that alice links.
	start, err = svc.BeginExternalLink(ctx, bob, id, identity.Confirmation{Password: externalTestPassword}, "/settings/linked-accounts")
	if err != nil {
		t.Fatal(err)
	}
	if _, out := e.runIntent(e.signedIn("bob@example.com"), start, id, map[string]any{"sub": "subject-alice"}); out.Get("external_error") != "linked_elsewhere" {
		t.Errorf("link of alice's account by bob = %v", out)
	}
	if l, _ := e.db.IdentityRepository().FindLinkedExternalIdentity(ctx, id, "subject-alice"); l.UserID != alice {
		t.Errorf("subject-alice is linked to %q", l.UserID)
	}
}

// TestExternalConfirmation confirms operations by signing in again at a linked provider, and
// unlinks while another way to sign in stays (ADR-0106 §3.6, §3.7).
func TestExternalConfirmation(t *testing.T) {
	t.Parallel()
	e := newExternalEnv(t)
	ctx := t.Context()
	svc := e.s.app.Identity
	carol := e.user("carol", "carol@example.com")
	e.user("dave", "dave@example.com")
	id := e.provider("IdP", false)
	other := e.provider("Other", false)
	e.link(id, "subject-carol", carol)
	browser := e.signedIn("carol@example.com")

	confirm := func(op identity.ConfirmedOperation, claims map[string]any) (string, url.Values) {
		t.Helper()
		start, err := svc.BeginExternalConfirmation(ctx, carol, id, op, "/settings/password")
		if err != nil {
			t.Fatal(err)
		}
		return e.runIntent(browser, start, id, claims)
	}
	fresh := func(sub string) map[string]any { return map[string]any{"sub": sub, "auth_time": time.Now().Unix()} }

	if _, err := svc.BeginExternalConfirmation(ctx, carol, other, identity.OperationPasswordChange, ""); !errors.Is(err, identity.ErrLoginProviderNotLinked) {
		t.Errorf("BeginExternalConfirmation with a provider that carol does not link = %v", err)
	}

	t.Run("an old authentication", func(t *testing.T) {
		if _, out := confirm(identity.OperationPasswordChange, map[string]any{"sub": "subject-carol", "auth_time": time.Now().Add(-time.Hour).Unix()}); out.Get("external_error") != "failed" {
			t.Errorf("confirmation with an old auth_time = %v", out)
		}
		if _, out := confirm(identity.OperationPasswordChange, map[string]any{"sub": "subject-carol"}); out.Get("external_error") != "failed" {
			t.Errorf("confirmation without auth_time = %v", out)
		}
	})

	t.Run("another account", func(t *testing.T) {
		if _, out := confirm(identity.OperationPasswordChange, fresh("subject-someone")); out.Get("external_error") != "mismatch" {
			t.Errorf("confirmation with another account = %v", out)
		}
	})

	t.Run("re-authentication", func(t *testing.T) {
		authURL, out := confirm(identity.OperationPasswordChange, fresh("subject-carol"))
		q, _ := url.Parse(authURL)
		if q.Query().Get("max_age") != "0" || q.Query().Get("prompt") != "login" {
			t.Errorf("authorization request = %s, want max_age=0 and prompt=login", authURL)
		}
		token := out.Get("external_confirmation")
		if token == "" {
			t.Fatalf("confirmation outcome = %v", out)
		}
		// The token confirms its operation only.
		if err := svc.DeleteOwnAccount(ctx, carol, identity.Confirmation{External: token}); !errors.Is(err, identity.ErrExternalConfirmationExpired) {
			t.Errorf("DeleteOwnAccount with a password change's token = %v", err)
		}
		_, out = confirm(identity.OperationPasswordChange, fresh("subject-carol"))
		token = out.Get("external_confirmation")
		in := identity.ChangePasswordInput{UserID: carol, Confirmation: identity.Confirmation{External: token}, NewPassword: "another horse battery staple"}
		if err := svc.ChangePassword(ctx, in); err != nil {
			t.Fatalf("ChangePassword = %v", err)
		}
		// It works once.
		if err := svc.ChangePassword(ctx, in); !errors.Is(err, identity.ErrExternalConfirmationExpired) {
			t.Errorf("replayed ChangePassword = %v", err)
		}
		// The change ended the browser's session.
		browser = e.signedInWith("carol@example.com", in.NewPassword)
	})

	t.Run("removing the password and unlinking", func(t *testing.T) {
		_, out := confirm(identity.OperationPasswordRemoval, fresh("subject-carol"))
		if err := svc.RemovePassword(ctx, identity.RemovePasswordInput{UserID: carol, External: out.Get("external_confirmation")}); err != nil {
			t.Fatalf("RemovePassword = %v", err)
		}
		if has, err := svc.HasPassword(ctx, carol); err != nil || has {
			t.Errorf("HasPassword = %v, %v", has, err)
		}
		list, err := svc.LinkedIdentities(ctx, carol)
		if err != nil || len(list.Links) != 1 {
			t.Fatalf("LinkedIdentities = %+v, %v", list, err)
		}
		// The link is carol's last way to sign in.
		if err := svc.UnlinkExternalIdentity(ctx, carol, list.Links[0].LinkID); !errors.Is(err, identity.ErrLastSignInMethod) {
			t.Errorf("UnlinkExternalIdentity of the last way to sign in = %v", err)
		}
		if err := svc.UnlinkExternalIdentity(ctx, carol, "unknown"); !errors.Is(err, identity.ErrNotFound) {
			t.Errorf("UnlinkExternalIdentity of an unknown link = %v", err)
		}
		// With another link, it goes.
		e.link(other, "subject-carol-2", carol)
		if err := svc.UnlinkExternalIdentity(ctx, carol, list.Links[0].LinkID); err != nil {
			t.Errorf("UnlinkExternalIdentity = %v", err)
		}
		if _, err := e.db.IdentityRepository().FindLinkedExternalIdentity(ctx, id, "subject-carol"); !errors.Is(err, identity.ErrNotFound) {
			t.Errorf("FindLinkedExternalIdentity after the unlink = %v", err)
		}
	})
}
