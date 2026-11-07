// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"testing"

	"github.com/mycroblog/my/internal/platform/config"
)

func TestDeepLTranslationProviders(t *testing.T) {
	t.Parallel()
	if got := deeplTranslationProviders(&config.Config{}); got != nil {
		t.Errorf("without a key = %+v, want none", got)
	}
	got := deeplTranslationProviders(&config.Config{TranslatorDeepLAPIKey: "key:fx"})
	if len(got) != 1 || got[0].ID != "deepl" || got[0].Name != "DeepL" || got[0].URL != "https://www.deepl.com" || got[0].LogoURL != "" || got[0].Translator == nil {
		t.Errorf("with a key = %+v, want DeepL", got)
	}
}
