// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"encoding/base32"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The two-factor authentication cases of AccountRepositoryContract (FR-005).

// totpUser is the session user with TOTP enabled: its secret and recovery codes.
type totpUser struct {
	reg    appidentity.Registration
	secret []byte
	codes  []string
}

// code returns the TOTP code of the user at the env's time, after moving the clock to the next
// step so that every call returns a code not used before.
func (u totpUser) code(e *accountEnv) string {
	e.clock.Advance(identity.TOTPPeriod)
	return identity.TOTPCode(u.secret, identity.TOTPStep(e.clock.Now()))
}

func (e *accountEnv) enableTOTP(t *testing.T, reg appidentity.Registration) totpUser {
	t.Helper()
	ctx := t.Context()
	setup, err := e.identity.BeginTOTPSetup(ctx, reg.User.ID)
	if err != nil {
		t.Fatalf("BeginTOTPSetup = %v", err)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup.Secret)
	if err != nil || len(secret) != identity.TOTPSecretSize {
		t.Fatalf("setup secret %q = %d bytes, %v; want %d bytes of base32", setup.Secret, len(secret), err, identity.TOTPSecretSize)
	}
	codes, err := e.identity.EnableTOTP(ctx, reg.User.ID, setup.Token, identity.TOTPCode(secret, identity.TOTPStep(e.clock.Now())))
	if err != nil {
		t.Fatalf("EnableTOTP = %v", err)
	}
	return totpUser{reg: reg, secret: secret, codes: codes}
}

// signInFirstStep signs in with the session user's password and returns the challenge.
func (e *accountEnv) signInFirstStep(t *testing.T) string {
	t.Helper()
	_, err := e.identity.SignIn(t.Context(), appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword, IP: "192.0.2.1"})
	var need *appidentity.SecondFactorRequiredError
	if !errors.As(err, &need) || !errors.Is(err, appidentity.ErrSecondFactorRequired) ||
		!strings.HasPrefix(need.Challenge, appidentity.SecondFactorChallengePrefix) {
		t.Fatalf("SignIn = %v, want a *SecondFactorRequiredError with a challenge", err)
	}
	return need.Challenge
}

func (e *accountEnv) completeSignIn(challenge, code string) (appidentity.SignedIn, error) {
	return e.identity.CompleteSignIn(context.Background(), appidentity.SecondFactorInput{Challenge: challenge, Code: code, IP: "192.0.2.1", UserAgent: "ua"})
}

func totpEnrollsAndSignsIn(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	if st, err := e.identity.TwoFactorStatus(ctx, reg.User.ID); err != nil || st.Enabled {
		t.Fatalf("TwoFactorStatus before enrolling = %+v, %v; want disabled", st, err)
	}
	setup, err := e.identity.BeginTOTPSetup(ctx, reg.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(setup.URI)
	if err != nil || u.Scheme != "otpauth" || u.Host != "totp" || u.Path != "/"+contractHost+":alice@"+contractHost ||
		u.Query().Get("secret") != setup.Secret || u.Query().Get("issuer") != contractHost || u.Query().Get("digits") != "6" {
		t.Errorf("setup URI = %q, want otpauth://totp/%s:alice@%s with the secret", setup.URI, contractHost, contractHost)
	}
	secret, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup.Secret)
	now := identity.TOTPStep(e.clock.Now())
	if _, err := e.identity.EnableTOTP(ctx, reg.User.ID, setup.Token, identity.TOTPCode(secret, now+5)); !errors.Is(err, appidentity.ErrInvalidSecondFactor) {
		t.Errorf("EnableTOTP(wrong code) = %v, want ErrInvalidSecondFactor", err)
	}
	other := e.register(t, "mallory")
	if _, err := e.identity.EnableTOTP(ctx, other.User.ID, setup.Token, identity.TOTPCode(secret, identity.TOTPStep(e.clock.Now()))); !errors.Is(err, appidentity.ErrInvalidTOTPSetup) {
		t.Errorf("EnableTOTP(another user's setup) = %v, want ErrInvalidTOTPSetup", err)
	}
	if _, err := e.identity.EnableTOTP(ctx, reg.User.ID, "garbage", "123456"); !errors.Is(err, appidentity.ErrInvalidTOTPSetup) {
		t.Errorf("EnableTOTP(garbage setup) = %v, want ErrInvalidTOTPSetup", err)
	}
	enabledCode := identity.TOTPCode(secret, identity.TOTPStep(e.clock.Now()))
	codes, err := e.identity.EnableTOTP(ctx, reg.User.ID, setup.Token, enabledCode)
	if err != nil || len(codes) != identity.RecoveryCodeCount {
		t.Fatalf("EnableTOTP = %d codes, %v; want %d", len(codes), err, identity.RecoveryCodeCount)
	}
	st, err := e.identity.TwoFactorStatus(ctx, reg.User.ID)
	if err != nil || !st.Enabled || !st.EnrolledAt.Equal(e.clock.Now()) || st.RecoveryCodesRemaining != identity.RecoveryCodeCount {
		t.Errorf("TwoFactorStatus = %+v, %v; want enabled now with %d recovery codes", st, err, identity.RecoveryCodeCount)
	}
	if _, err := e.identity.BeginTOTPSetup(ctx, reg.User.ID); !errors.Is(err, appidentity.ErrTwoFactorEnabled) {
		t.Errorf("BeginTOTPSetup when enabled = %v, want ErrTwoFactorEnabled", err)
	}

	// The password alone starts no session; the code that confirmed the setup does not either.
	challenge := e.signInFirstStep(t)
	if _, err := e.completeSignIn(challenge, enabledCode); !errors.Is(err, appidentity.ErrInvalidSecondFactor) {
		t.Errorf("CompleteSignIn(the code used to enable) = %v, want ErrInvalidSecondFactor", err)
	}
	if _, err := e.completeSignIn("my_2fa_unknown", identity.TOTPCode(secret, identity.TOTPStep(e.clock.Now())+1)); !errors.Is(err, appidentity.ErrSecondFactorExpired) {
		t.Errorf("CompleteSignIn(unknown challenge) = %v, want ErrSecondFactorExpired", err)
	}
	user := totpUser{reg: reg, secret: secret, codes: codes}
	code := user.code(e)
	got, err := e.completeSignIn(challenge, " "+code[:3]+" "+code[3:])
	if err != nil {
		t.Fatalf("CompleteSignIn = %v", err)
	}
	if s, err := e.identity.Authenticate(ctx, got.Token); err != nil || s.UserID != reg.User.ID {
		t.Errorf("Authenticate(new session) = %+v, %v; want a session of %s", s, err, reg.User.ID)
	}
	if o, err := e.h.Identity.FindSessionOrigin(ctx, got.Session.ID); err != nil || o.IP != "192.0.2.1" {
		t.Errorf("FindSessionOrigin = %+v, %v; want the sign-in's origin", o, err)
	}
	if _, err := e.completeSignIn(challenge, user.code(e)); !errors.Is(err, appidentity.ErrSecondFactorExpired) {
		t.Errorf("CompleteSignIn(used challenge) = %v, want ErrSecondFactorExpired", err)
	}
	// A code works once.
	challenge = e.signInFirstStep(t)
	if _, err := e.completeSignIn(challenge, code); !errors.Is(err, appidentity.ErrInvalidSecondFactor) {
		t.Errorf("CompleteSignIn(reused code) = %v, want ErrInvalidSecondFactor", err)
	}
	// The challenge expires.
	e.clock.Advance(appidentity.SecondFactorChallengeTTL)
	if _, err := e.completeSignIn(challenge, identity.TOTPCode(secret, identity.TOTPStep(e.clock.Now()))); !errors.Is(err, appidentity.ErrSecondFactorExpired) {
		t.Errorf("CompleteSignIn(expired challenge) = %v, want ErrSecondFactorExpired", err)
	}
	// A setup token expires.
	setup, err = e.identity.BeginTOTPSetup(ctx, other.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(appidentity.TOTPSetupTTL)
	secret, _ = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup.Secret)
	if _, err := e.identity.EnableTOTP(ctx, other.User.ID, setup.Token, identity.TOTPCode(secret, identity.TOTPStep(e.clock.Now()))); !errors.Is(err, appidentity.ErrInvalidTOTPSetup) {
		t.Errorf("EnableTOTP(expired setup) = %v, want ErrInvalidTOTPSetup", err)
	}
}

func totpRecoveryCodesWorkOnce(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	u := e.enableTOTP(t, e.sessionUser(t))
	got, err := e.completeSignIn(e.signInFirstStep(t), " "+strings.ToUpper(u.codes[3])+" ")
	if err != nil {
		t.Fatalf("CompleteSignIn(recovery code) = %v", err)
	}
	if _, err := e.identity.Authenticate(ctx, got.Token); err != nil {
		t.Errorf("Authenticate = %v", err)
	}
	if st, _ := e.identity.TwoFactorStatus(ctx, u.reg.User.ID); st.RecoveryCodesRemaining != identity.RecoveryCodeCount-1 {
		t.Errorf("recovery codes remaining = %d, want %d", st.RecoveryCodesRemaining, identity.RecoveryCodeCount-1)
	}
	challenge := e.signInFirstStep(t)
	for _, code := range []string{u.codes[3], "aaaa-aaaa-aaaa-aaaa", "not a code"} {
		if _, err := e.completeSignIn(challenge, code); !errors.Is(err, appidentity.ErrInvalidSecondFactor) {
			t.Errorf("CompleteSignIn(%q) = %v, want ErrInvalidSecondFactor", code, err)
		}
	}
	if _, err := e.completeSignIn(challenge, u.codes[4]); err != nil {
		t.Errorf("CompleteSignIn(another recovery code) = %v", err)
	}
}

func totpRateLimitsWrongCodes(t *testing.T, e *accountEnv) {
	u := e.enableTOTP(t, e.sessionUser(t))
	challenge := e.signInFirstStep(t)
	attempt := func(ip, code string) error {
		_, err := e.identity.CompleteSignIn(t.Context(), appidentity.SecondFactorInput{Challenge: challenge, Code: code, IP: ip})
		return err
	}
	// Wrong codes count against the client's address...
	for i := range int(appidentity.LoginIPLimit.Max) {
		if err := attempt("198.51.100.1", "aaaa-aaaa-aaaa-aaaa"); !errors.Is(err, appidentity.ErrInvalidSecondFactor) {
			t.Fatalf("attempt %d = %v, want ErrInvalidSecondFactor", i+1, err)
		}
	}
	if err := attempt("198.51.100.1", u.code(e)); !errors.Is(err, kernel.ErrRateLimited) {
		t.Errorf("CompleteSignIn from the address after %d wrong codes = %v, want rate limited", appidentity.LoginIPLimit.Max, err)
	}
	// ...and against the user, from any address.
	for i := int(appidentity.LoginIPLimit.Max); i < secondFactorLimitMax(); i++ {
		if err := attempt("198.51.100."+strconv.Itoa(i+10), "aaaa-aaaa-aaaa-aaaa"); !errors.Is(err, appidentity.ErrInvalidSecondFactor) {
			t.Fatalf("attempt %d = %v, want ErrInvalidSecondFactor", i+1, err)
		}
	}
	if err := attempt("203.0.113.1", u.code(e)); !errors.Is(err, kernel.ErrRateLimited) {
		t.Errorf("CompleteSignIn after %d wrong codes = %v, want rate limited", secondFactorLimitMax(), err)
	}
	if err := e.identity.DisableTOTP(t.Context(), u.reg.User.ID, u.code(e)); !errors.Is(err, kernel.ErrRateLimited) {
		t.Errorf("DisableTOTP after wrong codes = %v, want rate limited", err)
	}
}

// secondFactorLimitMax is the number of wrong codes appidentity.SecondFactorLimit allows.
func secondFactorLimitMax() int { return int(appidentity.SecondFactorLimit.Max) }

func totpRegeneratesAndDisables(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	u := e.enableTOTP(t, e.sessionUser(t))
	if _, err := e.identity.RegenerateRecoveryCodes(ctx, u.reg.User.ID, "123"); !errors.Is(err, appidentity.ErrInvalidSecondFactor) {
		t.Errorf("RegenerateRecoveryCodes(malformed) = %v, want ErrInvalidSecondFactor", err)
	}
	fresh, err := e.identity.RegenerateRecoveryCodes(ctx, u.reg.User.ID, u.code(e))
	if err != nil || len(fresh) != identity.RecoveryCodeCount {
		t.Fatalf("RegenerateRecoveryCodes = %d codes, %v", len(fresh), err)
	}
	// The app keeps working; the old recovery codes do not.
	if _, err := e.completeSignIn(e.signInFirstStep(t), u.code(e)); err != nil {
		t.Errorf("CompleteSignIn(code after regenerating) = %v", err)
	}
	challenge := e.signInFirstStep(t)
	if _, err := e.completeSignIn(challenge, u.codes[0]); !errors.Is(err, appidentity.ErrInvalidSecondFactor) {
		t.Errorf("CompleteSignIn(old recovery code) = %v, want ErrInvalidSecondFactor", err)
	}
	if _, err := e.completeSignIn(challenge, fresh[0]); err != nil {
		t.Errorf("CompleteSignIn(new recovery code) = %v", err)
	}

	if err := e.identity.DisableTOTP(ctx, u.reg.User.ID, fresh[0]); !errors.Is(err, appidentity.ErrInvalidSecondFactor) {
		t.Errorf("DisableTOTP(used recovery code) = %v, want ErrInvalidSecondFactor", err)
	}
	if err := e.identity.DisableTOTP(ctx, u.reg.User.ID, fresh[1]); err != nil {
		t.Fatalf("DisableTOTP = %v", err)
	}
	if st, err := e.identity.TwoFactorStatus(ctx, u.reg.User.ID); err != nil || st.Enabled {
		t.Errorf("TwoFactorStatus after disabling = %+v, %v; want disabled", st, err)
	}
	if err := e.identity.DisableTOTP(ctx, u.reg.User.ID, u.code(e)); !errors.Is(err, appidentity.ErrTwoFactorNotEnabled) {
		t.Errorf("DisableTOTP twice = %v, want ErrTwoFactorNotEnabled", err)
	}
	e.signIn(t, appidentity.SignInInput{})
	// Enrolling again works.
	e.clock.Advance(time.Second)
	e.enableTOTP(t, u.reg)
	e.signInFirstStep(t)
}

// failingIncrKV is a KeyValueStore whose IncrBy fails once it has succeeded ok times.
type failingIncrKV struct {
	kernel.KeyValueStore
	ok int
}

var errIncrFailed = errors.New("testkit: IncrBy failed")

func (k *failingIncrKV) IncrBy(ctx context.Context, namespace, key string, n int64, ttl time.Duration) (int64, error) {
	if k.ok == 0 {
		return 0, errIncrFailed
	}
	k.ok--
	return k.KeyValueStore.IncrBy(ctx, namespace, key, n, ttl)
}

// totpKeepsCodesWhenAStepFails fails to record the used TOTP step after a confirmation: enabling
// TOTP enrolls nothing, and regenerating the recovery codes keeps the old ones, so that no
// recovery codes are stored that the user never saw.
func totpKeepsCodesWhenAStepFails(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	failing := func(ok int) *appidentity.Service {
		return e.newIdentityWith(func(d *appidentity.Deps) { d.KV = &failingIncrKV{KeyValueStore: e.kv, ok: ok} })
	}

	setup, err := e.identity.BeginTOTPSetup(ctx, reg.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	secret, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup.Secret)
	code := identity.TOTPCode(secret, identity.TOTPStep(e.clock.Now()))
	if _, err := failing(0).EnableTOTP(ctx, reg.User.ID, setup.Token, code); !errors.Is(err, errIncrFailed) {
		t.Fatalf("EnableTOTP with a failing store = %v, want its error", err)
	}
	if st, err := e.identity.TwoFactorStatus(ctx, reg.User.ID); err != nil || st.Enabled {
		t.Fatalf("TwoFactorStatus after a failed EnableTOTP = %+v, %v; want disabled", st, err)
	}

	u := e.enableTOTP(t, reg)
	// The confirmation's step is recorded; recording it under the new enrollment fails.
	if _, err := failing(1).RegenerateRecoveryCodes(ctx, reg.User.ID, u.code(e)); !errors.Is(err, errIncrFailed) {
		t.Fatalf("RegenerateRecoveryCodes with a failing store = %v, want its error", err)
	}
	if _, err := e.completeSignIn(e.signInFirstStep(t), u.codes[0]); err != nil {
		t.Errorf("CompleteSignIn(recovery code from before the failure) = %v, want it to work", err)
	}
}
