// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/mycroblog/my/internal/domain/identity"
)

// Limiting registration and sign-in to the external login providers. While the instance setting
// login_providers_only is on and at least one provider is available to sign in with, passwords
// and passkeys sign nobody in, password registration stops, and no new password or passkey can
// be made. With no such provider the limit does not apply, so that disabling the last provider
// cannot lock everyone out. The setting and the providers are read on every use, so a change
// applies from the next request.
//
// The limit is about starting a session and about credentials that start one. It leaves alone
// what happens inside a session: a password or a passkey still confirms an operation (confirm),
// which is how a user without a link links a provider while limited, and existing sessions,
// access tokens, and refresh tokens keep working. Two-factor authentication after a provider's
// sign-in stays too.

// ErrLoginProvidersOnly reports an operation that the limit refuses. The refusal comes before
// anything looks at the account, so the answer is the same whether or not the account exists
// and whatever was sent.
var ErrLoginProvidersOnly = errors.New("identity: registration and sign-in are limited to login providers")

// loginProvidersOnly returns whether the limit applies and, while it does, the providers
// available to sign in with.
func (s *Service) loginProvidersOnly(ctx context.Context) (bool, []ExternalLoginProvider, error) {
	if s.d.LoginProvidersOnly == nil {
		return false, nil, nil
	}
	on, err := s.d.LoginProvidersOnly(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("identity: read the limit to login providers: %w", err)
	}
	if !on {
		return false, nil, nil
	}
	providers, err := s.ExternalLoginProviders(ctx)
	if err != nil {
		return false, nil, err
	}
	return len(providers) > 0, providers, nil
}

// LoginProvidersOnly reports whether registration and sign-in are limited to the login providers
// right now: the setting is on and at least one provider is available to sign in with.
func (s *Service) LoginProvidersOnly(ctx context.Context) (bool, error) {
	limited, _, err := s.loginProvidersOnly(ctx)
	return limited, err
}

// refuseWhileLoginProvidersOnly fails with ErrLoginProvidersOnly while the limit applies.
func (s *Service) refuseWhileLoginProvidersOnly(ctx context.Context) error {
	switch limited, err := s.LoginProvidersOnly(ctx); {
	case err != nil:
		return err
	case limited:
		return ErrLoginProvidersOnly
	}
	return nil
}

// LoginProvidersOnlyImpact tells what the limit means now, for those who turn it on or change the
// providers under it.
type LoginProvidersOnlyImpact struct {
	// SettingOn is the value of the setting.
	SettingOn bool
	// EnabledProviders counts the providers available to sign in with. With none, the limit does
	// not apply, whatever the setting says.
	EnabledProviders int
	// UsersWithoutProvider counts the active local users with no link to one of those providers:
	// those who could not sign in while the limit applies, however they sign in today. Their
	// sessions and tokens keep working.
	UsersWithoutProvider int
	// IncludesViewer tells that the administrator who is looking is one of them.
	IncludesViewer bool
}

// Limited reports whether the limit applies.
func (i LoginProvidersOnlyImpact) Limited() bool { return i.SettingOn && i.EnabledProviders > 0 }

// LoginProvidersOnlyImpact counts who the limit would keep from signing in, whether or not the
// setting is on. viewerUserID is the administrator who is looking, "" for the CLI.
func (s *Service) LoginProvidersOnlyImpact(ctx context.Context, viewerUserID string) (LoginProvidersOnlyImpact, error) {
	var out LoginProvidersOnlyImpact
	if s.d.LoginProvidersOnly != nil {
		on, err := s.d.LoginProvidersOnly(ctx)
		if err != nil {
			return out, fmt.Errorf("identity: read the limit to login providers: %w", err)
		}
		out.SettingOn = on
	}
	providers, err := s.ExternalLoginProviders(ctx)
	if err != nil {
		return out, err
	}
	out.EnabledProviders = len(providers)
	if len(providers) == 0 {
		return out, nil
	}
	ids := make([]string, len(providers))
	for i, p := range providers {
		ids[i] = p.ID
	}
	if out.UsersWithoutProvider, err = s.d.Repository.CountActiveLocalUsersWithoutLoginProvider(ctx, ids); err != nil {
		return out, err
	}
	if viewerUserID != "" {
		links, err := s.d.Repository.ListLinkedExternalIdentities(ctx, viewerUserID)
		if err != nil {
			return out, err
		}
		out.IncludesViewer = !slices.ContainsFunc(links, func(l identity.LinkedExternalIdentity) bool { return slices.Contains(ids, l.ProviderID) })
	}
	return out, nil
}
