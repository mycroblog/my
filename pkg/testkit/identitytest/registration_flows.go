// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
)

// RegistrationFlows verifies the sign-up use cases on a database adapter, the flows of ADR-0153
// §1, which internal/flowtest/identity runs once on SQLite; RegistrationRepositoryContract checks
// the registration methods of the identity repository on every adapter. The flows check the
// registration persistence of the
// identity repository (data-model §4.1) through the sign-up use cases, one case per
// registration mode: open mode approves at once (or waits for a moderator when email cannot be
// verified), approval mode waits for a moderator's decision, invite mode needs a valid invite,
// and closed mode refuses. It also checks that concurrent sign-ups never redeem an invite more
// than its maximum uses, that concurrent decisions on a registration decide it once, that an email verification token works exactly once (also when
// redeemed concurrently), that a failed sign-up writes nothing, that sign-ups are rate-limited
// per address, the invite listing and revocation, that a sign-up records its origin, which
// the purge erases after the retention, the count of pending registrations, and the holders of
// a permission with where their moderator notifications are emailed, and the invites that members
// create (ADR-0106): who may, the caps, and the check of their creator at every sign-up in the
// open, approval, and invite modes. It uses the same harness as AccountRepositoryContract.
func RegistrationFlows(t *testing.T, newHarness AccountFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *AccountEnv)
	}{
		{"OpenModeApprovesAtOnce", registrationOpen},
		{"OpenModeWithoutEmailWaitsForApproval", registrationOpenWithoutEmail},
		{"ApprovalModeWaitsForAModerator", registrationApproval},
		{"InviteModeNeedsAValidInvite", registrationInvite},
		{"ClosedModeRefuses", registrationClosed},
		{"InviteUsesHoldUnderConcurrency", registrationInviteConcurrency},
		{"ConcurrentDecisionsDecideOnce", registrationDecisionConcurrency},
		{"VerificationTokensWorkOnce", registrationTokenOnce},
		{"FailedSignUpsWriteNothing", registrationFailureWritesNothing},
		{"RateLimitsSignUpsPerAddress", registrationRateLimit},
		{"ListsAndRevokesInvites", registrationInvites},
		{"AdminsAreApprovedAndVerified", registrationAdmin},
		{"RecordsAndPurgesRegistrationOrigins", registrationRecordsOrigins},
		{"CountsPendingAndFindsModerators", registrationModerators},
		{"MembersCreateInvitesUnderTheCaps", memberInvitesCreate},
		{"InvitesWorkInEveryOpenModeWhileTheirCreatorMayInvite", memberInvitesSignUp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, NewAccountEnv(t, newHarness)) })
	}
}

// RegistrationEnv is an identity service in one registration mode with a recording mailer.
type RegistrationEnv struct {
	*appidentity.Service
	mailer *testkit.Mailer
}

// Registration returns the identity service with the registration mode and whether sign-ups
// verify their email, and the mailer that it sends through.
func (e *AccountEnv) Registration(mode identity.RegistrationMode, requireVerification bool) RegistrationEnv {
	m := &testkit.Mailer{}
	return RegistrationEnv{
		Service: e.newIdentityWith(func(d *appidentity.Deps) {
			d.RegistrationMode = func(context.Context) (identity.RegistrationMode, error) { return mode, nil }
			d.Mailer, d.RequireEmailVerification = m, requireVerification
		}),
		mailer: m,
	}
}

// SignUpInput is the sign-up of the user name.
func SignUpInput(name string) appidentity.SignUpInput {
	return appidentity.SignUpInput{
		Username: name, Email: name + "@example.com", Password: "correct horse " + name, Locale: "ja", UserAgent: "contract",
	}
}

// MustSignUp signs up in and fails t when it cannot.
func (r RegistrationEnv) MustSignUp(t *testing.T, in appidentity.SignUpInput) appidentity.SignedUp {
	t.Helper()
	got, err := r.SignUp(t.Context(), in)
	if err != nil {
		t.Fatalf("SignUp(%s) = %v", in.Username, err)
	}
	return got
}

func (r RegistrationEnv) access(t *testing.T, userID string) appidentity.Access {
	t.Helper()
	a, err := r.UserAccess(t.Context(), userID)
	if err != nil {
		t.Fatalf("UserAccess(%s) = %v", userID, err)
	}
	return a
}

var verifyLink = regexp.MustCompile(`https://` + regexp.QuoteMeta(testkit.ContractHost) + `/auth/verify_email\?token=(\S+)`)

// lastToken returns the token of the newest verification email sent to address.
func (r RegistrationEnv) lastToken(t *testing.T, address string) string {
	t.Helper()
	sent := r.mailer.Sent()
	for i := len(sent) - 1; i >= 0; i-- {
		if sent[i].To != address {
			continue
		}
		m := verifyLink.FindStringSubmatch(sent[i].Text)
		if m == nil {
			t.Fatalf("verification email without a link:\n%s", sent[i].Text)
		}
		token, err := url.QueryUnescape(m[1])
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	t.Fatalf("no email to %s among %d", address, len(sent))
	return ""
}

func registrationOpen(t *testing.T, e *AccountEnv) {
	r := e.Registration(identity.RegistrationOpen, true)
	in := SignUpInput("alice")
	in.Reason = "ignored outside approval mode"
	got := r.MustSignUp(t, in)
	if got.State != identity.RegistrationApproved {
		t.Fatalf("SignUp = state %s, want approved", got.State)
	}
	app, err := e.H.Identity.FindRegistrationApplication(t.Context(), got.User.ID)
	if err != nil || app.Reason != "" || app.InviteID != "" || !app.SubmittedAt.Equal(got.User.CreatedAt) {
		t.Errorf("application = %+v, %v; want one without reason or invite", app, err)
	}
	if byID, err := e.H.Identity.FindRegistrationApplicationByID(t.Context(), app.ID); err != nil || byID.UserID != got.User.ID {
		t.Errorf("FindRegistrationApplicationByID = %+v, %v; want the application of %s", byID, err, got.User.ID)
	}
	a := r.access(t, got.User.ID)
	if a.Registration != identity.RegistrationApproved || a.EmailVerified || a.Email != in.Email || a.Allowed() || !a.AwaitingVerification() {
		t.Errorf("access before verification = %+v; want approved, unverified, not allowed", a)
	}
	// The sign-up signed the browser in.
	if s, err := r.Authenticate(t.Context(), got.Session.Token); err != nil || s.UserID != got.User.ID {
		t.Errorf("Authenticate(sign-up session) = %+v, %v", s, err)
	}
	// The verification email is in the user's locale and links to the confirmation page.
	sent := r.mailer.Sent()
	if len(sent) != 1 || sent[0].To != in.Email || !strings.Contains(sent[0].Subject, testkit.ContractHost) || !strings.Contains(sent[0].Text, "24 時間") {
		t.Fatalf("sent = %+v; want one Japanese verification email to %s", sent, in.Email)
	}
	token := r.lastToken(t, in.Email)
	if !strings.HasPrefix(token, appidentity.EmailVerificationTokenPrefix) {
		t.Errorf("token = %q", token)
	}
	if userID, err := r.VerifyEmail(t.Context(), token); err != nil || userID != got.User.ID {
		t.Fatalf("VerifyEmail = %q, %v", userID, err)
	}
	if a := r.access(t, got.User.ID); !a.EmailVerified || !a.Allowed() {
		t.Errorf("access after verification = %+v; want allowed", a)
	}
	if prefs := e.preferences(t, got.User.ID); prefs.Locale != "ja" {
		t.Errorf("locale = %q, want the sign-up's", prefs.Locale)
	}
}

func registrationOpenWithoutEmail(t *testing.T, e *AccountEnv) {
	r := e.Registration(identity.RegistrationOpen, false)
	got := r.MustSignUp(t, SignUpInput("alice"))
	if got.State != identity.RegistrationPending {
		t.Fatalf("state = %s; want pending, since no email proves the address", got.State)
	}
	a := r.access(t, got.User.ID)
	if a.Allowed() || a.AwaitingVerification() {
		t.Errorf("access = %+v; want pending without a verification requirement", a)
	}
	if _, err := r.DecideRegistration(t.Context(), appidentity.DecideInput{UserID: got.User.ID, Decision: identity.DecisionApproved}); err != nil {
		t.Fatalf("DecideRegistration = %v", err)
	}
	if a := r.access(t, got.User.ID); !a.Allowed() {
		t.Errorf("access after approval = %+v; want allowed without verification", a)
	}
}

func registrationApproval(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	r := e.Registration(identity.RegistrationApproval, true)
	in := SignUpInput("alice")
	in.Reason = "  I like ducks.  "
	alice := r.MustSignUp(t, in)
	e.Clock.Advance(time.Second)
	bob := r.MustSignUp(t, SignUpInput("bob"))
	if alice.State != identity.RegistrationPending || bob.State != identity.RegistrationPending {
		t.Fatalf("states = %s, %s; want pending", alice.State, bob.State)
	}
	if a := r.access(t, alice.User.ID); a.Allowed() || a.Registration != identity.RegistrationPending {
		t.Errorf("access = %+v; want pending", a)
	}
	pending, err := r.ListRegistrations(ctx, identity.RegistrationPending, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 || pending[0].Application.UserID != alice.User.ID || pending[1].Application.UserID != bob.User.ID {
		t.Fatalf("pending = %+v; want alice then bob", pending)
	}
	if after, err := r.ListRegistrationsAfter(ctx, identity.RegistrationPending, pending[0].Position(), 10); err != nil ||
		len(after) != 1 || after[0].Application.UserID != bob.User.ID {
		t.Errorf("pending after alice = %+v, %v; want bob", after, err)
	}
	if one, err := r.Registration(ctx, bob.User.ID); err != nil || one.Application.UserID != bob.User.ID || one.Username != "bob" ||
		one.AccountID != bob.AccountID || one.Email != "bob@example.com" || one.State.State != identity.RegistrationPending {
		t.Errorf("Registration(bob) = %+v, %v", one, err)
	}
	if _, err := r.Registration(ctx, testkit.NamedID("nobody")); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("Registration(nobody) = %v, want ErrNotFound", err)
	}
	p := pending[0]
	if p.Username != "alice" || p.Email != in.Email || p.EmailVerified || p.Application.Reason != "I like ducks." ||
		!p.State.Since.Equal(alice.User.CreatedAt) {
		t.Errorf("pending[0] = %+v", p)
	}

	e.Clock.Advance(time.Minute)
	// A note that a dialect cannot store as it is fails alike on every dialect (data-model §1.2)
	// and decides nothing.
	for _, note := range []string{"wel\x00come", "\xff", strings.Repeat("あ", identity.MaxDecisionNoteLength+1)} {
		_, err := r.DecideRegistration(ctx, appidentity.DecideInput{UserID: alice.User.ID, Decision: identity.DecisionApproved, Note: note})
		if !errors.Is(err, identity.ErrInvalidNote) {
			t.Errorf("DecideRegistration(note %q...) = %v, want ErrInvalidNote", note[:1], err)
		}
	}
	if a := r.access(t, alice.User.ID); a.Registration != identity.RegistrationPending {
		t.Errorf("access after invalid decisions = %+v; want pending", a)
	}
	if e, err := r.Registration(ctx, alice.User.ID); err != nil || e.DecisionNote != "" {
		t.Errorf("Registration(pending).DecisionNote = %q, %v; want none", e.DecisionNote, err)
	}
	d, err := r.DecideRegistration(ctx, appidentity.DecideInput{UserID: alice.User.ID, Decision: identity.DecisionApproved, DecidedByUserID: testkit.NamedID("mod"), Note: "welcome"})
	if err != nil || d.DecidedBy != identity.DecidedByModerator || d.ApplicationID != p.Application.ID {
		t.Fatalf("approve = %+v, %v", d, err)
	}
	if _, err := r.DecideRegistration(ctx, appidentity.DecideInput{UserID: alice.User.ID, Decision: identity.DecisionRejected}); !errors.Is(err, appidentity.ErrNotPending) {
		t.Errorf("deciding again = %v, want ErrNotPending", err)
	}
	if _, err := r.DecideRegistration(ctx, appidentity.DecideInput{UserID: bob.User.ID, Decision: identity.DecisionRejected}); err != nil {
		t.Fatal(err)
	}
	if e, err := r.Registration(ctx, alice.User.ID); err != nil || e.DecisionNote != "welcome" {
		t.Errorf("Registration(approved).DecisionNote = %q, %v; want the note, which a second decision does not change", e.DecisionNote, err)
	}
	if e, err := r.Registration(ctx, bob.User.ID); err != nil || e.DecisionNote != "" {
		t.Errorf("Registration(rejected without a note).DecisionNote = %q, %v; want none", e.DecisionNote, err)
	}
	if _, err := r.DecideRegistration(ctx, appidentity.DecideInput{UserID: testkit.NamedID("nobody"), Decision: identity.DecisionApproved}); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("deciding on an unknown user = %v, want ErrNotFound", err)
	}
	st, err := e.H.Identity.FindRegistrationState(ctx, alice.User.ID)
	if err != nil || st.State != identity.RegistrationApproved || !st.Since.Equal(e.Clock.Now()) {
		t.Errorf("alice's state = %+v, %v; want approved now", st, err)
	}
	if a := r.access(t, bob.User.ID); a.Registration != identity.RegistrationRejected || a.Allowed() {
		t.Errorf("bob's access = %+v; want rejected", a)
	}
	if pending, _ := r.ListRegistrations(ctx, identity.RegistrationPending, 10); len(pending) != 0 {
		t.Errorf("pending after the decisions = %+v", pending)
	}
	if rejected, _ := r.ListRegistrations(ctx, identity.RegistrationRejected, 10); len(rejected) != 1 || rejected[0].Username != "bob" {
		t.Errorf("rejected = %+v", rejected)
	}
}

func registrationInvite(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	r := e.Registration(identity.RegistrationInvite, true)
	if _, err := r.SignUp(ctx, SignUpInput("alice")); !errors.Is(err, appidentity.ErrInviteRequired) {
		t.Errorf("SignUp without a code = %v, want ErrInviteRequired", err)
	}
	in := SignUpInput("alice")
	in.InviteCode = "NOSUCHCODE"
	if _, err := r.SignUp(ctx, in); !errors.Is(err, appidentity.ErrInvalidInvite) {
		t.Errorf("SignUp with an unknown code = %v, want ErrInvalidInvite", err)
	}
	inv, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{CreatedByUserID: testkit.NamedID("admin"), MaxUses: 1, ExpiresIn: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Code) != identity.InviteCodeLength || inv.MaxUses != 1 || inv.ExpiresAt == nil {
		t.Fatalf("invite = %+v", inv)
	}
	// Codes compare without case, spaces, and hyphens.
	in.InviteCode = strings.ToLower(inv.Code[:4]) + "-" + inv.Code[4:8] + " " + inv.Code[8:]
	got := r.MustSignUp(t, in)
	if got.State != identity.RegistrationApproved {
		t.Errorf("state = %s; want approved", got.State)
	}
	if app, err := e.H.Identity.FindRegistrationApplication(ctx, got.User.ID); err != nil || app.InviteID != inv.ID {
		t.Errorf("application = %+v, %v; want the invite", app, err)
	}
	bob := SignUpInput("bob")
	bob.InviteCode = inv.Code
	if _, err := r.SignUp(ctx, bob); !errors.Is(err, appidentity.ErrInvalidInvite) {
		t.Errorf("SignUp with a used-up invite = %v, want ErrInvalidInvite", err)
	}

	expiring, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{ExpiresIn: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	e.Clock.Advance(time.Minute)
	bob.InviteCode = expiring.Code
	if _, err := r.SignUp(ctx, bob); !errors.Is(err, appidentity.ErrInvalidInvite) {
		t.Errorf("SignUp with an expired invite = %v, want ErrInvalidInvite", err)
	}
	revoked, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RevokeInvite(ctx, revoked.Code, ""); err != nil {
		t.Fatal(err)
	}
	bob.InviteCode = revoked.Code
	if _, err := r.SignUp(ctx, bob); !errors.Is(err, appidentity.ErrInvalidInvite) {
		t.Errorf("SignUp with a revoked invite = %v, want ErrInvalidInvite", err)
	}
	unlimited, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{})
	if err != nil {
		t.Fatal(err)
	}
	bob.InviteCode = unlimited.Code
	r.MustSignUp(t, bob)
}

func registrationClosed(t *testing.T, e *AccountEnv) {
	r := e.Registration(identity.RegistrationClosed, true)
	if _, err := r.SignUp(t.Context(), SignUpInput("alice")); !errors.Is(err, appidentity.ErrRegistrationClosed) {
		t.Fatalf("SignUp = %v, want ErrRegistrationClosed", err)
	}
	if _, err := e.H.Identity.FindUserIDByEmail(t.Context(), "alice@example.com"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("a refused sign-up wrote a user: %v", err)
	}
	if len(r.mailer.Sent()) != 0 {
		t.Error("a refused sign-up sent email")
	}
}

func registrationInviteConcurrency(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	r := e.Registration(identity.RegistrationInvite, true)
	const maxUses, applicants = 3, 8
	inv, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{MaxUses: maxUses})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, applicants)
	for i := range applicants {
		wg.Go(func() {
			in := SignUpInput(fmt.Sprintf("user%d", i))
			in.InviteCode = inv.Code
			_, errs[i] = r.SignUp(ctx, in)
		})
	}
	wg.Wait()
	ok := 0
	for i, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, appidentity.ErrInvalidInvite):
			t.Errorf("applicant %d: %v", i, err)
		}
	}
	if ok != maxUses {
		t.Errorf("%d sign-ups redeemed an invite of %d uses", ok, maxUses)
	}
	list, err := r.ListInvites(ctx, "", 10)
	if err != nil || len(list) != 1 || list[0].Uses != maxUses || list[0].Usable(e.Clock.Now()) {
		t.Errorf("ListInvites = %+v, %v; want %d uses and used up", list, err, maxUses)
	}
}

func registrationDecisionConcurrency(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	r := e.Registration(identity.RegistrationApproval, true)
	alice := r.MustSignUp(t, SignUpInput("alice"))
	const moderators = 6
	var wg sync.WaitGroup
	errs := make([]error, moderators)
	decisions := make([]identity.Decision, moderators)
	for i := range moderators {
		decisions[i] = identity.DecisionApproved
		if i%2 == 1 {
			decisions[i] = identity.DecisionRejected
		}
		wg.Go(func() {
			_, errs[i] = r.DecideRegistration(ctx, appidentity.DecideInput{UserID: alice.User.ID, Decision: decisions[i], DecidedByUserID: testkit.NamedID(fmt.Sprintf("mod%d", i))})
		})
	}
	wg.Wait()
	var won []identity.Decision
	for i, err := range errs {
		switch {
		case err == nil:
			won = append(won, decisions[i])
		case !errors.Is(err, appidentity.ErrNotPending):
			t.Errorf("moderator %d: %v", i, err)
		}
	}
	if len(won) != 1 {
		t.Fatalf("%d of %d concurrent decisions succeeded, want 1", len(won), moderators)
	}
	if st, err := e.H.Identity.FindRegistrationState(ctx, alice.User.ID); err != nil || st.State != won[0].State() {
		t.Errorf("state = %+v, %v; want %s", st, err, won[0].State())
	}
}

func registrationTokenOnce(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	r := e.Registration(identity.RegistrationOpen, true)
	alice := r.MustSignUp(t, SignUpInput("alice"))
	token := r.lastToken(t, "alice@example.com")
	for _, bad := range []string{"", "my_evt_unknown", strings.TrimPrefix(token, appidentity.EmailVerificationTokenPrefix), token + "x"} {
		if _, err := r.VerifyEmail(ctx, bad); !errors.Is(err, appidentity.ErrInvalidToken) {
			t.Errorf("VerifyEmail(%q) = %v, want ErrInvalidToken", bad, err)
		}
	}
	// Concurrent redemptions of one token: exactly one wins.
	const tries = 5
	var wg sync.WaitGroup
	errs := make([]error, tries)
	for i := range tries {
		wg.Go(func() { _, errs[i] = r.VerifyEmail(ctx, token) })
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, appidentity.ErrInvalidToken):
			t.Errorf("VerifyEmail = %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d of %d concurrent redemptions succeeded, want 1", ok, tries)
	}
	if _, err := r.VerifyEmail(ctx, token); !errors.Is(err, appidentity.ErrInvalidToken) {
		t.Errorf("VerifyEmail again = %v, want ErrInvalidToken", err)
	}
	if err := r.ResendVerificationEmail(ctx, alice.User.ID); !errors.Is(err, appidentity.ErrAlreadyVerified) {
		t.Errorf("ResendVerificationEmail after verification = %v, want ErrAlreadyVerified", err)
	}

	// A resent link works; an expired one does not.
	bob := r.MustSignUp(t, SignUpInput("bob"))
	first := r.lastToken(t, "bob@example.com")
	if err := r.ResendVerificationEmail(ctx, bob.User.ID); err != nil {
		t.Fatalf("ResendVerificationEmail = %v", err)
	}
	second := r.lastToken(t, "bob@example.com")
	if second == first {
		t.Fatal("the resent email carries the same token")
	}
	e.Clock.Advance(appidentity.EmailVerificationTTL)
	if _, err := r.VerifyEmail(ctx, second); !errors.Is(err, appidentity.ErrInvalidToken) {
		t.Errorf("VerifyEmail(expired) = %v, want ErrInvalidToken", err)
	}
	for range appidentity.VerificationEmailLimit.Max {
		if err := r.ResendVerificationEmail(ctx, bob.User.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.ResendVerificationEmail(ctx, bob.User.ID); !errors.Is(err, kernel.ErrRateLimited) {
		t.Errorf("ResendVerificationEmail beyond the limit = %v, want ErrRateLimited", err)
	}
}

func registrationFailureWritesNothing(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	r := e.Registration(identity.RegistrationInvite, true)
	inv, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	alice := SignUpInput("alice")
	alice.InviteCode = inv.Code
	if _, err := e.Identity.CreateAdmin(ctx, appidentity.CreateAdminInput{Username: "root", Email: alice.Email, Password: "admin password"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SignUp(ctx, alice); !errors.Is(err, appidentity.ErrEmailTaken) {
		t.Fatalf("SignUp with a taken email = %v, want ErrEmailTaken", err)
	}
	taken := alice
	taken.Email, taken.Username = "other@example.com", "ROOT"
	if _, err := r.SignUp(ctx, taken); !errors.Is(err, appaccount.ErrUsernameTaken) {
		t.Fatalf("SignUp with a taken username = %v, want ErrUsernameTaken", err)
	}
	// Neither failure used the invite or left a user behind.
	alice.Email = "alice@example.org"
	got := r.MustSignUp(t, alice)
	if _, err := e.H.Identity.FindUserIDByEmail(ctx, "other@example.com"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("the failed sign-up left its email address: %v", err)
	}
	if pending, _ := r.ListRegistrations(ctx, identity.RegistrationApproved, 10); len(pending) != 1 || pending[0].Application.UserID != got.User.ID {
		t.Errorf("registrations = %+v; want only alice's", pending)
	}
	for _, bad := range []appidentity.SignUpInput{
		{Username: "bad name", Email: "x@example.com", Password: "long enough", InviteCode: inv.Code},
		{Username: "x", Email: "not an address", Password: "long enough", InviteCode: inv.Code},
		{Username: "x", Email: "x@example.com", Password: "short", InviteCode: inv.Code},
	} {
		if _, err := r.SignUp(ctx, bad); err == nil {
			t.Errorf("SignUp(%+v) succeeded", bad)
		}
	}
	for _, r := range []string{strings.Repeat("あ", identity.MaxRegistrationReasonLength+1), "ducks\x00", "\xff"} {
		reason := SignUpInput("carol")
		reason.Reason = r
		if _, err := e.Registration(identity.RegistrationApproval, true).SignUp(ctx, reason); !errors.Is(err, identity.ErrInvalidReason) {
			t.Errorf("SignUp with the reason %q... = %v, want ErrInvalidReason", r[:1], err)
		}
	}
	nul := SignUpInput("carol")
	nul.Email = "carol\x00@example.com"
	if _, err := e.Registration(identity.RegistrationApproval, true).SignUp(ctx, nul); !errors.Is(err, identity.ErrInvalidEmail) {
		t.Errorf("SignUp with NUL in the email = %v, want ErrInvalidEmail", err)
	}
}

func registrationRateLimit(t *testing.T, e *AccountEnv) {
	r := e.Registration(identity.RegistrationOpen, true)
	for i := range appidentity.SignUpIPLimit.Max {
		in := SignUpInput(fmt.Sprintf("user%d", i))
		in.IP = "192.0.2.1"
		r.MustSignUp(t, in)
	}
	in := SignUpInput("late")
	in.IP = "192.0.2.1"
	var limited *kernel.RateLimitedError
	if _, err := r.SignUp(t.Context(), in); !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Fatalf("SignUp beyond the limit = %v, want *kernel.RateLimitedError", err)
	}
	in.IP = "198.51.100.1"
	r.MustSignUp(t, in)
}

func registrationInvites(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	rec := &AuditRecorder{}
	e.Projectors.Register(appmoderation.EventAuditRecorded, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		rec.Add(ev.(appmoderation.AuditRecorded).Entry)
		return nil
	})
	r := e.Registration(identity.RegistrationInvite, true)
	a, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{MaxUses: 5})
	if err != nil {
		t.Fatal(err)
	}
	e.Clock.Advance(time.Second)
	b, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{CreatedByUserID: testkit.NamedID("admin"), ExpiresIn: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if a.Code == b.Code {
		t.Fatal("two invites got the same code")
	}
	if _, err := r.RevokeInvite(ctx, strings.ToLower(a.Code), testkit.NamedID("admin")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RevokeInvite(ctx, a.Code, ""); err != nil {
		t.Errorf("revoking again = %v, want nil", err)
	}
	if _, err := r.RevokeInvite(ctx, "NOSUCHCODE", ""); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("revoking an unknown code = %v, want ErrNotFound", err)
	}
	list, err := r.ListInvites(ctx, "", 10)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListInvites = %+v, %v", list, err)
	}
	now := e.Clock.Now()
	if list[0].ID != b.ID || list[0].Revoked || !list[0].Usable(now) || list[0].CreatedByUserID != testkit.NamedID("admin") ||
		list[0].ExpiresAt == nil || !list[0].ExpiresAt.Equal(b.CreatedAt.Add(24*time.Hour)) {
		t.Errorf("list[0] = %+v; want b, usable", list[0])
	}
	if list[1].ID != a.ID || !list[1].Revoked || list[1].Usable(now) || list[1].MaxUses != 5 || list[1].CreatedByUserID != "" {
		t.Errorf("list[1] = %+v; want a, revoked", list[1])
	}
	if _, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{MaxUses: -1}); err == nil {
		t.Error("CreateInvite with negative uses succeeded")
	}
	if list[0].State != appidentity.InviteUsable || list[1].State != appidentity.InviteRevoked ||
		list[0].StateAt(b.CreatedAt.Add(24*time.Hour)) != appidentity.InviteExpired {
		t.Errorf("states = %s, %s, expired b %s; want usable, revoked, expired",
			list[0].State, list[1].State, list[0].StateAt(b.CreatedAt.Add(24*time.Hour)))
	}

	// The second page starts after the last invite of the first.
	page, err := r.ListInvites(ctx, b.ID, 10)
	if err != nil || len(page) != 1 || page[0].ID != a.ID {
		t.Errorf("ListInvites(before b) = %+v, %v; want a", page, err)
	}
	got, err := r.RevokeInviteByID(ctx, b.ID, testkit.NamedID("admin"))
	if err != nil || got.ID != b.ID || !got.Revoked || got.Uses != 0 || got.Code != b.Code || got.State != appidentity.InviteRevoked {
		t.Errorf("RevokeInviteByID(b) = %+v, %v; want b, revoked", got, err)
	}
	if _, err := r.RevokeInviteByID(ctx, testkit.NamedID("no-such-invite"), testkit.NamedID("admin")); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("RevokeInviteByID(unknown) = %v, want ErrNotFound", err)
	}

	// Each creation and revocation is audited once, by the user or the CLI; revoking a revoked
	// invite again changes nothing and is not. The label never holds the whole code.
	type audited struct {
		action moderation.AuditAction
		invite identity.Invite
		actor  moderation.AuditActorKind
		user   string
	}
	want := []audited{
		{moderation.AuditInviteCreated, a, moderation.AuditActorCLI, ""},
		{moderation.AuditInviteCreated, b, moderation.AuditActorUser, testkit.NamedID("admin")},
		{moderation.AuditInviteRevoked, a, moderation.AuditActorUser, testkit.NamedID("admin")},
		{moderation.AuditInviteRevoked, b, moderation.AuditActorUser, testkit.NamedID("admin")},
	}
	entries := rec.All()
	if len(entries) != len(want) {
		t.Fatalf("audit entries = %+v, want %d", entries, len(want))
	}
	for i, w := range want {
		g := entries[i]
		if g.Action != w.action || g.TargetKind != moderation.AuditTargetInvite || g.TargetID != w.invite.ID ||
			g.SourceEventID != w.invite.ID || g.ActorKind != w.actor || g.ActorUserID != w.user ||
			g.TargetLabel != w.invite.Code[:4]+"…" {
			t.Errorf("audit entry %d = %+v, want %s of %s", i, g, w.action, w.invite.ID)
		}
		if err := g.Validate(); err != nil {
			t.Errorf("audit entry %d: %v", i, err)
		}
	}
}

func registrationAdmin(t *testing.T, e *AccountEnv) {
	r := e.Registration(identity.RegistrationClosed, true)
	reg, err := r.CreateAdmin(t.Context(), appidentity.CreateAdminInput{Username: "root", Email: "root@example.com", Password: "admin password"})
	if err != nil {
		t.Fatal(err)
	}
	if a := r.access(t, reg.User.ID); !a.Allowed() || !a.EmailVerified || a.Registration != identity.RegistrationApproved {
		t.Errorf("admin access = %+v; want allowed", a)
	}
}

// registrationModerators checks what the moderator notifications read of the identity module:
// the count of pending registrations, the holders of a permission, and their mailboxes.
func registrationModerators(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	r := e.Registration(identity.RegistrationApproval, true)
	count := func(want int64) {
		t.Helper()
		if n, err := r.CountPendingRegistrations(ctx); err != nil || n != want {
			t.Errorf("CountPendingRegistrations = %d, %v; want %d", n, err, want)
		}
	}
	root, err := r.CreateAdmin(ctx, appidentity.CreateAdminInput{Username: "root", Email: "root@example.com", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	count(0)
	alice := r.MustSignUp(t, SignUpInput("alice"))
	count(1)

	holders := func(p identity.Permission) []appidentity.PermissionHolder {
		t.Helper()
		var out []appidentity.PermissionHolder
		err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			out, err = r.PermissionHolders(ctx, tx, p)
			return err
		})
		if err != nil {
			t.Fatalf("PermissionHolders(%s) = %v", p, err)
		}
		return out
	}
	want := []appidentity.PermissionHolder{{UserID: root.User.ID, AccountID: root.AccountID}}
	for _, p := range []identity.Permission{identity.PermManageReports, identity.PermManageAccounts} {
		if got := holders(p); !reflect.DeepEqual(got, want) {
			t.Errorf("PermissionHolders(%s) = %+v, want %+v", p, got, want)
		}
	}

	mailbox := func(userID string, wantOK bool) {
		t.Helper()
		box, ok, err := r.NotificationMailbox(ctx, userID)
		if err != nil || ok != wantOK || (ok && (box.Address != "root@example.com" || box.Locale != "en" || box.EmailAddressID == "")) {
			t.Errorf("NotificationMailbox(%s) = %+v, %v, %v; want ok %v", userID, box, ok, err, wantOK)
		}
	}
	mailbox(root.User.ID, true)
	mailbox(alice.User.ID, false) // an unverified address gets no email

	if _, err := r.DecideRegistration(ctx, appidentity.DecideInput{UserID: alice.User.ID, Decision: identity.DecisionApproved}); err != nil {
		t.Fatal(err)
	}
	count(0)
}
