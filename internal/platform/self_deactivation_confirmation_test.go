// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/base32"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/account"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

// lifecycleOf returns the state of the account of a username.
func (e *deactivationEnv) lifecycleOf(username string) account.LifecycleStateName {
	e.t.Helper()
	_, id := e.userOf(username)
	a, err := e.app.Accounts.FindByID(e.t.Context(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return a.Lifecycle.State
}

// deactivationMails counts the emails that state the date of the deletion.
func (e *deactivationEnv) deactivationMails(due time.Time) int {
	n := 0
	for _, m := range e.mailer.Sent() {
		if strings.Contains(m.Text, deletionDate(due)) {
			n++
		}
	}
	return n
}

// TestSelfDeactivationConfirmedByASecondFactor enables TOTP through the client API and confirms the
// deactivation with a code of the authenticator app and with a recovery code, on the real use
// cases and database; a wrong code, the code that enabled TOTP, and a spent recovery code change
// nothing. The sign-in after it asks for the second factor before it reaches the status page.
func TestSelfDeactivationConfirmedByASecondFactor(t *testing.T) {
	t.Parallel()
	e := newDeactivationEnv(t)

	var setup clientapi.TotpSetup
	if res := e.do("alice", http.MethodPost, "/two_factor/totp/setup", nil, &setup); res.StatusCode != http.StatusOK {
		t.Fatalf("POST /two_factor/totp/setup = %d", res.StatusCode)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup.Secret)
	if err != nil {
		t.Fatal(err)
	}
	code := func() string {
		return domainidentity.TOTPCode(secret, domainidentity.TOTPStep(e.clock.Now()))
	}
	enabling := code()
	var enabled clientapi.RecoveryCodes
	if res := e.do("alice", http.MethodPost, "/two_factor/totp", map[string]any{"setup_token": setup.SetupToken, "code": enabling}, &enabled); res.StatusCode != http.StatusOK {
		t.Fatalf("POST /two_factor/totp = %d", res.StatusCode)
	}
	if len(enabled.RecoveryCodes) < 2 {
		t.Fatalf("recovery codes = %v", enabled.RecoveryCodes)
	}

	refused := func(what, code string) {
		t.Helper()
		if p := e.problemOf("alice", http.MethodPost, "/accounts/deactivation", map[string]any{"two_factor_code": code}, http.StatusUnprocessableEntity); p != clientapi.ProblemTypeInvalidTwoFactorCode {
			t.Errorf("%s: %s", what, p)
		}
		if st := e.lifecycleOf("alice"); st != account.StateActive {
			t.Errorf("%s: the state = %s, want active", what, st)
		}
	}
	refused("a wrong code", "000000")
	refused("the code that enabled TOTP", enabling)

	e.clock.Advance(domainidentity.TOTPPeriod)
	var d clientapi.Deactivation
	if res := e.do("alice", http.MethodPost, "/accounts/deactivation", map[string]any{"two_factor_code": code()}, &d); res.StatusCode != http.StatusOK {
		t.Fatalf("deactivate with a code = %d", res.StatusCode)
	}
	if st := e.lifecycleOf("alice"); st != account.StateDeactivated {
		t.Errorf("the state after a code = %s", st)
	}
	if n := e.deactivationMails(d.DeletionDueAt); n != 1 {
		t.Errorf("%d emails state the date after a code, want 1", n)
	}
	if res := e.do("alice", http.MethodGet, "/accounts/verify_credentials", nil, nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("the token after a code = %d, want 401", res.StatusCode)
	}

	// The sign-in goes through the second factor to the status page, which brings the account back.
	s := e.screens()
	s.signIn("alice")
	if strings.Contains(s.page(), authpages.PathReactivate) {
		t.Error("the password alone reached the status page")
	}
	e.clock.Advance(domainidentity.TOTPPeriod)
	s = e.screens()
	s.signIn("alice")
	if rec := s.do(http.MethodPost, authpages.PathSignInTwoFactor, s.form(authpages.PathSignInTwoFactor, "code", code())); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST the second factor = %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(s.page(), authpages.PathReactivate) {
		t.Error("the second factor did not reach the status page")
	}
	if rec := s.do(http.MethodPost, authpages.PathReactivate, s.form(authpages.PathSignIn)); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST reactivate = %d %s", rec.Code, rec.Body)
	}

	// A recovery code confirms it too, once.
	e.clock.Advance(account.DeactivationGap)
	e.freshToken("alice")
	recovery := enabled.RecoveryCodes[0]
	if res := e.do("alice", http.MethodPost, "/accounts/deactivation", map[string]any{"two_factor_code": recovery}, &d); res.StatusCode != http.StatusOK {
		t.Fatalf("deactivate with a recovery code = %d", res.StatusCode)
	}
	if st := e.lifecycleOf("alice"); st != account.StateDeactivated {
		t.Errorf("the state after a recovery code = %s", st)
	}
	s = e.screens()
	s.signIn("alice")
	if rec := s.do(http.MethodPost, authpages.PathSignInTwoFactor, s.form(authpages.PathSignInTwoFactor, "code", enabled.RecoveryCodes[1])); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST the second factor = %d %s", rec.Code, rec.Body)
	}
	if rec := s.do(http.MethodPost, authpages.PathReactivate, s.form(authpages.PathSignIn)); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST reactivate = %d %s", rec.Code, rec.Body)
	}
	e.clock.Advance(account.DeactivationGap)
	e.freshToken("alice")
	refused("a spent recovery code", recovery)
}

// marshal returns v as JSON.
func (e *deactivationEnv) marshal(v any) []byte {
	e.t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		e.t.Fatal(err)
	}
	return b
}

// addPasskey registers a software authenticator on a user's account through the client API.
func (e *deactivationEnv) addPasskey(user string) *identitytest.SoftwarePasskey {
	e.t.Helper()
	var begun clientapi.PasskeyRegistration
	if res := e.do(user, http.MethodPost, "/passkeys/registration", map[string]any{"name": "phone", "password": deactivationPassword}, &begun); res.StatusCode != http.StatusOK {
		e.t.Fatalf("POST /passkeys/registration = %d", res.StatusCode)
	}
	key := identitytest.NewSoftwarePasskey(e.t, "example.test", "https://example.test")
	if res := e.do(user, http.MethodPost, "/passkeys", map[string]any{
		"registration_token": begun.RegistrationToken, "credential": json.RawMessage(key.Register(e.t, e.marshal(begun.Options))),
	}, nil); res.StatusCode != http.StatusCreated {
		e.t.Fatalf("POST /passkeys = %d", res.StatusCode)
	}
	return key
}

// passkeyBody asks the server for a confirmation as user, and answers it with key.
func (e *deactivationEnv) passkeyBody(user string, key *identitytest.SoftwarePasskey) map[string]any {
	e.t.Helper()
	var c clientapi.PasskeyConfirmation
	if res := e.do(user, http.MethodPost, "/passkeys/confirmation", nil, &c); res.StatusCode != http.StatusOK {
		e.t.Fatalf("POST /passkeys/confirmation = %d", res.StatusCode)
	}
	userID, _ := e.userOf(user)
	return map[string]any{"passkey": map[string]any{
		"confirmation_token": c.ConfirmationToken,
		"credential":         json.RawMessage(key.SignIn(e.t, e.marshal(c.Options), []byte(userID))),
	}}
}

// TestSelfDeactivationConfirmedByAPasskey registers a passkey and confirms the deactivation with it
// through the client API, on the real WebAuthn ceremonies with a software authenticator. A key that
// is not registered, another user's key or confirmation, an answer from another origin, and a
// confirmation used once change nothing.
func TestSelfDeactivationConfirmedByAPasskey(t *testing.T) {
	t.Parallel()
	e := newDeactivationEnv(t)
	key := e.addPasskey("alice")
	bobKey := e.addPasskey("bob")

	refused := func(what string, body map[string]any, want string) {
		t.Helper()
		if p := e.problemOf("alice", http.MethodPost, "/accounts/deactivation", body, http.StatusUnprocessableEntity); p != want {
			t.Errorf("%s: %s, want %s", what, p, want)
		}
		if st := e.lifecycleOf("alice"); st != account.StateActive {
			t.Errorf("%s: the state = %s, want active", what, st)
		}
	}
	refused("a key that is not registered", e.passkeyBody("alice", identitytest.NewSoftwarePasskey(t, "example.test", "https://example.test")), clientapi.ProblemTypeInvalidPasskey)
	refused("bob's key", e.passkeyBody("alice", bobKey), clientapi.ProblemTypeInvalidPasskey)
	refused("a confirmation of bob's", e.passkeyBody("bob", bobKey), clientapi.ProblemTypePasskeyConfirmationExpired)
	refused("an answer from another origin", e.passkeyBody("alice", key.At("https://evil.example")), clientapi.ProblemTypeInvalidPasskey)

	// The refusals above count against the limit of the attempts of the hour.
	e.clock.Advance(time.Hour)
	e.freshToken("alice")
	body := e.passkeyBody("alice", key)
	var d clientapi.Deactivation
	if res := e.do("alice", http.MethodPost, "/accounts/deactivation", body, &d); res.StatusCode != http.StatusOK {
		t.Fatalf("deactivate with a passkey = %d", res.StatusCode)
	}
	if st := e.lifecycleOf("alice"); st != account.StateDeactivated {
		t.Errorf("the state after a passkey = %s", st)
	}
	if n := e.deactivationMails(d.DeletionDueAt); n != 1 {
		t.Errorf("%d emails state the date, want 1", n)
	}

	// The confirmation works once: back again, the same one is spent.
	e.reactivate("alice")
	e.clock.Advance(account.DeactivationGap)
	e.freshToken("alice")
	refused("a confirmation used once", body, clientapi.ProblemTypePasskeyConfirmationExpired)
}

// startConfirmation asks the client API to confirm op with the provider, answers as the provider
// with claims, and returns the outcome in the fragment.
func startConfirmation(e *externalEnv, browser *authClient, token, providerID string, op identity.ConfirmedOperation, claims map[string]any) url.Values {
	e.t.Helper()
	code, body := e.jsonRequest(http.MethodPost, "/api/v1/linked_identities/confirmation", token,
		`{"provider_id":"`+providerID+`","operation":"`+string(op)+`","return_to":"/settings/account"}`)
	if code != http.StatusOK {
		e.t.Fatalf("POST /linked_identities/confirmation = %d %s", code, body)
	}
	var start struct {
		StartURL string `json:"start_url"`
	}
	if err := json.Unmarshal([]byte(body), &start); err != nil {
		e.t.Fatal(err)
	}
	_, out := e.runIntent(browser, identity.ExternalIntent{StartURL: start.StartURL}, providerID, claims)
	return out
}

// TestSelfDeactivationConfirmedByALinkedProvider removes the password of a user with a link to a
// provider and confirms the deactivation by signing in again there, through the client API and the
// fake issuer: the outcome names the operation and the provider, and the status page that the
// provider's sign-in reaches asks for no password to delete the account.
func TestSelfDeactivationConfirmedByALinkedProvider(t *testing.T) {
	t.Parallel()
	e := newExternalEnvWith(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	ctx := t.Context()
	svc := e.s.app.Identity
	e.user("root", "root@example.com") // the administrator who stays
	carol := e.user("carol", "carol@example.com")
	id := e.provider("IdP", false)
	e.link(id, "subject-carol", carol)
	browser := e.signedIn("carol@example.com")
	token := issueTokens(t, e.s.app, carol, "").AccessToken
	fresh := func(sub string) map[string]any { return map[string]any{"sub": sub, "auth_time": time.Now().Unix()} }

	removal := startConfirmation(e, browser, token, id, identity.OperationPasswordRemoval, fresh("subject-carol"))
	if err := svc.RemovePassword(ctx, identity.RemovePasswordInput{UserID: carol, External: removal.Get("external_confirmation")}); err != nil {
		t.Fatalf("RemovePassword = %v", err)
	}
	// The removal ended the other sessions and tokens.
	token = issueTokens(t, e.s.app, carol, "").AccessToken
	browser = e.client()
	if resp, b := e.signInWith(browser, id, map[string]any{"sub": "subject-carol"}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sign-in at the provider = %d\n%s", resp.StatusCode, b)
	}

	state := func() account.LifecycleStateName {
		t.Helper()
		st, err := e.s.app.Accounts.UserLifecycle(ctx, carol)
		if err != nil {
			t.Fatal(err)
		}
		return st.State
	}
	deactivate := func(body string) (int, string) {
		t.Helper()
		return e.jsonRequest(http.MethodPost, "/api/v1/accounts/deactivation", token, body)
	}

	// Another account at the provider gives no confirmation.
	if out := startConfirmation(e, browser, token, id, identity.OperationAccountDeactivation, fresh("subject-someone")); out.Get("external_error") != "mismatch" || out.Get("external_confirmation") != "" {
		t.Errorf("a confirmation with another account = %v", out)
	}
	// One for another operation does not deactivate, and a made-up one neither.
	other := startConfirmation(e, browser, token, id, identity.OperationPasswordChange, fresh("subject-carol"))
	for what, tok := range map[string]string{"for another operation": other.Get("external_confirmation"), "made up": "not-a-token"} {
		code, body := deactivate(`{"external_confirmation":"` + tok + `"}`)
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, clientapi.ProblemTypeExternalConfirmationExpired) {
			t.Errorf("a confirmation %s = %d %s", what, code, body)
		}
		if st := state(); st != account.StateActive {
			t.Errorf("a confirmation %s: the state = %s", what, st)
		}
	}

	out := startConfirmation(e, browser, token, id, identity.OperationAccountDeactivation, fresh("subject-carol"))
	if out.Get("operation") != string(identity.OperationAccountDeactivation) || out.Get("provider") != id || out.Get("external_confirmation") == "" {
		t.Fatalf("the outcome = %v", out)
	}
	code, body := deactivate(`{"external_confirmation":"` + out.Get("external_confirmation") + `"}`)
	if code != http.StatusOK || !strings.Contains(body, "deletion_due_at") {
		t.Fatalf("deactivate with the provider = %d %s", code, body)
	}
	if st := state(); st != account.StateDeactivated {
		t.Errorf("the state = %s, want deactivated", st)
	}
	if code, _ := e.jsonRequest(http.MethodGet, "/api/v1/accounts/verify_credentials", token, ""); code != http.StatusUnauthorized {
		t.Errorf("the token after it = %d, want 401", code)
	}

	// A sign-in at the provider reaches the status page, which asks for no password, and the
	// sign-in confirms the deletion.
	c := e.client()
	if resp, b := e.signInWith(c, id, map[string]any{"sub": "subject-carol"}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sign-in at the provider = %d\n%s", resp.StatusCode, b)
	}
	_, page := c.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	if !strings.Contains(page, authpages.PathReactivate) || strings.Contains(page, `name="password"`) {
		t.Errorf("the status page of a user without a password:\n%s", page)
	}
	if resp, b := c.do(http.MethodPost, authpages.PathDeleteDeactivated, c.formAt(authpages.PathSignIn), nil); resp.StatusCode != http.StatusOK {
		t.Errorf("POST delete = %d\n%s", resp.StatusCode, b)
	}
	if st := state(); st != account.StateDeleted {
		t.Errorf("the state after the deletion = %s, want deleted", st)
	}
}
