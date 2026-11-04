// SPDX-License-Identifier: AGPL-3.0-only

// Package siteverify is the kernel.HumanVerifier for the CAPTCHA services that share the
// siteverify API (ADR-0106 §7): Cloudflare Turnstile, hCaptcha, and reCAPTCHA v2 with the
// checkbox. The browser's widget puts a token into the form; Verify posts the secret key and
// the token to the service through the SSRF-guarded kernel.HTTPClient and checks the JSON
// answer. The differences between the services are the presets: the siteverify URL, the
// widget's script, form field, and Content-Security-Policy origins, and whether the service
// reports the action and wants the site key.
package siteverify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
)

// MaxTokenBytes bounds the token that Verify sends; a longer one fails without a request.
const MaxTokenBytes = 16 << 10

// Secret is the secret key of the service. It formats and logs as [REDACTED] (ADR-0019 §6).
type Secret string

const redacted = "[REDACTED]"

// String implements fmt.Stringer.
func (Secret) String() string { return redacted }

// GoString implements fmt.GoStringer.
func (Secret) GoString() string { return redacted }

// Format implements fmt.Formatter, so that every verb prints [REDACTED].
func (Secret) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redacted) }

// LogValue implements slog.LogValuer.
func (Secret) LogValue() slog.Value { return slog.StringValue(redacted) }

// preset is what differs between the services.
type preset struct {
	verifyURL string
	widget    kernel.HumanCheckWidget
	// reportsAction: the service binds the token to the widget's action and reports it.
	reportsAction bool
	// sendsSiteKey: the service recommends sending the site key along with the token.
	sendsSiteKey bool
}

// presets are the services of ADR-0106 §13, checked against their documentation.
var presets = map[kernel.HumanCheckService]preset{
	kernel.HumanCheckTurnstile: {
		verifyURL: "https://challenges.cloudflare.com/turnstile/v0/siteverify",
		widget: kernel.HumanCheckWidget{
			Name:              "Cloudflare Turnstile",
			ScriptURL:         "https://challenges.cloudflare.com/turnstile/v0/api.js",
			ContainerClass:    "cf-turnstile",
			ResponseField:     "cf-turnstile-response",
			ActionAttribute:   "data-action",
			LanguageAttribute: "data-language",
			PrivacyPolicyURL:  "https://www.cloudflare.com/privacypolicy/",
			CSP: kernel.HumanCheckCSP{
				ScriptSrc: []string{"https://challenges.cloudflare.com"},
				FrameSrc:  []string{"https://challenges.cloudflare.com"},
			},
		},
		reportsAction: true,
	},
	kernel.HumanCheckHCaptcha: {
		verifyURL: "https://api.hcaptcha.com/siteverify",
		widget: kernel.HumanCheckWidget{
			Name:             "hCaptcha",
			ScriptURL:        "https://js.hcaptcha.com/1/api.js",
			ContainerClass:   "h-captcha",
			ResponseField:    "h-captcha-response",
			LanguageParam:    "hl",
			PrivacyPolicyURL: "https://www.hcaptcha.com/privacy",
			TermsURL:         "https://www.hcaptcha.com/terms",
			CSP: kernel.HumanCheckCSP{
				ScriptSrc:  []string{"https://hcaptcha.com", "https://*.hcaptcha.com"},
				FrameSrc:   []string{"https://hcaptcha.com", "https://*.hcaptcha.com"},
				StyleSrc:   []string{"https://hcaptcha.com", "https://*.hcaptcha.com"},
				ConnectSrc: []string{"https://hcaptcha.com", "https://*.hcaptcha.com"},
			},
		},
		sendsSiteKey: true,
	},
	kernel.HumanCheckReCAPTCHA: {
		verifyURL: "https://www.google.com/recaptcha/api/siteverify",
		widget: kernel.HumanCheckWidget{
			Name:             "reCAPTCHA",
			ScriptURL:        "https://www.google.com/recaptcha/api.js",
			ContainerClass:   "g-recaptcha",
			ResponseField:    "g-recaptcha-response",
			LanguageParam:    "hl",
			PrivacyPolicyURL: "https://policies.google.com/privacy",
			TermsURL:         "https://policies.google.com/terms",
			CSP: kernel.HumanCheckCSP{
				ScriptSrc:  []string{"https://www.google.com/recaptcha/", "https://www.gstatic.com/recaptcha/"},
				FrameSrc:   []string{"https://www.google.com/recaptcha/", "https://recaptcha.google.com/recaptcha/"},
				ConnectSrc: []string{"https://www.google.com/recaptcha/"},
			},
		},
	},
}

// Services returns the services that New accepts, in a stable order.
func Services() []kernel.HumanCheckService {
	return []kernel.HumanCheckService{kernel.HumanCheckTurnstile, kernel.HumanCheckHCaptcha, kernel.HumanCheckReCAPTCHA}
}

// ReportsAction reports whether the service binds a token to the widget's action and reports
// it back, so that Verify checks the action.
func ReportsAction(s kernel.HumanCheckService) bool { return presets[s].reportsAction }

// SendsSiteKey reports whether Verify sends the site key along with the token.
func SendsSiteKey(s kernel.HumanCheckService) bool { return presets[s].sendsSiteKey }

// Options configure a Verifier.
type Options struct {
	Service   kernel.HumanCheckService
	SiteKey   string
	SecretKey Secret
	// Host is the instance's host (the host setting, with an optional port). The service's
	// answer must name it as the host name where the widget was solved.
	Host string
	// HTTP sends the requests; it is required.
	HTTP kernel.HTTPClient
	// Logger records a secret key that the service refuses; the default is slog.Default().
	Logger *slog.Logger
	// VerifyURL replaces the preset's siteverify URL. Tests only, for a stub of the service.
	VerifyURL string
}

// Verifier is the kernel.HumanVerifier of one service. It is safe for concurrent use.
type Verifier struct {
	preset    preset
	service   kernel.HumanCheckService
	siteKey   string
	secret    Secret
	host      string
	http      kernel.HTTPClient
	logger    *slog.Logger
	verifyURL string
}

var _ kernel.HumanVerifier = (*Verifier)(nil)

// New returns the Verifier of o.Service. It fails for an unknown service or a missing key,
// host, or HTTP client.
func New(o Options) (*Verifier, error) {
	p, ok := presets[o.Service]
	if !ok {
		return nil, fmt.Errorf("siteverify: unknown service %q", o.Service)
	}
	if o.SiteKey == "" || o.SecretKey == "" {
		return nil, errors.New("siteverify: the site key and the secret key are required")
	}
	if o.Host == "" || o.HTTP == nil {
		return nil, errors.New("siteverify: the host and the HTTP client are required")
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	host := o.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	v := &Verifier{
		preset: p, service: o.Service, siteKey: o.SiteKey, secret: o.SecretKey,
		host: strings.ToLower(host), http: o.HTTP, logger: o.Logger, verifyURL: p.verifyURL,
	}
	if o.VerifyURL != "" {
		v.verifyURL = o.VerifyURL
	}
	return v, nil
}

// Widget returns what the pages need to render the service's widget.
func (v *Verifier) Widget() kernel.HumanCheckWidget {
	w := v.preset.widget
	w.Service, w.SiteKey = v.service, v.siteKey
	w.CSP = kernel.HumanCheckCSP{
		ScriptSrc:  slices.Clone(w.CSP.ScriptSrc),
		FrameSrc:   slices.Clone(w.CSP.FrameSrc),
		StyleSrc:   slices.Clone(w.CSP.StyleSrc),
		ConnectSrc: slices.Clone(w.CSP.ConnectSrc),
	}
	return w
}

// answer is the JSON that siteverify returns.
type answer struct {
	Success    bool     `json:"success"`
	Hostname   string   `json:"hostname"`
	Action     string   `json:"action"`
	ErrorCodes []string `json:"error-codes"`
}

// Error codes that mean the server's own keys are wrong, not the token.
var configErrorCodes = []string{"missing-input-secret", "invalid-input-secret", "sitekey-secret-mismatch"}

// Verify implements kernel.HumanVerifier. It sends no remoteip (ADR-0106 §9: the browser
// already reached the service itself) and never retries, since a token is valid once.
func (v *Verifier) Verify(ctx context.Context, token, action string) error {
	if token == "" {
		return fmt.Errorf("%w: no token", kernel.ErrHumanCheckFailed)
	}
	if len(token) > MaxTokenBytes {
		return fmt.Errorf("%w: the token is longer than %d bytes", kernel.ErrHumanCheckFailed, MaxTokenBytes)
	}
	form := url.Values{"secret": {string(v.secret)}, "response": {token}}
	if v.preset.sendsSiteKey {
		form.Set("sitekey", v.siteKey)
	}
	resp, err := v.http.Do(ctx, &kernel.HTTPRequest{
		Method: http.MethodPost,
		URL:    v.verifyURL,
		Header: http.Header{
			"Content-Type": {"application/x-www-form-urlencoded"},
			"Accept":       {"application/json"},
		},
		Body:    []byte(form.Encode()),
		Purpose: kernel.HTTPPurposeHumanCheck,
	})
	if err != nil {
		return fmt.Errorf("%w: %s: %w", kernel.ErrHumanCheckUnavailable, v.service, err)
	}
	body, err := resp.ReadAll()
	if err != nil {
		return fmt.Errorf("%w: %s: read the answer: %w", kernel.ErrHumanCheckUnavailable, v.service, err)
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: %s answered %d", kernel.ErrHumanCheckUnavailable, v.service, resp.StatusCode)
	}
	var a answer
	if err := json.Unmarshal(body, &a); err != nil {
		return fmt.Errorf("%w: %s answered %d with an unreadable body: %w", kernel.ErrHumanCheckUnavailable, v.service, resp.StatusCode, err)
	}
	if !a.Success {
		if slices.ContainsFunc(a.ErrorCodes, func(c string) bool { return slices.Contains(configErrorCodes, c) }) {
			v.logger.ErrorContext(ctx, "the CAPTCHA service refused the secret key or the site key; check captcha_site_key and captcha_secret_key",
				slog.String("captcha", string(v.service)), slog.Any("error_codes", a.ErrorCodes))
			return fmt.Errorf("%w: %s refused the keys: %v", kernel.ErrHumanCheckUnavailable, v.service, a.ErrorCodes)
		}
		if slices.Contains(a.ErrorCodes, "internal-error") || resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%w: %s answered %d: %v", kernel.ErrHumanCheckUnavailable, v.service, resp.StatusCode, a.ErrorCodes)
		}
		return fmt.Errorf("%w: %s: %v", kernel.ErrHumanCheckFailed, v.service, a.ErrorCodes)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s answered %d", kernel.ErrHumanCheckUnavailable, v.service, resp.StatusCode)
	}
	if !strings.EqualFold(a.Hostname, v.host) {
		return fmt.Errorf("%w: %s: the token was issued for the host %q", kernel.ErrHumanCheckFailed, v.service, a.Hostname)
	}
	if v.preset.reportsAction && a.Action != action {
		return fmt.Errorf("%w: %s: the token was issued for the action %q, not %q", kernel.ErrHumanCheckFailed, v.service, a.Action, action)
	}
	return nil
}
