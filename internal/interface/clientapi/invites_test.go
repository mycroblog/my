// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeOwnInvites keeps one member's invites newest first; err fails the next creation, and
// refusal is why nobody may create one.
type fakeOwnInvites struct {
	list    []appidentity.InviteStatus
	creates []appidentity.CreateOwnInviteInput
	revokes []string
	err     error
	refusal identity.InviteRefusal
}

func (f *fakeOwnInvites) ListOwnInvites(_ context.Context, userID, before string, limit int) (appidentity.OwnInvites, error) {
	out := appidentity.OwnInvites{
		MayCreate: userID == userOf(aliceAccountID) && f.refusal == "", Refusal: f.refusal, Usable: len(f.list),
		Caps: identity.MemberInviteCaps{MaxActive: 5, MaxUses: 10, MaxExpiresIn: 30 * 24 * time.Hour},
	}
	for _, st := range f.list {
		if st.CreatedByUserID == userID && (before == "" || st.ID < before) && len(out.Invites) < limit {
			out.Invites = append(out.Invites, st)
		}
	}
	return out, nil
}

func (f *fakeOwnInvites) CreateOwnInvite(_ context.Context, in appidentity.CreateOwnInviteInput) (appidentity.InviteStatus, error) {
	if f.err != nil {
		return appidentity.InviteStatus{}, f.err
	}
	f.creates = append(f.creates, in)
	// The service's defaults: one use, a week.
	if in.MaxUses == 0 {
		in.MaxUses = 1
	}
	if in.ExpiresIn == 0 {
		in.ExpiresIn = 7 * 24 * time.Hour
	}
	at := adminNow.Add(in.ExpiresIn)
	st := appidentity.InviteStatus{Invite: identity.Invite{
		ID: fmt.Sprintf("01890a5d-ac96-774b-bcce-b302099a8%03d", len(f.creates)), Code: "QRSTUVWXYZ234567",
		CreatedByUserID: in.UserID, CreatorKind: identity.InviteByMember, MaxUses: in.MaxUses, ExpiresAt: &at, CreatedAt: adminNow,
	}, State: appidentity.InviteUsable}
	f.list = append([]appidentity.InviteStatus{st}, f.list...)
	return st, nil
}

func (f *fakeOwnInvites) RevokeOwnInvite(_ context.Context, userID, id string) (appidentity.InviteStatus, error) {
	f.revokes = append(f.revokes, id)
	for i, st := range f.list {
		if st.ID == id && st.CreatedByUserID == userID {
			f.list[i].Revoked, f.list[i].State = true, appidentity.InviteRevoked
			return f.list[i], nil
		}
	}
	return appidentity.InviteStatus{}, appidentity.ErrNotFound
}

func newOwnInvitesEnv(t *testing.T) (*authEnv, *fakeOwnInvites) {
	t.Helper()
	f := &fakeOwnInvites{}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.OwnInvites, o.SignUpPath = f, "/auth/sign_up" })
	e.auth.tokens["read"] = bearer("read:accounts", aliceAccountID)
	e.auth.tokens["write"] = bearer("write:accounts", aliceAccountID)
	e.auth.tokens["bob-write"] = bearer("write:accounts", bobAccountID)
	return e, f
}

// The body of a new invite is optional, since every property of it is.
func TestOwnInviteWithoutABody(t *testing.T) {
	e, _ := newOwnInvitesEnv(t)
	if res := e.send(t, http.MethodPost, "/invites", "write", ""); res.StatusCode != http.StatusCreated {
		t.Errorf("an invite without a body = %d, want 201", res.StatusCode)
	}
}

// A member creates, lists, and revokes their own invites (ADR-0106 §18).
func TestOwnInvites(t *testing.T) {
	e, f := newOwnInvitesEnv(t)
	res := e.sendJSON(t, http.MethodPost, "/invites", "write", `{"max_uses":3,"expires_in":3600}`)
	got := decodeJSON[clientapi.OwnInvite](t, res)
	if res.StatusCode != http.StatusCreated || got.Code != "QRSTUVWXYZ234567" || got.Url != "https://social.example/auth/sign_up?invite=QRSTUVWXYZ234567" ||
		got.MaxUses != 3 || got.Uses != 0 || got.State != clientapi.AdminInviteStateUsable || !got.ExpiresAt.Equal(adminNow.Add(time.Hour)) {
		t.Fatalf("create = %d %+v", res.StatusCode, got)
	}
	want := appidentity.CreateOwnInviteInput{UserID: userOf(aliceAccountID), MaxUses: 3, ExpiresIn: time.Hour}
	if len(f.creates) != 1 || f.creates[0] != want {
		t.Errorf("creates = %+v, want %+v", f.creates, want)
	}
	// Without values, the service picks the defaults.
	e.sendJSON(t, http.MethodPost, "/invites", "write", `{}`)
	if f.creates[1] != (appidentity.CreateOwnInviteInput{UserID: userOf(aliceAccountID)}) {
		t.Errorf("defaults = %+v", f.creates[1])
	}

	res = e.send(t, http.MethodGet, "/invites?limit=1", "read", "")
	page := decodeJSON[clientapi.OwnInvitePage](t, res)
	wantLimits := clientapi.MemberInviteLimits{MaxActive: 5, MaxUses: 10, MaxExpiresIn: 30 * 86400, DefaultMaxUses: 1, DefaultExpiresIn: 7 * 86400}
	if res.StatusCode != http.StatusOK || !page.CanCreate || page.Usable != 2 || page.Limits != wantLimits || len(page.Items) != 1 ||
		page.Items[0].Id == got.Id || page.NextCursor == nil {
		t.Fatalf("first page = %d %+v", res.StatusCode, page)
	}
	page = decodeJSON[clientapi.OwnInvitePage](t, e.send(t, http.MethodGet, "/invites?limit=1&cursor="+*page.NextCursor, "read", ""))
	if len(page.Items) != 1 || page.Items[0].Id != got.Id || page.NextCursor != nil {
		t.Errorf("second page = %+v", page)
	}

	res = e.send(t, http.MethodPost, "/invites/"+got.Id+"/revoke", "bob-write", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("bob revoking alice's invite = %d, want 404", res.StatusCode)
	}
	res = e.send(t, http.MethodPost, "/invites/"+got.Id+"/revoke", "write", "")
	if revoked := decodeJSON[clientapi.OwnInvite](t, res); res.StatusCode != http.StatusOK || revoked.State != clientapi.AdminInviteStateRevoked {
		t.Errorf("revoke = %d %+v", res.StatusCode, revoked)
	}
	// Reading needs no write scope, and creating needs one.
	if res := e.send(t, http.MethodPost, "/invites", "read", `{}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("create with read:accounts = %d, want 403", res.StatusCode)
	}
}

// The refusals of the service are problems (ADR-0106 §14, §15).
func TestOwnInviteProblems(t *testing.T) {
	e, f := newOwnInvitesEnv(t)
	for _, tc := range []struct {
		err    error
		body   string
		status int
		typ    string
		field  string
	}{
		{appidentity.ErrInvitesNotAllowed, `{}`, http.StatusForbidden, clientapi.ProblemTypeInvitesNotAllowed, ""},
		{appidentity.ErrTooManyInvites, `{}`, http.StatusConflict, clientapi.ProblemTypeTooManyInvites, ""},
		{identity.ErrInviteUsesOverCap, `{"max_uses":11}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeBlank, "max_uses"},
		{identity.ErrInviteExpiryOverCap, `{"expires_in":31536000}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeBlank, "expires_in"},
	} {
		f.err = tc.err
		res := e.sendJSON(t, http.MethodPost, "/invites", "write", tc.body)
		p := decodeProblem(t, res)
		if res.StatusCode != tc.status || p.Type != tc.typ || (tc.field != "" && (p.Field == nil || *p.Field != tc.field)) {
			t.Errorf("%v = %d %+v, want %d %s", tc.err, res.StatusCode, p, tc.status, tc.typ)
		}
	}
}

// inviteRefusals are the causes of a user who may not create invites (#3116).
var inviteRefusals = []identity.InviteRefusal{
	identity.InviteRefusalRegistrationClosed, identity.InviteRefusalNotPermitted,
	identity.InviteRefusalNotApproved, identity.InviteRefusalRestricted,
}

// A refused creation says its cause: the problem carries it as reason, and the detail is the
// sentence of that cause in the locale of the request, which no other cause and no refusal
// without a cause shares (#3116).
func TestOwnInviteRefusalSaysItsCause(t *testing.T) {
	e, f := newOwnInvitesEnv(t)
	messages, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	details := map[string]map[identity.InviteRefusal]string{}
	// alice's saved locale is ja; bob has none, and sendJSON asks for en.
	for _, who := range []struct{ token, locale string }{{"bob-write", "en"}, {"write", "ja"}} {
		l := messages.Localizer(who.locale)
		refuse := func(err error) (clientapi.InvitesNotAllowedProblem, string) {
			t.Helper()
			f.err = err
			res := e.sendJSON(t, http.MethodPost, "/invites", who.token, `{}`)
			p := decodeJSON[clientapi.InvitesNotAllowedProblem](t, res)
			if res.StatusCode != http.StatusForbidden || p.Status != http.StatusForbidden || p.Type != clientapi.ProblemTypeInvitesNotAllowed ||
				p.Title != l.ServerErrorsInvitesNotAllowed() || res.Header.Get("Content-Language") != who.locale || p.Detail == nil || *p.Detail == "" {
				t.Fatalf("%s: %v = %d %q %+v; want the 403 of invites-not-allowed with its title and a detail",
					who.locale, err, res.StatusCode, res.Header.Get("Content-Language"), p)
			}
			return p, *p.Detail
		}
		// A refusal without a cause has no reason and the sentence of no cause.
		bare, fallback := refuse(appidentity.ErrInvitesNotAllowed)
		if bare.Reason != nil || fallback != l.ServerErrorsInvitesNotAllowedDetail("") {
			t.Errorf("%s: a refusal without a cause = reason %q, detail %q; want no reason, %q",
				who.locale, reasonOf(bare.Reason), fallback, l.ServerErrorsInvitesNotAllowedDetail(""))
		}
		details[who.locale] = map[identity.InviteRefusal]string{"": fallback}
		for _, reason := range inviteRefusals {
			p, detail := refuse(&appidentity.InvitesNotAllowedError{Reason: reason})
			if p.Reason == nil || !p.Reason.Valid() || reasonOf(p.Reason) != string(reason) {
				t.Errorf("%s: reason = %q, want %s, a value of the contract", who.locale, reasonOf(p.Reason), reason)
			}
			if want := l.ServerErrorsInvitesNotAllowedDetail(string(reason)); detail != want {
				t.Errorf("%s: the detail of %s = %q, want %q", who.locale, reason, detail, want)
			}
			for other, sentence := range details[who.locale] {
				if sentence == detail {
					t.Errorf("%s: %s and %q read the same: %q", who.locale, reason, other, detail)
				}
			}
			details[who.locale][reason] = detail
		}
	}
	for _, reason := range inviteRefusals {
		if details["en"][reason] == details["ja"][reason] {
			t.Errorf("%s reads the same in en and ja: %q", reason, details["en"][reason])
		}
	}
}

// The list says why the user may not create invites, and omits the reason of a user who may
// (#3116).
func TestOwnInvitesListSaysTheRefusal(t *testing.T) {
	e, f := newOwnInvitesEnv(t)
	res := e.send(t, http.MethodGet, "/invites", "read", "")
	raw := decodeJSON[map[string]json.RawMessage](t, res)
	if _, has := raw["cannot_create_reason"]; res.StatusCode != http.StatusOK || string(raw["can_create"]) != "true" || has {
		t.Errorf("a user who may create = %d %s; want can_create and no cannot_create_reason", res.StatusCode, raw)
	}
	for _, reason := range inviteRefusals {
		f.refusal = reason
		res := e.send(t, http.MethodGet, "/invites", "read", "")
		page := decodeJSON[clientapi.OwnInvitePage](t, res)
		if res.StatusCode != http.StatusOK || page.CanCreate || page.CannotCreateReason == nil || !page.CannotCreateReason.Valid() ||
			reasonOf(page.CannotCreateReason) != string(reason) {
			t.Errorf("%s: the list = %d can_create %t, cannot_create_reason %q", reason, res.StatusCode, page.CanCreate, reasonOf(page.CannotCreateReason))
		}
	}
}

// reasonOf is the reason of a response, "" when it has none.
func reasonOf(r *clientapi.InvitesNotAllowedReason) string {
	if r == nil {
		return ""
	}
	return string(*r)
}
