// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform

import (
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	translatorplugin "github.com/mycroblog/my/internal/adapter/translator/plugin"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/platform/config"
)

// pluginTranslationProviders returns the plugin of the translator kind as a provider that the instance settings can
// enable, when the configuration gives it a destination (ADR-0148 §14). It opens no connection: a plugin that is down at
// start fails translations, and the server still starts (ADR-0146 §15). Only the name reaches a client, never the
// destination (ADR-0146 §5).
func pluginTranslationProviders(cfg *config.Config, meter kernel.Meter) ([]translation.Provider, error) {
	if !cfg.PluginDestination(kind.Translator).Enabled() {
		return nil, nil
	}
	conn, err := NewPluginConn(cfg, kind.Translator, meter)
	if err != nil {
		return nil, err
	}
	return []translation.Provider{{ID: translatorplugin.ProviderID, Name: cfg.PluginTranslatorName, Translator: translatorplugin.New(conn)}}, nil
}
