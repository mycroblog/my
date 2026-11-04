// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"net/http"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/i18n"
)

// The CAPTCHA widget of the sign-in, registration, and password reset forms (ADR-0106 §11). With
// Options.HumanCheck set, those three pages, and no other, load the service's script, show its
// widget with a notice about the data the service receives and links to its documents, and
// widen their Content-Security-Policy by the service's origins. The form posts the token in
// the service's response field, and the identity module checks it.

// headerColorScheme is the client hint of the browser's color scheme (Sec-CH-Prefers-Color-Scheme),
// which picks the theme of the widgets that cannot follow the page by themselves.
const headerColorScheme = "Sec-CH-Prefers-Color-Scheme"

// humanCheckView is what templates/human_check.html renders.
type humanCheckView struct {
	ScriptSrc      string
	ContainerClass string
	SiteKey        string
	// Action is the data-action of the widget, empty when the service does not bind a token
	// to an action; Language is its data-language, empty when the script's URL selects it.
	Action   string
	Language string
	Theme    string
	// Notice tells which service checks the visitor and what it receives; Links point to the
	// service's privacy policy and terms.
	Notice string
	Links  []humanCheckLink
}

type humanCheckLink struct {
	URL, Title string
}

// humanCheck returns the widget of the page that checks action, or nil without a CAPTCHA.
func (h *Handler) humanCheck(w http.ResponseWriter, r *http.Request, l *i18n.Localizer, action string) *humanCheckView {
	wd := h.o.HumanCheck
	if wd == nil {
		return nil
	}
	v := &humanCheckView{ScriptSrc: wd.ScriptSrc(l.Locale()), ContainerClass: wd.ContainerClass, SiteKey: wd.SiteKey}
	if wd.ActionAttribute == "data-action" {
		v.Action = action
	}
	if wd.LanguageAttribute == "data-language" {
		v.Language = l.Locale()
	}
	// Turnstile follows the browser's color scheme as the page does; the others take light or
	// dark, which the client hint tells when the browser sends it.
	if wd.Service == kernel.HumanCheckTurnstile {
		v.Theme = "auto"
	} else {
		hd := w.Header()
		// No Critical-CH: a browser would repeat the request for it, which a POST must not be.
		hd.Set("Accept-CH", headerColorScheme)
		hd.Add("Vary", headerColorScheme)
		v.Theme = "light"
		if strings.EqualFold(strings.Trim(r.Header.Get(headerColorScheme), `"`), "dark") {
			v.Theme = "dark"
		}
	}
	owner := wd.Name
	if wd.Service == kernel.HumanCheckReCAPTCHA {
		// Google asks sites that use reCAPTCHA to show this statement (reCAPTCHA FAQ).
		v.Notice = l.ServerAuthHumanCheckNoticeRecaptcha()
		owner = "Google"
	} else {
		v.Notice = l.ServerAuthHumanCheckNotice(wd.Name)
	}
	if wd.PrivacyPolicyURL != "" {
		v.Links = append(v.Links, humanCheckLink{URL: wd.PrivacyPolicyURL, Title: l.ServerAuthHumanCheckPrivacyLink(owner)})
	}
	if wd.TermsURL != "" {
		v.Links = append(v.Links, humanCheckLink{URL: wd.TermsURL, Title: l.ServerAuthHumanCheckTermsLink(owner)})
	}
	return v
}

// humanCheckToken returns the token that the widget put into the posted form, "" without a
// CAPTCHA.
func (h *Handler) humanCheckToken(r *http.Request) string {
	if h.o.HumanCheck == nil {
		return ""
	}
	return r.PostForm.Get(h.o.HumanCheck.ResponseField)
}

// humanCheckCSP returns the Content-Security-Policy sources that the widget needs, on top of
// contentSecurityPolicy's; it never adds 'unsafe-inline' or 'unsafe-eval' (ADR-0106 §11).
func (h *Handler) humanCheckCSP() kernel.HumanCheckCSP {
	if h.o.HumanCheck == nil {
		return kernel.HumanCheckCSP{}
	}
	return h.o.HumanCheck.CSP
}
