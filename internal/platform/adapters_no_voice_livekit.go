// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_voice_livekit

package platform

import (
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/platform/config"
)

// newLiveKitTransport reports that the build tag no_voice_livekit left the LiveKit adapter out.
func (a *App) newLiveKitTransport(*config.Config) (appvoice.Transport, error) {
	return nil, config.ExcludedError("voice", "livekit", "no_voice_livekit")
}
