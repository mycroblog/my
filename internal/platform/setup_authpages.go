// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpAuthPages registers the sign-in screens. They load the stylesheet the web build makes for
// them (ADR-0030), so they come after setUpWeb.
func (a *App) setUpAuthPages(cfg *config.Config) {
	authpages.New(authpages.Options{
		Identity: a.Identity, OAuth: a.Identity, Issuer: cfg.ServerOrigin(), FormActionOrigins: []string{cfg.WebOrigin},
		Locales: a.Locales, Stylesheet: a.webStylesheet, StatusPageURL: cfg.StatusPageURL, Logger: a.Logger, Errors: a.ErrorReporter,
		Moderation: a.Moderation, UserLocales: a.Identity, Documents: a.Instance,
		InstanceName: func(ctx context.Context) string {
			if name := a.instanceName(ctx); name != "" {
				return name
			}
			return cfg.Host
		},
		HumanCheck:      a.HumanCheckWidget,
		ThemeStylesheet: a.themeStylesheet,
		// A passkey signs in instead of the password where the browser has one (FR-004).
		Passkeys: a.Identity, PasskeyScript: a.webPasskeyScript,
		// A forgotten password is reset with a link, by email or from an administrator (FR-002).
		PasswordReset: a.Identity,
		// Notification emails stop from their own link, without signing in (ADR-0086 §4).
		MailUnsubscribe: a.Identity,
		// Users sign in and sign up with the login providers that administrators set up (FR-010).
		ExternalLogins: a.Identity,
	}).Register(surfaceRouter{a, surfaceOAuth})
}
