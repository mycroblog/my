// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform"
)

// TestMemberInvites drives the invites that members create on the real database (ADR-0108):
// the instance settings decide who may and the caps, a member's invite approves at once in
// approval mode, and a limit on its creator stops it.
func TestMemberInvites(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	s := start(t, cfg, platform.ServeOptions{}, nil)
	ctx := t.Context()
	set := func(key instance.SettingKey, value string) {
		t.Helper()
		if _, err := s.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
			t.Fatal(err)
		}
	}
	signUp := func(name, code string) (*authClient, int) {
		t.Helper()
		c := &authClient{t: t, s: s, cookies: map[string]string{}}
		resp, _ := c.do(http.MethodPost, authpages.PathSignUp, c.formAt(authpages.PathSignUp,
			"username", name, "email", name+"@example.com", "password", "correct horse battery", "invite_code", code), nil)
		return c, resp.StatusCode
	}
	set(instance.SettingRegistrationMode, string(instance.RegistrationApproval))
	alice, err := s.app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "alice", Password: "correct horse battery"})
	if err != nil {
		t.Fatal(err)
	}

	// permitted (the default): alice holds no create_invites.
	if _, err := s.app.Identity.CreateOwnInvite(ctx, identity.CreateOwnInviteInput{UserID: alice.User.ID}); !errors.Is(err, identity.ErrInvitesNotAllowed) {
		t.Fatalf("CreateOwnInvite while permitted = %v, want ErrInvitesNotAllowed", err)
	}
	set(instance.SettingInviteCreators, string(instance.InviteCreatorsEveryone))
	set(instance.SettingMemberInviteMaxActive, "1")
	inv, err := s.app.Identity.CreateOwnInvite(ctx, identity.CreateOwnInviteInput{UserID: alice.User.ID, MaxUses: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.app.Identity.CreateOwnInvite(ctx, identity.CreateOwnInviteInput{UserID: alice.User.ID}); !errors.Is(err, identity.ErrTooManyInvites) {
		t.Errorf("a second invite under member_invite_max_active = 1: %v", err)
	}

	// approval: the invite approves at once.
	bob, status := signUp("bob", inv.Code)
	if status != http.StatusSeeOther {
		t.Fatalf("sign-up with alice's invite = %d", status)
	}
	if _, body := bob.do(http.MethodGet, authpages.PathSignIn, nil, nil); strings.Contains(body, "waiting for a moderator") {
		t.Errorf("an invited user waits for a moderator:\n%s", body)
	}

	// A limit on alice stops her invite; lifting it lets it work again.
	action, err := s.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: alice.AccountID, Kind: moderation.ActionLimit, ReasonCategory: moderation.CategorySpam,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, status := signUp("carol", inv.Code); status != http.StatusUnprocessableEntity {
		t.Errorf("sign-up with the invite of a limited member = %d, want 422", status)
	}
	if err := s.app.Moderation.LiftAccountAction(ctx, action.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, status := signUp("dave", inv.Code); status != http.StatusSeeOther {
		t.Errorf("sign-up after the limit is lifted = %d", status)
	}

	// Back to permitted, the invite stops working.
	set(instance.SettingInviteCreators, string(instance.InviteCreatorsPermitted))
	if _, status := signUp("erin", inv.Code); status != http.StatusUnprocessableEntity {
		t.Errorf("sign-up after invite_creators went back to permitted = %d, want 422", status)
	}
}
