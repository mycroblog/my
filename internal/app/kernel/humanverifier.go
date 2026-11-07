// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"context"
	"errors"
	"net/url"
)

// HumanVerifier checks the token that a CAPTCHA widget put into a form (ADR-0106). The adapter
// lives in internal/adapter/captcha/siteverify and passes kerneltest.HumanVerifierContract. When the
// captcha setting is none there is no HumanVerifier: callers receive nil and check nothing.
//
// Verify sends the token to the service through the HTTPClient (HTTPPurposeHumanCheck), so it
// runs outside transactions (ADR-0003). A token is valid once, so Verify never retries.
type HumanVerifier interface {
	// Verify checks token for action, the operation that the widget was rendered for
	// (HumanCheckActionSignIn, HumanCheckActionSignUp, HumanCheckActionPasswordReset). It returns nil for a valid token, an error
	// wrapping ErrHumanCheckFailed for a token that is empty, invalid, expired, already used, or
	// issued for another host name or another action, and an error wrapping
	// ErrHumanCheckUnavailable when the service cannot answer: it is unreachable, times out,
	// answers with a 5xx status or an unreadable body, or refuses the secret key.
	Verify(ctx context.Context, token, action string) error
}

// The actions that the sign-in, registration, and password reset forms verify (ADR-0106 §9).
const (
	HumanCheckActionSignIn        = "sign_in"
	HumanCheckActionSignUp        = "sign_up"
	HumanCheckActionPasswordReset = "password_reset"
)

var (
	// ErrHumanCheckFailed reports a token that does not prove a human solved the challenge.
	ErrHumanCheckFailed = errors.New("human check failed")
	// ErrHumanCheckUnavailable reports that the CAPTCHA service could not check the token.
	ErrHumanCheckUnavailable = errors.New("human check unavailable")
)

// HumanCheckService names a CAPTCHA service (the captcha setting).
type HumanCheckService string

// Services.
const (
	HumanCheckTurnstile HumanCheckService = "turnstile"
	HumanCheckHCaptcha  HumanCheckService = "hcaptcha"
	// HumanCheckReCAPTCHA is reCAPTCHA v2 with the checkbox.
	HumanCheckReCAPTCHA HumanCheckService = "recaptcha"
)

// HumanCheckWidget is what a page needs to render the widget of the configured service and to
// read its token back (ADR-0106 §7, §11). The adapter's preset fills it; the composition root
// hands it to the pages that render the widget.
type HumanCheckWidget struct {
	Service HumanCheckService
	// Name is the service's name as the notice next to the widget shows it, such as "hCaptcha".
	Name string
	// SiteKey is the public key that the widget's data-sitekey attribute carries.
	SiteKey string
	// ScriptURL is the service's script; ScriptSrc adds the language to it.
	ScriptURL string
	// ContainerClass is the class of the element that the script turns into the widget.
	ContainerClass string
	// ResponseField is the name of the form field that the widget fills with the token.
	ResponseField string
	// ActionAttribute is the container's attribute that binds the token to an action, such as
	// data-action; empty when the service does not report the action back.
	ActionAttribute string
	// LanguageAttribute is the container's attribute that selects the widget's language, such
	// as data-language; empty when the script's LanguageParam does instead.
	LanguageAttribute string
	// LanguageParam is the query parameter of ScriptURL that selects the widget's language,
	// such as hl; empty when LanguageAttribute does instead.
	LanguageParam string
	// PrivacyPolicyURL and TermsURL are the service's documents that the notice links to;
	// TermsURL is empty when the service does not ask for the link.
	PrivacyPolicyURL string
	TermsURL         string
	// CSP lists the origins that the pages with the widget add to their Content-Security-Policy.
	CSP HumanCheckCSP
}

// HumanCheckCSP lists the sources that a widget needs per Content-Security-Policy directive.
type HumanCheckCSP struct {
	ScriptSrc  []string
	FrameSrc   []string
	StyleSrc   []string
	ConnectSrc []string
}

// ScriptSrc returns the URL of the widget's script for a page in the BCP 47 language lang:
// ScriptURL with LanguageParam set to lang, or ScriptURL itself when the service takes the
// language from LanguageAttribute or lang is empty.
func (w HumanCheckWidget) ScriptSrc(lang string) string {
	if w.LanguageParam == "" || lang == "" {
		return w.ScriptURL
	}
	u, err := url.Parse(w.ScriptURL)
	if err != nil {
		return w.ScriptURL
	}
	q := u.Query()
	q.Set(w.LanguageParam, lang)
	u.RawQuery = q.Encode()
	return u.String()
}
