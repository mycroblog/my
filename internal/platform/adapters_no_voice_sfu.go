// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_voice_sfu

package platform

import (
	"github.com/mycroblog/my/internal/adapter/voice/p2p"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/platform/config"
)

// newSFU reports that the build tag no_voice_sfu left the SFU out.
func (a *App) newSFU(*config.Config, p2p.TURN) (appvoice.Transport, VoiceNode, error) {
	return nil, nil, config.ExcludedError("voice", "sfu", "no_voice_sfu")
}
