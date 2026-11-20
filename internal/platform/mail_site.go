// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/instance"
)

// mailSite returns the instance as every email names it (ADR-0086 §2, §3): its display name, or
// its host when it has none, as the sender's name and in the signature; the server's origin; and
// the contact_email setting as the Reply-To and the signature's contact. It is read per email,
// so that a change applies at once; a failure to read the settings is logged and leaves the
// contact out. The colors are the published theme's (ADR-0059 §2, ADR-0086 §2), and the defaults
// of the design tokens without one or when it cannot be read.
func (a *App) mailSite(ctx context.Context) kernel.MailSite {
	site := kernel.MailSite{Name: a.instanceName(ctx), URL: a.Config.ServerOrigin(), Colors: mailColors(a.mailTheme(ctx))}
	if site.Name == "" {
		site.Name = a.Config.Host
	}
	settings, err := a.Instance.Settings(ctx)
	if err != nil {
		a.Logger.WarnContext(ctx, "read the instance's contact email", slog.Any("error", err))
		return site
	}
	site.ContactEmail = settings.ContactEmail()
	// The instance's own logo heads the HTML version (ADR-0059); a failure to read it leaves the
	// header without one.
	brands, err := a.Instance.Brands(ctx)
	if err != nil {
		a.Logger.WarnContext(ctx, "read the instance's logo", slog.Any("error", err))
		return site
	}
	site.Logo = a.mailLogo(brands.Logo, instance.RenditionLogoMail)
	site.LogoDark = a.mailLogo(brands.LogoDark, instance.RenditionLogoDarkMail)
	return site
}

// mailLogo returns the mail rendition of a logo revision at its URL, or nil without one.
func (a *App) mailLogo(rev *instance.BrandRevision, r instance.BrandRendition) *kernel.MailLogo {
	if rev == nil {
		return nil
	}
	f, ok := rev.File(r)
	if !ok {
		return nil
	}
	return &kernel.MailLogo{URL: a.Config.ServerOrigin() + instance.BrandFilePath(rev.ID, r), Width: f.Width, Height: f.Height}
}

// mailTheme returns the overrides of the theme in effect, nil when none is published; a failure to
// read it is logged and leaves the defaults.
func (a *App) mailTheme(ctx context.Context) []instance.ThemeTokenValue {
	rev, ok, err := a.Instance.Theme(ctx)
	if err != nil {
		a.Logger.WarnContext(ctx, "read the instance theme for an email", slog.Any("error", err))
		return nil
	}
	if !ok {
		return nil
	}
	return rev.Set.Tokens
}

// mailColors returns the colors of the HTML version of an email with the overrides of a theme,
// the defaults of the design tokens for the tokens it leaves alone.
func mailColors(tokens []instance.ThemeTokenValue) kernel.MailColors {
	return kernel.MailColorsFrom(func(token string, dark bool) string {
		if dark {
			return instance.ThemeColor(tokens, token, instance.SchemeDark)
		}
		return instance.ThemeColor(tokens, token, instance.SchemeLight)
	})
}
