// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"github.com/mycroblog/my/internal/app/translation"
)

// setUpTranslation builds the translation of posts (ADR-0148). No setting enables a provider or
// raises a limit yet, so nothing goes to a provider and every request ends in the view check, the
// author's own language, or the refusal that no provider is available. It comes after the status
// reader, and its projector after the content module's, whose lock it relies on.
func (a *App) setUpTranslation(repo translation.Repository, opts AppOptions) {
	var providers translation.Providers = translation.NoProviders{}
	if opts.TranslationProviders != nil {
		providers = opts.TranslationProviders
	}
	var limiter translation.Limiter = translation.ClosedLimiter{}
	if opts.TranslationLimiter != nil {
		limiter = opts.TranslationLimiter
	}
	translation.RegisterProjectors(a.Projectors, repo)
	a.Translation = translation.NewService(translation.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Statuses: a.Statuses, Providers: providers, Limiter: limiter,
		Sanitizer: newHTMLSanitizer(), KV: a.KV, Clock: a.Clock, IDs: a.IDs, Meter: a.Meter, Logger: a.Logger,
	})
	a.Translation.RegisterJobs(a.Handlers, a.Scheduler)
}
