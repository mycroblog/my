// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	appaccount "github.com/mycroblog/my/internal/app/account"
	apphook "github.com/mycroblog/my/internal/app/hook"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/hook"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform"
)

// pluginNote is what the plugins of these tests write for the moderators; no applicant may read it.
const pluginNote = "the plugin's reason for the moderators"

// signupPlugin is a plugin of net/http and JSON that answers what a test sets and keeps what it was
// sent.
type signupPlugin struct {
	verdict atomic.Value // the verdict's name; "" answers 503, "block" waits for the deadline
	mu      sync.Mutex
	bodies  []signupRequest
}

type signupRequest struct {
	Hook       string `json:"hook"`
	SubjectKey string `json:"subjectKey"`
	Signup     struct {
		Email string `json:"email"`
		IP    string `json:"ip"`
	} `json:"signup"`
}

func (p *signupPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body signupRequest
	_ = json.Unmarshal(raw, &body)
	p.mu.Lock()
	p.bodies = append(p.bodies, body)
	p.mu.Unlock()
	switch v, _ := p.verdict.Load().(string); v {
	case "":
		http.Error(w, "down", http.StatusServiceUnavailable)
	case "block":
		<-r.Context().Done()
	default:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"verdict": v, "reason": "REASON_SPAM", "note": pluginNote})
	}
}

func (p *signupPlugin) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.bodies)
}

func (p *signupPlugin) last() signupRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.bodies[len(p.bodies)-1]
}

func newSignupPlugin(t *testing.T, verdict string) (*signupPlugin, string) {
	t.Helper()
	p := new(signupPlugin)
	p.verdict.Store(verdict)
	mux := http.NewServeMux()
	mux.Handle("POST /my.plugin.hook.v1.HookService/Check", p)
	return p, fakeplugin.ServeUnix(t, mux)
}

// signupHookEnv is a server that asks p about sign-ups and verifies email addresses, so that an open
// sign-up is approved at once unless something holds it.
type signupHookEnv struct {
	*statusesClient
	p *signupPlugin
	n atomic.Int64
}

func newSignupHookEnv(t *testing.T, timeout string) *signupHookEnv {
	t.Helper()
	p, addr := newSignupPlugin(t, "VERDICT_ALLOW")
	c := newStatusesClientWith(t, map[string]string{
		"MY_PLUGIN_SIGNUP_CHECK_ADDRESS": addr, "MY_PLUGIN_SIGNUP_CHECK_TIMEOUT": timeout,
		"MY_MAIL": "smtp", "MY_MAIL_FROM": "noreply@example.test", "MY_MAIL_SMTP_HOST": "smtp.example.test",
	}, platform.AppOptions{}, "carol")
	e := &signupHookEnv{statusesClient: c, p: p}
	e.set(instance.SettingRegistrationMode, string(instance.RegistrationOpen))
	return e
}

func (e *signupHookEnv) set(key instance.SettingKey, value string) {
	e.t.Helper()
	if _, err := e.app.Instance.SetSetting(e.t.Context(), appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
		e.t.Fatal(err)
	}
}

// signUp signs a new applicant up from an address of its own, which SignUpIPLimit does not count
// against the others.
func (e *signupHookEnv) signUp(ctx context.Context, invite string) (appidentity.SignedUp, string, error) {
	n := strconv.FormatInt(e.n.Add(1), 10)
	email := "applicant" + n + "@example.com"
	got, err := e.app.Identity.SignUp(ctx, appidentity.SignUpInput{
		Username: "applicant" + n, Email: email, Password: "correct horse battery staple",
		InviteCode: invite, IP: "192.0.2." + n,
	})
	return got, email, err
}

// decisionOf returns the record of the decision on an account.
func (e *signupHookEnv) decisionOf(accountID string, outcome hook.Outcome) (hook.Record, bool) {
	e.t.Helper()
	records, err := e.app.Hooks.List(e.t.Context(), apphook.ListQuery{Hook: hook.SignupCheck, Outcome: outcome, Limit: 100})
	if err != nil {
		e.t.Fatal(err)
	}
	for _, r := range records {
		if r.Decision.AccountID == accountID {
			return r, true
		}
	}
	return hook.Record{}, false
}

// TestSignupCheck drives the hook signup.check on the assembled server with a real plugin over a
// Unix socket, from observe through enforce.
func TestSignupCheck(t *testing.T) {
	t.Parallel()
	e := newSignupHookEnv(t, "20s")
	ctx := t.Context()

	wantState := func(t *testing.T, want domainidentity.RegistrationState) appidentity.SignedUp {
		t.Helper()
		got, _, err := e.signUp(ctx, "")
		if err != nil {
			t.Fatalf("SignUp: %v", err)
		}
		if got.State != want {
			t.Errorf("State = %q, want %q", got.State, want)
		}
		return got
	}

	t.Run("observe records and acts on nothing, and sends the address and the IP", func(t *testing.T) {
		e.p.verdict.Store("VERDICT_REJECT")
		before := e.p.calls()
		got, email, err := e.signUp(ctx, "")
		if err != nil || got.State != domainidentity.RegistrationApproved {
			t.Fatalf("SignUp = %+v, %v", got, err)
		}
		if e.p.calls() != before+1 {
			t.Fatalf("plugin calls = %d, want %d", e.p.calls(), before+1)
		}
		body := e.p.last()
		if body.Hook != "signup.check" || body.SubjectKey != got.AccountID || body.Signup.Email != email || !strings.HasPrefix(body.Signup.IP, "192.0.2.") {
			t.Errorf("sent %+v, account %s, email %s", body, got.AccountID, email)
		}
		rec, ok := e.decisionOf(got.AccountID, hook.Reject)
		if !ok || rec.Decision.Mode != hook.ModeObserve || rec.Decision.SubjectKey != got.AccountID {
			t.Errorf("decision = %+v, %v", rec, ok)
		}
	})

	e.set(instance.SettingHookSignupCheckMode, string(hook.ModeEnforce))

	t.Run("allow", func(t *testing.T) {
		e.p.verdict.Store("VERDICT_ALLOW")
		wantState(t, domainidentity.RegistrationApproved)
	})

	t.Run("reject has no effect unless enabled", func(t *testing.T) {
		e.p.verdict.Store("VERDICT_REJECT")
		got := wantState(t, domainidentity.RegistrationApproved)
		if _, ok := e.decisionOf(got.AccountID, hook.Reject); !ok {
			t.Error("the reject was not recorded")
		}
	})

	t.Run("require_approval holds as the block list's switch to approval does", func(t *testing.T) {
		e.p.verdict.Store("VERDICT_REQUIRE_APPROVAL")
		got := wantState(t, domainidentity.RegistrationPending)
		rec, ok := e.decisionOf(got.AccountID, hook.RequireApproval)
		if !ok {
			t.Fatal("no decision")
		}
		audit, err := e.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetHookDecision}, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(audit) != 1 || audit[0].Action != moderation.AuditHookDecisionApplied || audit[0].ActorKind != moderation.AuditActorSystem || audit[0].TargetID != rec.Decision.ID {
			t.Errorf("audit = %+v", audit)
		}

		e.p.verdict.Store("VERDICT_ALLOW")
		if _, err := e.app.Moderation.AddSignupBlock(ctx, appmoderation.SignupBlockInput{
			Kind: moderation.SignupBlockIPRange, Value: "198.51.100.0/24", Severity: moderation.SignupRequireApproval,
		}); err != nil {
			t.Fatal(err)
		}
		blocked, err := e.app.Identity.SignUp(ctx, appidentity.SignUpInput{
			Username: "heldbyblock", Email: "heldbyblock@example.com", Password: "correct horse battery staple", IP: "198.51.100.7",
		})
		if err != nil || blocked.State != got.State {
			t.Errorf("the block list's switch = %+v, %v; want the state %q", blocked, err, got.State)
		}
	})

	e.set(instance.SettingHookSignupCheckReject, "true")

	t.Run("reject refuses with the error of the block list, and writes no account", func(t *testing.T) {
		e.p.verdict.Store("VERDICT_REJECT")
		_, email, err := e.signUp(ctx, "")
		if !errors.Is(err, appidentity.ErrSignUpRefused) || strings.Contains(err.Error(), pluginNote) {
			t.Fatalf("SignUp = %v, want ErrSignUpRefused alone", err)
		}
		e.p.verdict.Store("VERDICT_ALLOW")
		// The refused address stayed free.
		if _, err := e.app.Identity.SignUp(ctx, appidentity.SignUpInput{
			Username: "secondtry", Email: email, Password: "correct horse battery staple", IP: "203.0.113.9",
		}); err != nil {
			t.Errorf("the second try = %v", err)
		}
	})

	t.Run("an invite never asks", func(t *testing.T) {
		invite, err := e.app.Identity.CreateInvite(ctx, appidentity.CreateInviteInput{})
		if err != nil {
			t.Fatal(err)
		}
		e.p.verdict.Store("VERDICT_REJECT")
		before := e.p.calls()
		if _, _, err := e.signUp(ctx, invite.Code); err != nil {
			t.Fatalf("SignUp with an invite = %v", err)
		}
		if e.p.calls() != before {
			t.Errorf("the plugin was called %d times", e.p.calls()-before)
		}
	})

	t.Run("a sign-up the block list refuses never asks", func(t *testing.T) {
		if _, err := e.app.Moderation.AddSignupBlock(ctx, appmoderation.SignupBlockInput{
			Kind: moderation.SignupBlockEmailDomain, Value: "blocked.example", Severity: moderation.SignupReject,
		}); err != nil {
			t.Fatal(err)
		}
		before := e.p.calls()
		if _, err := e.app.Identity.SignUp(ctx, appidentity.SignUpInput{
			Username: "blocked", Email: "blocked@blocked.example", Password: "correct horse battery staple", IP: "203.0.113.10",
		}); !errors.Is(err, appidentity.ErrSignUpRefused) {
			t.Fatalf("SignUp = %v", err)
		}
		if e.p.calls() != before {
			t.Errorf("the plugin was called %d times", e.p.calls()-before)
		}
	})

	t.Run("a failure passes by default and holds when chosen", func(t *testing.T) {
		e.p.verdict.Store("")
		got := wantState(t, domainidentity.RegistrationApproved)
		if _, ok := e.decisionOf(got.AccountID, hook.FailedError); !ok {
			t.Error("the failure was not recorded")
		}
		e.set(instance.SettingHookSignupCheckOnFailure, string(hook.ApproveOnFailure))
		wantState(t, domainidentity.RegistrationPending)
		e.set(instance.SettingHookSignupCheckOnFailure, string(hook.PassOnFailure))
	})

	t.Run("deleting the account removes the decision", func(t *testing.T) {
		e.p.verdict.Store("VERDICT_REQUIRE_APPROVAL")
		got := wantState(t, domainidentity.RegistrationPending)
		if _, ok := e.decisionOf(got.AccountID, hook.RequireApproval); !ok {
			t.Fatal("no decision")
		}
		if _, err := e.app.Hooks.EraseAccountData(ctx, appaccount.ErasureTarget{AccountID: got.AccountID}, "", 100); err != nil {
			t.Fatal(err)
		}
		if _, ok := e.decisionOf(got.AccountID, hook.RequireApproval); ok {
			t.Error("the decision outlived the account")
		}
	})

	t.Run("off calls nothing", func(t *testing.T) {
		e.set(instance.SettingHookSignupCheckMode, string(hook.ModeOff))
		e.p.verdict.Store("VERDICT_REJECT")
		before := e.p.calls()
		wantState(t, domainidentity.RegistrationApproved)
		if e.p.calls() != before {
			t.Errorf("the plugin was called %d times", e.p.calls()-before)
		}
	})
}

// TestSignupCheckBoundedWait checks that a plugin that never answers holds a sign-up up for the
// hook's deadline alone: it is recorded as a timeout, and the sign-up goes on by the failure
// behaviour.
func TestSignupCheckBoundedWait(t *testing.T) {
	t.Parallel()
	e := newSignupHookEnv(t, "100ms")
	e.set(instance.SettingHookSignupCheckMode, string(hook.ModeEnforce))
	e.p.verdict.Store("block")

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	got, _, err := e.signUp(ctx, "")
	if err != nil {
		t.Fatalf("SignUp with a plugin that never answers = %v", err)
	}
	if got.State != domainidentity.RegistrationApproved {
		t.Errorf("State = %q", got.State)
	}
	rec, ok := e.decisionOf(got.AccountID, hook.FailedTimeout)
	if !ok {
		t.Fatal("the timeout was not recorded")
	}
	if rec.Decision.Latency < 100*time.Millisecond {
		t.Errorf("latency = %v, shorter than the deadline", rec.Decision.Latency)
	}
}

// failingCaptcha fails every siteverify request.
type failingCaptcha struct{}

func (failingCaptcha) Do(_ context.Context, r *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	return &kernel.HTTPResponse{
		StatusCode: http.StatusOK, Header: http.Header{}, URL: r.URL,
		Body: io.NopCloser(strings.NewReader(`{"success":false,"error-codes":["invalid-input-response"]}`)),
	}, nil
}

// TestSignupCheckAfterCaptcha checks that a request that fails the CAPTCHA never reaches the plugin.
func TestSignupCheckAfterCaptcha(t *testing.T) {
	t.Parallel()
	p, addr := newSignupPlugin(t, "VERDICT_ALLOW")
	c := newStatusesClientWith(t, map[string]string{
		"MY_PLUGIN_SIGNUP_CHECK_ADDRESS": addr,
		"MY_CAPTCHA":                     "turnstile", "MY_CAPTCHA_SITE_KEY": "site-key", "MY_CAPTCHA_SECRET_KEY": "secret-key",
	}, platform.AppOptions{HTTPClient: failingCaptcha{}})
	ctx := t.Context()
	if _, err := c.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{
		Key: instance.SettingRegistrationMode, Value: string(instance.RegistrationOpen),
	}); err != nil {
		t.Fatal(err)
	}
	_, err := c.app.Identity.SignUp(ctx, appidentity.SignUpInput{
		Username: "applicant", Email: "applicant@example.com", Password: "correct horse battery staple",
		IP: "192.0.2.1", HumanCheckToken: "token",
	})
	if !errors.Is(err, kernel.ErrHumanCheckFailed) {
		t.Fatalf("SignUp = %v, want ErrHumanCheckFailed", err)
	}
	if p.calls() != 0 {
		t.Errorf("the plugin was called %d times", p.calls())
	}
}

// TestSignupCheckExternalPage checks the hook on a sign-up through a login provider, from the
// registration form: a reject shows the fixed text, with nothing of the plugin's answer.
func TestSignupCheckExternalPage(t *testing.T) {
	t.Parallel()
	p, addr := newSignupPlugin(t, "VERDICT_REJECT")
	e := newExternalEnvWith(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_PLUGIN_SIGNUP_CHECK_ADDRESS": addr, "MY_PLUGIN_SIGNUP_CHECK_TIMEOUT": "20s"})
	ctx := t.Context()
	for _, s := range []appinstance.SetSettingInput{
		{Key: instance.SettingHookSignupCheckMode, Value: string(hook.ModeEnforce)},
		{Key: instance.SettingHookSignupCheckReject, Value: "true"},
	} {
		if _, err := e.s.app.Instance.SetSetting(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	e.setMode(instance.RegistrationOpen)
	id := e.provider("IdP", true)
	c := e.client()
	resp, body := e.signInWith(c, id, map[string]any{"sub": "subject"})
	m := ticketField.FindStringSubmatch(body)
	if resp.StatusCode != http.StatusOK || m == nil {
		t.Fatalf("callback = %d\n%s", resp.StatusCode, body)
	}
	resp, body = c.do(http.MethodPost, authpages.PathExternalSignUp, url.Values{
		"csrf_token": {c.cookies[authpages.CookieCSRF]}, "ticket": {m[1]}, "username": {"applicant"}, "email": {"applicant@example.com"},
	}, nil)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "This server cannot accept this sign-up.") || strings.Contains(body, pluginNote) {
		t.Errorf("sign-up = %d\n%s", resp.StatusCode, body)
	}
	if p.calls() != 1 || p.last().Signup.Email != "applicant@example.com" {
		t.Errorf("plugin calls = %d", p.calls())
	}
}
