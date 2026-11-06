// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"testing"

	"github.com/mycroblog/my/internal/app/translation"
)

// TestTranslationEndsWithinTheWriteTimeout checks that a translation request, which may wait for a
// lock and then ask several providers, answers before the HTTP server cuts the response off.
func TestTranslationEndsWithinTheWriteTimeout(t *testing.T) {
	t.Parallel()
	if translation.RequestTimeout >= httpWriteTimeout {
		t.Errorf("translation.RequestTimeout = %v, want below the write timeout %v", translation.RequestTimeout, httpWriteTimeout)
	}
	if translation.ProviderTimeout >= translation.RequestTimeout {
		t.Errorf("translation.ProviderTimeout = %v, want below translation.RequestTimeout %v", translation.ProviderTimeout, translation.RequestTimeout)
	}
}
