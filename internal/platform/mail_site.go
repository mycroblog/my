// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/app/kernel"
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
	return site
}
