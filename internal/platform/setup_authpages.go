// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/interface/web"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpAuthPages registers the sign-in screens. They are documents of the web build's auth entry
// (ADR-0090 §16), so they come after setUpWeb.
func (a *App) setUpAuthPages(cfg *config.Config, entry web.Entry) {
	authpages.New(authpages.Options{
		Identity: a.Identity, OAuth: a.Identity, Issuer: cfg.ServerOrigin(), FormActionOrigins: []string{cfg.WebOrigin},
		Locales: a.Locales, StatusPageURL: cfg.StatusPageURL, Logger: a.Logger, Errors: a.ErrorReporter,
		Entry: authpages.Entry{
			Script: entry.Script, Stylesheets: entry.Stylesheets, Preloads: entry.Preloads,
			ViteClient: entry.ViteClient, DevOrigin: entry.DevOrigin,
		},
		Clock:      a.Clock,
		Moderation: a.Moderation, UserLocales: a.Identity, Documents: a.Instance,
		InstanceName: func(ctx context.Context) string {
			if name := a.instanceName(ctx); name != "" {
				return name
			}
			return cfg.Host
		},
		// The server's introduction beside the form: its summary and its thumbnail as the banner.
		InstanceDescription: a.instanceSummary,
		Banner:              a.instanceBanner,
		MediaOrigin:         cfg.MediaOrigin,
		// The consent names the account that grants the application.
		Accounts: signedInAccounts{a}, AccountDomain: cfg.AccountDomain,
		HumanCheck:      a.HumanCheckWidget,
		ThemeStylesheet: a.themeStylesheet,
		Logos:           a.brandLogos,
		// A passkey signs in instead of the password where the browser has one (FR-004).
		Passkeys: a.Identity,
		// A forgotten password is reset with a link, by email or from an administrator (FR-002).
		PasswordReset: a.Identity,
		// Notification emails stop from their own link, without signing in (ADR-0086 §4).
		MailUnsubscribe: a.Identity,
		// Users sign in and sign up with the login providers that administrators set up (FR-010).
		ExternalLogins: a.Identity,
		Deactivation:   a.Identity,
	}).Register(surfaceRouter{a, surfaceOAuth})
}

// instanceBanner returns the URL of the instance's thumbnail, which the sign-in screens show as
// the server's banner, or "" when there is none or it cannot be read (the screens then show none).
func (a *App) instanceBanner(ctx context.Context) string {
	img, err := a.instanceThumbnail(ctx)
	if err != nil {
		a.Logger.WarnContext(ctx, "read the instance thumbnail", slog.Any("error", err))
		return ""
	}
	if img == nil {
		return ""
	}
	return img.URL
}

// signedInAccounts names the local account of a signed-in user for the consent screen.
type signedInAccounts struct{ a *App }

func (s signedInAccounts) SignedInAccount(ctx context.Context, userID string) (authpages.SignedInAccount, error) {
	la, err := s.a.Accounts.LocalAccountOfUser(ctx, userID)
	if err != nil {
		return authpages.SignedInAccount{}, err
	}
	out := authpages.SignedInAccount{Username: la.Username}
	actor, err := s.a.Accounts.FindByID(ctx, la.AccountID)
	if err != nil {
		return authpages.SignedInAccount{}, err
	}
	if actor.Profile != nil {
		out.DisplayName = actor.Profile.DisplayName
	}
	return out, nil
}
