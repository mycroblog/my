// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// memberInvites is an identity service whose registration mode, member invite settings, and
// moderation restrictions the test changes (ADR-0108).
type memberInvites struct {
	*appidentity.Service
	mu         sync.Mutex
	mode       identity.RegistrationMode
	settings   appidentity.MemberInviteSettings
	restricted map[string]bool
	requireApp map[string]bool // email addresses that a sign-up block sends to approval
}

func (e *accountEnv) memberInvites(mode identity.RegistrationMode) *memberInvites {
	m := &memberInvites{
		mode: mode, settings: appidentity.DefaultMemberInviteSettings(), restricted: map[string]bool{}, requireApp: map[string]bool{},
	}
	m.Service = e.newIdentityWith(func(d *appidentity.Deps) {
		d.RegistrationMode = func(context.Context) (identity.RegistrationMode, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			return m.mode, nil
		}
		d.MemberInvites = func(context.Context) (appidentity.MemberInviteSettings, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			return m.settings, nil
		}
		d.InviterRestricted = func(_ context.Context, accountID string) (bool, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			return m.restricted[accountID], nil
		}
		d.SignupBlocks = signupScreenFunc(func(_ context.Context, email, _ string) (appmoderation.SignupBlockSeverity, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.requireApp[email] {
				return appmoderation.SignupRequireApproval, nil
			}
			return "", nil
		})
		d.RequireEmailVerification = true
	})
	return m
}

func (m *memberInvites) set(change func(*memberInvites)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	change(m)
}

// signupScreenFunc is an appidentity.SignupScreen of a function.
type signupScreenFunc func(ctx context.Context, email, ip string) (appmoderation.SignupBlockSeverity, error)

func (f signupScreenFunc) ScreenSignup(ctx context.Context, email, ip string) (appmoderation.SignupBlockSeverity, error) {
	return f(ctx, email, ip)
}

// inviter registers a user who holds a role with only create_invites.
func (e *accountEnv) inviter(t *testing.T, username string) appidentity.Registration {
	t.Helper()
	ctx := t.Context()
	reg := e.register(t, username)
	if _, err := e.identity.CreateRole(ctx, appidentity.CreateRoleInput{
		Key: "inviters", Position: 10, Permissions: []identity.Permission{identity.PermCreateInvites},
	}); err != nil && !errors.Is(err, appidentity.ErrRoleKeyTaken) {
		t.Fatalf("CreateRole = %v", err)
	}
	if _, err := e.identity.GrantRole(ctx, appidentity.RoleChangeInput{UserID: reg.User.ID, RoleKey: "inviters"}); err != nil {
		t.Fatalf("GrantRole = %v", err)
	}
	return reg
}

func (m *memberInvites) create(t *testing.T, userID string, uses int, expiresIn time.Duration) appidentity.InviteStatus {
	t.Helper()
	inv, err := m.CreateOwnInvite(t.Context(), appidentity.CreateOwnInviteInput{UserID: userID, MaxUses: uses, ExpiresIn: expiresIn})
	if err != nil {
		t.Fatalf("CreateOwnInvite(%s) = %v", userID, err)
	}
	return inv
}

// memberInvitesCreate checks who may create invites of their own, the caps, the listing, the
// revocation, and the audit (ADR-0108 §1, §2, §5, §6).
func memberInvitesCreate(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	rec := &auditRecorder{}
	e.projectors.Register(appmoderation.EventAuditRecorded, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		rec.add(ev.(appmoderation.AuditRecorded).Entry)
		return nil
	})
	m := e.memberInvites(identity.RegistrationApproval)
	alice := e.inviter(t, "alice")
	bob := e.register(t, "bob")

	// Only the holders of create_invites while invite_creators is permitted.
	if _, err := m.CreateOwnInvite(ctx, appidentity.CreateOwnInviteInput{UserID: bob.User.ID}); !errors.Is(err, appidentity.ErrInvitesNotAllowed) {
		t.Errorf("bob without create_invites = %v, want ErrInvitesNotAllowed", err)
	}
	own, err := m.ListOwnInvites(ctx, bob.User.ID, "", 10)
	if err != nil || own.MayCreate || len(own.Invites) != 0 {
		t.Errorf("bob's invites = %+v, %v; want none, may not create", own, err)
	}
	first := m.create(t, alice.User.ID, 0, 0)
	if first.CreatorKind != identity.InviteByMember || first.MaxUses != 1 || first.ExpiresAt == nil ||
		!first.ExpiresAt.Equal(first.CreatedAt.Add(7*24*time.Hour)) || first.State != appidentity.InviteUsable {
		t.Errorf("a default invite = %+v; want a member's, 1 use, 7 days", first)
	}
	// everyone lets bob too.
	m.set(func(m *memberInvites) { m.settings.Everyone = true })
	bobs := m.create(t, bob.User.ID, 2, time.Hour)
	if bobs.MaxUses != 2 || !bobs.ExpiresAt.Equal(bobs.CreatedAt.Add(time.Hour)) {
		t.Errorf("bob's invite = %+v; want 2 uses, an hour", bobs)
	}
	// A suspended or limited account, and closed registration, may not.
	m.set(func(m *memberInvites) { m.restricted[bob.AccountID] = true })
	if _, err := m.CreateOwnInvite(ctx, appidentity.CreateOwnInviteInput{UserID: bob.User.ID}); !errors.Is(err, appidentity.ErrInvitesNotAllowed) {
		t.Errorf("a restricted bob = %v, want ErrInvitesNotAllowed", err)
	}
	m.set(func(m *memberInvites) { m.restricted[bob.AccountID] = false; m.mode = identity.RegistrationClosed })
	if _, err := m.CreateOwnInvite(ctx, appidentity.CreateOwnInviteInput{UserID: alice.User.ID}); !errors.Is(err, appidentity.ErrInvitesNotAllowed) {
		t.Errorf("closed = %v, want ErrInvitesNotAllowed", err)
	}
	if _, err := m.CreateInvite(ctx, appidentity.CreateInviteInput{}); !errors.Is(err, appidentity.ErrRegistrationClosed) {
		t.Errorf("a staff invite while closed = %v, want ErrRegistrationClosed", err)
	}
	m.set(func(m *memberInvites) { m.mode = identity.RegistrationOpen; m.settings.Everyone = false })

	// The caps: uses, lifetime, and the usable invites a member holds.
	m.set(func(m *memberInvites) {
		m.settings.Caps = identity.MemberInviteCaps{MaxActive: 3, MaxUses: 5, MaxExpiresIn: 24 * time.Hour}
	})
	if _, err := m.CreateOwnInvite(ctx, appidentity.CreateOwnInviteInput{UserID: alice.User.ID, MaxUses: 6}); !errors.Is(err, identity.ErrInviteUsesOverCap) {
		t.Errorf("6 uses over a cap of 5 = %v", err)
	}
	if _, err := m.CreateOwnInvite(ctx, appidentity.CreateOwnInviteInput{UserID: alice.User.ID, ExpiresIn: 25 * time.Hour}); !errors.Is(err, identity.ErrInviteExpiryOverCap) {
		t.Errorf("25 hours over a cap of a day = %v", err)
	}
	short := m.create(t, alice.User.ID, 0, 0)
	if !short.ExpiresAt.Equal(short.CreatedAt.Add(24 * time.Hour)) {
		t.Errorf("the default lifetime under a cap of a day = %v", short.ExpiresAt)
	}
	third := m.create(t, alice.User.ID, 5, 0)
	if _, err := m.CreateOwnInvite(ctx, appidentity.CreateOwnInviteInput{UserID: alice.User.ID}); !errors.Is(err, appidentity.ErrTooManyInvites) {
		t.Errorf("a fourth usable invite = %v, want ErrTooManyInvites", err)
	}
	// A staff invite of the same user does not count, and nor does a revoked one.
	if _, err := m.CreateInvite(ctx, appidentity.CreateInviteInput{CreatedByUserID: alice.User.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RevokeOwnInvite(ctx, bob.User.ID, third.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("bob revoking alice's invite = %v, want ErrNotFound", err)
	}
	revoked, err := m.RevokeOwnInvite(ctx, alice.User.ID, third.ID)
	if err != nil || revoked.State != appidentity.InviteRevoked {
		t.Fatalf("RevokeOwnInvite = %+v, %v", revoked, err)
	}
	fourth := m.create(t, alice.User.ID, 0, 0)

	own, err = m.ListOwnInvites(ctx, alice.User.ID, "", 10)
	if err != nil || !own.MayCreate || own.Usable != 3 || own.Caps.MaxActive != 3 || len(own.Invites) != 4 {
		t.Fatalf("alice's invites = %+v, %v; want 4 listed, 3 usable", own, err)
	}
	if own.Invites[0].ID != fourth.ID || own.Invites[1].ID != third.ID || own.Invites[1].State != appidentity.InviteRevoked ||
		own.Invites[3].ID != first.ID {
		t.Errorf("alice's invites = %+v; want newest first, the third revoked", own.Invites)
	}
	page, err := m.ListOwnInvites(ctx, alice.User.ID, third.ID, 10)
	if err != nil || len(page.Invites) != 2 || page.Invites[0].ID != short.ID {
		t.Errorf("the page after the third = %+v, %v", page.Invites, err)
	}
	// Expired invites free their places: after a day, only the first, of a week, is usable.
	e.clock.Advance(24 * time.Hour)
	if own, err = m.ListOwnInvites(ctx, alice.User.ID, "", 10); err != nil || own.Usable != 1 || own.Invites[0].State != appidentity.InviteExpired {
		t.Errorf("after a day = %+v, %v; want the first usable", own, err)
	}

	// The creations and the revocation are audited with the member as the actor.
	var created, revokedN int
	for _, en := range rec.all() {
		if en.ActorUserID == alice.User.ID && en.ActorKind == moderation.AuditActorUser && en.TargetKind == moderation.AuditTargetInvite {
			switch en.Action {
			case moderation.AuditInviteCreated:
				created++
			case moderation.AuditInviteRevoked:
				revokedN++
			}
		}
	}
	// first, short, third, the staff invite, fourth
	if created != 5 || revokedN != 1 {
		t.Errorf("alice's audit = %d created, %d revoked; want 5, 1", created, revokedN)
	}
}

// memberInvitesSignUp checks the invites at sign-up in every mode, and the creator's check
// (ADR-0108 §3, §4).
func memberInvitesSignUp(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	m := e.memberInvites(identity.RegistrationApproval)
	alice := e.inviter(t, "alice")
	staff, err := m.CreateInvite(ctx, appidentity.CreateInviteInput{})
	if err != nil {
		t.Fatal(err)
	}
	member := m.create(t, alice.User.ID, 10, 0)
	n := 0
	signUp := func(code string) (appidentity.SignedUp, error) {
		n++
		in := signUpInput(fmt.Sprintf("user%d", n))
		in.InviteCode = code
		return m.SignUp(ctx, in)
	}
	mustSignUp := func(name, code string, want identity.RegistrationState) appidentity.SignedUp {
		t.Helper()
		got, err := signUp(code)
		if err != nil || got.State != want {
			t.Fatalf("%s: SignUp = %+v, %v; want %s", name, got.State, err, want)
		}
		return got
	}

	// approval: a usable invite, of the staff or of a member, approves at once; none waits.
	mustSignUp("approval without an invite", "", identity.RegistrationPending)
	viaStaff := mustSignUp("approval with a staff invite", staff.Code, identity.RegistrationApproved)
	viaMember := mustSignUp("approval with a member invite", member.Code, identity.RegistrationApproved)
	if app, err := e.h.Identity.FindRegistrationApplication(ctx, viaMember.User.ID); err != nil || app.InviteID != member.ID {
		t.Errorf("the application = %+v, %v; want the member's invite", app, err)
	}
	if app, err := e.h.Identity.FindRegistrationApplication(ctx, viaStaff.User.ID); err != nil || app.InviteID != staff.ID {
		t.Errorf("the application = %+v, %v; want the staff invite", app, err)
	}
	// A sign-up block that requires approval wins over the invite.
	in := signUpInput("blocked")
	in.InviteCode = member.Code
	m.set(func(m *memberInvites) { m.requireApp[in.Email] = true })
	if got, err := m.SignUp(ctx, in); err != nil || got.State != identity.RegistrationPending {
		t.Errorf("a blocked sign-up with an invite = %+v, %v; want pending", got.State, err)
	}

	// open: the invite is recorded, nothing else changes; invite: it is required.
	m.set(func(m *memberInvites) { m.mode = identity.RegistrationOpen })
	viaOpen := mustSignUp("open with an invite", member.Code, identity.RegistrationApproved)
	if app, err := e.h.Identity.FindRegistrationApplication(ctx, viaOpen.User.ID); err != nil || app.InviteID != member.ID {
		t.Errorf("open: the application = %+v, %v; want the invite", app, err)
	}
	mustSignUp("open without an invite", "", identity.RegistrationApproved)
	m.set(func(m *memberInvites) { m.mode = identity.RegistrationInvite })
	mustSignUp("invite with a member invite", member.Code, identity.RegistrationApproved)
	if _, err := signUp(""); !errors.Is(err, appidentity.ErrInviteRequired) {
		t.Errorf("invite without a code = %v", err)
	}

	// An unusable code fails in every mode; closed refuses every sign-up.
	for _, mode := range []identity.RegistrationMode{identity.RegistrationOpen, identity.RegistrationApproval, identity.RegistrationInvite} {
		m.set(func(m *memberInvites) { m.mode = mode })
		if _, err := signUp("NOSUCHCODE"); !errors.Is(err, appidentity.ErrInvalidInvite) {
			t.Errorf("%s with an unknown code = %v, want ErrInvalidInvite", mode, err)
		}
	}
	m.set(func(m *memberInvites) { m.mode = identity.RegistrationClosed })
	if _, err := signUp(member.Code); !errors.Is(err, appidentity.ErrRegistrationClosed) {
		t.Errorf("closed with an invite = %v, want ErrRegistrationClosed", err)
	}
	m.set(func(m *memberInvites) { m.mode = identity.RegistrationApproval })

	// The creator's check: each way of losing the right stops the invite, which works again
	// when the right comes back; a staff invite is never checked.
	invalid := func(name string) {
		t.Helper()
		if _, err := signUp(member.Code); !errors.Is(err, appidentity.ErrInvalidInvite) {
			t.Errorf("%s: SignUp = %v, want ErrInvalidInvite", name, err)
		}
		list, err := m.ListInvites(ctx, "", 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, st := range list {
			if st.ID == member.ID && st.State != appidentity.InviteCreatorIneligible {
				t.Errorf("%s: the admin list says %s, want creator_ineligible", name, st.State)
			}
		}
	}
	if _, err := e.identity.RevokeRole(ctx, appidentity.RoleChangeInput{UserID: alice.User.ID, RoleKey: "inviters"}); err != nil {
		t.Fatal(err)
	}
	invalid("the permission lost")
	own, err := m.ListOwnInvites(ctx, alice.User.ID, "", 10)
	if err != nil || own.MayCreate || own.Invites[0].State != appidentity.InviteCreatorIneligible {
		t.Errorf("alice's own invites = %+v, %v; want creator_ineligible", own, err)
	}
	m.set(func(m *memberInvites) { m.settings.Everyone = true })
	mustSignUp("everyone may invite", member.Code, identity.RegistrationApproved)
	m.set(func(m *memberInvites) { m.settings.Everyone = false })
	invalid("invite_creators back to permitted")
	if _, err := e.identity.GrantRole(ctx, appidentity.RoleChangeInput{UserID: alice.User.ID, RoleKey: "inviters"}); err != nil {
		t.Fatal(err)
	}
	m.set(func(m *memberInvites) { m.restricted[alice.AccountID] = true })
	invalid("suspended or limited")
	m.set(func(m *memberInvites) { m.restricted[alice.AccountID] = false })
	mustSignUp("the right back", member.Code, identity.RegistrationApproved)
	if _, err := e.accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: alice.AccountID, Kind: account.LifecycleDeleted, InitiatedBy: account.InitiatedBySelf,
	}); err != nil {
		t.Fatal(err)
	}
	invalid("deleted")
	mustSignUp("a staff invite of nobody's checks", staff.Code, identity.RegistrationApproved)
}
