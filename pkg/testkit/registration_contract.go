// SPDX-License-Identifier: AGPL-3.0-only

package testkit

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
)

// RegistrationRepositoryContract verifies the registration persistence of a database adapter's
// identity repository (data-model §4.1) through the sign-up use cases, one case per
// registration mode: open mode approves at once (or waits for a moderator when email cannot be
// verified), approval mode waits for a moderator's decision, invite mode needs a valid invite,
// and closed mode refuses. It also checks that concurrent sign-ups never redeem an invite more
// than its maximum uses, that concurrent decisions on a registration decide it once, that an email verification token works exactly once (also when
// redeemed concurrently), that a failed sign-up writes nothing, that sign-ups are rate-limited
// per address, the invite listing and revocation, that a sign-up records its origin, which
// the purge erases after the retention, the count of pending registrations, and the holders of
// a permission with where their moderator notifications are emailed, and the invites that members
// create (ADR-0108): who may, the caps, and the check of their creator at every sign-up in the
// open, approval, and invite modes. It uses the same harness as AccountRepositoryContract.
func RegistrationRepositoryContract(t *testing.T, newHarness AccountFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *accountEnv)
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
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newAccountEnv(t, newHarness)) })
	}
}

// registration is an identity service in one registration mode with a recording mailer.
type registration struct {
	*appidentity.Service
	mailer *Mailer
}

func (e *accountEnv) registration(mode identity.RegistrationMode, requireVerification bool) registration {
	m := &Mailer{}
	return registration{
		Service: e.newIdentityWith(func(d *appidentity.Deps) {
			d.RegistrationMode = func(context.Context) (identity.RegistrationMode, error) { return mode, nil }
			d.Mailer, d.RequireEmailVerification = m, requireVerification
		}),
		mailer: m,
	}
}

func signUpInput(name string) appidentity.SignUpInput {
	return appidentity.SignUpInput{
		Username: name, Email: name + "@example.com", Password: "correct horse " + name, Locale: "ja", UserAgent: "contract",
	}
}

func (r registration) signUp(t *testing.T, in appidentity.SignUpInput) appidentity.SignedUp {
	t.Helper()
	got, err := r.SignUp(t.Context(), in)
	if err != nil {
		t.Fatalf("SignUp(%s) = %v", in.Username, err)
	}
	return got
}

func (r registration) access(t *testing.T, userID string) appidentity.Access {
	t.Helper()
	a, err := r.UserAccess(t.Context(), userID)
	if err != nil {
		t.Fatalf("UserAccess(%s) = %v", userID, err)
	}
	return a
}

var verifyLink = regexp.MustCompile(`https://` + regexp.QuoteMeta(contractHost) + `/auth/verify_email\?token=(\S+)`)

// lastToken returns the token of the newest verification email sent to address.
func (r registration) lastToken(t *testing.T, address string) string {
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

func registrationOpen(t *testing.T, e *accountEnv) {
	r := e.registration(identity.RegistrationOpen, true)
	in := signUpInput("alice")
	in.Reason = "ignored outside approval mode"
	got := r.signUp(t, in)
	if got.State != identity.RegistrationApproved {
		t.Fatalf("SignUp = state %s, want approved", got.State)
	}
	app, err := e.h.Identity.FindRegistrationApplication(t.Context(), got.User.ID)
	if err != nil || app.Reason != "" || app.InviteID != "" || !app.SubmittedAt.Equal(got.User.CreatedAt) {
		t.Errorf("application = %+v, %v; want one without reason or invite", app, err)
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
	if len(sent) != 1 || sent[0].To != in.Email || !strings.Contains(sent[0].Subject, contractHost) || !strings.Contains(sent[0].Text, "24 時間") {
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

func registrationOpenWithoutEmail(t *testing.T, e *accountEnv) {
	r := e.registration(identity.RegistrationOpen, false)
	got := r.signUp(t, signUpInput("alice"))
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

func registrationApproval(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	r := e.registration(identity.RegistrationApproval, true)
	in := signUpInput("alice")
	in.Reason = "  I like ducks.  "
	alice := r.signUp(t, in)
	e.clock.Advance(time.Second)
	bob := r.signUp(t, signUpInput("bob"))
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
	if _, err := r.Registration(ctx, NamedID("nobody")); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("Registration(nobody) = %v, want ErrNotFound", err)
	}
	p := pending[0]
	if p.Username != "alice" || p.Email != in.Email || p.EmailVerified || p.Application.Reason != "I like ducks." ||
		!p.State.Since.Equal(alice.User.CreatedAt) {
		t.Errorf("pending[0] = %+v", p)
	}

	e.clock.Advance(time.Minute)
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
	d, err := r.DecideRegistration(ctx, appidentity.DecideInput{UserID: alice.User.ID, Decision: identity.DecisionApproved, DecidedByUserID: NamedID("mod"), Note: "welcome"})
	if err != nil || d.DecidedBy != identity.DecidedByModerator || d.ApplicationID != p.Application.ID {
		t.Fatalf("approve = %+v, %v", d, err)
	}
	if _, err := r.DecideRegistration(ctx, appidentity.DecideInput{UserID: alice.User.ID, Decision: identity.DecisionRejected}); !errors.Is(err, appidentity.ErrNotPending) {
		t.Errorf("deciding again = %v, want ErrNotPending", err)
	}
	if _, err := r.DecideRegistration(ctx, appidentity.DecideInput{UserID: bob.User.ID, Decision: identity.DecisionRejected}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DecideRegistration(ctx, appidentity.DecideInput{UserID: NamedID("nobody"), Decision: identity.DecisionApproved}); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("deciding on an unknown user = %v, want ErrNotFound", err)
	}
	st, err := e.h.Identity.FindRegistrationState(ctx, alice.User.ID)
	if err != nil || st.State != identity.RegistrationApproved || !st.Since.Equal(e.clock.Now()) {
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

func registrationInvite(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	r := e.registration(identity.RegistrationInvite, true)
	if _, err := r.SignUp(ctx, signUpInput("alice")); !errors.Is(err, appidentity.ErrInviteRequired) {
		t.Errorf("SignUp without a code = %v, want ErrInviteRequired", err)
	}
	in := signUpInput("alice")
	in.InviteCode = "NOSUCHCODE"
	if _, err := r.SignUp(ctx, in); !errors.Is(err, appidentity.ErrInvalidInvite) {
		t.Errorf("SignUp with an unknown code = %v, want ErrInvalidInvite", err)
	}
	inv, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{CreatedByUserID: NamedID("admin"), MaxUses: 1, ExpiresIn: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Code) != identity.InviteCodeLength || inv.MaxUses != 1 || inv.ExpiresAt == nil {
		t.Fatalf("invite = %+v", inv)
	}
	// Codes compare without case, spaces, and hyphens.
	in.InviteCode = strings.ToLower(inv.Code[:4]) + "-" + inv.Code[4:8] + " " + inv.Code[8:]
	got := r.signUp(t, in)
	if got.State != identity.RegistrationApproved {
		t.Errorf("state = %s; want approved", got.State)
	}
	if app, err := e.h.Identity.FindRegistrationApplication(ctx, got.User.ID); err != nil || app.InviteID != inv.ID {
		t.Errorf("application = %+v, %v; want the invite", app, err)
	}
	bob := signUpInput("bob")
	bob.InviteCode = inv.Code
	if _, err := r.SignUp(ctx, bob); !errors.Is(err, appidentity.ErrInvalidInvite) {
		t.Errorf("SignUp with a used-up invite = %v, want ErrInvalidInvite", err)
	}

	expiring, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{ExpiresIn: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Minute)
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
	r.signUp(t, bob)
}

func registrationClosed(t *testing.T, e *accountEnv) {
	r := e.registration(identity.RegistrationClosed, true)
	if _, err := r.SignUp(t.Context(), signUpInput("alice")); !errors.Is(err, appidentity.ErrRegistrationClosed) {
		t.Fatalf("SignUp = %v, want ErrRegistrationClosed", err)
	}
	if _, err := e.h.Identity.FindUserIDByEmail(t.Context(), "alice@example.com"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("a refused sign-up wrote a user: %v", err)
	}
	if len(r.mailer.Sent()) != 0 {
		t.Error("a refused sign-up sent email")
	}
}

func registrationInviteConcurrency(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	r := e.registration(identity.RegistrationInvite, true)
	const maxUses, applicants = 3, 8
	inv, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{MaxUses: maxUses})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, applicants)
	for i := range applicants {
		wg.Go(func() {
			in := signUpInput(fmt.Sprintf("user%d", i))
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
	if err != nil || len(list) != 1 || list[0].Uses != maxUses || list[0].Usable(e.clock.Now()) {
		t.Errorf("ListInvites = %+v, %v; want %d uses and used up", list, err, maxUses)
	}
}

func registrationDecisionConcurrency(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	r := e.registration(identity.RegistrationApproval, true)
	alice := r.signUp(t, signUpInput("alice"))
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
			_, errs[i] = r.DecideRegistration(ctx, appidentity.DecideInput{UserID: alice.User.ID, Decision: decisions[i], DecidedByUserID: NamedID(fmt.Sprintf("mod%d", i))})
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
	if st, err := e.h.Identity.FindRegistrationState(ctx, alice.User.ID); err != nil || st.State != won[0].State() {
		t.Errorf("state = %+v, %v; want %s", st, err, won[0].State())
	}
}

func registrationTokenOnce(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	r := e.registration(identity.RegistrationOpen, true)
	alice := r.signUp(t, signUpInput("alice"))
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
	bob := r.signUp(t, signUpInput("bob"))
	first := r.lastToken(t, "bob@example.com")
	if err := r.ResendVerificationEmail(ctx, bob.User.ID); err != nil {
		t.Fatalf("ResendVerificationEmail = %v", err)
	}
	second := r.lastToken(t, "bob@example.com")
	if second == first {
		t.Fatal("the resent email carries the same token")
	}
	e.clock.Advance(appidentity.EmailVerificationTTL)
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

func registrationFailureWritesNothing(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	r := e.registration(identity.RegistrationInvite, true)
	inv, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	alice := signUpInput("alice")
	alice.InviteCode = inv.Code
	if _, err := e.identity.CreateAdmin(ctx, appidentity.CreateAdminInput{Username: "root", Email: alice.Email, Password: "admin password"}); err != nil {
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
	got := r.signUp(t, alice)
	if _, err := e.h.Identity.FindUserIDByEmail(ctx, "other@example.com"); !errors.Is(err, appidentity.ErrNotFound) {
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
		reason := signUpInput("carol")
		reason.Reason = r
		if _, err := e.registration(identity.RegistrationApproval, true).SignUp(ctx, reason); !errors.Is(err, identity.ErrInvalidReason) {
			t.Errorf("SignUp with the reason %q... = %v, want ErrInvalidReason", r[:1], err)
		}
	}
	nul := signUpInput("carol")
	nul.Email = "carol\x00@example.com"
	if _, err := e.registration(identity.RegistrationApproval, true).SignUp(ctx, nul); !errors.Is(err, identity.ErrInvalidEmail) {
		t.Errorf("SignUp with NUL in the email = %v, want ErrInvalidEmail", err)
	}
}

func registrationRateLimit(t *testing.T, e *accountEnv) {
	r := e.registration(identity.RegistrationOpen, true)
	for i := range appidentity.SignUpIPLimit.Max {
		in := signUpInput(fmt.Sprintf("user%d", i))
		in.IP = "192.0.2.1"
		r.signUp(t, in)
	}
	in := signUpInput("late")
	in.IP = "192.0.2.1"
	var limited *kernel.RateLimitedError
	if _, err := r.SignUp(t.Context(), in); !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Fatalf("SignUp beyond the limit = %v, want *kernel.RateLimitedError", err)
	}
	in.IP = "198.51.100.1"
	r.signUp(t, in)
}

func registrationInvites(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	rec := &auditRecorder{}
	e.projectors.Register(appmoderation.EventAuditRecorded, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		rec.add(ev.(appmoderation.AuditRecorded).Entry)
		return nil
	})
	r := e.registration(identity.RegistrationInvite, true)
	a, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{MaxUses: 5})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Second)
	b, err := r.CreateInvite(ctx, appidentity.CreateInviteInput{CreatedByUserID: NamedID("admin"), ExpiresIn: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if a.Code == b.Code {
		t.Fatal("two invites got the same code")
	}
	if _, err := r.RevokeInvite(ctx, strings.ToLower(a.Code), NamedID("admin")); err != nil {
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
	now := e.clock.Now()
	if list[0].ID != b.ID || list[0].Revoked || !list[0].Usable(now) || list[0].CreatedByUserID != NamedID("admin") ||
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
	got, err := r.RevokeInviteByID(ctx, b.ID, NamedID("admin"))
	if err != nil || got.ID != b.ID || !got.Revoked || got.Uses != 0 || got.Code != b.Code || got.State != appidentity.InviteRevoked {
		t.Errorf("RevokeInviteByID(b) = %+v, %v; want b, revoked", got, err)
	}
	if _, err := r.RevokeInviteByID(ctx, NamedID("no-such-invite"), NamedID("admin")); !errors.Is(err, appidentity.ErrNotFound) {
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
		{moderation.AuditInviteCreated, b, moderation.AuditActorUser, NamedID("admin")},
		{moderation.AuditInviteRevoked, a, moderation.AuditActorUser, NamedID("admin")},
		{moderation.AuditInviteRevoked, b, moderation.AuditActorUser, NamedID("admin")},
	}
	entries := rec.all()
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

func registrationAdmin(t *testing.T, e *accountEnv) {
	r := e.registration(identity.RegistrationClosed, true)
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
func registrationModerators(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	r := e.registration(identity.RegistrationApproval, true)
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
	alice := r.signUp(t, signUpInput("alice"))
	count(1)

	holders := func(p identity.Permission) []appidentity.PermissionHolder {
		t.Helper()
		var out []appidentity.PermissionHolder
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
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
