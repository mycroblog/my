// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"github.com/mycroblog/my/internal/interface/oauth"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpOAuth registers the OAuth token endpoints.
func (a *App) setUpOAuth(cfg *config.Config) error {
	tokens, err := oauth.New(oauth.Options{Identity: a.Identity, WebOrigin: cfg.WebOrigin, Origin: cfg.ServerOrigin(), Logger: a.Logger})
	if err != nil {
		return err
	}
	tokens.Register(surfaceRouter{a, surfaceOAuth})
	return nil
}
