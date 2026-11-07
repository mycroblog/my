// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/adapter/translator/deepl"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/platform/config"
)

// deeplTranslationProviders returns DeepL as a provider that the instance settings can enable, when the configuration
// gives it a key (ADR-0148 §15). The key stays in the adapter: the settings and the API see only the name and the link,
// which DeepL's terms require next to a translation. It makes no call, so a key that DeepL refuses fails translations and
// the server still starts.
func deeplTranslationProviders(cfg *config.Config) []translation.Provider {
	if cfg.TranslatorDeepLAPIKey == "" {
		return nil
	}
	return []translation.Provider{{
		ID:   deepl.ProviderID,
		Name: "DeepL",
		URL:  "https://www.deepl.com",
		Translator: deepl.New(deepl.Options{
			APIKey: cfg.TranslatorDeepLAPIKey,
			URL:    cfg.TranslatorDeepLURL,
			// DeepL is a destination that the operator configured: a client of its own that follows no redirects, so
			// that the key reaches that API only (ADR-0146 §7).
			HTTPClient: &http.Client{
				Timeout:       15 * time.Second,
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			},
		}),
	}}
}
