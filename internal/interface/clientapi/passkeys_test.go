// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakePasskeys are the passkeys of alice: password "secret" or the passkey confirmation token
// "conf-1" confirms a registration, token "reg-1" is a valid registration, and a credential
// {"id":"ok"} verifies. err, when set, fails every registration. noPassword makes the last
// passkey the last way to sign in.
type fakePasskeys struct {
	list       []identity.PasskeyCredential
	err        error
	calls      []string
	response   []byte
	noPassword bool
}

func (f *fakePasskeys) BeginPasskeyConfirmation(_ context.Context, userID string) (appidentity.PasskeyConfirmation, error) {
	f.calls = append(f.calls, "confirm "+userID)
	return appidentity.PasskeyConfirmation{
		Options: json.RawMessage(`{"challenge":"Y29uZmlybQ","rpId":"social.example"}`), Token: "conf-1", ExpiresAt: passkeyCreatedAt,
	}, nil
}

var passkeyCreatedAt = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func (f *fakePasskeys) Passkeys(_ context.Context, userID string) ([]identity.PasskeyCredential, error) {
	f.calls = append(f.calls, "list "+userID)
	return f.list, nil
}

func (f *fakePasskeys) BeginPasskeyRegistration(_ context.Context, userID, name string, c appidentity.Confirmation) (appidentity.PasskeyRegistration, error) {
	f.calls = append(f.calls, "begin "+userID+" "+name)
	switch {
	case f.err != nil:
		return appidentity.PasskeyRegistration{}, f.err
	case name == "":
		return appidentity.PasskeyRegistration{}, identity.ErrInvalidPasskeyName
	case c.Passkey.Token != "" && c.Passkey.Token != "conf-1":
		return appidentity.PasskeyRegistration{}, appidentity.ErrPasskeyConfirmationExpired
	case c.Passkey.Token == "" && c.Password != "secret":
		return appidentity.PasskeyRegistration{}, appidentity.ErrWrongPassword
	}
	return appidentity.PasskeyRegistration{
		Options: json.RawMessage(`{"challenge":"Y2hhbGxlbmdl","rp":{"id":"social.example"}}`), Token: "reg-1", ExpiresAt: passkeyCreatedAt,
	}, nil
}

func (f *fakePasskeys) FinishPasskeyRegistration(_ context.Context, userID, token string, response []byte) (identity.PasskeyCredential, error) {
	f.calls = append(f.calls, "finish "+userID)
	f.response = response
	var r struct{ ID string }
	_ = json.Unmarshal(response, &r)
	switch {
	case f.err != nil:
		return identity.PasskeyCredential{}, f.err
	case token != "reg-1":
		return identity.PasskeyCredential{}, appidentity.ErrPasskeyCeremonyExpired
	case r.ID != "ok":
		return identity.PasskeyCredential{}, appidentity.ErrInvalidPasskey
	}
	c := identity.PasskeyCredential{ID: "019a0000-0000-7000-8000-0000000000a1", UserID: userID, Name: "phone", CreatedAt: passkeyCreatedAt}
	f.list = append(f.list, c)
	return c, nil
}

func (f *fakePasskeys) RemovePasskey(_ context.Context, userID, id string) error {
	f.calls = append(f.calls, "remove "+userID+" "+id)
	for i, c := range f.list {
		if c.ID == id {
			if f.noPassword && len(f.list) == 1 {
				return appidentity.ErrLastSignInMethod
			}
			f.list = slices.Delete(f.list, i, i+1)
			return nil
		}
	}
	return appidentity.ErrNotFound
}

func newPasskeyEnv(t *testing.T) (*authEnv, *fakePasskeys) {
	t.Helper()
	f := &fakePasskeys{}
	return newAuthEnv(t, func(o *clientapi.Options) { o.Passkeys = f }), f
}

func TestPasskeyRegistration(t *testing.T) {
	e, f := newPasskeyEnv(t)
	res := e.send(t, http.MethodGet, "/passkeys", "read-accounts", "")
	if list := decodeJSON[clientapi.PasskeyList](t, res); res.StatusCode != http.StatusOK || len(list.Items) != 0 ||
		res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("list = %d %+v, want none", res.StatusCode, list)
	}

	for _, tc := range []struct {
		body, typ string
		status    int
	}{
		{`{"name":"phone","password":"wrong"}`, clientapi.ProblemTypeWrongPassword, http.StatusUnprocessableEntity},
		{`{"name":"phone","password":""}`, clientapi.ProblemTypeBlank, http.StatusUnprocessableEntity},
		{`{"name":"phone"}`, clientapi.ProblemTypeBlank, http.StatusUnprocessableEntity},
		{`{"name":"phone","passkey":{"confirmation_token":"old","credential":{"id":"ok"}}}`, clientapi.ProblemTypePasskeyConfirmationExpired, http.StatusUnprocessableEntity},
	} {
		res := e.send(t, http.MethodPost, "/passkeys/registration", "write", tc.body)
		if p := decodeProblem(t, res); res.StatusCode != tc.status || p.Type != tc.typ {
			t.Errorf("begin %s = %d %+v, want %d %s", tc.body, res.StatusCode, p, tc.status, tc.typ)
		}
	}
	res = e.send(t, http.MethodPost, "/passkeys/registration", "write", `{"name":"phone","password":"secret"}`)
	reg := decodeJSON[clientapi.PasskeyRegistration](t, res)
	if res.StatusCode != http.StatusOK || reg.RegistrationToken != "reg-1" || reg.Options["challenge"] != "Y2hhbGxlbmdl" ||
		!reg.ExpiresAt.Equal(passkeyCreatedAt) || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("begin = %d %+v", res.StatusCode, reg)
	}

	for _, tc := range []struct {
		body, typ string
		status    int
	}{
		{`{"registration_token":"reg-1","credential":{"id":"bad"}}`, clientapi.ProblemTypeInvalidPasskey, http.StatusUnprocessableEntity},
		{`{"registration_token":"old","credential":{"id":"ok"}}`, clientapi.ProblemTypePasskeyRegistrationExpired, http.StatusUnprocessableEntity},
		{`{"registration_token":"reg-1","credential":{}}`, clientapi.ProblemTypeBlank, http.StatusUnprocessableEntity},
	} {
		res := e.send(t, http.MethodPost, "/passkeys", "write", tc.body)
		if p := decodeProblem(t, res); res.StatusCode != tc.status || p.Type != tc.typ {
			t.Errorf("create %s = %d %+v, want %d %s", tc.body, res.StatusCode, p, tc.status, tc.typ)
		}
	}
	res = e.send(t, http.MethodPost, "/passkeys", "write", `{"registration_token":"reg-1","credential":{"id":"ok","response":{"clientDataJSON":"e30"}}}`)
	if p := decodeJSON[clientapi.Passkey](t, res); res.StatusCode != http.StatusCreated || p.Name != "phone" || !p.CreatedAt.Equal(passkeyCreatedAt) {
		t.Errorf("create = %d %+v", res.StatusCode, p)
	}
	var sent map[string]any
	if err := json.Unmarshal(f.response, &sent); err != nil || sent["id"] != "ok" || sent["response"] == nil {
		t.Errorf("the credential reached the identity module as %s, %v", f.response, err)
	}
	res = e.send(t, http.MethodGet, "/passkeys", "read-accounts", "")
	if list := decodeJSON[clientapi.PasskeyList](t, res); len(list.Items) != 1 || list.Items[0].Id != "019a0000-0000-7000-8000-0000000000a1" {
		t.Errorf("list after registering = %+v", list)
	}
	if want := "begin " + userOf(aliceAccountID) + " phone"; !slices.Contains(f.calls, want) {
		t.Errorf("calls = %v, want %q", f.calls, want)
	}

	f.err = appidentity.ErrTooManyPasskeys
	res = e.send(t, http.MethodPost, "/passkeys/registration", "write", `{"name":"phone","password":"secret"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeTooManyPasskeys {
		t.Errorf("begin over the limit = %d %+v, want 409", res.StatusCode, p)
	}
	f.err = &kernel.RateLimitedError{RetryAfter: 90 * time.Second}
	res = e.send(t, http.MethodPost, "/passkeys/registration", "write", `{"name":"phone","password":"secret"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") != "90" {
		t.Errorf("rate limited = %d %+v (Retry-After %q), want 429 after 90 s", res.StatusCode, p, res.Header.Get("Retry-After"))
	}
	f.err = identity.ErrInvalidPasskeyName
	res = e.send(t, http.MethodPost, "/passkeys/registration", "write", `{"name":"phone","password":"secret"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeBlank {
		t.Errorf("begin with a bad name = %d %+v, want 400", res.StatusCode, p)
	}
	res = e.send(t, http.MethodPost, "/passkeys/registration", "read-accounts", `{"name":"phone","password":"secret"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
		t.Errorf("begin with read:accounts = %d %+v, want 403", res.StatusCode, p)
	}
}

func TestPasskeyConfirmation(t *testing.T) {
	e, f := newPasskeyEnv(t)
	res := e.send(t, http.MethodPost, "/passkeys/confirmation", "write", "")
	c := decodeJSON[clientapi.PasskeyConfirmation](t, res)
	if res.StatusCode != http.StatusOK || c.ConfirmationToken != "conf-1" || c.Options["challenge"] != "Y29uZmlybQ" ||
		!c.ExpiresAt.Equal(passkeyCreatedAt) || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("confirmation = %d %+v", res.StatusCode, c)
	}
	if want := "confirm " + userOf(aliceAccountID); !slices.Contains(f.calls, want) {
		t.Errorf("calls = %v, want %q", f.calls, want)
	}
	res = e.send(t, http.MethodPost, "/passkeys/registration", "write", `{"name":"phone","passkey":{"confirmation_token":"conf-1","credential":{"id":"ok"}}}`)
	if reg := decodeJSON[clientapi.PasskeyRegistration](t, res); res.StatusCode != http.StatusOK || reg.RegistrationToken != "reg-1" {
		t.Errorf("begin a registration confirmed with a passkey = %d %+v", res.StatusCode, reg)
	}
	if res := e.send(t, http.MethodPost, "/passkeys/confirmation", "read-accounts", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("confirmation with read:accounts = %d, want 403", res.StatusCode)
	}
}

func TestPasskeyRemoval(t *testing.T) {
	e, f := newPasskeyEnv(t)
	f.list = []identity.PasskeyCredential{{ID: "019a0000-0000-7000-8000-0000000000a1", Name: "phone", CreatedAt: passkeyCreatedAt}}
	res := e.send(t, http.MethodDelete, "/passkeys/019a0000-0000-7000-8000-0000000000a2", "write", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusNotFound {
		t.Errorf("remove unknown = %d %+v, want 404", res.StatusCode, p)
	}
	f.noPassword = true
	res = e.send(t, http.MethodDelete, "/passkeys/019a0000-0000-7000-8000-0000000000a1", "write", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeLastSignInMethod {
		t.Errorf("remove the last way to sign in = %d %+v, want 409 last-sign-in-method", res.StatusCode, p)
	}
	f.noPassword = false
	res = e.send(t, http.MethodDelete, "/passkeys/019a0000-0000-7000-8000-0000000000a1", "write", "")
	if res.StatusCode != http.StatusNoContent || len(f.list) != 0 {
		t.Errorf("remove = %d, list %+v; want 204 and none", res.StatusCode, f.list)
	}
}

func TestPasskeysWithoutTheFeature(t *testing.T) {
	e := newAuthEnv(t, func(*clientapi.Options) {})
	if res := e.send(t, http.MethodGet, "/passkeys", "read-accounts", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("list without passkeys = %d, want 404", res.StatusCode)
	}
}
