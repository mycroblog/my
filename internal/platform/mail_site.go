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
// contact out.
func (a *App) mailSite(ctx context.Context) kernel.MailSite {
	site := kernel.MailSite{Name: a.instanceName(ctx), URL: a.Config.ServerOrigin()}
	if site.Name == "" {
		site.Name = a.Config.Host
	}
	settings, err := a.Instance.Settings(ctx)
	if err != nil {
		a.Logger.WarnContext(ctx, "read the instance's contact email", slog.Any("error", err))
		return site
	}
	site.ContactEmail = settings.ContactEmail()
	// The instance's own logo heads the HTML version (ADR-0116); a failure to read it leaves the
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
