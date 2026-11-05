// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_plugins

package platform

import (
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/platform/config"
)

// pluginTranslationProviders returns no provider: the configuration of this build refuses every plugin address, and
// translation_providers ignores an ID that the server has no provider for.
func pluginTranslationProviders(*config.Config, kernel.Meter) ([]translation.Provider, error) {
	return nil, nil
}
