// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/blob"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeOwnAccount deletes accounts and changes passwords confirmed with the password "secret" or
// the passkey confirmation token "good"; the current password "limited" is rate-limited, the
// token "expired" expired, and other tokens do not verify. Removing the password needs a passkey
// of the user, who has one unless noPasskey.
type fakeOwnAccount struct {
	deleted   []string
	changed   []appidentity.ChangePasswordInput
	removed   []appidentity.RemovePasswordInput
	noPasskey bool
}

func (f *fakeOwnAccount) confirm(c appidentity.Confirmation) error {
	if c.Passkey.Token != "" {
		switch c.Passkey.Token {
		case "good":
			return nil
		case "expired":
			return appidentity.ErrPasskeyConfirmationExpired
		}
		return appidentity.ErrInvalidPasskey
	}
	switch c.Password {
	case "limited":
		return &kernel.RateLimitedError{RetryAfter: 90 * time.Second}
	case "secret":
		return nil
	}
	return appidentity.ErrWrongPassword
}

func (f *fakeOwnAccount) HasPassword(context.Context, string) (bool, error) {
	return len(f.removed) == 0, nil
}

func (f *fakeOwnAccount) ChangePassword(_ context.Context, in appidentity.ChangePasswordInput) error {
	if err := f.confirm(in.Confirmation); err != nil {
		return err
	}
	f.changed = append(f.changed, in)
	return nil
}

func (f *fakeOwnAccount) RemovePassword(_ context.Context, in appidentity.RemovePasswordInput) error {
	if err := f.confirm(appidentity.Confirmation{Passkey: in.Passkey}); err != nil {
		return err
	}
	if f.noPasskey {
		return appidentity.ErrLastSignInMethod
	}
	f.removed = append(f.removed, in)
	return nil
}

func (f *fakeOwnAccount) DeleteOwnAccount(_ context.Context, userID string, c appidentity.Confirmation) error {
	if err := f.confirm(c); err != nil {
		return err
	}
	f.deleted = append(f.deleted, userID)
	return nil
}

// fakeExports keeps the exports in memory, newest first; a ready export's archive is its ID.
type fakeExports struct {
	now     time.Time
	exports []identity.DataExport
}

func (f *fakeExports) RequestExport(_ context.Context, userID string) (identity.DataExport, error) {
	for _, e := range f.exports {
		if e.UserID == userID && e.Result == nil {
			return identity.DataExport{}, appidentity.ErrExportInProgress
		}
	}
	e := identity.DataExport{ID: fmt.Sprintf("01890a5d-ac96-774b-bcce-b302099d%04d", len(f.exports)+1), UserID: userID, RequestedAt: f.now}
	f.exports = append([]identity.DataExport{e}, f.exports...)
	return e, nil
}

func (f *fakeExports) ListExports(_ context.Context, userID string) ([]identity.DataExport, error) {
	var out []identity.DataExport
	for _, e := range f.exports {
		if e.UserID == userID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeExports) OpenExport(_ context.Context, userID, id string) (io.ReadCloser, blob.Info, error) {
	for _, e := range f.exports {
		if e.ID != id || e.UserID != userID {
			continue
		}
		if e.State(f.now) != identity.ExportStateReady {
			return nil, blob.Info{}, appidentity.ErrExportNotReady
		}
		return io.NopCloser(strings.NewReader(e.ID)), blob.Info{Size: int64(len(e.ID))}, nil
	}
	return nil, blob.Info{}, appidentity.ErrNotFound
}

func (f *fakeExports) Now() time.Time { return f.now }

func TestDeleteOwnAccount(t *testing.T) {
	own := &fakeOwnAccount{}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.OwnAccount = own })
	alice := userOf(aliceAccountID)

	res := e.sendJSON(t, http.MethodPost, "/accounts/delete", "write", `{"password":"wrong"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeWrongPassword || res.Header.Get("Content-Language") != "ja" {
		t.Errorf("wrong password = %d %+v, want 422 wrong-password in ja", res.StatusCode, p)
	}
	if res := e.sendJSON(t, http.MethodPost, "/accounts/delete", "read", `{"password":"secret"}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("without write:accounts = %d, want 403", res.StatusCode)
	}
	if len(own.deleted) != 0 {
		t.Fatalf("deleted = %v, want none yet", own.deleted)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/delete", "write", `{}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusBadRequest || p.Detail == nil || !strings.Contains(*p.Detail, "password") {
		t.Errorf("no confirmation = %d %+v, want 400 on password", res.StatusCode, p)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/delete", "write", `{"passkey":{"confirmation_token":"bad","credential":{"id":"x"}}}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeInvalidPasskey {
		t.Errorf("wrong passkey = %d %+v, want 422 invalid-passkey", res.StatusCode, p)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/delete", "write", `{"password":"secret"}`)
	if res.StatusCode != http.StatusNoContent || len(own.deleted) != 1 || own.deleted[0] != alice {
		t.Errorf("delete = %d, deleted %v; want 204 and %s", res.StatusCode, own.deleted, alice)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/delete", "write", `{"passkey":{"confirmation_token":"good","credential":{"id":"x"}}}`)
	if res.StatusCode != http.StatusNoContent || len(own.deleted) != 2 {
		t.Errorf("delete with a passkey = %d, deleted %v; want 204", res.StatusCode, own.deleted)
	}
	// Past the limit of attempts, the password is not checked.
	for range clientapi.DeleteAccountLimit.Max {
		e.sendJSON(t, http.MethodPost, "/accounts/delete", "write", `{"password":"wrong"}`)
	}
	if res := e.sendJSON(t, http.MethodPost, "/accounts/delete", "write", `{"password":"secret"}`); res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("past the limit = %d, want 429", res.StatusCode)
	}
}

func TestChangePassword(t *testing.T) {
	own := &fakeOwnAccount{}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.OwnAccount = own })
	alice := userOf(aliceAccountID)

	res := e.sendJSON(t, http.MethodPost, "/accounts/password", "write", `{"current_password":"wrong","new_password":"new password"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeWrongPassword {
		t.Errorf("wrong password = %d %+v, want 422 wrong-password", res.StatusCode, p)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/password", "write", `{"current_password":"secret","new_password":"short"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusBadRequest || p.Detail == nil || !strings.Contains(*p.Detail, "new_password") {
		t.Errorf("short new password = %d %+v, want 400 on new_password", res.StatusCode, p)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/password", "write", `{"current_password":"limited","new_password":"new password"}`)
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") != "90" {
		t.Errorf("rate-limited = %d Retry-After %q, want 429 after 90", res.StatusCode, res.Header.Get("Retry-After"))
	}
	if res := e.sendJSON(t, http.MethodPost, "/accounts/password", "read", `{"current_password":"secret","new_password":"new password"}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("without write:accounts = %d, want 403", res.StatusCode)
	}
	if len(own.changed) != 0 {
		t.Fatalf("changed = %+v, want none yet", own.changed)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/password", "write", `{"current_password":"secret","new_password":"new password"}`)
	if res.StatusCode != http.StatusNoContent || len(own.changed) != 1 {
		t.Fatalf("change = %d, changed %+v; want 204 and one change", res.StatusCode, own.changed)
	}
	if got := own.changed[0]; got.UserID != alice || got.NewPassword != "new password" || got.KeepFamilyID != testkit.NamedID("fam-write") {
		t.Errorf("change = %+v, want alice's new password keeping the request's grant", got)
	}

	// A passkey confirms a change in place of the current password.
	res = e.sendJSON(t, http.MethodPost, "/accounts/password", "write", `{"passkey":{"confirmation_token":"expired","credential":{"id":"x"}},"new_password":"new password"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypePasskeyConfirmationExpired {
		t.Errorf("expired passkey confirmation = %d %+v, want 422 passkey-confirmation-expired", res.StatusCode, p)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/password", "write", `{"passkey":{"confirmation_token":"","credential":{}},"new_password":"new password"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusBadRequest || p.Detail == nil || !strings.Contains(*p.Detail, "passkey") {
		t.Errorf("empty passkey = %d %+v, want 400 on passkey", res.StatusCode, p)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/password", "write", `{"passkey":{"confirmation_token":"good","credential":{"id":"x"}},"new_password":"new password"}`)
	if res.StatusCode != http.StatusNoContent || len(own.changed) != 2 || own.changed[1].Confirmation.Passkey.Token != "good" ||
		string(own.changed[1].Confirmation.Passkey.Response) != `{"id":"x"}` {
		t.Errorf("change with a passkey = %d, changed %+v; want 204 confirmed with the passkey", res.StatusCode, own.changed)
	}
}

func TestRemovePassword(t *testing.T) {
	own := &fakeOwnAccount{}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.OwnAccount = own })
	alice := userOf(aliceAccountID)

	res := e.send(t, http.MethodGet, "/accounts/password", "read-accounts", "")
	if got := decodeJSON[clientapi.PasswordStatus](t, res); res.StatusCode != http.StatusOK || !got.HasPassword ||
		res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("GET /accounts/password = %d %+v, want 200 with a password, no-store", res.StatusCode, got)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/password/remove", "write", `{"passkey":{"confirmation_token":"bad","credential":{"id":"x"}}}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeInvalidPasskey {
		t.Errorf("wrong passkey = %d %+v, want 422 invalid-passkey", res.StatusCode, p)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/password/remove", "write", `{"passkey":{"confirmation_token":"","credential":{}}}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("an empty passkey = %d, want 400", res.StatusCode)
	}
	own.noPasskey = true
	res = e.sendJSON(t, http.MethodPost, "/accounts/password/remove", "write", `{"passkey":{"confirmation_token":"good","credential":{"id":"x"}}}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeLastSignInMethod {
		t.Errorf("without a passkey = %d %+v, want 409 last-sign-in-method", res.StatusCode, p)
	}
	own.noPasskey = false
	if res := e.sendJSON(t, http.MethodPost, "/accounts/password/remove", "read", `{"passkey":{"confirmation_token":"good","credential":{"id":"x"}}}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("without write:accounts = %d, want 403", res.StatusCode)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/password/remove", "write", `{"passkey":{"confirmation_token":"good","credential":{"id":"x"}}}`)
	if res.StatusCode != http.StatusNoContent || len(own.removed) != 1 || own.removed[0].UserID != alice || own.removed[0].KeepFamilyID != testkit.NamedID("fam-write") {
		t.Fatalf("remove = %d, removed %+v; want 204 keeping the request's grant", res.StatusCode, own.removed)
	}
	res = e.send(t, http.MethodGet, "/accounts/password", "read-accounts", "")
	if got := decodeJSON[clientapi.PasswordStatus](t, res); got.HasPassword {
		t.Errorf("GET /accounts/password after the removal = %+v, want no password", got)
	}
}

func TestDataExports(t *testing.T) {
	f := &fakeExports{now: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Exports = f })

	res := e.send(t, http.MethodPost, "/exports", "write", "")
	if res.StatusCode != http.StatusAccepted || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("POST /exports = %d, want 202 no-store", res.StatusCode)
	}
	pending := decodeJSON[clientapi.DataExport](t, res)
	if pending.State != clientapi.DataExportStatePending || !pending.RequestedAt.Equal(f.now) || pending.CompletedAt != nil || pending.FileSize != nil {
		t.Errorf("new export = %+v, want pending", pending)
	}
	res = e.send(t, http.MethodPost, "/exports", "write", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeExportInProgress {
		t.Errorf("second POST /exports = %d %+v, want 409 export-in-progress", res.StatusCode, p)
	}
	archive := "/exports/" + pending.Id + "/archive"
	res = e.send(t, http.MethodGet, archive, "read-accounts", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeExportNotReady {
		t.Errorf("GET archive of a pending export = %d %+v, want 409 export-not-ready", res.StatusCode, p)
	}

	// The build ends.
	f.exports[0].Result = &identity.DataExportResult{
		ExportID: pending.Id, Outcome: identity.ExportReady, BlobKey: "k", FileSize: int64(len(pending.Id)),
		ExpiresAt: f.now.Add(time.Hour), CompletedAt: f.now,
	}
	res = e.send(t, http.MethodGet, "/exports", "read-accounts", "")
	list := decodeJSON[clientapi.DataExportList](t, res)
	if len(list.Items) != 1 || list.Items[0].State != clientapi.DataExportStateReady || list.Items[0].FileSize == nil ||
		*list.Items[0].FileSize != int64(len(pending.Id)) || list.Items[0].ExpiresAt == nil {
		t.Errorf("GET /exports = %+v, want the ready export", list)
	}
	res = e.send(t, http.MethodGet, archive, "read-accounts", "")
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/zip" || string(body) != pending.Id ||
		res.Header.Get("Content-Disposition") != `attachment; filename="export-`+pending.Id+`.zip"` {
		t.Errorf("GET archive = %d %q %v, want the archive", res.StatusCode, body, res.Header)
	}
	if res := e.send(t, http.MethodGet, archive, "bob", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET another user's archive = %d, want 404", res.StatusCode)
	}

	// An expired export is left out of the list.
	f.now = f.now.Add(time.Hour)
	res = e.send(t, http.MethodGet, "/exports", "read-accounts", "")
	if list := decodeJSON[clientapi.DataExportList](t, res); len(list.Items) != 0 {
		t.Errorf("GET /exports after the expiry = %+v, want none", list)
	}
	if res := e.send(t, http.MethodGet, "/exports", "follow-push", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("GET /exports without read:accounts = %d, want 403", res.StatusCode)
	}
}

// fakeVerificationEmails answers the requests for verification emails with err, and records
// the users of the ones it queues.
type fakeVerificationEmails struct {
	err  error
	sent []string
}

func (f *fakeVerificationEmails) ResendVerificationEmail(_ context.Context, userID string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, userID)
	return nil
}

func TestResendVerificationEmail(t *testing.T) {
	const path = "/accounts/email/verification"
	emails := &fakeVerificationEmails{}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.VerificationEmails, o.SendsEmail = emails, true })

	if res := e.send(t, http.MethodPost, path, "read", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("without write:accounts = %d, want 403", res.StatusCode)
	}
	if res := e.send(t, http.MethodPost, path, "write", ""); res.StatusCode != http.StatusAccepted ||
		len(emails.sent) != 1 || emails.sent[0] != userOf(aliceAccountID) {
		t.Fatalf("send = %d, sent %v; want 202 and one email to alice", res.StatusCode, emails.sent)
	}

	emails.err = &kernel.RateLimitedError{RetryAfter: 90 * time.Second}
	if res := e.send(t, http.MethodPost, path, "write", ""); res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") != "90" {
		t.Errorf("rate-limited = %d Retry-After %q, want 429 after 90", res.StatusCode, res.Header.Get("Retry-After"))
	}
	emails.err = appidentity.ErrAlreadyVerified
	res := e.send(t, http.MethodPost, path, "write", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeEmailAlreadyVerified {
		t.Errorf("verified address = %d %+v, want 409 email-already-verified", res.StatusCode, p)
	}
	emails.err = appidentity.ErrNotFound
	if res := e.send(t, http.MethodPost, path, "write", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("no address = %d, want 404", res.StatusCode)
	}

	// An instance that sends no email answers 404.
	emails.err = nil
	e = newAuthEnv(t, func(o *clientapi.Options) { o.VerificationEmails = emails })
	if res := e.send(t, http.MethodPost, path, "write", ""); res.StatusCode != http.StatusNotFound || len(emails.sent) != 1 {
		t.Errorf("without email = %d, sent %v; want 404 and no email", res.StatusCode, emails.sent)
	}
}
