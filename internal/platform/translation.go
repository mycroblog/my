// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/translation"
)

// setUpTranslation builds the translation of posts (ADR-0148): the instance settings choose among
// the registered providers and set the limits, read per request. It comes after the status reader,
// and its projector after the content module's, whose lock it relies on.
func (a *App) setUpTranslation(repo translation.Repository, opts AppOptions) {
	// Nothing registers a provider yet: an adapter adds itself to this list, with its API key from
	// the deployment configuration, so that no key reaches the settings or the API (ADR-0148 §15).
	registered := opts.TranslationRegistered
	var providers translation.Providers = translation.ConfiguredProviders{
		Registered: registered,
		Settings: func(ctx context.Context) (translation.ProviderSettings, error) {
			s, err := a.Instance.Settings(ctx)
			if err != nil {
				return translation.ProviderSettings{}, err
			}
			return translation.ProviderSettings{Enabled: s.TranslationProviders(), Visibilities: s.TranslationProviderVisibilities()}, nil
		},
	}
	if opts.TranslationProviders != nil {
		providers = opts.TranslationProviders
	}
	var limiter translation.Limiter = translation.UsageLimiter{
		UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock,
		Limits: func(ctx context.Context) (translation.Limits, error) {
			s, err := a.Instance.Settings(ctx)
			if err != nil {
				return translation.Limits{}, err
			}
			return translation.Limits{Account: s.TranslationUserMonthlyCharacters(), Server: s.TranslationServerMonthlyCharacters()}, nil
		},
	}
	if opts.TranslationLimiter != nil {
		limiter = opts.TranslationLimiter
	}
	infos := make([]translation.ProviderInfo, len(registered))
	for i, p := range registered {
		infos[i] = p.Info()
	}
	translation.RegisterProjectors(a.Projectors, repo)
	a.Translation = translation.NewService(translation.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Statuses: a.Statuses, Providers: providers, Limiter: limiter,
		Sanitizer: newHTMLSanitizer(), KV: a.KV, Clock: a.Clock, IDs: a.IDs, Registered: infos, Meter: a.Meter, Logger: a.Logger,
		RetentionDays: func(ctx context.Context) (int, error) {
			s, err := a.Instance.Settings(ctx)
			if err != nil {
				return 0, err
			}
			return s.TranslationRetentionDays(), nil
		},
	})
	a.Translation.RegisterJobs(a.Handlers, a.Scheduler)
}
