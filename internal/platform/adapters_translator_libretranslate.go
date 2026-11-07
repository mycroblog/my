// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/adapter/translator/libretranslate"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/platform/config"
)

// libreTranslateTimeout bounds one call of the LibreTranslate server. The service stops a call
// at 20 seconds anyway (ADR-0148 §8); this bounds a call without that deadline.
const libreTranslateTimeout = 30 * time.Second

// libreTranslateProviders returns LibreTranslate as a provider that the instance settings can
// enable, when translator_libretranslate_url is set (ADR-0148 §15). It makes no call, so a server
// that is down at start fails translations and the server still starts. The operator configured
// the URL, which may be a private address, so the client is the adapter's own and follows no
// redirects, which keeps the API key on that server.
func libreTranslateProviders(cfg *config.Config) ([]translation.Provider, error) {
	if cfg.TranslatorLibreTranslateURL == "" {
		return nil, nil
	}
	t, err := libretranslate.New(libretranslate.Options{
		URL:    cfg.TranslatorLibreTranslateURL,
		APIKey: cfg.TranslatorLibreTranslateAPIKey,
		HTTPClient: &http.Client{
			Timeout:       libreTranslateTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	})
	if err != nil {
		return nil, err
	}
	return []translation.Provider{{
		ID: libretranslate.ProviderID, Name: "LibreTranslate", URL: "https://libretranslate.com", Translator: t,
	}}, nil
}
