// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_voice_sora

package platform

import (
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/platform/config"
)

// newSoraTransport reports that the build tag no_voice_sora left the Sora adapter out.
func (a *App) newSoraTransport(*config.Config) (appvoice.Transport, error) {
	return nil, config.ExcludedError("voice", "sora", "no_voice_sora")
}
