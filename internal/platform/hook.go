// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"time"

	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpHooks builds the decision hooks (ADR-0147). A hook gets a plugin only when its address is
// set, so a server without any opens no connection and adds nothing to the paths that ask. It reads
// the instance settings and the holds through a, at each use, so it can run before those modules
// and the modules that own the subjects can take a.Hooks when they are built.
func (a *App) setUpHooks(cfg *config.Config, repo apphook.Repository, statuses apphook.Statuses) error {
	plugins, err := newHookPlugins(cfg, a)
	if err != nil {
		return err
	}
	a.Hooks = apphook.NewService(apphook.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Settings: hookSettings{a}, Plugins: plugins,
		Holds: heldAccountsOf{a}, Statuses: statuses, Clock: a.Clock, IDs: a.IDs, Meter: a.Meter,
	})
	a.Hooks.RegisterJobs(a.Handlers, a.Scheduler)
	return nil
}

// hookSettings reads the settings of the hooks on the instance module.
type hookSettings struct{ a *App }

// ReviewUnfollowedDirect implements federation.ReviewSettings.
func (s hookSettings) ReviewUnfollowedDirect(ctx context.Context) (bool, error) {
	settings, err := s.a.Instance.Settings(ctx)
	if err != nil {
		return false, err
	}
	value, _ := settings.Value(instance.SettingHookInboundStatusUnfollowedDirect)
	return value == "true", nil
}

func (s hookSettings) HookMode(ctx context.Context, n hook.Name) (hook.Mode, error) {
	settings, err := s.a.Instance.Settings(ctx)
	if err != nil {
		return "", err
	}
	key, ok := instance.HookModeKey(string(n))
	if !ok {
		return hook.ModeOff, nil
	}
	value, _ := settings.Value(key)
	return hook.Mode(value), nil
}

func (s hookSettings) HookOnFailure(ctx context.Context, n hook.Name) (hook.OnFailure, error) {
	key, ok := instance.HookOnFailureKey(string(n))
	if !ok {
		return hook.PassOnFailure, nil
	}
	settings, err := s.a.Instance.Settings(ctx)
	if err != nil {
		return "", err
	}
	value, _ := settings.Value(key)
	return hook.OnFailure(value), nil
}

func (s hookSettings) HookRetention(ctx context.Context) (decisions, appealWindow time.Duration, err error) {
	settings, err := s.a.Instance.Settings(ctx)
	if err != nil {
		return 0, 0, err
	}
	return settings.Retention(instance.SettingRetentionHookDecisions), settings.AppealWindow(), nil
}

// heldAccountsOf reads the holds on the moderation module.
type heldAccountsOf struct{ a *App }

func (h heldAccountsOf) HeldAccountIDs(ctx context.Context) ([]string, error) {
	return h.a.Moderation.HeldAccountIDs(ctx)
}
