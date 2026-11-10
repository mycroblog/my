// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	domainmoderation "github.com/mycroblog/my/internal/domain/moderation"
)

// federationScopes reads the instance setting federation_scope (ADR-0159 §2) for the moderation
// module, which the instance module imports, per call, so that a change applies at once. It reads
// a.Instance per call, so that it can be wired before the instance service is.
type federationScopes struct{ a *App }

// FederationScope implements moderation.FederationScopes.
func (s federationScopes) FederationScope(ctx context.Context) (domainmoderation.FederationScope, error) {
	v, err := s.a.Instance.FederationScope(ctx)
	return domainmoderation.FederationScope(v), err
}

// FederationScopeTx implements moderation.FederationScopes.
func (s federationScopes) FederationScopeTx(ctx context.Context, tx uow.Tx) (domainmoderation.FederationScope, error) {
	v, err := s.a.Instance.FederationScopeTx(ctx, tx)
	return domainmoderation.FederationScope(v), err
}

// registerFederationScopeSwitch makes the moderation module cut off the domains that the allowlist
// does not allow when federation_scope becomes allowlist, in the transaction of the change
// (ADR-0159 §3), whichever entry point (the admin API, the CLI) changed it.
func (a *App) registerFederationScopeSwitch() {
	a.Projectors.Register(appinstance.EventSettingRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appinstance.SettingRevised)
		if ev.Revision.Key != domaininstance.SettingFederationScope {
			return nil
		}
		return a.Moderation.FederationScopeSwitchedTx(ctx, tx,
			moderation.FederationScope(ev.Previous), moderation.FederationScope(ev.Revision.Value), ev.Revision.RevisedByUserID)
	})
}

// authorizedFetch reads whether the ActivityPub GETs need a verified signature: authorized_fetch,
// or the allowlist, which requires it (ADR-0159 §2).
type authorizedFetch struct{ a *App }

// AuthorizedFetch implements federation.FetchSettings.
func (f authorizedFetch) AuthorizedFetch(ctx context.Context) (bool, error) {
	return f.a.Instance.EffectiveAuthorizedFetch(ctx)
}
