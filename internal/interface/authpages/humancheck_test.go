// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/authpages"
)

// The widgets as the siteverify presets fill them (ADR-0106 §13).
var (
	turnstile = kernel.HumanCheckWidget{
		Service: kernel.HumanCheckTurnstile, Name: "Cloudflare Turnstile", SiteKey: "turnstile-site-key",
		ScriptURL: "https://challenges.cloudflare.com/turnstile/v0/api.js", ContainerClass: "cf-turnstile",
		ResponseField: "cf-turnstile-response", ActionAttribute: "data-action", LanguageAttribute: "data-language",
		PrivacyPolicyURL: "https://www.cloudflare.com/privacypolicy/",
		CSP: kernel.HumanCheckCSP{
			ScriptSrc: []string{"https://challenges.cloudflare.com"}, FrameSrc: []string{"https://challenges.cloudflare.com"},
		},
	}
	hcaptcha = kernel.HumanCheckWidget{
		Service: kernel.HumanCheckHCaptcha, Name: "hCaptcha", SiteKey: "hcaptcha-site-key",
		ScriptURL: "https://js.hcaptcha.com/1/api.js", ContainerClass: "h-captcha", ResponseField: "h-captcha-response",
		LanguageParam: "hl", PrivacyPolicyURL: "https://www.hcaptcha.com/privacy", TermsURL: "https://www.hcaptcha.com/terms",
		CSP: kernel.HumanCheckCSP{
			ScriptSrc: []string{"https://hcaptcha.com", "https://*.hcaptcha.com"}, FrameSrc: []string{"https://hcaptcha.com", "https://*.hcaptcha.com"},
			StyleSrc: []string{"https://hcaptcha.com", "https://*.hcaptcha.com"}, ConnectSrc: []string{"https://hcaptcha.com", "https://*.hcaptcha.com"},
		},
	}
	recaptcha = kernel.HumanCheckWidget{
		Service: kernel.HumanCheckReCAPTCHA, Name: "reCAPTCHA", SiteKey: "recaptcha-site-key",
		ScriptURL: "https://www.google.com/recaptcha/api.js", ContainerClass: "g-recaptcha", ResponseField: "g-recaptcha-response",
		LanguageParam: "hl", PrivacyPolicyURL: "https://policies.google.com/privacy", TermsURL: "https://policies.google.com/terms",
		CSP: kernel.HumanCheckCSP{
			ScriptSrc:  []string{"https://www.google.com/recaptcha/", "https://www.gstatic.com/recaptcha/"},
			FrameSrc:   []string{"https://www.google.com/recaptcha/", "https://recaptcha.google.com/recaptcha/"},
			ConnectSrc: []string{"https://www.google.com/recaptcha/"},
		},
	}
)

// humanIdentity is fakeIdentity whose SignIn and SignUp fail with err, as the identity module
// does when the CAPTCHA check fails, and record the token they got.
type humanIdentity struct {
	*fakeIdentity
	err    error
	tokens []string
}

func (f *humanIdentity) SignIn(ctx context.Context, in appidentity.SignInInput) (appidentity.SignedIn, error) {
	f.tokens = append(f.tokens, in.HumanCheckToken)
	if f.err != nil {
		return appidentity.SignedIn{}, f.err
	}
	return f.fakeIdentity.SignIn(ctx, in)
}

func (f *humanIdentity) SignUp(ctx context.Context, in appidentity.SignUpInput) (appidentity.SignedUp, error) {
	f.tokens = append(f.tokens, in.HumanCheckToken)
	if f.err != nil {
		return appidentity.SignedUp{}, f.err
	}
	return f.fakeIdentity.SignUp(ctx, in)
}

func newHumanServer(t *testing.T, id authpages.Identity, w *kernel.HumanCheckWidget) http.Handler {
	t.Helper()
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := mux{http.NewServeMux()}
	authpages.New(authpages.Options{Identity: id, Locales: &i18n.Resolver{Bundle: b}, HumanCheck: w}).Register(m)
	return m
}

// directive returns the sources of a directive of a Content-Security-Policy, nil when absent.
func directive(csp, name string) []string {
	for d := range strings.SplitSeq(csp, ";") {
		f := strings.Fields(d)
		if len(f) > 0 && f[0] == name {
			return f[1:]
		}
	}
	return nil
}

// TestNoHumanCheck checks that without a CAPTCHA the forms have no widget and the policy
// admits nothing more (captcha = none).
func TestNoHumanCheck(t *testing.T) {
	b := newBrowser(t, newServer(t, newFakeIdentity()))
	for _, path := range []string{authpages.PathSignIn, authpages.PathSignUp} {
		w := b.do(http.MethodGet, path, nil, nil)
		csp := w.Header().Get("Content-Security-Policy")
		if got := directive(csp, "script-src"); fmt.Sprint(got) != "['self']" || directive(csp, "frame-src") != nil || directive(csp, "connect-src") != nil {
			t.Errorf("%s: Content-Security-Policy = %q, want this site's scripts only", path, csp)
		}
		if data := screenData(t, w); at(data, "humanCheck") != nil {
			t.Errorf("%s: humanCheck = %v, want none", path, at(data, "humanCheck"))
		}
		if w.Header().Get("Accept-CH") != "" {
			t.Errorf("%s: Accept-CH = %q", path, w.Header().Get("Accept-CH"))
		}
	}
}

func TestHumanCheckWidget(t *testing.T) {
	for _, tc := range []struct {
		widget kernel.HumanCheckWidget
		// want are the fields of the screen's humanCheck, with ACTION for the form's action;
		// notice is a part of its notice, and links are its links as "url title".
		want   map[string]string
		notice string
		links  []string
	}{
		{turnstile, map[string]string{
			"scriptSrc": "https://challenges.cloudflare.com/turnstile/v0/api.js", "containerClass": "cf-turnstile",
			"siteKey": "turnstile-site-key", "action": "ACTION", "language": "ja", "theme": "auto",
		}, "このページは Cloudflare Turnstile でボットかどうかを確認します。", []string{
			"https://www.cloudflare.com/privacypolicy/ Cloudflare Turnstile のプライバシーポリシー",
		}},
		{hcaptcha, map[string]string{
			"scriptSrc": "https://js.hcaptcha.com/1/api.js?hl=ja", "containerClass": "h-captcha",
			"siteKey": "hcaptcha-site-key", "action": "", "language": "", "theme": "light",
		}, "このページは hCaptcha でボットかどうかを確認します。", []string{
			"https://www.hcaptcha.com/privacy hCaptcha のプライバシーポリシー",
			"https://www.hcaptcha.com/terms hCaptcha の利用規約",
		}},
		{recaptcha, map[string]string{
			"scriptSrc": "https://www.google.com/recaptcha/api.js?hl=ja", "containerClass": "g-recaptcha",
			"siteKey": "recaptcha-site-key", "action": "", "language": "", "theme": "light",
		}, "このサイトは reCAPTCHA で保護されており、Google のプライバシーポリシーと利用規約が適用されます。", []string{
			"https://policies.google.com/privacy Google のプライバシーポリシー",
			"https://policies.google.com/terms Google の利用規約",
		}},
	} {
		t.Run(string(tc.widget.Service), func(t *testing.T) {
			b := newBrowser(t, newHumanServer(t, newFakeIdentity(), &tc.widget))
			for path, action := range map[string]string{authpages.PathSignIn: "sign_in", authpages.PathSignUp: "sign_up"} {
				w := b.do(http.MethodGet, path, nil, map[string]string{"Accept-Language": "ja"})
				if w.Code != http.StatusOK {
					t.Fatalf("GET %s = %d", path, w.Code)
				}
				data := screenData(t, w)
				for key, want := range tc.want {
					if got := text(data, "humanCheck."+key); got != strings.ReplaceAll(want, "ACTION", action) {
						t.Errorf("%s: humanCheck.%s = %q, want %q", path, key, got, want)
					}
				}
				if !strings.Contains(text(data, "humanCheck.notice"), tc.notice) {
					t.Errorf("%s: humanCheck.notice = %q, want %q in it", path, text(data, "humanCheck.notice"), tc.notice)
				}
				links, _ := at(data, "humanCheck.links").([]any)
				var got []string
				for i := range links {
					p := "humanCheck.links." + strconv.Itoa(i) + "."
					got = append(got, text(data, p+"url")+" "+text(data, p+"title"))
				}
				if !slices.Equal(got, tc.links) {
					t.Errorf("%s: humanCheck.links = %q, want %q", path, got, tc.links)
				}

				csp := w.Header().Get("Content-Security-Policy")
				if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
					t.Errorf("%s: Content-Security-Policy = %q", path, csp)
				}
				for _, d := range []struct {
					name string
					want []string
				}{
					{"default-src", []string{"'none'"}},
					{"script-src", append([]string{"'self'"}, tc.widget.CSP.ScriptSrc...)},
					{"frame-src", tc.widget.CSP.FrameSrc},
					{"connect-src", tc.widget.CSP.ConnectSrc},
					{"style-src", append([]string{"'self'"}, tc.widget.CSP.StyleSrc...)},
					{"img-src", []string{"'self'"}},
					{"form-action", []string{"'self'"}},
					{"frame-ancestors", []string{"'none'"}},
					{"base-uri", []string{"'none'"}},
				} {
					if got := directive(csp, d.name); fmt.Sprint(got) != fmt.Sprint(d.want) {
						t.Errorf("%s: %s = %v, want %v (%s)", path, d.name, got, d.want, csp)
					}
				}
			}
		})
	}
}

// TestHumanCheckTheme checks that the widgets that cannot follow the page's color scheme ask
// for it with the client hint and use it.
func TestHumanCheckTheme(t *testing.T) {
	b := newBrowser(t, newHumanServer(t, newFakeIdentity(), &hcaptcha))
	w := b.do(http.MethodGet, authpages.PathSignIn, nil, map[string]string{"Sec-CH-Prefers-Color-Scheme": `"dark"`})
	if w.Header().Get("Accept-CH") != "Sec-CH-Prefers-Color-Scheme" || w.Header().Get("Critical-CH") != "" ||
		!strings.Contains(strings.Join(w.Header().Values("Vary"), ","), "Sec-CH-Prefers-Color-Scheme") {
		t.Errorf("headers = %v", w.Header())
	}
	if got := text(screenData(t, w), "humanCheck.theme"); got != "dark" {
		t.Errorf("humanCheck.theme = %q, want dark", got)
	}
}

// TestHumanCheckOnlyOnTheForms checks that the other pages keep their policy and show no
// widget: a closed registration and the error pages.
func TestHumanCheckOnlyOnTheForms(t *testing.T) {
	id := newFakeIdentity()
	b := newBrowser(t, newHumanServer(t, id, &turnstile))
	id.mode = "closed"
	w := b.do(http.MethodGet, authpages.PathSignUp, nil, nil)
	if csp := w.Header().Get("Content-Security-Policy"); strings.Contains(csp, "challenges.cloudflare.com") || at(screenData(t, w), "humanCheck") != nil {
		t.Errorf("closed sign-up: %q\n%s", csp, w.Body.String())
	}
	w = b.do(http.MethodGet, authpages.PathVerifyEmail, nil, nil)
	if csp := w.Header().Get("Content-Security-Policy"); strings.Contains(csp, "challenges.cloudflare.com") {
		t.Errorf("error page: %q", csp)
	}
}

func TestHumanCheckSignIn(t *testing.T) {
	id := &humanIdentity{fakeIdentity: newFakeIdentity()}
	b := newBrowser(t, newHumanServer(t, id, &turnstile))
	page := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)

	id.err = fmt.Errorf("%w: invalid-input-response", kernel.ErrHumanCheckFailed)
	w := b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", "alice@example.com", "password", "correct horse", "cf-turnstile-response", "bad"), nil)
	if w.Code != http.StatusUnprocessableEntity || b.cookies[authpages.CookieSession] != "" {
		t.Fatalf("sign-in with a bad token = %d", w.Code)
	}
	// The form comes back with the widget for another try and keeps the address.
	if data := screenData(t, w); text(data, "error") != "Complete the check that you are not a bot, then try again." ||
		text(data, "humanCheck.containerClass") != "cf-turnstile" || text(data, "email") != "alice@example.com" {
		t.Errorf("sign-in with a bad token: %v", data)
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "challenges.cloudflare.com") {
		t.Errorf("the form shown again lacks the widget's policy")
	}

	id.err = nil
	w = b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", "alice@example.com", "password", "correct horse", "cf-turnstile-response", "good"), nil)
	if w.Code != http.StatusSeeOther || b.cookies[authpages.CookieSession] == "" {
		t.Fatalf("sign-in with a good token = %d", w.Code)
	}
	if got := strings.Join(id.tokens, ","); got != "bad,good" {
		t.Errorf("tokens = %s, want bad,good", got)
	}
}

func TestHumanCheckSignUp(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		want   string
	}{
		{fmt.Errorf("%w: timeout-or-duplicate", kernel.ErrHumanCheckFailed), http.StatusUnprocessableEntity, "Complete the check that you are not a bot, then try again."},
		{fmt.Errorf("%w: 503", kernel.ErrHumanCheckUnavailable), http.StatusServiceUnavailable, "The check that you are not a bot is not available right now. Try again later."},
	} {
		id := &humanIdentity{fakeIdentity: newFakeIdentity(), err: tc.err}
		b := newBrowser(t, newHumanServer(t, id, &hcaptcha))
		page := b.do(http.MethodGet, authpages.PathSignUp, nil, nil)
		w := b.do(http.MethodPost, authpages.PathSignUp, b.form(page,
			"username", "bob", "email", "bob@example.com", "password", "correct horse battery", "h-captcha-response", "token"), nil)
		if w.Code != tc.status || b.cookies[authpages.CookieSession] != "" {
			t.Errorf("sign-up with %v = %d, want %d", tc.err, w.Code, tc.status)
		}
		if data := screenData(t, w); text(data, "error") != tc.want || text(data, "signUp.username") != "bob" ||
			text(data, "humanCheck.containerClass") != "h-captcha" {
			t.Errorf("sign-up with %v: %v", tc.err, data)
		}
		if len(id.tokens) != 1 || id.tokens[0] != "token" {
			t.Errorf("tokens = %v", id.tokens)
		}
	}
}
