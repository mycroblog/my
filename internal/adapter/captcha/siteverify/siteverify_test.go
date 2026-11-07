// SPDX-License-Identifier: AGPL-3.0-only

package siteverify

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/httpclient"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit/kerneltest"
)

func TestContract(t *testing.T) {
	for _, s := range Services() {
		t.Run(string(s), func(t *testing.T) {
			svc := kerneltest.HumanVerifierService{ReportsAction: ReportsAction(s), SendsSiteKey: SendsSiteKey(s)}
			kerneltest.HumanVerifierContract(t, svc, func(t *testing.T, stub kerneltest.HumanVerifierStub) kernel.HumanVerifier {
				v, err := New(Options{
					Service: s, SiteKey: stub.SiteKey, SecretKey: Secret(stub.SecretKey), Host: stub.Host,
					HTTP: httpclient.New(httpclient.Options{
						AllowHTTP: true, Ports: []int{int(stub.Addr.Port())}, ExemptAddrPorts: []netip.AddrPort{stub.Addr},
					}),
					Logger:    stub.Logger,
					VerifyURL: stub.VerifyURL,
				})
				if err != nil {
					t.Fatal(err)
				}
				return v
			})
		})
	}
}

func TestPresets(t *testing.T) {
	want := map[kernel.HumanCheckService]struct{ verify, field string }{
		kernel.HumanCheckTurnstile: {"https://challenges.cloudflare.com/turnstile/v0/siteverify", "cf-turnstile-response"},
		kernel.HumanCheckHCaptcha:  {"https://api.hcaptcha.com/siteverify", "h-captcha-response"},
		kernel.HumanCheckReCAPTCHA: {"https://www.google.com/recaptcha/api/siteverify", "g-recaptcha-response"},
	}
	for _, s := range Services() {
		p := presets[s]
		if p.verifyURL != want[s].verify || p.widget.ResponseField != want[s].field {
			t.Errorf("%s: siteverify %q, field %q; want %q, %q", s, p.verifyURL, p.widget.ResponseField, want[s].verify, want[s].field)
		}
		w := p.widget
		if (w.LanguageAttribute == "") == (w.LanguageParam == "") {
			t.Errorf("%s: exactly one of LanguageAttribute and LanguageParam must be set", s)
		}
		if w.Name == "" || w.ContainerClass == "" || w.PrivacyPolicyURL == "" || len(w.CSP.ScriptSrc) == 0 || len(w.CSP.FrameSrc) == 0 {
			t.Errorf("%s: incomplete widget %+v", s, w)
		}
		for _, u := range append([]string{p.verifyURL, w.ScriptURL, w.PrivacyPolicyURL}, w.CSP.ScriptSrc...) {
			if !strings.HasPrefix(u, "https://") {
				t.Errorf("%s: %q is not https", s, u)
			}
		}
		if (w.ActionAttribute != "") != p.reportsAction {
			t.Errorf("%s: ActionAttribute %q but reportsAction %v", s, w.ActionAttribute, p.reportsAction)
		}
	}
}

func TestWidget(t *testing.T) {
	v, err := New(Options{Service: kernel.HumanCheckHCaptcha, SiteKey: "site", SecretKey: "secret", Host: "social.example", HTTP: stubHTTP{}})
	if err != nil {
		t.Fatal(err)
	}
	w := v.Widget()
	if w.Service != kernel.HumanCheckHCaptcha || w.SiteKey != "site" {
		t.Errorf("Widget() = %+v", w)
	}
	if got := w.ScriptSrc("ja"); got != "https://js.hcaptcha.com/1/api.js?hl=ja" {
		t.Errorf("ScriptSrc(ja) = %q", got)
	}
	w.CSP.ScriptSrc[0] = "https://evil.example"
	if v.Widget().CSP.ScriptSrc[0] == "https://evil.example" {
		t.Error("Widget() shares the preset's slices")
	}
	tv, _ := New(Options{Service: kernel.HumanCheckTurnstile, SiteKey: "site", SecretKey: "secret", Host: "social.example", HTTP: stubHTTP{}})
	if got := tv.Widget().ScriptSrc("ja"); got != "https://challenges.cloudflare.com/turnstile/v0/api.js" {
		t.Errorf("Turnstile ScriptSrc(ja) = %q; the language goes in data-language", got)
	}
}

func TestNewRefusesIncompleteOptions(t *testing.T) {
	ok := Options{Service: kernel.HumanCheckTurnstile, SiteKey: "site", SecretKey: "secret", Host: "social.example", HTTP: stubHTTP{}}
	for name, mutate := range map[string]func(*Options){
		"service":    func(o *Options) { o.Service = "recaptcha-v3" },
		"site key":   func(o *Options) { o.SiteKey = "" },
		"secret key": func(o *Options) { o.SecretKey = "" },
		"host":       func(o *Options) { o.Host = "" },
		"http":       func(o *Options) { o.HTTP = nil },
	} {
		o := ok
		mutate(&o)
		if _, err := New(o); err == nil {
			t.Errorf("New() without %s succeeded", name)
		}
	}
}

func TestComparesTheHostWithoutPort(t *testing.T) {
	h := stubHTTP{body: `{"success":true,"hostname":"localhost"}`}
	v, err := New(Options{Service: kernel.HumanCheckReCAPTCHA, SiteKey: "site", SecretKey: "secret", Host: "localhost:8080", HTTP: h})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Verify(t.Context(), "token", kernel.HumanCheckActionSignIn); err != nil {
		t.Errorf("Verify() = %v", err)
	}
}

func TestSecretIsRedacted(t *testing.T) {
	s := Secret("0x5eCrE7")
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("x", "secret", s)
	for _, out := range []string{fmt.Sprint(s), fmt.Sprintf("%s %v %q %#v %x", s, s, s, s, s), buf.String(), fmt.Sprintf("%+v", Options{SecretKey: s})} {
		if strings.Contains(out, "0x5eCrE7") || strings.Contains(out, fmt.Sprintf("%x", "0x5eCrE7")) {
			t.Errorf("the secret leaks: %s", out)
		}
	}
	if !strings.Contains(buf.String(), "[REDACTED]") {
		t.Errorf("log = %s, want [REDACTED]", buf.String())
	}
}

// stubHTTP answers every request with body and checks the request's form.
type stubHTTP struct{ body string }

func (s stubHTTP) Do(_ context.Context, r *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	if r.Purpose != kernel.HTTPPurposeHumanCheck {
		return nil, fmt.Errorf("purpose %d", r.Purpose)
	}
	if _, err := url.ParseQuery(string(r.Body)); err != nil {
		return nil, err
	}
	return &kernel.HTTPResponse{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(s.body))}, nil
}
