// SPDX-License-Identifier: AGPL-3.0-only

package translation

import (
	"context"
	"fmt"
	"slices"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
)

// ProviderSettings are the operator's choices of providers, from the instance settings.
type ProviderSettings struct {
	// Enabled are the IDs of the enabled providers, the first tried first
	// (translation_providers).
	Enabled []string
	// Visibilities are the visibilities that a provider may receive, by ID
	// (translation_provider_visibilities). A provider not named takes the default of ADR-0148 §5.
	Visibilities map[string][]string
}

// ConfiguredProviders is the Providers of a server: those of Registered that the settings enable,
// in the settings' order. An enabled ID that this server does not have does nothing, and with none
// enabled nothing is sent (ADR-0148 §3).
type ConfiguredProviders struct {
	// Registered are the providers that this server has, with their adapters. Their Visibilities
	// are not used: the settings decide those.
	Registered []Provider
	// Settings reads the settings, per request, so that a change takes effect at once.
	Settings func(ctx context.Context) (ProviderSettings, error)
}

// Providers implements Providers.
func (c ConfiguredProviders) Providers(ctx context.Context) ([]Provider, error) {
	settings, err := c.Settings(ctx)
	if err != nil {
		return nil, fmt.Errorf("translation: read the provider settings: %w", err)
	}
	out := make([]Provider, 0, len(settings.Enabled))
	for _, id := range settings.Enabled {
		i := slices.IndexFunc(c.Registered, func(p Provider) bool { return p.ID == id })
		if i < 0 {
			continue
		}
		p := c.Registered[i]
		p.Visibilities = nil
		if allowed, ok := settings.Visibilities[id]; ok {
			p.Visibilities = make([]appcontent.Visibility, len(allowed))
			for j, v := range allowed {
				p.Visibilities[j] = appcontent.Visibility(v)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// UsageLimiter is the Limiter of a server: it counts in the usage rows and reads the limits per
// request. A limit of 0 sends nothing, and no value is unlimited (ADR-0148 §11).
type UsageLimiter struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Clock      kernel.Clock
	// Limits reads translation_user_monthly_characters and translation_server_monthly_characters.
	Limits func(ctx context.Context) (Limits, error)
}

// Reserve implements Limiter. Months are in UTC.
func (l UsageLimiter) Reserve(ctx context.Context, accountID string, characters int) error {
	limits, err := l.Limits(ctx)
	if err != nil {
		return fmt.Errorf("translation: read the limits: %w", err)
	}
	if limits.Account <= 0 || limits.Server <= 0 {
		return ErrLimitReached
	}
	month := l.Clock.Now().UTC().Format(monthFormat)
	return l.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := l.Repository.AddUsageWithin(ctx, tx, accountID, month, int64(characters), limits)
		if err != nil {
			return err
		}
		if !ok {
			return ErrLimitReached
		}
		return nil
	})
}
