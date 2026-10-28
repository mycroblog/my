// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeOwnInvites keeps one member's invites newest first; err fails the next creation.
type fakeOwnInvites struct {
	list    []appidentity.InviteStatus
	creates []appidentity.CreateOwnInviteInput
	revokes []string
	err     error
}

func (f *fakeOwnInvites) ListOwnInvites(_ context.Context, userID, before string, limit int) (appidentity.OwnInvites, error) {
	out := appidentity.OwnInvites{
		MayCreate: userID == userOf(aliceAccountID), Usable: len(f.list),
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

// A member creates, lists, and revokes their own invites (ADR-0108 §5).
func TestOwnInvites(t *testing.T) {
	e, f := newOwnInvitesEnv(t)
	res := e.sendJSON(t, http.MethodPost, "/invites", "write", `{"max_uses":3,"expires_in":3600}`)
	got := decodeJSON[clientapi.OwnInvite](t, res)
	if res.StatusCode != http.StatusOK || got.Code != "QRSTUVWXYZ234567" || got.Url != "https://social.example/auth/sign_up?invite=QRSTUVWXYZ234567" ||
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

// The refusals of the service are problems (ADR-0108 §1, §2).
func TestOwnInviteProblems(t *testing.T) {
	e, f := newOwnInvitesEnv(t)
	for _, tc := range []struct {
		err    error
		body   string
		status int
		typ    string
		detail string
	}{
		{appidentity.ErrInvitesNotAllowed, `{}`, http.StatusForbidden, clientapi.ProblemTypeInvitesNotAllowed, ""},
		{appidentity.ErrTooManyInvites, `{}`, http.StatusConflict, clientapi.ProblemTypeTooManyInvites, ""},
		{identity.ErrInviteUsesOverCap, `{"max_uses":11}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeBlank, "max_uses"},
		{identity.ErrInviteExpiryOverCap, `{"expires_in":31536000}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeBlank, "expires_in"},
	} {
		f.err = tc.err
		res := e.sendJSON(t, http.MethodPost, "/invites", "write", tc.body)
		p := decodeProblem(t, res)
		if res.StatusCode != tc.status || p.Type != tc.typ || (tc.detail != "" && (p.Detail == nil || !strings.Contains(*p.Detail, tc.detail))) {
			t.Errorf("%v = %d %+v, want %d %s", tc.err, res.StatusCode, p, tc.status, tc.typ)
		}
	}
}
